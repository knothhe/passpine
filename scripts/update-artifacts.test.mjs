import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import { test } from 'node:test';
import { createUpdateFeed, decodeZip, encodeZip, readUpdateSigningConfig, stageUpdate, updateBuildConfig, validateUpdate, verifyFeed } from './update-artifacts.mjs';

const targets = ['client-darwin-arm64', 'client-linux-amd64', 'client-win32-amd64'];
const keys = () => {
  const { publicKey, privateKey } = crypto.generateKeyPairSync('ed25519');
  return { publicKey: publicKey.export({ format: 'der', type: 'spki' }).subarray(-32).toString('base64'), privateKey: privateKey.export({ format: 'pem', type: 'pkcs8' }) };
};
function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-updates-'));
  t.after(() => fs.rmSync(root, { force: true, recursive: true }));
  return root;
}
function build(root, version, changed) {
  const dist = path.join(root, version, 'dist');
  const write = (name, data, mode = 0o755) => {
    const file = path.join(dist, name); fs.mkdirSync(path.dirname(file), { recursive: true }); fs.writeFileSync(file, data, { mode });
  };
  // Include nontrivial executable and resource bytes in every platform fixture.
  const host = Buffer.concat(Array.from({ length: 256 }, (_, i) => crypto.createHash('sha256').update(`host ${i}`).digest()));
  for (const extension of ['', '.exe']) { write(`passpine${extension}`, changed ? 'new client binary' : 'old client binary'); write(`passpine-host${extension}`, host); }
  write('Passpine.app/Contents/MacOS/passpine', changed ? 'new mac client' : 'old mac client');
  write('Passpine.app/Contents/MacOS/passpine-host', host);
  write('Passpine.app/Contents/Info.plist', `<plist>${version}</plist>`, 0o644);
  write('Passpine.app/Contents/Resources/icon.icns', host, 0o644);
  return targets.map(target => {
    const directory = path.join(root, version, target);
    return { target, directory, update: stageUpdate(version, target, path.dirname(dist), directory) };
  });
}
function feed(root, version, updates, signingKeys) {
  return createUpdateFeed({ version, repo: 'knothhe/passpine', updates,
    output: path.join(root, version, 'signed'), ...signingKeys });
}

test('release signing flags use the pinned public key independently of cwd and empty CI variables', t => {
  const root = fixture(t), pinned = readUpdateSigningConfig(), config = updateBuildConfig({});
  assert.equal(config.key, pinned.publicKey); assert.match(config.key, /^[A-Za-z0-9+/]{43}=$/);
  assert.equal(config.feed, 'https://github.com/knothhe/passpine/releases/latest/download/update-feed.json');
  assert.deepEqual(updateBuildConfig({ PASSPINE_UPDATE_PUBLIC_KEY: '', PASSPINE_UPDATE_FEED_URL: '' }), config);
  const signingKeys = keys(), script = fileURLToPath(new URL('./update-artifacts.mjs', import.meta.url));
  const flags = execFileSync(process.execPath, [script, 'ldflags'], { cwd: root, encoding: 'utf8', env: {
    ...process.env, PASSPINE_UPDATE_PUBLIC_KEY: '', PASSPINE_UPDATE_FEED_URL: '', PASSPINE_UPDATE_PRIVATE_KEY: signingKeys.privateKey,
  } }).trim();
  assert.equal(flags, `-X main.updateFeedURL=${config.feed} -X main.updatePublicKey=${config.key}`);
  assert.doesNotMatch(flags, /PRIVATE KEY|PASSPINE_UPDATE_PRIVATE_KEY/);
  assert.ok(!flags.includes(signingKeys.privateKey));
  assert.deepEqual(Object.keys(pinned).sort(), ['algorithm', 'feedURL', 'publicKey', 'repository', 'schema']);
  assert.doesNotMatch(JSON.stringify(pinned), /PRIVATE KEY|privateKey|private_key/i);
});

