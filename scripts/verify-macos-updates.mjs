#!/usr/bin/env node
// Opt-in native installation verification. All destinations, data and processes
// belong to a fresh /private/tmp tree. Never invokes make install or registration.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import https from 'node:https';
import { spawn, execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { stageUpdate, decodeZip, encodeZip } from './update-artifacts.mjs';

const repo = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
if (process.platform !== 'darwin' || process.arch !== 'arm64') throw new Error('This opt-in verifier requires macOS arm64.');
if (!process.argv.includes('--run')) throw new Error('Use --run to execute real isolated macOS applications and updater helpers.');
const work = fs.mkdtempSync('/private/tmp/passpine-update-verification-');
const oldVersion = '99.0.0', newVersion = '99.0.1';
const pair = crypto.generateKeyPairSync('ed25519');
const publicKey = pair.publicKey.export({ type: 'spki', format: 'der' }).subarray(-32).toString('base64');
const sha = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const hashFile = file => sha(fs.readFileSync(file));
const check = (condition, message) => { if (!condition) throw new Error(message); };
const inside = file => path.resolve(file).startsWith(work + path.sep);
const reports = [], routes = new Map(), pids = new Set();
let server;
const baseEnv = { ...process.env, GOCACHE: process.env.GOCACHE || '/tmp/passpine-update-go-cache',
  XDG_RUNTIME_DIR: path.join(work, 'runtime'), TMPDIR: path.join(work, 'runtime'),
  CFFIXED_USER_HOME: path.join(work, 'foundation'), GNUPGHOME: path.join(work, 'gnupg'),
  GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: '/dev/null', GIT_OPTIONAL_LOCKS: '0',
  PASSPINE_VERIFY_ROOT: work };
for (const key of ['XDG_RUNTIME_DIR', 'CFFIXED_USER_HOME', 'GNUPGHOME']) fs.mkdirSync(baseEnv[key], { recursive: true, mode: 0o700 });
function command(binary, args, options = {}) {
  return execFileSync(binary, args, { cwd: repo, env: baseEnv, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], maxBuffer: 8 << 20, ...options });
}
async function run(binary, args, options = {}) {
  const log = options.log || path.join(work, 'build.log');
  const fd = fs.openSync(log, 'a', 0o600);
  const child = spawn(binary, args, { cwd: repo, env: baseEnv, stdio: ['ignore', fd, fd], ...options });
  fs.closeSync(fd);
  await new Promise((resolve, reject) => { child.once('error', reject); child.once('exit', (code, signal) => code === 0 ? resolve() : reject(new Error(`${path.basename(binary)} failed (${code ?? signal}); log: ${log}`))); });
}
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
async function until(predicate, message, timeout = 45000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) { const result = predicate(); if (result) return result; await pause(100); }
  throw new Error(message);
}
function processCommand(pid) {
  try { return command('/bin/ps', ['-p', String(pid), '-o', 'command=', '-ww']).trim(); } catch { return ''; }
}
function isolatedProcesses() {
  return command('/bin/ps', ['-axo', 'pid=,command=', '-ww']).split('\n').flatMap(line => {
    const match = /^\s*(\d+)\s+(.+)$/.exec(line);
    return match && match[2].startsWith(work + '/') ? [{ pid: Number(match[1]), command: match[2] }] : [];
  });
}
async function stop(pid) {
  if (!processCommand(pid).startsWith(work + '/')) return;
  try { process.kill(pid, 'SIGTERM'); } catch {}
  for (let i = 0; i < 30 && processCommand(pid).startsWith(work + '/'); i++) await pause(100);
  if (processCommand(pid).startsWith(work + '/')) { try { process.kill(pid, 'SIGKILL'); } catch {} }
}
function treeHashes(root, filter = () => true) {
  const result = {};
  function walk(dir) {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const file = path.join(dir, entry.name), relative = path.relative(root, file);
      if (entry.isDirectory()) walk(file);
      else if (entry.isFile() && filter(relative)) result[relative] = hashFile(file);
      else if (entry.isSymbolicLink()) throw new Error('A verification fixture contains a symlink.');
    }
  }
  walk(root); return result;
}
function protectedHashes(data) {
  return treeHashes(data, relative => relative === 'preferences.json' || relative.startsWith('keys/') || relative.startsWith('demo-vault/'));
}
function writeJSON(file, value) { check(inside(file), 'Attempt to write outside the isolated tree.'); fs.writeFileSync(file, JSON.stringify(value, null, 2), { mode: 0o600 }); }
function readJSON(file) { try { return JSON.parse(fs.readFileSync(file, 'utf8')); } catch { return null; } }
function plist(version) {
  return `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>CFBundleName</key><string>Passpine Isolated Verification</string><key>CFBundleIdentifier</key><string>com.knothhe.passpine.isolated.${path.basename(work).split('-').at(-1).toLowerCase()}</string><key>CFBundleExecutable</key><string>passpine</string><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleShortVersionString</key><string>${version}</string><key>CFBundleVersion</key><string>${version}</string><key>LSMinimumSystemVersion</key><string>13.0</string><key>NSHighResolutionCapable</key><true/><key>NSPrincipalClass</key><string>NSApplication</string></dict></plist>`;
}

