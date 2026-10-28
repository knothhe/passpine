import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

const posixOnly = { skip: process.platform === 'win32' };
const identitySHA1 = 'A'.repeat(40), certificateSHA256 = 'b'.repeat(64);
const shellQuote = value => `'${value.replaceAll("'", "'\\''")}'`;

function fixture(t, failure = '') {
  const work = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'passpine macOS dev build ')));
  t.after(() => fs.rmSync(work, { recursive: true, force: true }));
  const root = path.join(work, 'source repository'), bin = path.join(work, 'command fakes');
  const install = path.join(work, 'installed apps'), output = path.join(root, 'dist/dev');
  const log = path.join(work, 'command-log.jsonl'), stub = path.join(work, 'commands.mjs');
  fs.mkdirSync(root); fs.mkdirSync(bin); fs.mkdirSync(install);
  // Keep the complete script dependency tree. Only external tool boundaries
  // are faked; future script imports must not be omitted from this fixture.
  fs.cpSync(fileURLToPath(new URL('./', import.meta.url)), path.join(root, 'scripts'), { recursive: true });
  fs.copyFileSync(new URL('../Makefile', import.meta.url), path.join(root, 'Makefile'));
  const installer = path.join(root, 'scripts/install-client.sh');
  // Linux can exercise macOS shell ordering using the same source; the only
  // platform path shim replaces Apple's absolute metadata reader with a fake.
  fs.writeFileSync(installer, fs.readFileSync(installer, 'utf8').replaceAll('/usr/libexec/PlistBuddy', 'PlistBuddy'));
  const assetDirectory = path.join(root, 'client/assets/dev/monochrome');
  fs.mkdirSync(assetDirectory, { recursive: true });
  fs.writeFileSync(path.join(assetDirectory, 'app-icon-dev.icns'), 'isolated Dev icon');
  const destination = path.join(install, 'Passpine Dev.app');
  const installedMacOS = path.join(destination, 'Contents/MacOS');
  fs.mkdirSync(installedMacOS, { recursive: true });
  fs.writeFileSync(path.join(destination, 'Contents/Info.plist'), 'isolated installed app metadata');
  fs.writeFileSync(path.join(installedMacOS, 'passpine-host'), '#!/bin/sh\nif [ "$1" = "--build-channel" ]; then printf "dev\\n"; else exit 19; fi\n', { mode: 0o755 });
  const oldMarker = path.join(destination, 'old-install-marker');
  fs.writeFileSync(oldMarker, 'previous isolated installation');
  fs.writeFileSync(stub, `
import fs from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
const [tool, ...args] = process.argv.slice(2);
const root = process.env.PASSPINE_TEST_SOURCE;
const output = path.join(root, 'dist/dev');
const failure = process.env.PASSPINE_TEST_FAILURE;
const event = (name, details = {}) => fs.appendFileSync(process.env.PASSPINE_TEST_COMMAND_LOG, JSON.stringify({ name, args, ...details }) + '\\n');
const stop = (message, status = 37) => { process.stderr.write(message + '\\n'); process.exit(status); };
const binary = filename => {
  fs.mkdirSync(path.dirname(filename), { recursive: true });
  fs.writeFileSync(filename, '#!/bin/sh\\ncase "$1" in\\n  --build-channel) printf "dev\\\\n" ;;\\n  --register) printf "registered\\\\n" >> "$PASSPINE_TEST_REGISTRATION" ;;\\n  --seed-test-vault) exit 0 ;;\\n  *) exit 0 ;;\\nesac\\n', { mode: 0o755 });
};
if (tool === 'node') {
  const script = path.basename(args[0] ?? '');
  if (script === 'build-channel.mjs') {
    event('channel-' + args[1]);
    if (args[1] === 'shell') console.log(['dev', '0.1.13-dev.isolated', '0.1.13', output, 'Passpine Dev', 'com.knothhe.passpine.dev', 'Passpine-Dev', '-X main.version=isolated', '-X main.version=isolated'].join('\\t'));
    else if (args[1] === 'write') fs.writeFileSync(path.join(output, 'build-channel.json'), '{}');
  } else if (script === 'macos-dev-signing.mjs') {
    event('resolve');
    if (failure === 'missing' || failure === 'stale') stop('No usable local Dev signing identity (' + failure + '). Run node scripts/macos-dev-signing.mjs setup.');
    console.log(JSON.stringify({ identitySHA1: '${identitySHA1}', certificateSHA256: '${certificateSHA256}' }));
  } else if (script === 'macos-signing.mjs') {
    event('verify-dev', { target: args[2], fingerprint: args[3] });
    if (failure === 'verify' || (failure === 'staged-verify' && args[2].includes('/.passpine-dev.'))) stop('isolated fixed-signature verification failure');
    if (args[1] !== 'verify-dev' || args[3] !== '${certificateSHA256}') stop('verification did not receive the selected public fingerprint');
  } else if (script === 'client-installation.mjs') {
    event('installation-' + args[1]);
  } else if (['dependencies.mjs', 'version.mjs', 'dev-icons.mjs'].includes(script)) {
    event(script);
  } else {
    // The actual JSON parsers and heredocs in the shell scripts still run.
    const child = spawnSync(process.execPath, args, { stdio: 'inherit', env: process.env });
    process.exit(child.status ?? 1);
  }
} else if (tool === 'uname') {
  console.log(args[0] === '-m' ? 'arm64' : 'Darwin');
} else if (tool === 'go' || tool === 'xcrun') {
  event(tool);
  const index = args.indexOf('-o');
  if (index < 0 || !args[index + 1]) stop('isolated compiler received no output path');
  binary(args[index + 1]);
} else if (tool === 'codesign') {
  event('codesign');
  if ((failure === 'codesign' && args.includes('--force')) || (failure === 'integrity' && args.includes('--verify'))) stop('isolated codesign failure');
} else if (tool === 'ditto') {
  if (args.includes('-c')) {
    event('archive');
    fs.writeFileSync(args.at(-1), 'isolated archive');
  } else {
    event('stage-copy');
    fs.cpSync(args[0], args[1], { recursive: true });
  }
} else if (tool === 'PlistBuddy') {
  console.log('com.knothhe.passpine.dev');
} else stop('unexpected isolated command: ' + tool);
`);
  for (const tool of ['node', 'uname', 'go', 'xcrun', 'codesign', 'ditto', 'PlistBuddy']) {
    fs.writeFileSync(path.join(bin, tool), `#!/bin/sh\nexec ${shellQuote(process.execPath)} ${shellQuote(stub)} ${shellQuote(tool)} "$@"\n`, { mode: 0o755 });
  }
  const env = {
    ...process.env, PATH: `${bin}${path.delimiter}${process.env.PATH}`, CGO_ENABLED: '1', PASSPINE_BUILD_CHANNEL: 'dev',
    PASSPINE_TEST_SOURCE: root, PASSPINE_TEST_COMMAND_LOG: log, PASSPINE_TEST_FAILURE: failure,
    PASSPINE_TEST_REGISTRATION: path.join(work, 'registrations'), PASSPINE_SKIP_TEST_VAULT: '1',
    PASSPINE_MACOS_SIGNING_KEYCHAIN: path.join(work, 'isolated keychain.keychain-db'),
  };
  const run = (command = 'make') => spawnSync(command, command === 'make' ? ['client:install', `CLIENT_INSTALL_DIR=${install}`] : [installer, install], { cwd: root, env, encoding: 'utf8' });
  const events = () => fs.existsSync(log) ? fs.readFileSync(log, 'utf8').trim().split('\n').filter(Boolean).map(line => JSON.parse(line)) : [];
  return { root, install, output, destination, oldMarker, env, events, run };
}

