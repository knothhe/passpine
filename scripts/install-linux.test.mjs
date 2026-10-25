import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

const installScript = fileURLToPath(new URL('./install-linux.sh', import.meta.url));
const packageScript = fileURLToPath(new URL('./package-client-linux.sh', import.meta.url));
const official = 'https://github.com/knothhe/passpine/releases';

// LibreSSL and OpenSSL before 1.1.1 cannot verify raw Ed25519 signatures.
function opensslVerifiesEd25519() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-openssl-'));
  try {
    const { publicKey, privateKey } = crypto.generateKeyPairSync('ed25519');
    fs.writeFileSync(path.join(root, 'key.pem'), publicKey.export({ type: 'spki', format: 'pem' }));
    fs.writeFileSync(path.join(root, 'data'), 'data');
    fs.writeFileSync(path.join(root, 'sig'), crypto.sign(null, Buffer.from('data'), privateKey));
    return spawnSync('openssl', ['pkeyutl', '-verify', '-pubin', '-inkey', path.join(root, 'key.pem'), '-rawin', '-in', path.join(root, 'data'), '-sigfile', path.join(root, 'sig')]).status === 0;
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
}
const skip = process.platform === 'win32' ? 'The Linux installer uses POSIX sh and tar'
  : !opensslVerifiesEd25519() && 'openssl on PATH cannot verify Ed25519 signatures';

const sha256 = data => crypto.createHash('sha256').update(data).digest('hex');

function fixture(t, { version = '1.2.3', distro = 'ID=arch' } = {}) {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-linux-install-')));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const log = path.join(root, 'log');
  const dist = path.join(root, 'dist');
  fs.mkdirSync(dist);
  fs.writeFileSync(path.join(dist, 'passpine'), '#!/bin/sh\n[ "$1" = --build-channel ] && echo release\nexit 0\n', { mode: 0o755 });
  fs.writeFileSync(path.join(dist, 'passpine-host'), `#!/bin/sh\ncase "$1" in --build-channel) echo release ;; *) printf 'host %s\\n' "$1" >> "${log}" ;; esac\n`, { mode: 0o755 });
  execFileSync('bash', [packageScript, version, 'amd64', dist]);
  const name = `Passpine-${version}-linux-amd64.tar.gz`;
  const archive = fs.readFileSync(path.join(dist, name));
  const releases = path.join(root, 'releases');
  const download = path.join(releases, 'download', `v${version}`);
  fs.mkdirSync(download, { recursive: true });
  fs.mkdirSync(path.join(releases, 'latest/download'), { recursive: true });
  fs.writeFileSync(path.join(download, name), archive);

  const { publicKey, privateKey } = crypto.generateKeyPairSync('ed25519');
  const rawKey = publicKey.export({ type: 'spki', format: 'der' }).subarray(-32).toString('base64');
  const files = ['passpine', 'passpine-host'].map(file => {
    const data = fs.readFileSync(path.join(dist, file));
    return { path: file, mode: 493, size: data.length, sha256: sha256(data) };
  });
  const feed = ({ key = privateKey, packageSha256 = sha256(archive), url = `${official}/download/v${version}/${name}` } = {}) => {
    const payload = Buffer.from(`${JSON.stringify({
      schema: 1, version: '0.1.5', assets: [],
      installerUpdate: { schema: 2, version, assets: [
        { os: 'darwin', arch: 'arm64', format: 'app-zip', full: { url: `${official}/download/v${version}/mac.zip`, size: 1, sha256: '0'.repeat(64) }, files: [] },
        { os: 'linux', arch: 'amd64', format: 'tar-gzip', full: { url, size: archive.length, sha256: packageSha256 }, files },
      ] },
    })}\n`);
    const envelope = `${JSON.stringify({ payload: payload.toString('base64'), signature: crypto.sign(null, payload, key).toString('base64') })}\n`;
    for (const dir of [download, path.join(releases, 'latest/download')]) fs.writeFileSync(path.join(dir, 'update-feed.json'), envelope);
  };
  feed();

  const tools = path.join(root, 'tools');
  fs.mkdirSync(tools);
  const tool = (name, body) => fs.writeFileSync(path.join(tools, name), `#!/bin/sh\n${body}\n`, { mode: 0o755 });
  tool('uname', '[ "$1" = -m ] && echo x86_64 || echo Linux');
  tool('node', 'echo "The Linux installer must not require Node.js" >&2; exit 99');
  tool('sudo', `printf 'sudo\\n' >> "${log}"; exec "$@"`);
  tool('update-desktop-database', `printf 'desktop %s\\n' "$1" >> "${log}"`);
  tool('pacman', `[ "$1" = -Q ] && exit 1; printf 'pacman %s\\n' "$*" >> "${log}"`);
  tool('dpkg-query', '[ "$3" = libgtk-3-0t64 ] && printf "install ok installed"; exit 0');
  tool('apt-get', `printf 'apt-get %s\\n' "$*" >> "${log}"; exit 1`);
  const osRelease = path.join(root, 'os-release');
  fs.writeFileSync(osRelease, `${distro}\n`);

  const home = path.join(root, 'home');
  fs.mkdirSync(home);
  const env = {
    ...Object.fromEntries(Object.entries(process.env).filter(([key]) => !/^(PASSPINE_|XDG_)/.test(key))),
    HOME: home,
    PATH: `${tools}${path.delimiter}${process.env.PATH}`,
    PASSPINE_RELEASES_URL: `file://${releases}`,
    PASSPINE_INSTALL_PUBLIC_KEY: rawKey,
    PASSPINE_OS_RELEASE: osRelease,
  };
  const run = (args = [], extra = {}) => spawnSync('sh', [installScript, ...args], { env: { ...env, ...extra }, encoding: 'utf8' });
  const bin = path.join(home, '.local/bin');
  const desktop = path.join(home, '.local/share/applications/com.knothhe.passpine.desktop');
  const icon = path.join(home, '.local/share/icons/hicolor/256x256/apps/com.knothhe.passpine.png');
  const logged = () => fs.existsSync(log) ? fs.readFileSync(log, 'utf8') : '';
  return { root, dist, home, env, run, bin, desktop, icon, logged, feed, releases, name };
}

