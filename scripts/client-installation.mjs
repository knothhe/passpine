import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';

const appID = 'com.knothhe.passpine.dev';
const recordName = '.passpine-dev-install.json';
const repository = fileURLToPath(new URL('../', import.meta.url));
const digest = data => crypto.createHash('sha256').update(data).digest('hex');
const exists = file => { try { fs.lstatSync(file); return true; } catch (error) { if (error.code === 'ENOENT') return false; throw error; } };
const contains = (parent, child) => { const relative = path.relative(parent, child); return relative === '' || (!relative.startsWith(`..${path.sep}`) && relative !== '..' && !path.isAbsolute(relative)); };

export function realPath(file) {
  const absolute = path.resolve(file);
  if (exists(absolute)) {
    if (fs.lstatSync(absolute).isSymbolicLink()) throw new Error(`Refusing symbolic link: ${absolute}`);
    const resolved = fs.realpathSync(absolute);
    if (resolved !== absolute) throw new Error(`Refusing aliased installation path: ${absolute}`);
    return resolved;
  }
  const parent = path.dirname(absolute);
  if (parent === absolute) return absolute;
  return path.join(realPath(parent), path.basename(absolute));
}

export function installationLayout(directory, { platform = process.platform, home = os.homedir(), env = process.env } = {}) {
  home = fs.realpathSync(home);
  const installDirectory = path.resolve(directory || path.join(home, platform === 'darwin' ? 'Applications' : '.local/bin'));
  realPath(installDirectory);
  const configHome = platform === 'darwin' ? path.join(home, 'Library/Application Support') : (env.XDG_CONFIG_HOME || path.join(home, '.config'));
  const protectedRoots = [repository, path.join(configHome, 'Passpine'), path.join(configHome, 'Passpine Dev'), env.PASSPINE_HOME, env.PASSPINE_DEV_HOME].filter(Boolean);
  const target = path.join(installDirectory, platform === 'darwin' ? 'Passpine Dev.app' : 'passpine-dev');
  for (const root of protectedRoots) {
    let normalized = path.resolve(root);
    if (exists(normalized)) normalized = fs.realpathSync(normalized);
    if (contains(normalized, target) || contains(target, normalized)) throw new Error('Installation overlaps source or application data.');
  }
  const host = platform === 'darwin' ? path.join(target, 'Contents/MacOS/passpine-host') : path.join(installDirectory, 'passpine-dev-host');
  const executable = platform === 'darwin' ? path.join(target, 'Contents/MacOS/passpine') : target;
  const dataHome = env.XDG_DATA_HOME || path.join(home, '.local/share');
  if (!path.isAbsolute(dataHome) || !path.isAbsolute(configHome)) throw new Error('XDG paths must be absolute.');
  const extras = platform === 'linux' ? [path.join(dataHome, 'applications', `${appID}.desktop`), path.join(dataHome, 'icons/hicolor/256x256/apps', `${appID}.png`)] : [];
  for (const file of [host, ...extras]) for (const root of protectedRoots) {
    const normalized = exists(root) ? fs.realpathSync(root) : path.resolve(root);
    if (contains(normalized,file) || contains(file,normalized)) throw new Error('Installation resource overlaps source or application data.');
  }
  return { platform, home, configHome, installDirectory, target, executable, host, record: path.join(installDirectory, recordName), programs: platform === 'darwin' ? [target] : [target, host], extras };
}

export function treeHash(file) {
  realPath(file);
  const info = fs.lstatSync(file);
  if (info.isFile()) return digest(fs.readFileSync(file));
  if (!info.isDirectory()) throw new Error(`Unexpected installation file: ${file}`);
  return digest(JSON.stringify(fs.readdirSync(file).sort().map(name => [name, treeHash(path.join(file, name))])));
}

function treeFiles(directory, relative = '') {
  realPath(path.join(directory, relative));
  const result = {};
  for (const name of fs.readdirSync(path.join(directory, relative)).sort()) {
    const child = path.join(relative, name), file = path.join(directory, child);
    realPath(file);
    const info = fs.lstatSync(file);
    if (info.isDirectory()) Object.assign(result, treeFiles(directory, child));
    else if (info.isFile()) result[child] = digest(fs.readFileSync(file));
    else throw new Error(`Unexpected installation entry: ${file}`);
  }
  return result;
}

