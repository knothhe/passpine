// Opt-in acceptance on ephemeral CI machines: real install, native startup,
// graceful exit, repeated uninstall and data preservation. No release publishing.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync, spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { installationLayout } from './client-installation.mjs';

if (process.argv.slice(2).join(' ') !== '--run' || process.env.GITHUB_ACTIONS !== 'true' || !process.env.RUNNER_TEMP) throw new Error('Run only with --run on an ephemeral CI runner.');
const project = fileURLToPath(new URL('../', import.meta.url));
const root = fs.mkdtempSync(path.join(fs.realpathSync(process.env.RUNNER_TEMP), 'passpine-dev-acceptance-'));
const home = path.join(root, 'home'); fs.mkdirSync(home);
const directory = path.join(root, 'installed');
const data = path.join(root, 'data'); fs.mkdirSync(data);
const env = { ...process.env, HOME: home, USERPROFILE: home, APPDATA: path.join(home, 'AppData/Roaming'), LOCALAPPDATA: path.join(home, 'AppData/Local'), XDG_CONFIG_HOME: path.join(home, '.config'), XDG_DATA_HOME: path.join(home, '.local/share'), XDG_RUNTIME_DIR: path.join(root, 'runtime'), PASSPINE_DEV_HOME: data, PASSPINE_BUILD_CHANNEL: 'dev', PASSPINE_SKIP_TEST_VAULT: '1' };
fs.mkdirSync(env.XDG_RUNTIME_DIR, { mode: 0o700 });
for (const key of ['APPDATA', 'LOCALAPPDATA', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME']) fs.mkdirSync(env[key], { recursive: true });
const run = (file, args, options = {}) => execFileSync(file, args, { cwd: project, env, stdio: 'inherit', timeout: 300000, ...options });
const powershell = script => run('powershell.exe', ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', path.join(project, 'scripts', script), directory]);
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const wait = async predicate => { for (let i = 0; i < 300; i++) { if (predicate()) return; await pause(100); } throw new Error('Timed out waiting for native application lifecycle.'); };
const windows = process.platform === 'win32';
if (windows) {
  // The test must never adopt a pre-existing registration on a reused runner.
  const prior = spawn('reg.exe', ['query', 'HKCU\\Software\\Passpine Dev'], { env, stdio: 'ignore' });
  if (await new Promise(resolve => prior.once('exit', resolve)) === 0) throw new Error('Dev registration already exists; use a fresh runner.');
  run('powershell.exe', ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', 'scripts/build-client.ps1']);
  powershell('install-client.ps1');
} else {
  run('bash', ['scripts/build-client.sh']);
  run('bash', ['scripts/install-client.sh', directory]);
}
const layout = windows ? { executable: path.join(directory, 'passpine.exe'), target: directory } : installationLayout(directory, { home, env });
const metadata = JSON.parse(run(layout.executable, ['--build-info'], { stdio: ['ignore', 'pipe', 'inherit'], encoding: 'utf8' }));
if (metadata.channel !== 'dev' || metadata.os !== process.platform.replace('win32', 'windows') || !metadata.commit) throw new Error('Installed build metadata mismatch.');
const sentinel = path.join(data, 'preserve-user-data'); fs.writeFileSync(sentinel, crypto.randomBytes(64));
const sentinelHash = crypto.createHash('sha256').update(fs.readFileSync(sentinel)).digest('hex');
const output = fs.openSync(path.join(root, 'native.log'), 'w');
const child = spawn(layout.executable, ['--preview', '--data-dir', data], { env, stdio: ['ignore', output, output] });
fs.closeSync(output);
let exited = false, exitCode;
child.once('exit', code => { exited = true; exitCode = code; });
try {
  await wait(() => {
    if (exited) throw new Error(`Native startup failed (${exitCode}); see ${root}/native.log`);
    return fs.existsSync(path.join(data, 'control/browser-endpoint.json')) && fs.existsSync(path.join(data, 'preferences.json'));
  });
  const uninstall = () => windows ? powershell('uninstall-client.ps1') : run(process.execPath, ['scripts/client-installation.mjs', 'uninstall', directory]);
  uninstall();
  await wait(() => exited);
  if (exitCode !== 0) throw new Error(`Native shutdown failed: ${exitCode}`);
  uninstall();
  if (fs.existsSync(layout.executable)) throw new Error('Installed executable remains.');
  if (crypto.createHash('sha256').update(fs.readFileSync(sentinel)).digest('hex') !== sentinelHash) throw new Error('Uninstall changed user data.');
  fs.writeFileSync(path.join(root, 'result.json'), JSON.stringify({ platform: process.platform, metadata, install: true, nativeStartup: true, gracefulExit: true, repeatedUninstall: true, userDataPreserved: true }, null, 2));
  console.log(`Dev installation acceptance passed: ${root}`);
} catch (error) {
  // This is the exact synthetic test child, never a user installation. A Go
  // stack dump exposes native-loop deadlocks instead of leaving only a timeout.
  if (process.platform === 'linux' && !exited) {
    child.kill('SIGQUIT');
    await Promise.race([new Promise(resolve => child.once('exit', resolve)), pause(2000)]);
  }
  console.error(fs.readFileSync(path.join(root, 'native.log'), 'utf8').slice(-128 * 1024));
  throw error;
} finally {
  // Never force-stop a user process. The exact test child is the only fallback.
  if (!exited) { try { run(layout.executable, ['--quit-for-uninstall']); } catch { /* Keep diagnostics on failure. */ } }
}