const ok = result => assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);

test('installs the latest verified release, its missing GTK package, desktop entry and browser connection', { skip }, t => {
  const { dist, run, bin, desktop, icon, logged } = fixture(t);
  const result = run();
  ok(result);
  assert.match(result.stdout, /installed Passpine 1\.2\.3/);
  for (const binary of ['passpine', 'passpine-host']) {
    assert.deepEqual(fs.readFileSync(path.join(bin, binary)), fs.readFileSync(path.join(dist, binary)));
    assert.equal(fs.statSync(path.join(bin, binary)).mode & 0o777, 0o755);
  }
  assert.match(fs.readFileSync(desktop, 'utf8'), new RegExp(`^Exec="${bin}/passpine"$`, 'm'));
  assert.ok(fs.statSync(icon).size > 0);
  assert.match(logged(), /pacman -S --needed --noconfirm gtk3\n/);
  assert.match(logged(), /host --register\n/);
});

test('runs when piped into sh, installs a requested version and upgrades its own installation', { skip }, t => {
  const { env, run, bin, logged } = fixture(t);
  ok(run(['--skip-deps']));
  const piped = spawnSync('sh', ['-s', '--', '--version', 'v1.2.3', '--skip-deps'], { env, input: fs.readFileSync(installScript), encoding: 'utf8' });
  ok(piped);
  assert.match(piped.stdout, /installed Passpine 1\.2\.3/);
  assert.equal(logged().match(/host --register/g).length, 2);
  assert.doesNotMatch(logged(), /pacman/);
  assert.ok(fs.existsSync(path.join(bin, 'passpine')));
  assert.notEqual(run(['--version', '9.9.9', '--skip-deps']).status, 0);
  assert.notEqual(run(['--version', '../1.2.3', '--skip-deps']).status, 0);
});

test('detects the Debian time64 GTK package without calling apt-get', { skip }, t => {
  const { run, logged } = fixture(t, { distro: 'ID=ubuntu\nID_LIKE=debian' });
  ok(run());
  assert.doesNotMatch(logged(), /apt-get/);
});