try {
  const keyFile = path.join(work, 'tls.key'), certFile = path.join(work, 'tls.crt'), config = path.join(work, 'tls.cnf');
  fs.writeFileSync(config, '[req]\ndistinguished_name=dn\nx509_extensions=ext\nprompt=no\n[dn]\nCN=127.0.0.1\n[ext]\nsubjectAltName=IP:127.0.0.1\nbasicConstraints=critical,CA:TRUE\nkeyUsage=critical,digitalSignature,keyCertSign\n');
  command('/usr/bin/openssl', ['req', '-new', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1', '-keyout', keyFile, '-out', certFile, '-config', config]);
  fs.chmodSync(keyFile, 0o600);
  server = https.createServer({ key: fs.readFileSync(keyFile), cert: fs.readFileSync(certFile) }, (req, res) => {
    const route = routes.get(req.url);
    if (!route) { res.writeHead(404); res.end(); return; }
    route.hits++;
    const bytes = typeof route.bytes === 'function' ? route.bytes() : route.bytes;
    res.writeHead(200, { 'content-type': route.type || 'application/octet-stream', 'content-length': bytes.length }); res.end(bytes);
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const origin = `https://127.0.0.1:${server.address().port}`;
  console.log(`Isolated verification directory: ${work}`);
  console.log('Building real Passpine application binaries and the test controller…');
  const binaryDir = path.join(work, 'binaries'); fs.mkdirSync(binaryDir);
  const client = path.join(repo, 'client');
  // This opt-in harness deliberately tests release-channel helpers with a fresh
  // test key; ordinary local builds remain Dev and cannot install updates.
  const commonFlags = `-s -w -X github.com/knothhe/passpine/client/buildinfo.Channel=release -X main.updatePublicKey=${publicKey} -X main.updateFeedURL=${origin}/offline`;
  for (const [label, version] of [['old', oldVersion], ['new', newVersion]]) {
    await run('go', ['build', '-trimpath', `-ldflags=${commonFlags} -X main.version=${version}`, '-o', path.join(binaryDir, label), './cmd/passpine'], { cwd: client });
  }
  const testChannelFlag = '-X github.com/knothhe/passpine/client/buildinfo.Channel=release';
  await run('go', ['build', '-trimpath', `-ldflags=${testChannelFlag}`, '-o', path.join(binaryDir, 'host'), './cmd/passpine-host'], { cwd: client });
  await run('go', ['build', `-ldflags=${testChannelFlag}`, '-o', path.join(binaryDir, 'controller'), './update/testdata/macos-runner'], { cwd: client });
  const mainSource = path.join(client, 'cmd/passpine/main.go');
  const desktopSource = path.join(client, 'desktop/desktop.go');
  const crashMain = path.join(work, 'crash-main.go');
  let crashSource = fs.readFileSync(mainSource, 'utf8');
  if (!/^\s*"os"\s*$/m.test(crashSource)) crashSource = crashSource.replace('"log"', '"log"\n"os"');
  fs.writeFileSync(crashMain, crashSource.replace('if deviceunlock.RunHelper() {', `if os.Getenv("PASSPINE_VERIFY_CRASH_MARKER") != "" && !(len(os.Args) == 2 && os.Args[1] == "--version") { _ = os.WriteFile(os.Getenv("PASSPINE_VERIFY_CRASH_MARKER"), []byte("injected failure before service/UI startup"), 0600); os.Exit(86) }\nif deviceunlock.RunHelper() {`));
  const noAckDesktop = path.join(work, 'noack-desktop.go');
  fs.writeFileSync(noAckDesktop, fs.readFileSync(desktopSource, 'utf8').replace('update.ConfirmStartup(service.DataDirectory())', '/* Isolated fault injection: omit startup acknowledgement. */'));
  for (const [label, replacement] of [['crash', { [mainSource]: crashMain }], ['noack', { [desktopSource]: noAckDesktop }]]) {
    const overlay = path.join(work, `${label}-overlay.json`); writeJSON(overlay, { Replace: replacement });
    await run('go', ['build', '-trimpath', `-overlay=${overlay}`, `-ldflags=${commonFlags} -X main.version=${newVersion}`, '-o', path.join(binaryDir, label), './cmd/passpine'], { cwd: client });
  }
  const templates = {};
  for (const label of ['old', 'new', 'crash', 'noack', 'bad-signature']) {
    const root = path.join(work, 'templates', label), bundle = path.join(root, 'dist', 'Passpine.app');
    fs.mkdirSync(path.join(bundle, 'Contents', 'MacOS'), { recursive: true });
    fs.mkdirSync(path.join(bundle, 'Contents', 'Resources'), { recursive: true });
    fs.copyFileSync(path.join(binaryDir, label === 'bad-signature' ? 'new' : label), path.join(bundle, 'Contents', 'MacOS', 'passpine'));
    fs.copyFileSync(path.join(binaryDir, 'host'), path.join(bundle, 'Contents', 'MacOS', 'passpine-host'));
    fs.copyFileSync(path.join(repo, 'client', 'assets', 'Passpine-monochrome.icns'), path.join(bundle, 'Contents', 'Resources', 'Passpine.icns'));
    fs.writeFileSync(path.join(bundle, 'Contents', 'Info.plist'), plist(label === 'old' ? oldVersion : newVersion));
    command('/usr/bin/codesign', ['--force', '--deep', '--sign', '-', bundle]);
    command('/usr/bin/codesign', ['--verify', '--deep', '--strict', bundle]);
    if (label === 'bad-signature') fs.appendFileSync(path.join(bundle, 'Contents', 'Resources', 'Passpine.icns'), 'signed-feed-but-invalid-OS-signature');
    const asset = stageUpdate(label === 'old' ? oldVersion : newVersion, 'client-darwin-arm64', root, root);
    templates[label] = { root, bundle, asset, bytes: fs.readFileSync(path.join(root, asset.full.name)), hashes: treeHashes(bundle) };
  }
  const scenarios = [
    { name: 'incremental-install', template: 'new', delta: true, result: 'success' },
    { name: 'incremental-fallback-full', template: 'new', delta: true, corruptDelta: true, result: 'success' },
    { name: 'startup-crash-rollback', template: 'crash', result: 'rollback' },
    { name: 'startup-ack-timeout-rollback', template: 'noack', result: 'rollback' },
    { name: 'invalid-bundle-signature', template: 'bad-signature', result: 'rejected' },
    { name: 'tampered-package-before-install', template: 'new', result: 'preflight-rejected' },
  ];
  for (const scenario of scenarios) {
    console.log(`Running ${scenario.name}…`);
    const started = Date.now(), root = path.join(work, scenario.name), bundle = path.join(root, 'Passpine.app'), data = path.join(root, 'data');
    fs.mkdirSync(root); fs.cpSync(templates.old.bundle, bundle, { recursive: true }); fs.mkdirSync(data, { mode: 0o700 });
    const executable = path.join(bundle, 'Contents', 'MacOS', 'passpine');
    const env = { ...baseEnv, PASSPINE_HOME: data, PASSPINE_VERIFY_MODE: 'seed' };
    await run(path.join(binaryDir, 'controller'), ['--preview', '--data-dir', data], { env, log: path.join(root, 'seed.log') });
    const beforeData = protectedHashes(data), beforeBundle = treeHashes(bundle);
    check(Object.keys(beforeData).some(name => name.endsWith('.gpg')), 'No encrypted fictional fixture was created.');
    const target = templates[scenario.template], fullPath = `/${scenario.name}/full.zip`, deltaPath = `/${scenario.name}/delta.zip`, feedPath = `/${scenario.name}/feed.json`;
    const fullRoute = { bytes: target.bytes, hits: 0 }; routes.set(fullPath, fullRoute);
    const asset = { os: 'darwin', arch: 'arm64', files: target.asset.files, full: { url: origin + fullPath, sha256: sha(target.bytes), size: target.bytes.length }, deltas: [] };
    let deltaRoute;
    if (scenario.delta) {
      const changed = decodeZip(target.bytes).filter(entry => !templates.old.asset.files.some(file => file.path === entry.path && file.sha256 === sha(entry.data) && file.mode === entry.mode));
      const delta = encodeZip(changed);
      check(delta.length < target.bytes.length, 'The fixture delta is not smaller than the full package.');
      asset.deltas.push({ fromVersion: oldVersion, url: origin + deltaPath, sha256: sha(delta), size: delta.length });
      const delivered = Buffer.from(delta); if (scenario.corruptDelta) delivered[0] ^= 1;
      deltaRoute = { bytes: delivered, hits: 0 }; routes.set(deltaPath, deltaRoute);
    }
    const payload = Buffer.from(JSON.stringify({ schema: 1, version: newVersion, notes: { en: 'Isolated test update', 'zh-CN': '隔离验证更新' }, assets: [asset] }));
    routes.set(feedPath, { bytes: Buffer.from(JSON.stringify({ payload: payload.toString('base64'), signature: crypto.sign(null, payload, pair.privateKey).toString('base64') })), hits: 0 });
    Object.assign(env, { PASSPINE_VERIFY_MODE: 'install', PASSPINE_VERIFY_CURRENT_VERSION: oldVersion, PASSPINE_VERIFY_PUBLIC_KEY: publicKey, PASSPINE_VERIFY_FEED_URL: origin + feedPath, PASSPINE_VERIFY_CA_FILE: certFile, PASSPINE_VERIFY_INSTALL_ROOT: bundle });
    if (scenario.template === 'crash') env.PASSPINE_VERIFY_CRASH_MARKER = path.join(root, 'injected-crash.marker');
    const log = fs.openSync(path.join(root, 'process.log'), 'a', 0o600);
    const controller = spawn(path.join(binaryDir, 'controller'), ['--preview', '--data-dir', data], { cwd: root, env, stdio: ['ignore', log, log] }); fs.closeSync(log);
    check(controller.pid, 'Controller did not launch.'); pids.add(controller.pid);
    let controllerExit;
    controller.once('exit', (code, signal) => { controllerExit = { code, signal }; });
    const ready = await until(() => readJSON(path.join(data, 'probe-ready.json')), 'Update did not become ready.');
    const snapshot = ready.snapshot || ready.Snapshot || ready;
    if (scenario.delta) check(deltaRoute.hits === 1, 'Delta was not attempted exactly once.');
    if (scenario.corruptDelta) check(fullRoute.hits === 1 && snapshot.Fallback, 'Corrupt delta did not fall back to the full package.');
    if (scenario.delta && !scenario.corruptDelta) check(fullRoute.hits === 0 && snapshot.UsedDelta, 'Unnecessary full download replaced a valid delta.');
    check(JSON.stringify(beforeBundle) === JSON.stringify(treeHashes(bundle)), 'Download modified the installed test app.');
    if (scenario.result === 'preflight-rejected') fs.appendFileSync(path.join(data, 'updates', 'package.zip'), 'tampered');
    fs.writeFileSync(path.join(data, 'install.signal'), 'install');
    let observedNewBundle = false;
    if (scenario.result === 'preflight-rejected') {
      const error = await until(() => readJSON(path.join(data, 'probe-error.json')), 'Tampered package was not rejected.');
      check((error.snapshot || error).ErrorCode === 'verification', 'Tampered package produced the wrong error.');
      await until(() => controllerExit, 'Failed preflight controller did not exit.');
      check(JSON.stringify(beforeBundle) === JSON.stringify(treeHashes(bundle)), 'Preflight rejection altered the application.');
    } else {
      await until(() => readJSON(path.join(data, 'probe-helper-started.json')), 'Production helper did not launch.');
      await until(() => processCommand(controller.pid).startsWith(executable + ' '), 'Controller did not become the real old Passpine process.');
      await pause(600);
      check(JSON.stringify(beforeBundle) === JSON.stringify(treeHashes(bundle)), 'Helper replaced files before the old application exited.');
      check(fs.readdirSync(path.join(data, 'updates')).some(name => name.startsWith('helper-')), 'Isolated helper exited before its parent.');
      await stop(controller.pid); // The only parent terminated is the verified temporary application PID.
      await until(() => {
        if (hashFile(executable) === target.hashes['Contents/MacOS/passpine']) observedNewBundle = true;
        const failure = readJSON(path.join(data, 'updates', 'install-result.json'));
        const helpers = fs.readdirSync(path.join(data, 'updates')).some(name => name.startsWith('helper-'));
        if (helpers) return false;
        if (scenario.result === 'success') return !fs.existsSync(path.join(data, 'updates', 'pending.json')) && hashFile(executable) === target.hashes['Contents/MacOS/passpine'];
        return failure?.errorCode === 'install' && hashFile(executable) === beforeBundle['Contents/MacOS/passpine'];
      }, 'Production installation/rollback did not complete.', 55000);
      const expected = scenario.result === 'success' ? target.hashes : beforeBundle;
      check(JSON.stringify(expected) === JSON.stringify(treeHashes(bundle)), 'Installed/restored bundle does not exactly match its source.');
      if (scenario.result === 'rollback') check(observedNewBundle, 'Rollback scenario never exchanged in the new bundle.');
      if (scenario.template === 'crash') check(fs.existsSync(env.PASSPINE_VERIFY_CRASH_MARKER), 'Injected failing application was not executed.');
      const expectedVersion = scenario.result === 'success' ? newVersion : oldVersion;
      check(command(executable, ['--version'], { env }).includes(expectedVersion), 'Installed application reports the wrong version.');
      command('/usr/bin/codesign', ['--verify', '--deep', '--strict', bundle]);
      await until(() => isolatedProcesses().some(item => item.command.startsWith(executable + ' ')), 'Healthy target or restored old UI did not remain running.');
    }
    check(JSON.stringify(beforeData) === JSON.stringify(protectedHashes(data)), 'Fictional vault, keys, or preferences changed.');
    const row = { scenario: scenario.name, result: 'passed', seconds: Number(((Date.now() - started) / 1000).toFixed(1)), deltaRequests: deltaRoute?.hits || 0, fullRequests: fullRoute.hits, observedNewBundle, protectedFileCount: Object.keys(beforeData).length, bundleFiles: Object.keys(beforeBundle).length };
    reports.push(row); writeJSON(path.join(root, 'result.json'), row);
    for (const process of isolatedProcesses()) await stop(process.pid);
    check(isolatedProcesses().length === 0, 'An isolated child process remained after cleanup.');
    console.log(`PASS ${scenario.name} (${row.seconds}s, ${row.protectedFileCount} protected files unchanged)`);
  }
  const summary = { platform: process.platform, arch: process.arch, oldVersion, newVersion, root: work, productionUpdaterUnmodified: true, appMode: 'real production Passpine --preview; faults only in disposable Go overlays', scenarios: reports };
  writeJSON(path.join(work, 'summary.json'), summary);
  console.log(JSON.stringify(summary, null, 2));
} catch (error) {
  console.error(error.stack || error); process.exitCode = 1;
} finally {
  for (const process of isolatedProcesses()) await stop(process.pid);
  for (const pid of pids) await stop(pid);
  if (server) await new Promise(resolve => server.close(resolve));
  try { fs.rmSync(path.join(work, 'tls.key')); } catch {}
  console.log(`Verification artifacts: ${work}`);
}
