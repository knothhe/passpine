import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import zlib from 'node:zlib';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import { test } from 'node:test';
import { createUpdateFeed, decodeZip, installerPackageFor, readLegacyUpdateFeed, readUpdateSigningConfig, stageUpdate, updateBuildConfig, validateUpdate, verifyFeed } from './update-artifacts.mjs';
import { createInstallerFixtures, createOrdinaryInstaller, encodeZip, installerTest, legacyFeedBytes, signingKeys as keys, targets } from './test-fixtures/update-installer.mjs';

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-updates-'));
  t.after(() => fs.rmSync(root, { force: true, recursive: true }));
  return root;
}
function build(root, version, changed, selected = targets) {
  const input = path.join(root, version); createInstallerFixtures(input, version, changed);
  return selected.map(target => {
    const directory = path.join(input, target); createOrdinaryInstaller(input, directory, version, target);
    return { target, directory, update: stageUpdate(version, target, input, directory) };
  });
}
function feed(root, version, updates, signingKeys, extra = {}) {
  return createUpdateFeed({ version, repo: 'knothhe/passpine', updates,
    output: path.join(root, version, 'signed'), legacyFeedBytes: legacyFeedBytes(signingKeys, updates), ...signingKeys, ...extra });
}
function rewriteArtifact(item, bytes) {
  fs.writeFileSync(path.join(item.directory, item.update.full.name), bytes);
  item.update.full.size = bytes.length;
  item.update.full.sha256 = crypto.createHash('sha256').update(bytes).digest('hex');
}
function resignFeed(bytes, signingKeys, mutate) {
  const envelope = JSON.parse(bytes), payload = JSON.parse(Buffer.from(envelope.payload, 'base64')); mutate(payload);
  const encoded = Buffer.from(JSON.stringify(payload));
  return Buffer.from(JSON.stringify({ payload: encoded.toString('base64'), signature: crypto.sign(null, encoded, signingKeys.privateKey).toString('base64') }));
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

test('staging reuses ordinary macOS ZIP and Linux TAR content without writing an update package', t => {
  const root = fixture(t), updates = build(root, '0.1.6', false, targets.slice(0, 2));
  for (const item of updates) {
    const full = validateUpdate(item.update, item.target, item.directory, '0.1.6');
    assert.equal(item.update.format, item.target.includes('darwin') ? 'app-zip' : 'tar-gzip');
    assert.equal(item.update.files.length, item.target.includes('darwin') ? 4 : 2);
    assert.deepEqual(fs.readdirSync(item.directory), [item.update.full.name]);
    assert.ok(!full.name.includes('-update-'));
    const before = fs.readFileSync(full.path); fs.appendFileSync(full.path, 'tampered');
    assert.throws(() => validateUpdate(item.update, item.target, item.directory), /Installer checksum mismatch/); fs.writeFileSync(full.path, before);
    const mismatched = structuredClone(item.update); mismatched.files[0].sha256 = '0'.repeat(64);
    assert.throws(() => validateUpdate(mismatched, item.target, item.directory), /file checksum mismatch/);
    assert.throws(() => validateUpdate({ ...item.update, format: 'zip' }, item.target, item.directory), /format mismatch/);
    assert.throws(() => validateUpdate(item.update, item.target, item.directory, '0.1.7'), /artifact name/);
  }
});

test('Windows full update reuses and statically verifies the ordinary NSIS installer', installerTest, t => {
  const root = fixture(t), [item] = build(root, '0.1.6', false, [targets[2]]);
  assert.equal(item.update.format, 'nsis');
  assert.equal(item.update.full.name, 'Passpine-0.1.6-windows-amd64-setup.exe');
  assert.deepEqual(item.update.files.map(file => file.path), ['passpine-host.exe', 'passpine.exe']);
  assert.deepEqual(fs.readdirSync(item.directory), [item.update.full.name]);
  const altered = structuredClone(item.update); altered.files[0].sha256 = '0'.repeat(64);
  assert.throws(() => validateUpdate(altered, item.target, item.directory), /file checksum mismatch/);
  const original = fs.readFileSync(path.join(item.directory, item.update.full.name)), marker = Buffer.from('passpine-update-extract-v1-a8e19eb9-3dc3-4ccf-8b58-97546f3051f0\0', 'utf16le');
  const missing = Buffer.from(original), markerOffset = missing.indexOf(marker); assert.ok(markerOffset > 0); missing[markerOffset] ^= 1;
  // A matching string appended outside PE resources must not authorize execution.
  rewriteArtifact(item, Buffer.concat([missing, marker]));
  assert.throws(() => validateUpdate(item.update, item.target, item.directory), /extraction protocol/);
  rewriteArtifact(item, Buffer.from('MZ corrupt NSIS fixture'));
  assert.throws(() => validateUpdate(item.update, item.target, item.directory), /PE header|Invalid Windows/);
});

test('signed feed authenticates a schema 2 installer extension while preserving the verified v0.1.5 legacy payload', installerTest, t => {
  const root = fixture(t), signingKeys = keys(), updates = build(root, '0.1.6', true), legacy = legacyFeedBytes(signingKeys, updates);
  const artifacts = feed(root, '0.1.6', updates, signingKeys), bytes = fs.readFileSync(artifacts[0].path), parsed = verifyFeed(bytes, signingKeys.publicKey);
  assert.equal(artifacts.length, 1); assert.equal(artifacts[0].name, 'update-feed.json');
  assert.deepEqual({ schema: parsed.schema, version: parsed.version, assets: parsed.assets }, readLegacyUpdateFeed(signingKeys.publicKey, legacy));
  assert.equal(parsed.version, '0.1.5'); assert.equal(parsed.installerUpdate.schema, 2); assert.equal(parsed.installerUpdate.version, '0.1.6');
  assert.deepEqual(Object.keys(parsed).sort(), ['assets', 'installerUpdate', 'schema', 'version']);
  assert.equal(parsed.installerUpdate.assets.length, 3);
  for (const asset of parsed.installerUpdate.assets) {
    assert.deepEqual(Object.keys(asset).sort(), ['arch', 'files', 'format', 'full', 'os']);
    assert.ok(!asset.full.url.includes('-update-'));
    const item = updates.find(item => item.update.os === asset.os), expected = installerPackageFor('0.1.6', item.target);
    assert.equal(asset.full.url, `https://github.com/knothhe/passpine/releases/download/v0.1.6/${expected.name}`);
    assert.equal(asset.full.sha256, item.update.full.sha256); assert.equal(asset.full.size, item.update.full.size);
  }
  assert.throws(() => verifyFeed(bytes, keys().publicKey), /signature verification/);
  const tampered = JSON.parse(bytes), payload = JSON.parse(Buffer.from(tampered.payload, 'base64')); payload.installerUpdate.version = '9.0.0';
  tampered.payload = Buffer.from(JSON.stringify(payload)).toString('base64');
  assert.throws(() => verifyFeed(Buffer.from(JSON.stringify(tampered)), signingKeys.publicKey), /signature verification/);
  assert.throws(() => feed(root, '0.1.6', updates, { ...signingKeys, privateKey: keys().privateKey }), /does not match/);
  assert.throws(() => feed(root, '0.1.6', updates.slice(0, 2), signingKeys), /incomplete/);
  assert.throws(() => feed(root, '0.1.5', updates, signingKeys), /newer than/);
  for (const mutate of [feed => { feed.version = '0.1.6'; }, feed => { feed.installerUpdate.schema = 3; }, feed => { feed.installerUpdate.assets[0].format = 'zip'; },
    feed => { feed.installerUpdate.assets[0].full.url = 'https://example.com/unsafe.exe'; },
    feed => { feed.installerUpdate.assets[1] = feed.installerUpdate.assets[0]; }]) {
    assert.throws(() => verifyFeed(resignFeed(bytes, signingKeys, mutate), signingKeys.publicKey), /Invalid.*installer|Legacy/);
  }
});

test('publication is deterministic, emits only the feed and never consults a moving previous release', installerTest, t => {
  const root = fixture(t), signingKeys = keys(), updates = build(root, '0.1.6', true);
  const legacy = legacyFeedBytes(signingKeys, updates);
  const artifacts = feed(root, '0.1.6', updates, signingKeys, { legacyFeedBytes: legacy, previousFeed: Buffer.from('invalid'), notes: { en: 'must not be embedded' } });
  const before = fs.readFileSync(artifacts[0].path);
  const retry = feed(root, '0.1.6', [...updates].reverse(), signingKeys, { legacyFeedBytes: legacy });
  assert.deepEqual(fs.readFileSync(retry[0].path), before);
  assert.deepEqual(fs.readdirSync(path.dirname(retry[0].path)), ['update-feed.json']);
  assert.ok(!Buffer.from(JSON.parse(before).payload, 'base64').includes(Buffer.from('must not be embedded')));
});

test('the checked-in compatibility feed is authenticated by the unchanged official public key', () => {
  const config = readUpdateSigningConfig(), legacy = readLegacyUpdateFeed(config.publicKey);
  assert.equal(legacy.version, '0.1.5'); assert.equal(legacy.assets.length, 3);
  assert.ok(legacy.assets.every(asset => asset.full.url.includes('/v0.1.5/Passpine-0.1.5-update-')));
});

test('legacy compatibility rejects changed bytes, another key, version, target and package URL', t => {
  const root = fixture(t), signingKeys = keys(), items = build(root, '0.1.6', false, targets.slice(0, 2));
  const windows = { update: { os: 'windows', arch: 'amd64', files: ['passpine.exe', 'passpine-host.exe'].map(name => ({ path: name, size: 1, mode: 0o755, sha256: '1'.repeat(64) })) } };
  const legacy = legacyFeedBytes(signingKeys, [...items, windows]);
  assert.equal(readLegacyUpdateFeed(signingKeys.publicKey, legacy).version, '0.1.5');
  assert.throws(() => readLegacyUpdateFeed(keys().publicKey, legacy), /signature verification/);
  for (const mutate of [feed => { feed.version = '0.1.4'; }, feed => { feed.schema = 2; }, feed => { feed.assets.pop(); },
    feed => { feed.assets[0].full.url = feed.assets[0].full.url.replace('knothhe/passpine', 'another/project'); },
    feed => { feed.assets[1] = feed.assets[0]; }, feed => { feed.notes = {}; }]) {
    assert.throws(() => readLegacyUpdateFeed(signingKeys.publicKey, resignFeed(legacy, signingKeys, mutate)), /Legacy|legacy|Invalid update/);
  }
  const changed = JSON.parse(legacy); changed.payload = Buffer.from('tampered').toString('base64');
  assert.throws(() => readLegacyUpdateFeed(signingKeys.publicKey, Buffer.from(JSON.stringify(changed))), /signature verification/);
});

test('macOS ZIP validation rejects unexpected roots, resource metadata, symlinks, duplicates and traversal', t => {
  const root = fixture(t), [item] = build(root, '0.1.6', false, [targets[0]]), original = fs.readFileSync(path.join(item.directory, item.update.full.name));
  const entries = decodeZip(original);
  for (const name of ['__MACOSX/Passpine.app/._Contents', 'Other.app/Contents/data', 'Passpine.app/../escape']) {
    rewriteArtifact(item, encodeZip([...entries, { path: name, data: Buffer.from('unexpected'), mode: 0o644 }]));
    assert.throws(() => validateUpdate(item.update, item.target, item.directory), /Unexpected macOS|Unsafe ZIP/);
  }
  assert.throws(() => encodeZip([entries[0], entries[0]]), /Duplicate/);
  const symlink = Buffer.from(original), central = symlink.indexOf(Buffer.from('504b0102', 'hex'));
  symlink.writeUInt32LE((0o120777 << 16) >>> 0, central + 38); rewriteArtifact(item, symlink);
  assert.throws(() => validateUpdate(item.update, item.target, item.directory), /non-regular/);
  const corrupted = Buffer.from(original); corrupted[40] ^= 64;
  assert.throws(() => decodeZip(corrupted), /Invalid|Corrupt|incorrect|invalid/);
  assert.throws(() => decodeZip(Buffer.alloc(0)), /Invalid/);
});

test('Linux package rejects non-regular members, extras and archive content that differs from staged binaries', t => {
  const root = fixture(t), [item] = build(root, '0.1.6', false, [targets[1]]), dist = path.join(root, '0.1.6/dist');
  const archive = path.join(item.directory, item.update.full.name), original = fs.readFileSync(archive);
  rewriteArtifact(item, Buffer.concat([original, zlib.gzipSync(Buffer.alloc(512))]));
  assert.throws(() => validateUpdate(item.update, item.target, item.directory), /multiple GZIP/);
  rewriteArtifact(item, Buffer.concat([original, Buffer.from('trailer')]));
  assert.throws(() => validateUpdate(item.update, item.target, item.directory), /GZIP trailer/);
  const corrupted = Buffer.from(original); corrupted[corrupted.length - 8] ^= 1; rewriteArtifact(item, corrupted);
  assert.throws(() => validateUpdate(item.update, item.target, item.directory), /GZIP trailer/);
  fs.writeFileSync(path.join(dist, 'unexpected'), 'extra');
  execFileSync('tar', ['--format=ustar', '-czf', archive, '-C', dist, 'passpine', 'passpine-host', 'version.txt', 'install.sh', 'app-icon-monochrome.png', 'unexpected'], { env: { ...process.env, COPYFILE_DISABLE: '1' } });
  rewriteArtifact(item, fs.readFileSync(archive));
  assert.throws(() => validateUpdate(item.update, item.target, item.directory), /unexpected TAR/);
  if (process.platform !== 'win32') {
    fs.rmSync(path.join(dist, 'passpine')); fs.symlinkSync('passpine-host', path.join(dist, 'passpine'));
    execFileSync('tar', ['--format=ustar', '-czf', archive, '-C', dist, 'passpine', 'passpine-host', 'version.txt', 'install.sh', 'app-icon-monochrome.png'], { env: { ...process.env, COPYFILE_DISABLE: '1' } });
    rewriteArtifact(item, fs.readFileSync(archive));
    assert.throws(() => validateUpdate(item.update, item.target, item.directory), /unexpected TAR/);
  }
});

test('empty macOS resources are preserved and staging fails closed for symlinks, escaped paths and oversized manifests', t => {
  const root = fixture(t), input = path.join(root, 'input'); createInstallerFixtures(input, '0.1.6');
  const directory = path.join(root, 'package'), resource = path.join(input, 'dist/Passpine.app/Contents/Resources/empty'); fs.writeFileSync(resource, '');
  createOrdinaryInstaller(input, directory, '0.1.6', targets[0]);
  const update = stageUpdate('0.1.6', targets[0], input, directory);
  assert.equal(update.files.find(file => file.path.endsWith('/empty')).size, 0);
  const oversized = structuredClone(update); oversized.files[0].size = 512 * 1024 * 1024;
  assert.throws(() => validateUpdate(oversized, targets[0], directory), /512 MiB/);
  for (const name of ['../passpine', 'Contents/unsafe:name', 'Contents/unsafe\nname', 'Contents/unsafe\rname']) {
    const escaped = structuredClone(update); escaped.files[0].path = name;
    assert.throws(() => validateUpdate(escaped, targets[0], directory), /Unsafe/);
  }
  const unexpected = path.join(input, 'dist/Passpine.app/unexpected'); fs.writeFileSync(unexpected, 'unexpected');
  assert.throws(() => stageUpdate('0.1.6', targets[0], input, directory), /Unexpected update file/); fs.rmSync(unexpected);
  if (process.platform !== 'win32') {
    fs.symlinkSync('/etc/hosts', path.join(input, 'dist/Passpine.app/Contents/linked'));
    assert.throws(() => stageUpdate('0.1.6', targets[0], input, directory), /Symlink/);
  }
});

test('CI exposes the signing private key only to the publication job', () => {
  const buildWorkflow = fs.readFileSync(new URL('../.github/workflows/ci.yml', import.meta.url), 'utf8');
  const releaseWorkflow = fs.readFileSync(new URL('../.github/workflows/release.yml', import.meta.url), 'utf8');
  assert.doesNotMatch(buildWorkflow, /PASSPINE_UPDATE_PRIVATE_KEY/);
  const beforePublish = releaseWorkflow.split('  publish:')[0]; assert.doesNotMatch(beforePublish, /PASSPINE_UPDATE_PRIVATE_KEY/);
  assert.match(releaseWorkflow, /PASSPINE_UPDATE_PRIVATE_KEY: \$\{\{ secrets.PASSPINE_UPDATE_PRIVATE_KEY \}\}/);
});