test('missing and stale Dev identities stop make installation before build side effects', posixOnly, async t => {
  for (const failure of ['missing', 'stale']) await t.test(failure, t => {
    const f = fixture(t, failure), result = f.run();
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /node scripts\/macos-dev-signing\.mjs setup/);
    assert.equal(fs.readFileSync(f.oldMarker, 'utf8'), 'previous isolated installation');
    assert.equal(fs.existsSync(f.output), false);
    assert.ok(!f.events().some(event => ['dependencies.mjs', 'version.mjs', 'dev-icons.mjs', 'go', 'xcrun', 'installation-preflight', 'archive'].includes(event.name)));
    assert.equal(fs.existsSync(f.env.PASSPINE_TEST_REGISTRATION), false);
  });
});

test('signing and verification failures cannot replace an existing Dev installation', posixOnly, async t => {
  for (const failure of ['codesign', 'integrity', 'verify']) await t.test(failure, t => {
    const f = fixture(t, failure), result = f.run();
    assert.notEqual(result.status, 0);
    assert.equal(fs.readFileSync(f.oldMarker, 'utf8'), 'previous isolated installation');
    assert.ok(!f.events().some(event => ['archive', 'installation-preflight', 'stage-copy'].includes(event.name)));
    assert.equal(fs.existsSync(f.env.PASSPINE_TEST_REGISTRATION), false);
  });
});

