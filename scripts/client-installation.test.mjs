import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { installationLayout, recordInstallation, uninstallInstallation, installedProcesses } from './client-installation.mjs';

test('Windows GUI client commands wait for output and preserve nonzero exit codes without LASTEXITCODE', { skip: process.platform !== 'win32' && 'Requires Windows PowerShell and a native GUI executable' }, t => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-gui-command-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const output = execFileSync('powershell.exe', [
    '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File',
    fileURLToPath(new URL('./test-fixtures/windows-client-command.ps1', import.meta.url)), directory,
  ], { encoding: 'utf8', timeout: 60000 });
  assert.match(output, /Windows GUI client command checks passed/);
});

function fixture(t, platform = 'linux') {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-dev-uninstall-')));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const home = path.join(root, 'home'); fs.mkdirSync(home);
  const layout = installationLayout(path.join(root, 'installed'), { platform, home, env: {} });
  const write = (file, data = 'owned') => { fs.mkdirSync(path.dirname(file), { recursive: true }); fs.writeFileSync(file, data); };
  write(layout.executable, 'dev-client'); write(layout.host, 'dev-host');
  for (const file of layout.extras) write(file);
  if (platform === 'darwin') write(path.join(layout.target, 'Contents/Info.plist'));
  const calls = [];
  const run = (executable, args) => { calls.push([executable, args]); return executable.endsWith('PlistBuddy') ? 'com.knothhe.passpine.dev' : args[0] === '--build-channel' ? 'dev' : ''; };
  recordInstallation(layout, { run });
  const options = { run, running: () => [], pause: async () => {} };
  const manifest = platform === 'linux' ? path.join(home, '.config/google-chrome/NativeMessagingHosts/com.knothhe.passpine.dev.json') : path.join(home, 'Library/Application Support/Google/Chrome/NativeMessagingHosts/com.knothhe.passpine.dev.json');
  write(manifest, JSON.stringify({ name: 'com.knothhe.passpine.dev', path: layout.host, type: 'stdio' }));
  return { root, home, layout, calls, options, write, manifest };
}

test('Dev uninstall requests normal exit, removes only owned files, preserves data and is repeatable', async t => {
  const { home, layout, options, write, manifest, calls } = fixture(t);
  const data = path.join(home, '.config/Passpine Dev/preferences.json'); write(data, 'private preferences');
  const release = path.join(layout.installDirectory, 'passpine'); write(release, 'formal installation');
  await uninstallInstallation(layout, options);
  assert.ok(calls.some(([file, args]) => file === layout.executable && args[0] === '--quit-for-uninstall'));
  for (const file of [...layout.programs, ...layout.extras, manifest, layout.record]) assert.equal(fs.existsSync(file), false, file);
  assert.equal(fs.readFileSync(data, 'utf8'), 'private preferences');
  assert.equal(fs.readFileSync(release, 'utf8'), 'formal installation');
  assert.equal((await uninstallInstallation(layout, options)).absent, true);
});

test('a registration pointing to another installation is preserved', async t => {
  const { layout, options, write, manifest } = fixture(t);
  write(manifest, JSON.stringify({ name: 'com.knothhe.passpine.dev', path: '/other/dev-host', type: 'stdio' }));
  await uninstallInstallation(layout, options);
  assert.equal(JSON.parse(fs.readFileSync(manifest)).path, '/other/dev-host');
});

test('Linux login cleanup requires the exact executable and preserves another copy', { skip:process.platform === 'win32' && 'Desktop Entry fixture uses Unix executable paths' }, async t => {
  const { layout,options,write } = fixture(t);
  const owned = path.join(layout.configHome,'autostart/Passpine-Dev.desktop');
  const other = path.join(layout.configHome,'autostart/com.knothhe.passpine.dev.desktop');
  write(owned,`[Desktop Entry]\nType=Application\nName=Passpine Dev\nExec="${layout.executable}" --mygo-opened-at-login\n`);
  write(other,'[Desktop Entry]\nType=Application\nName=Passpine Dev\nExec="/other/passpine-dev" --mygo-opened-at-login\n');
  const result = await uninstallInstallation(layout,options);
  assert.equal(fs.existsSync(owned),false); assert.ok(fs.existsSync(other));
  assert.ok(result.preserved.includes(other));
});