function matches(item) {
  if (!item.tree) return treeHash(item.path) === item.sha256;
  return treeDirectories(item.path).every(directory => item.directories.includes(directory)) && Object.entries(treeFiles(item.path)).every(([file, sha256]) => item.tree[file] === sha256);
}

function treeDirectories(directory, relative = '') {
  const result = [];
  for (const name of fs.readdirSync(path.join(directory, relative))) {
    const child = path.join(relative,name), file = path.join(directory,child);
    realPath(file);
    if (fs.lstatSync(file).isDirectory()) { result.push(child); result.push(...treeDirectories(directory,child)); }
  }
  return result.sort();
}

function removeOwned(item, remove) {
  if (!item.tree) return remove(item.path);
  // Remove individually so a partial filesystem failure remains retryable.
  for (const file of Object.keys(item.tree)) {
    const absolute = path.join(item.path, file);
    if (exists(absolute)) { realPath(absolute); if (treeHash(absolute) !== item.tree[file]) throw new Error(`File changed: ${absolute}`); fs.unlinkSync(absolute); }
  }
  const prune = directory => {
    for (const name of fs.readdirSync(directory)) {
      const child = path.join(directory, name);
      realPath(child);
      if (!fs.lstatSync(child).isDirectory()) throw new Error(`Unowned file preserved: ${child}`);
      prune(child);
    }
    fs.rmdirSync(directory);
  };
  prune(item.path);
}

function verifyIdentity(layout, run) {
  for (const executable of [layout.executable, layout.host]) {
    realPath(executable);
    if (!fs.lstatSync(executable).isFile() || run(executable, ['--build-channel']).trim() !== 'dev') throw new Error(`Not a Dev installation: ${executable}`);
  }
  if (layout.platform === 'darwin') {
    const id = run('/usr/libexec/PlistBuddy', ['-c', 'Print CFBundleIdentifier', path.join(layout.target, 'Contents/Info.plist')]).trim();
    if (id !== appID) throw new Error('Bundle identity does not match Passpine Dev.');
    run('/usr/bin/codesign', ['--verify', '--deep', '--strict', layout.target]);
  }
}

const execute = (file, args) => execFileSync(file, args, { encoding: 'utf8', timeout: 75000, stdio: ['ignore', 'pipe', 'pipe'] });
function writeRecord(layout, record) {
  const temporary = `${layout.record}.${crypto.randomUUID()}.tmp`;
  try { fs.writeFileSync(temporary, `${JSON.stringify(record, null, 2)}\n`, { mode: 0o600, flag: 'wx' }); fs.renameSync(temporary, layout.record); }
  finally { if (exists(temporary)) fs.unlinkSync(temporary); }
}

export function recordInstallation(layout, { run = execute } = {}) {
  verifyIdentity(layout, run);
  if (exists(layout.record)) {
    realPath(layout.record);
    const prior = JSON.parse(fs.readFileSync(layout.record, 'utf8'));
    if (prior.channel !== 'dev' || prior.installDirectory !== layout.installDirectory) throw new Error('Existing installation record belongs to another installation.');
  }
  const files = [...layout.programs, ...layout.extras].filter(exists).map(file => ({ path: file, sha256: treeHash(file), ...(fs.lstatSync(file).isDirectory() ? { tree: treeFiles(file), directories:treeDirectories(file) } : {}) }));
  const record = { schema: 1, channel: 'dev', phase:'installed', platform: layout.platform, installDirectory: layout.installDirectory, files };
  writeRecord(layout, record);
  return record;
}

