import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

const packageScript = fileURLToPath(new URL('./package-client-linux.sh', import.meta.url));
const skip = process.platform === 'win32' && 'Linux release packaging uses Bash and tar';

function fixture(t) {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-linux-release-')));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const dist = path.join(root, 'dist');
  fs.mkdirSync(dist);
  fs.writeFileSync(path.join(dist, 'passpine'), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
  fs.writeFileSync(path.join(dist, 'passpine-host'), '#!/bin/sh\nprintf "%s\\n" "$1" > "$PASSPINE_TEST_REGISTER_LOG"\n', { mode: 0o755 });
  const packageRelease = (version = '1.2.3', arch = 'amd64') => {
    execFileSync('bash', [packageScript, version, arch, dist]);
    return path.join(dist, `Passpine-${version}-linux-${arch}.tar.gz`);
  };
  const extract = archive => {
    const release = path.join(root, 'release');
    fs.mkdirSync(release);
    execFileSync('tar', ['-xzf', archive, '-C', release]);
    return release;
  };
  const tools = path.join(root, 'tools');
  fs.mkdirSync(tools);
  fs.writeFileSync(path.join(tools, 'uname'), '#!/bin/sh\nprintf "Linux\\n"\n', { mode: 0o755 });
  fs.writeFileSync(path.join(tools, 'node'), '#!/bin/sh\necho "The release installer must not require Node.js" >&2\nexit 99\n', { mode: 0o755 });
  fs.writeFileSync(path.join(tools, 'update-desktop-database'), '#!/bin/sh\nprintf "%s\\n" "$1" > "$PASSPINE_TEST_DESKTOP_LOG"\n', { mode: 0o755 });
  const home = path.join(root, 'home');
  const env = {
    ...process.env,
    HOME: home,
    XDG_DATA_HOME: '',
    PATH: `${tools}${path.delimiter}${process.env.PATH}`,
    PASSPINE_TEST_REGISTER_LOG: path.join(root, 'registered'),
    PASSPINE_TEST_DESKTOP_LOG: path.join(root, 'desktop-updated'),
  };
  return { root, dist, home, env, packageRelease, extract };
}

test('Linux archives contain both executables, the full version, icon and executable installer for each architecture', { skip }, t => {
  const { packageRelease } = fixture(t);
  for (const arch of ['amd64', 'arm64']) {
    const archive = packageRelease('1.2.3-rc.1', arch);
    const entries = execFileSync('tar', ['-tzf', archive], { encoding: 'utf8' }).trim().split('\n');
    assert.deepEqual(entries.sort(), ['app-icon-monochrome.png', 'install.sh', 'passpine', 'passpine-host', 'version.txt']);
    assert.equal(execFileSync('tar', ['-xOzf', archive, 'version.txt'], { encoding: 'utf8' }), '1.2.3-rc.1\n');
    const details = execFileSync('tar', ['-tvzf', archive], { encoding: 'utf8' });
    for (const binary of ['passpine', 'passpine-host', 'install.sh']) {
      assert.ok(details.split('\n').some(line => line.startsWith('-rwxr-xr-x') && line.endsWith(` ${binary}`)), details);
    }
  }
});

test('the extracted release installs without Node, registers the host and creates the desktop entry and icon', { skip }, t => {
  const { home, env, packageRelease, extract } = fixture(t);
  const release = extract(packageRelease());
  execFileSync('bash', [path.join(release, 'install.sh')], { env });
  for (const binary of ['passpine', 'passpine-host']) {
    assert.equal(fs.statSync(path.join(home, '.local/bin', binary)).mode & 0o777, 0o755);
  }
  assert.equal(fs.readFileSync(env.PASSPINE_TEST_REGISTER_LOG, 'utf8'), '--register\n');
  const data = path.join(home, '.local/share');
  assert.equal(fs.readFileSync(env.PASSPINE_TEST_DESKTOP_LOG, 'utf8'), `${data}/applications\n`);
  assert.deepEqual(fs.readFileSync(path.join(data, 'icons/hicolor/256x256/apps/com.knothhe.passpine.png')), fs.readFileSync(path.join(release, 'app-icon-monochrome.png')));
  assert.match(fs.readFileSync(path.join(data, 'applications/com.knothhe.passpine.desktop'), 'utf8'), new RegExp(`Exec="${home}/\\.local/bin/passpine"\n`));
});

test('desktop entries preserve custom installation paths containing whitespace and reserved characters', { skip }, t => {
  const { root, env, packageRelease, extract } = fixture(t);
  const release = extract(packageRelease());
  const destination = path.join(root, 'bin "quote" \\ $cash `tick` %value\nnext\tend');
  const data = path.join(root, 'custom data');
  execFileSync('bash', [path.join(release, 'install.sh'), destination], { env: { ...env, XDG_DATA_HOME: data } });
  const desktop = fs.readFileSync(path.join(data, 'applications/com.knothhe.passpine.desktop'), 'utf8');
  const execLine = desktop.split('\n').find(line => line.startsWith('Exec=')).slice(5);
  const decoded = execLine.replace(/\\([\\nrt])/g, (_, value) => ({ '\\': '\\', n: '\n', r: '\r', t: '\t' })[value]);
  assert.ok(decoded.startsWith('"') && decoded.endsWith('"'));
  const executable = decoded.slice(1, -1).replace(/\\([\\"`$])/g, '$1').replace(/%%/g, '%');
  assert.equal(executable, path.join(destination, 'passpine'));
  assert.equal(fs.readFileSync(env.PASSPINE_TEST_REGISTER_LOG, 'utf8'), '--register\n');
});

test('invalid package inputs and unsafe installation destinations fail before replacing files', { skip }, t => {
  const { root, dist, home, env, packageRelease, extract } = fixture(t);
  assert.notEqual(spawnSync('bash', [packageScript, '../invalid', 'amd64', dist]).status, 0);
  assert.notEqual(spawnSync('bash', [packageScript, '1.2.3', '386', dist]).status, 0);
  const release = extract(packageRelease());
  const install = path.join(release, 'install.sh');
  assert.notEqual(spawnSync('bash', [install, release], { env }).status, 0);
  assert.notEqual(spawnSync('bash', [install], { env: { ...env, XDG_DATA_HOME: 'relative' } }).status, 0);
  const destination = path.join(home, '.local/bin');
  fs.mkdirSync(destination, { recursive: true });
  const original = path.join(root, 'original');
  fs.writeFileSync(original, 'preserve');
  fs.symlinkSync(original, path.join(destination, 'passpine-host'));
  assert.notEqual(spawnSync('bash', [install], { env }).status, 0);
  assert.equal(fs.readFileSync(original, 'utf8'), 'preserve');
  assert.equal(fs.existsSync(path.join(destination, 'passpine')), false);
  assert.equal(fs.existsSync(env.PASSPINE_TEST_REGISTER_LOG), false);
});