test('rejects forged feeds, mismatched packages and foreign package URLs before installing', { skip }, t => {
  const { run, bin, feed, releases, name, logged } = fixture(t);
  const cases = [
    [() => feed({ key: crypto.generateKeyPairSync('ed25519').privateKey }), /signature verification failed/],
    [() => feed({ packageSha256: '0'.repeat(64) }), /checksum mismatch/],
    [() => feed({ url: `https://example.com/${name}` }), /unexpected package URL/],
  ];
  for (const [prepare, message] of cases) {
    prepare();
    const result = run(['--skip-deps']);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, message);
  }
  feed();
  fs.appendFileSync(path.join(releases, 'download/v1.2.3', name), 'tampered');
  assert.match(run(['--skip-deps']).stderr, /checksum mismatch/);
  assert.equal(fs.existsSync(bin), false);
  assert.equal(logged(), '');
});

test('refuses to replace unrelated commands, symlinks or desktop entries of another installation', { skip }, t => {
  const { root, run, bin, desktop } = fixture(t);
  fs.mkdirSync(bin, { recursive: true });
  fs.writeFileSync(path.join(bin, 'passpine'), '#!/bin/sh\necho dev\n', { mode: 0o755 });
  assert.match(run(['--skip-deps']).stderr, /unrelated command/);
  assert.match(run(['--uninstall']).stderr, /unrelated command/);
  assert.equal(fs.readFileSync(path.join(bin, 'passpine'), 'utf8'), '#!/bin/sh\necho dev\n');
  fs.rmSync(path.join(bin, 'passpine'));
  const original = path.join(root, 'original');
  fs.writeFileSync(original, 'preserve');
  fs.symlinkSync(original, path.join(bin, 'passpine-host'));
  assert.match(run(['--skip-deps']).stderr, /symlink/);
  assert.equal(fs.readFileSync(original, 'utf8'), 'preserve');
  fs.rmSync(path.join(bin, 'passpine-host'));
  fs.mkdirSync(path.dirname(desktop), { recursive: true });
  fs.writeFileSync(desktop, '[Desktop Entry]\nExec="/opt/passpine/passpine"\n');
  assert.match(run(['--skip-deps']).stderr, /another installation/);
  assert.equal(fs.existsSync(path.join(bin, 'passpine')), false);
});

test('uninstall unregisters the browser connection and keeps application data', { skip }, t => {
  const { home, run, bin, desktop, icon, logged } = fixture(t);
  ok(run(['--skip-deps']));
  const data = path.join(home, '.config/Passpine');
  fs.mkdirSync(data, { recursive: true });
  fs.writeFileSync(path.join(data, 'settings.json'), '{}');
  const result = run(['--uninstall']);
  ok(result);
  assert.match(logged(), /host --unregister\n/);
  for (const file of [path.join(bin, 'passpine'), path.join(bin, 'passpine-host'), desktop, icon]) assert.equal(fs.existsSync(file), false, file);
  assert.equal(fs.readFileSync(path.join(data, 'settings.json'), 'utf8'), '{}');
  ok(run(['--uninstall']));
});

test('installation paths must be absolute, plainly quoted and outside the data directory', { skip }, t => {
  const { home, run } = fixture(t);
  assert.match(run(['--skip-deps'], { PASSPINE_BIN_DIR: 'relative' }).stderr, /absolute/);
  assert.match(run(['--skip-deps'], { PASSPINE_BIN_DIR: `${home}/bin "x"` }).stderr, /unsupported character/);
  assert.match(run(['--skip-deps'], { PASSPINE_BIN_DIR: `${home}/.config/Passpine/bin` }).stderr, /data directory/);
  assert.equal(fs.existsSync(path.join(home, '.config')), false);
});

test('the installer pins the update signing key and official release source', () => {
  const config = JSON.parse(fs.readFileSync(new URL('../config/update-signing.json', import.meta.url), 'utf8'));
  const script = fs.readFileSync(installScript, 'utf8');
  assert.match(script, new RegExp(`^official_public_key=${config.publicKey.replace(/[+/]/g, '\\$&')}$`, 'm'));
  assert.equal(config.feedURL, `${official}/latest/download/update-feed.json`);
  assert.match(script, new RegExp(`^official_releases_url=${official}$`, 'm'));
});