test('isolated development overrides must contain a valid public key and HTTPS feed', () => {
  const signingKeys = keys(), feed = 'https://updates.example.com/isolated/feed.json';
  assert.deepEqual(updateBuildConfig({ PASSPINE_UPDATE_PUBLIC_KEY: signingKeys.publicKey, PASSPINE_UPDATE_FEED_URL: feed }), { key: signingKeys.publicKey, feed });
  for (const value of ['invalid', signingKeys.privateKey, Buffer.alloc(31).toString('base64'), Buffer.alloc(33).toString('base64')]) assert.throws(() => updateBuildConfig({ PASSPINE_UPDATE_PUBLIC_KEY: value }), /public key/i);
  for (const url of ['http://example.com/feed.json', 'https://user:password@example.com/feed.json', 'https://example.com/a b', 'not a URL']) assert.throws(() => updateBuildConfig({ PASSPINE_UPDATE_FEED_URL: url }), /HTTPS|Invalid URL/);
});

test('invalid or missing pinned configuration fails closed even when overrides are supplied', t => {
  const root = fixture(t), filename = path.join(root, 'update-signing.json'), pinned = readUpdateSigningConfig(), signingKeys = keys();
  const overrides = { PASSPINE_UPDATE_PUBLIC_KEY: signingKeys.publicKey, PASSPINE_UPDATE_FEED_URL: 'https://updates.example.com/test.json' };
  for (const config of [
    null, [], {}, { ...pinned, schema: 2 }, { ...pinned, algorithm: 'rsa' }, { ...pinned, repository: 'someone/else' },
    { ...pinned, publicKey: '' }, { ...pinned, publicKey: signingKeys.privateKey },
    { ...pinned, feedURL: 'http://github.com/knothhe/passpine/releases/latest/download/update-feed.json' },
    { ...pinned, feedURL: 'https://github.com/someone/else/releases/latest/download/update-feed.json' },
    { ...pinned, privateKey: signingKeys.privateKey },
  ]) {
    fs.writeFileSync(filename, JSON.stringify(config));
    assert.throws(() => updateBuildConfig(overrides, filename), /configuration|public key|HTTPS/i);
  }
  fs.writeFileSync(filename, '{');
  assert.throws(() => updateBuildConfig(overrides, filename), SyntaxError);
  fs.rmSync(filename);
  assert.throws(() => updateBuildConfig(overrides, filename), /ENOENT/);
});

test('full ZIPs retain content and modes and work with the system ZIP reader', t => {
  const root = fixture(t), updates = build(root, '0.1.0', false);
  for (const { target, update, directory } of updates) {
    const full = validateUpdate(update, target, directory);
    assert.equal(update.files.length, target.includes('darwin') ? 4 : 2);
    assert.ok(update.files.every(file => file.mode === 0o755 || file.mode === 0o644));
    if (process.platform !== 'win32') execFileSync('unzip', ['-tq', full.path]);
    const binary = update.files.find(file => file.path.endsWith('passpine') || file.path === 'passpine.exe');
    // A macOS fixture created on Windows has no POSIX executable permission.
    // Native macOS staging preserves it; Linux/Windows binary modes are explicit.
    if (!target.includes('darwin') || process.platform !== 'win32') assert.equal(binary.mode, 0o755);
    const original = fs.readFileSync(full.path); fs.appendFileSync(full.path, 'tampered');
    assert.throws(() => validateUpdate(update, target, directory), /ZIP checksum mismatch/); fs.writeFileSync(full.path, original);
    const mismatched = structuredClone(update); mismatched.files[0].sha256 = '0'.repeat(64);
    assert.throws(() => validateUpdate(mismatched, target, directory), /file checksum mismatch/);
  }
});