function registrations(layout) {
  const directories = layout.platform === 'darwin'
    ? ['Google/Chrome', 'Google/Chrome Beta', 'Google/ChromeForTesting', 'Chromium', 'Microsoft Edge', 'BraveSoftware/Brave-Browser', 'Mozilla'].map(name => path.join(layout.home, 'Library/Application Support', name, 'NativeMessagingHosts'))
    : ['.config/google-chrome/NativeMessagingHosts', '.config/chromium/NativeMessagingHosts', '.config/microsoft-edge/NativeMessagingHosts', '.config/BraveSoftware/Brave-Browser/NativeMessagingHosts', '.mozilla/native-messaging-hosts'].map(name => path.join(layout.home, name));
  return directories.map(directory => path.join(directory, `${appID}.json`));
}

function loginRegistrations(layout) {
  if (layout.platform !== 'linux') return [];
  // Current MyGo builds fall back to the app name; include the explicit app ID
  // for packages that provide it. Ownership still requires the complete Exec.
  return ['Passpine-Dev', appID].map(id => path.join(layout.configHome,'autostart',`${id}.desktop`));
}

function ownedLoginRegistration(file, executable) {
  const quoted = '"' + [...executable].map(char => char === '\\' ? '\\\\\\\\' : char === '%' ? '%%' : ['"','`','$'].includes(char) ? '\\\\' + char : char).join('') + '"';
  const lines = fs.readFileSync(file,'utf8').split(/\r?\n/);
  return lines[0] === '[Desktop Entry]' && lines.includes('Type=Application') && lines.includes('Name=Passpine Dev') && lines.includes(`Exec=${quoted} --mygo-opened-at-login`);
}

export async function uninstallInstallation(layout, { run = execute, running = installedProcesses, pause = ms => new Promise(resolve => setTimeout(resolve, ms)), remove = file => fs.rmSync(file, { recursive: true }) } = {}) {
  realPath(layout.record);
  if (!exists(layout.record)) {
    if (layout.programs.some(exists)) throw new Error('Installation ownership record is missing. Reinstall this Dev build first; no files were removed.');
    // Without the record, residual registrations cannot be claimed safely.
    if (registrations(layout).some(file => exists(file) && JSON.parse(fs.readFileSync(file)).path === layout.host) || loginRegistrations(layout).some(file => exists(file) && ownedLoginRegistration(file,layout.executable))) throw new Error('Dev registration remains without an ownership record. Reinstall and retry.');
    return { removed: [], absent: true };
  }
  const record = JSON.parse(fs.readFileSync(layout.record, 'utf8'));
  const originalRecordHash = treeHash(layout.record);
  if (record.phase && !['installed','removing'].includes(record.phase)) throw new Error('Invalid installation removal phase.');
  const allowed = new Set([...layout.programs, ...layout.extras]);
  if (record.files?.some(item => item.tree && (!Array.isArray(item.directories) || item.directories.some(directory => typeof directory !== 'string' || !directory || path.isAbsolute(directory) || directory.split(/[\\/]/).includes('..'))))) throw new Error('Invalid bundle directory ownership record.');
  if (record.schema !== 1 || record.channel !== 'dev' || record.platform !== layout.platform || record.installDirectory !== layout.installDirectory || !Array.isArray(record.files) || record.files.length === 0 || record.files.some(item => !allowed.has(item.path) || !/^[a-f0-9]{64}$/.test(item.sha256) || (item.tree && (typeof item.tree !== 'object' || Object.entries(item.tree).some(([file, hash]) => path.isAbsolute(file) || file.split(/[\\/]/).includes('..') || !/^[a-f0-9]{64}$/.test(hash))))) || new Set(record.files.map(item => item.path)).size !== record.files.length || layout.programs.some(file => !record.files.some(item => item.path === file))) throw new Error('Invalid installation ownership record.');
  const validate = () => {
    for (const item of record.files) if (exists(item.path) && !matches(item)) throw new Error(`Installation file changed; preserved: ${item.path}`);
  };
  validate();
  for (const executable of [layout.executable, layout.host].filter(exists)) {
    if (run(executable, ['--build-channel']).trim() !== 'dev') throw new Error(`Not a Dev executable: ${executable}`);
  }
  if (record.phase === 'removing') {
    if (running(layout).length) throw new Error('A partially removed installation is running. Quit it before retrying.');
  } else if (exists(layout.executable)) {
    // This also unregisters the exact bundle's native login item. It never
    // kills a process and fails when edits or writes cannot finish.
    run(layout.executable, ['--quit-for-uninstall']);
    for (let attempt = 0; running(layout).length; attempt++) {
      if (attempt >= 100) throw new Error('Dev application is still running; finish pending edits and retry.');
      await pause(50);
    }
  } else if (running(layout).length) throw new Error('Installed process remains without its executable. Quit it manually and retry.');
  validate();
  if (treeHash(layout.record) !== originalRecordHash) throw new Error('Installation ownership changed during exit. Retry the command.');
  writeRecord(layout, { ...record,phase:'removing' });
  const ownedRegistrations = [];
  const preserved = [];
  for (const file of registrations(layout)) {
    if (!exists(file)) continue;
    realPath(file);
    const manifest = JSON.parse(fs.readFileSync(file, 'utf8'));
    if (manifest.name === appID && manifest.path === layout.host && manifest.type === 'stdio') ownedRegistrations.push({ path: file, sha256: treeHash(file) });
    else preserved.push(file);
  }
  for (const file of loginRegistrations(layout)) {
    if (!exists(file)) continue;
    realPath(file);
    if (ownedLoginRegistration(file,layout.executable)) ownedRegistrations.push({ path:file,sha256:treeHash(file) });
    else preserved.push(file);
  }
  const removed = [];
  const failures = [];
  // Keep the ownership record on partial failure so cleanup remains retryable.
  for (const item of [...ownedRegistrations, ...record.files.filter(item => !layout.programs.includes(item.path)), ...record.files.filter(item => layout.programs.includes(item.path))]) {
    if (!exists(item.path)) continue;
    try {
      if (!matches(item)) throw new Error('File changed during removal');
      removeOwned(item, remove); removed.push(item.path);
    } catch (error) { failures.push(`${item.path}: ${error.message}`); }
  }
  if (failures.length) throw new Error(`Partially removed: ${removed.join(', ')}\nRemaining (retry the same command):\n${failures.join('\n')}`);
  fs.unlinkSync(layout.record);
  return { removed, preserved, absent: false };
}