test('changed program content or release identity prevents every removal', async t => {
  const { layout, options, write, manifest } = fixture(t);
  write(layout.host, 'replaced helper');
  await assert.rejects(uninstallInstallation(layout, options), /changed/);
  assert.ok(fs.existsSync(layout.executable)); assert.ok(fs.existsSync(manifest));
  write(layout.host, 'dev-host');
  await assert.rejects(uninstallInstallation(layout, { ...options, run: () => 'release' }), /Not a Dev/);
  assert.ok(fs.existsSync(layout.executable));
});

test('missing or forged ownership records cannot claim an installation', async t => {
  const { layout, options, write } = fixture(t);
  fs.unlinkSync(layout.record);
  await assert.rejects(uninstallInstallation(layout, options), /record is missing/);
  write(layout.record, JSON.stringify({ schema: 1, channel: 'release' }));
  await assert.rejects(uninstallInstallation(layout, options), /Invalid/);
  assert.ok(fs.existsSync(layout.executable));
});

test('symlinked targets and application data overlap are rejected', async t => {
  const { root, home, layout, options } = fixture(t);
  fs.unlinkSync(layout.executable); fs.symlinkSync(layout.host, layout.executable);
  await assert.rejects(uninstallInstallation(layout, options), /symbolic link/);
  assert.throws(() => installationLayout(path.join(home, '.config/Passpine Dev'), { platform: 'linux', home, env: {} }), /overlaps/);
  fs.symlinkSync(layout.installDirectory, path.join(root, 'alias'));
  assert.throws(() => installationLayout(path.join(root, 'alias'), { platform: 'linux', home, env: {} }), /symbolic link/);
});

test('partial cleanup retains ownership and retries registrations after programs are gone', async t => {
  const { layout, options, manifest } = fixture(t);
  await assert.rejects(uninstallInstallation(layout, { ...options, remove: file => {
    if (file === manifest) throw new Error('temporary permission failure');
    fs.rmSync(file, { recursive: true });
  } }), /Partially removed/);
  assert.ok(fs.existsSync(layout.record)); assert.ok(fs.existsSync(manifest));
  assert.equal(fs.existsSync(layout.executable), false);
  await uninstallInstallation(layout, options);
  assert.equal(fs.existsSync(manifest), false); assert.equal(fs.existsSync(layout.record), false);
});

test('running application blocks removal when graceful exit cannot complete', async t => {
  const { layout, options } = fixture(t);
  await assert.rejects(uninstallInstallation(layout, { ...options, running: () => ['exact installed process'] }), /still running/);
  assert.ok(fs.existsSync(layout.executable)); assert.ok(fs.existsSync(layout.record));
});

test('Linux running process checks match proc executable paths, including deleted programs', t => {
  const { layout } = fixture(t);
  const processes = installedProcesses(layout, {
    processIDs: () => ['self', '101', '102', '103', '104', '105'],
    executable: pid => {
      if (pid === '105') throw Object.assign(new Error('process exited'), { code: 'ENOENT' });
      return { 101: layout.executable, 102: `${layout.host} (deleted)`, 103: '/another/passpine-dev', 104: `${layout.executable}-other` }[pid];
    },
  });
  assert.deepEqual(processes, ['101', '102']);
});

test('macOS bundle cleanup tolerates previously removed owned files and preserves unowned additions', async t => {
  const { layout, options, write } = fixture(t, 'darwin');
  const extra = path.join(layout.target, 'personal.txt'); write(extra, 'do not delete');
  await assert.rejects(uninstallInstallation(layout, options), /changed/);
  assert.equal(fs.readFileSync(extra, 'utf8'), 'do not delete');
  fs.unlinkSync(extra); fs.unlinkSync(layout.host); fs.unlinkSync(path.join(layout.target,'Contents/Info.plist'));
  const record = JSON.parse(fs.readFileSync(layout.record));
  record.phase = 'removing'; fs.writeFileSync(layout.record, JSON.stringify(record));
  await uninstallInstallation(layout, options);
  assert.equal(fs.existsSync(layout.target), false);
});