test('signed feeds authenticate exact payload bytes and reject unknown or mismatched signing keys', t => {
  const root = fixture(t), signingKeys = keys(), updates = build(root, '0.1.0', false);
  const artifacts = feed(root, '0.1.0', updates, signingKeys), filename = artifacts.find(item => item.name === 'update-feed.json').path;
  const bytes = fs.readFileSync(filename), parsed = verifyFeed(bytes, signingKeys.publicKey);
  assert.equal(artifacts.length, 4); assert.equal(parsed.version, '0.1.0'); assert.equal(parsed.assets.length, 3);
  assert.deepEqual(Object.keys(parsed).sort(), ['assets', 'schema', 'version']);
  assert.ok(parsed.assets.every(asset => !('deltas' in asset) && asset.full.url.startsWith('https://github.com/knothhe/passpine/releases/download/v0.1.0/')));
  assert.throws(() => verifyFeed(bytes, keys().publicKey), /signature verification/);
  const modified = JSON.parse(bytes); const payload = JSON.parse(Buffer.from(modified.payload, 'base64')); payload.version = '9.0.0';
  modified.payload = Buffer.from(JSON.stringify(payload)).toString('base64');
  assert.throws(() => verifyFeed(Buffer.from(JSON.stringify(modified)), signingKeys.publicKey), /signature verification/);
  assert.throws(() => feed(root, '0.1.0', updates, { ...signingKeys, privateKey: keys().privateKey }), /does not match/);
  assert.throws(() => feed(root, '0.1.0', updates.slice(0, 2), signingKeys), /incomplete/);
});

test('publication emits only full packages and a deterministic feed without consulting previous feeds or inline notes', t => {
  const root = fixture(t), signingKeys = keys(), current = build(root, '0.2.0', true);
  const options = { version: '0.2.0', repo: 'knothhe/passpine', updates: current, output: path.join(root, 'signed'), ...signingKeys };
  const artifacts = createUpdateFeed({ ...options,
    // Old callers may still supply these properties. Publication must neither
    // parse a baseline nor copy any of its optional presentation fields.
    previousFeed: Buffer.from('not a valid historical feed'), notes: { en: 'Old inline notes', 'zh-CN': '旧版内联说明' },
  });
  const parsed = verifyFeed(fs.readFileSync(artifacts.at(-1).path), signingKeys.publicKey);
  assert.equal(artifacts.length, 4);
  assert.deepEqual(Object.keys(parsed).sort(), ['assets', 'schema', 'version']);
  assert.deepEqual(fs.readdirSync(options.output), ['update-feed.json']);
  const beforeRetry = artifacts.map(item => ({ name: item.name, bytes: fs.readFileSync(item.path) }));
  const retried = createUpdateFeed({ ...options, updates: [...current].reverse() });
  for (const artifact of retried) assert.deepEqual(fs.readFileSync(artifact.path), beforeRetry.find(item => item.name === artifact.name).bytes, 'Retry must reproduce exact signed feed and full ZIP bytes.');
  for (const asset of parsed.assets) {
    assert.deepEqual(Object.keys(asset).sort(), ['arch', 'files', 'full', 'os']);
    const full = artifacts.find(item => asset.full.url.endsWith(item.name));
    assert.ok(full); assert.equal(full.size, asset.full.size);
    const entries = new Map(decodeZip(fs.readFileSync(full.path)).map(entry => [entry.path, entry]));
    assert.equal(entries.size, asset.files.length);
    for (const file of asset.files) {
      const entry = entries.get(file.path); assert.equal(crypto.createHash('sha256').update(entry.data).digest('hex'), file.sha256); assert.equal(entry.data.length, file.size); assert.equal(entry.mode, file.mode);
    }
  }
});