export function installedProcesses(layout, { processIDs = () => fs.readdirSync('/proc'), executable = pid => fs.readlinkSync(`/proc/${pid}/exe`) } = {}) {
  if (layout.platform === 'linux') {
    // Linux ps comm contains only a process name. procfs exposes the actual
    // executable, including an unlinked process from a partial uninstall.
    return processIDs().filter(pid => {
      if (!/^\d+$/.test(pid)) return false;
      try {
        const file = executable(pid).replace(/ \(deleted\)$/, '');
        return [layout.executable, layout.host].includes(file);
      } catch (error) {
        if (['ENOENT', 'ESRCH', 'EACCES', 'EPERM'].includes(error.code)) return false;
        throw error;
      }
    });
  }
  const output = execute('/bin/ps', ['-axo', 'pid=,comm=']);
  return output.split('\n').filter(line => {
    const match = line.trim().match(/^(\d+)\s+(.+)$/);
    return match && [layout.executable, layout.host].includes(match[2]);
  });
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const [action, directory, ...extra] = process.argv.slice(2);
    if (extra.length || !['preflight', 'record', 'uninstall'].includes(action) || !['darwin', 'linux'].includes(process.platform)) throw new Error('Usage: node scripts/client-installation.mjs preflight|record|uninstall [INSTALL_DIR] (macOS/Linux)');
    const layout = installationLayout(directory);
    if (action === 'preflight') { /* Path validation only; no mutations. */ }
    else if (action === 'record') recordInstallation(layout);
    else {
      const result = await uninstallInstallation(layout);
      console.log(result.absent ? 'Passpine Dev is not installed.' : `Removed Passpine Dev: ${result.removed.join(', ')}\nUser data and credentials were preserved.`);
      if (result.preserved?.length) console.log(`Preserved registrations belonging to other installations: ${result.preserved.join(', ')}`);
    }
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