test('successful Dev build signs exact program identities and verifies before archive and replacement', posixOnly, t => {
  const f = fixture(t), result = f.run();
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
  const events = f.events(), signatures = events.filter(event => event.name === 'codesign' && event.args.includes('--force'));
  assert.equal(signatures.length, 3);
  assert.deepEqual(signatures.map(event => event.args[event.args.indexOf('--identifier') + 1]), [
    'com.knothhe.passpine.dev.host', 'com.knothhe.passpine.dev.device-unlock', 'com.knothhe.passpine.dev',
  ]);
  for (const signature of signatures) {
    assert.equal(signature.args[signature.args.indexOf('--sign') + 1], identitySHA1);
    assert.equal(signature.args[signature.args.indexOf('--keychain') + 1], f.env.PASSPINE_MACOS_SIGNING_KEYCHAIN);
  }
  const firstVerification = events.findIndex(event => event.name === 'verify-dev');
  const archive = events.findIndex(event => event.name === 'archive');
  const channelRecord = events.findIndex(event => event.name === 'channel-write');
  assert.ok(firstVerification > events.findIndex(event => event.name === 'codesign' && event.args.includes('--verify')));
  assert.ok(archive > firstVerification && channelRecord > archive);
  assert.equal(events.filter(event => event.name === 'verify-dev').length, 3, 'build, source installation, and staged copy all need verification');
  assert.ok(events.filter(event => event.name === 'verify-dev').every(event => event.fingerprint === certificateSHA256));
  const stage = events.findIndex(event => event.name === 'stage-copy');
  const stagedVerification = events.findIndex(event => event.name === 'verify-dev' && event.target.includes('/.passpine-dev.'));
  const ownership = events.findIndex(event => event.name === 'installation-record');
  assert.ok(stage < stagedVerification && stagedVerification < ownership);
  assert.equal(fs.existsSync(f.oldMarker), false);
  assert.equal(fs.readFileSync(f.env.PASSPINE_TEST_REGISTRATION, 'utf8'), 'registered\n');
  assert.ok(fs.readdirSync(f.output).some(name => name.endsWith('-macos-arm64.zip')));
});

test('a rejected staged signature preserves the installed bundle and registration', posixOnly, t => {
  const f = fixture(t, 'staged-verify'), result = f.run();
  assert.notEqual(result.status, 0);
  assert.equal(fs.readFileSync(f.oldMarker, 'utf8'), 'previous isolated installation');
  assert.ok(f.events().some(event => event.name === 'archive'), 'build should succeed before the staged-copy rejection');
  assert.ok(f.events().some(event => event.name === 'stage-copy'));
  assert.ok(!f.events().some(event => event.name === 'installation-record'));
  assert.equal(fs.existsSync(f.env.PASSPINE_TEST_REGISTRATION), false);
  assert.deepEqual(fs.readdirSync(f.install), ['Passpine Dev.app']);
});

test('a relative explicit keychain keeps its original meaning after the build changes directory', posixOnly, t => {
  const f = fixture(t), expectedKeychain = f.env.PASSPINE_MACOS_SIGNING_KEYCHAIN;
  f.env.PASSPINE_MACOS_SIGNING_KEYCHAIN = path.relative(f.root, expectedKeychain);
  assert.ok(!path.isAbsolute(f.env.PASSPINE_MACOS_SIGNING_KEYCHAIN));
  const result = f.run();
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
  const signatures = f.events().filter(event => event.name === 'codesign' && event.args.includes('--force'));
  assert.equal(signatures.length, 3);
  for (const signature of signatures) assert.equal(signature.args[signature.args.indexOf('--keychain') + 1], expectedKeychain);
});

test('direct installation verifies the source before preflight or destination creation', posixOnly, t => {
  const f = fixture(t, 'verify');
  const source = path.join(f.output, 'Passpine Dev.app/Contents');
  fs.mkdirSync(path.join(source, 'MacOS'), { recursive: true });
  fs.writeFileSync(path.join(source, 'Info.plist'), 'isolated build metadata');
  const result = f.run('bash');
  assert.notEqual(result.status, 0);
  assert.ok(f.events().some(event => event.name === 'verify-dev'));
  assert.ok(!f.events().some(event => ['installation-preflight', 'stage-copy'].includes(event.name)));
  assert.equal(fs.readFileSync(f.oldMarker, 'utf8'), 'previous isolated installation');
});