test('verification accepts previously signed schema 1 extra fields and still authenticates every payload byte', t => {
  const root = fixture(t), signingKeys = keys(), updates = build(root, '0.1.0', false);
  const artifacts = feed(root, '0.1.0', updates, signingKeys);
  const parsed = verifyFeed(fs.readFileSync(artifacts.at(-1).path), signingKeys.publicKey);
  const legacy = { ...parsed, notes: { en: 'Legacy notes', 'zh-CN': '旧版说明' }, assets: parsed.assets.map(asset => ({ ...asset, deltas: [{ fromVersion: '0.0.1', ...asset.full }] })) };
  const payload = Buffer.from(JSON.stringify(legacy)), envelope = {
    payload: payload.toString('base64'), signature: crypto.sign(null, payload, signingKeys.privateKey).toString('base64'),
  };
  assert.deepEqual(verifyFeed(Buffer.from(JSON.stringify(envelope)), signingKeys.publicKey), legacy);
  legacy.notes.en = 'Tampered notes'; envelope.payload = Buffer.from(JSON.stringify(legacy)).toString('base64');
  assert.throws(() => verifyFeed(Buffer.from(JSON.stringify(envelope)), signingKeys.publicKey), /signature verification/);
});

test('empty macOS resources are preserved while oversized manifests are rejected', t => {
  const root = fixture(t); build(root, '0.1.0', false);
  const resource = path.join(root, '0.1.0/dist/Passpine.app/Contents/Resources/empty'); fs.writeFileSync(resource, '');
  const directory = path.join(root, 'empty'), update = stageUpdate('0.1.0', 'client-darwin-arm64', path.join(root, '0.1.0'), directory);
  assert.equal(update.files.find(file => file.path.endsWith('/empty')).size, 0);
  validateUpdate(update, 'client-darwin-arm64', directory);
  update.files[0].size = 512 * 1024 * 1024;
  assert.throws(() => validateUpdate(update, 'client-darwin-arm64', directory), /512 MiB/);
});

test('update staging rejects symlinks, escaped and platform-unexpected paths', t => {
  const root = fixture(t); build(root, '0.1.0', false);
  const mac = path.join(root, '0.1.0', 'dist', 'Passpine.app');
  fs.writeFileSync(path.join(mac, 'unexpected'), 'unexpected');
  assert.throws(() => stageUpdate('0.1.0', 'client-darwin-arm64', path.join(root, '0.1.0'), path.join(root, 'bad')), /Unexpected update file/);
  fs.rmSync(path.join(mac, 'unexpected'));
  if (process.platform !== 'win32') {
    fs.symlinkSync('/etc/hosts', path.join(mac, 'Contents', 'linked'));
    assert.throws(() => stageUpdate('0.1.0', 'client-darwin-arm64', path.join(root, '0.1.0'), path.join(root, 'bad')), /Symlink/);
  }
  const update = build(root, '0.2.0', true)[1]; update.update.files[0].path = '../passpine';
  assert.throws(() => validateUpdate(update.update, update.target, update.directory), /Unsafe/);
});

test('ZIP parser rejects duplicates and corruption before signing', () => {
  const entry = { path: 'passpine', mode: 0o755, data: Buffer.from('executable') };
  assert.throws(() => encodeZip([entry, entry]), /Duplicate/);
  const bytes = encodeZip([entry]); bytes[40] ^= 64;
  assert.throws(() => decodeZip(bytes), /Invalid|Corrupt|incorrect|invalid/);
});

test('CI exposes the signing private key only to the publication job', () => {
  const buildWorkflow = fs.readFileSync(new URL('../.github/workflows/ci.yml', import.meta.url), 'utf8');
  const releaseWorkflow = fs.readFileSync(new URL('../.github/workflows/release.yml', import.meta.url), 'utf8');
  assert.doesNotMatch(buildWorkflow, /PASSPINE_UPDATE_PRIVATE_KEY/);
  const beforePublish = releaseWorkflow.split('  publish:')[0]; assert.doesNotMatch(beforePublish, /PASSPINE_UPDATE_PRIVATE_KEY/);
  assert.match(releaseWorkflow, /PASSPINE_UPDATE_PRIVATE_KEY: \$\{\{ secrets.PASSPINE_UPDATE_PRIVATE_KEY \}\}/);
});
