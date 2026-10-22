import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { test } from 'node:test';
import { collectPackages, collectUpdates, packagesFor, stage, updatePublicationConfig, validatePackage, verifyTag } from './ci-release.mjs';
import { notesForTag } from './release-notes.mjs';
import { createUpdateFeed, stageUpdate, verifyFeed } from './update-artifacts.mjs';
import { createInstallerFixtures, createOrdinaryInstaller, installerTest, legacyFeedBytes } from './test-fixtures/update-installer.mjs';
const browserIdentity = JSON.parse(fs.readFileSync(new URL('../config/browser-identities.json', import.meta.url), 'utf8')).release;

// Read the repository's block-style workflow mappings without a runtime YAML dependency.
function workflowBlock(source, keys) {
  let lines = source.split(/\r?\n/);
  for (const [depth, key] of keys.entries()) {
    const prefix = `${'  '.repeat(depth)}${key}:`;
    const start = lines.findIndex(line => line.startsWith(prefix));
    assert.notEqual(start, -1, `Missing workflow mapping: ${keys.join('.')}`);
    let end = start + 1;
    while (end < lines.length) {
      const line = lines[end];
      if (line.trim() && !line.trimStart().startsWith('#') && line.search(/\S/) <= depth * 2) break;
      end++;
    }
    lines = lines.slice(start + 1, end);
  }
  return lines.join('\n');
}

function mappingKeys(block) {
  const entries = [...block.matchAll(/^( *)([A-Za-z_][\w-]*):/gm)];
  const indentation = Math.min(...entries.map(entry => entry[1].length));
  return entries.filter(entry => entry[1].length === indentation).map(entry => entry[2]);
}

test('CI runs only as a reusable release build with a required tag', () => {
  const ci = fs.readFileSync(new URL('../.github/workflows/ci.yml', import.meta.url), 'utf8');
  assert.deepEqual(mappingKeys(workflowBlock(ci, ['on'])), ['workflow_call'], 'Ordinary pushes and pull requests must not trigger CI.');
  const tag = workflowBlock(ci, ['on', 'workflow_call', 'inputs', 'tag']);
  assert.match(tag, /^ +required: true\s*$/m);
  assert.match(tag, /^ +type: string\s*$/m);
  assert.ok(!mappingKeys(tag).includes('default'), 'Release builds must receive an explicit tag.');
});

test('Release runs automatically only for version tags and passes the verified tag to CI', () => {
  const release = fs.readFileSync(new URL('../.github/workflows/release.yml', import.meta.url), 'utf8');
  assert.deepEqual(mappingKeys(workflowBlock(release, ['on'])), ['push', 'workflow_dispatch']);
  const push = workflowBlock(release, ['on', 'push']);
  assert.deepEqual(mappingKeys(push), ['tags'], 'A branch push must not start a release.');
  assert.match(push, /^ +tags: \['v\*'\]\s*$/m);
  const retryTag = workflowBlock(release, ['on', 'workflow_dispatch', 'inputs', 'tag']);
  assert.match(retryTag, /^ +required: true\s*$/m);
  assert.match(retryTag, /^ +type: string\s*$/m);
  const verify = workflowBlock(release, ['jobs', 'verify']);
  assert.match(verify, /RELEASE_TAG: \$\{\{ inputs\.tag \|\| github\.ref_name \}\}/);
  const build = workflowBlock(release, ['jobs', 'build']);
  assert.match(build, /^ +needs: verify\s*$/m);
  assert.match(build, /^ +uses: \.\/\.github\/workflows\/ci\.yml\s*$/m);
  assert.match(workflowBlock(release, ['jobs', 'build', 'with']), /^ +tag: \$\{\{ needs\.verify\.outputs\.tag \}\}\s*$/m);
});

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-ci-release-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const gitConfig = path.join(root, 'gitconfig');
  fs.writeFileSync(gitConfig, '');
  const git = (...args) => execFileSync('git', args, { cwd: root, encoding: 'utf8', env: { ...process.env, GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: gitConfig } }).trim();
  fs.mkdirSync(path.join(root, 'browser'));
  fs.writeFileSync(path.join(root, '.release.json'), JSON.stringify({ tag_prefix: 'v' }));
  for (const name of ['package.json', 'browser/package.json']) fs.writeFileSync(path.join(root, name), '{"version":"0.1.6"}\n');
  git('init', '--initial-branch=main');
  git('config', 'user.name', 'CI Fixture'); git('config', 'user.email', 'ci@example.test');
  git('config', 'commit.gpgsign', 'false'); git('config', 'tag.gpgsign', 'false');
  git('add', '.'); git('commit', '-m', 'feat: initial release'); git('tag', '-a', 'v0.1.6', '-m', 'Release v0.1.6');
  return { root, git };
}

test('CI verifies the exact tag checkout and synchronized package versions', t => {
  const { root, git } = fixture(t);
  assert.equal(verifyTag('v0.1.6', root).head, git('rev-parse', 'HEAD'));
  assert.throws(() => verifyTag('v0.2.0', root), /differs from the package versions/);
  assert.throws(() => verifyTag('main', root), /release tag/);
  fs.writeFileSync(path.join(root, 'browser/package.json'), '{"version":"0.2.0"}\n');
  assert.throws(() => verifyTag('v0.1.6', root), /differs from package.json/);
  git('add', '.'); git('commit', '-m', 'feat: next version');
  fs.writeFileSync(path.join(root, 'browser/package.json'), '{"version":"0.1.6"}\n');
  assert.throws(() => verifyTag('v0.1.6', root), /Checkout must point/);
});

test('standalone notes use the specified tag and exclude later commits', t => {
  const { root, git } = fixture(t);
  git('commit', '--allow-empty', '-m', 'feat: after release');
  const notes = notesForTag('fixture/passpine', 'v0.1.6', root);
  assert.match(notes, /Full Changelog/);
  assert.match(notes, /initial release/);
  assert.doesNotMatch(notes, /after release/);
});

const archiveTest = { skip: process.platform === 'win32' && 'ZIP archive fixtures require POSIX zip/unzip' };
function createPackage(root, name, version = '0.1.6') {
  const destination = path.join(root, 'packages');
  const input = path.join(root, 'input');
  fs.mkdirSync(destination, { recursive: true });
  fs.rmSync(input, { recursive: true, force: true }); fs.mkdirSync(input);
  const filename = path.join(destination, name);
  if (name.endsWith('.exe')) fs.writeFileSync(filename, 'MZ fixture installer');
  else if (name.endsWith('.tar.gz')) {
    for (const file of ['passpine', 'passpine-host', 'install.sh', 'app-icon-monochrome.png']) fs.writeFileSync(path.join(input, file), 'fixture');
    fs.writeFileSync(path.join(input, 'version.txt'), `${version}\n`);
    execFileSync('tar', ['-czf', filename, '-C', input, 'passpine', 'passpine-host', 'install.sh', 'app-icon-monochrome.png', 'version.txt']);
  } else {
    const browser = name.startsWith('passpine-');
    const entry = browser ? 'manifest.json' : 'Passpine.app/Contents/Info.plist';
    fs.mkdirSync(path.dirname(path.join(input, entry)), { recursive: true });
    fs.writeFileSync(path.join(input, entry), browser ? JSON.stringify({ version, key: browserIdentity.key, browser_specific_settings: { gecko: { id: browserIdentity.firefoxID } } }) : `<plist><key>CFBundleIdentifier</key><string>com.knothhe.passpine</string><key>CFBundleShortVersionString</key>\n<string>${version}</string><key>CFBundleVersion</key>\n<string>${version}</string></plist>`);
    execFileSync('zip', ['-q', filename, entry], { cwd: input });
  }
  const bytes = fs.readFileSync(filename);
  return { name, size: bytes.length, sha256: crypto.createHash('sha256').update(bytes).digest('hex') };
}

function packageSet(t) {
  const { root, git } = fixture(t);
  const metadata = verifyTag('v0.1.6', root);
  for (const target of ['client-darwin-arm64', 'client-linux-amd64', 'client-win32-amd64', 'browser']) {
    const artifacts = packagesFor(metadata.version, target).map(name => createPackage(root, name));
    fs.writeFileSync(path.join(root, 'packages', `manifest-${target}.json`), JSON.stringify({ schema: 2, channel: 'release', ...metadata, target, artifacts }));
  }
  return { root, git, directory: path.join(root, 'packages') };
}

test('publication requires exactly Mac arm64, Linux, Windows and both browser packages', archiveTest, t => {
  const { root, directory } = packageSet(t);
  const packages = collectPackages('v0.1.6', directory, root);
  assert.deepEqual(packages.map(({ name }) => name).sort(), [
    'Passpine-0.1.6-macos-arm64.zip',
    'Passpine-0.1.6-linux-amd64.tar.gz',
    'Passpine-0.1.6-windows-amd64-setup.exe',
    'passpine-0.1.6-chrome.zip',
    'passpine-0.1.6-firefox.zip',
  ].sort());
  for (const target of ['client-darwin-arm64', 'client-linux-amd64', 'client-win32-amd64', 'browser']) {
    const filename = path.join(directory, `manifest-${target}.json`);
    const manifest = fs.readFileSync(filename, 'utf8');
    fs.rmSync(filename);
    assert.throws(() => collectPackages('v0.1.6', directory, root), new RegExp(`incomplete.*${target}`));
    fs.writeFileSync(filename, manifest);
  }
  const filename = path.join(directory, 'manifest-browser.json');
  const manifest = JSON.parse(fs.readFileSync(filename, 'utf8'));
  manifest.artifacts.pop();
  fs.writeFileSync(filename, JSON.stringify(manifest));
  assert.throws(() => collectPackages('v0.1.6', directory, root), /Unexpected release package/);
});

test('CI release targets reject Intel Mac packages and provenance', archiveTest, t => {
  const { root, directory } = packageSet(t);
  assert.throws(() => packagesFor('0.1.6', 'client-darwin-x86_64'), /Unsupported release target/);
  const filename = path.join(directory, 'manifest-client-darwin-arm64.json');
  const manifest = JSON.parse(fs.readFileSync(filename, 'utf8'));
  const intelArtifact = createPackage(root, 'Passpine-0.1.6-macos-x86_64.zip');
  fs.writeFileSync(filename, JSON.stringify({ ...manifest, artifacts: [intelArtifact] }));
  assert.throws(() => collectPackages('v0.1.6', directory, root), /Unexpected release package/);
  fs.writeFileSync(filename, JSON.stringify(manifest));
  fs.writeFileSync(path.join(directory, 'manifest-client-darwin-x86_64.json'), JSON.stringify({
    ...manifest, target: 'client-darwin-x86_64', artifacts: [intelArtifact],
  }));
  assert.throws(() => collectPackages('v0.1.6', directory, root), /Invalid or duplicate/);
});

test('publication rejects changed packages, another commit and duplicate or unexpected manifest entries', archiveTest, t => {
  const { root, directory } = packageSet(t);
  const filename = path.join(directory, 'manifest-client-win32-amd64.json');
  const original = fs.readFileSync(filename, 'utf8');
  const manifest = JSON.parse(original);
  fs.writeFileSync(filename, JSON.stringify({ ...manifest, channel: 'dev' }));
  assert.throws(() => collectPackages('v0.1.6', directory, root), /Invalid or duplicate/);
  fs.writeFileSync(filename, JSON.stringify({ ...manifest, head: 'different-commit' }));
  assert.throws(() => collectPackages('v0.1.6', directory, root), /Invalid or duplicate/);
  fs.writeFileSync(filename, original);
  fs.writeFileSync(path.join(directory, 'manifest-duplicate.json'), original);
  assert.throws(() => collectPackages('v0.1.6', directory, root), /Invalid or duplicate/);
  fs.rmSync(path.join(directory, 'manifest-duplicate.json'));
  fs.writeFileSync(filename, JSON.stringify({ ...manifest, artifacts: [{ ...manifest.artifacts[0], name: '../unexpected.exe' }] }));
  assert.throws(() => collectPackages('v0.1.6', directory, root), /Unexpected release package/);
  fs.writeFileSync(filename, original);
  fs.appendFileSync(path.join(directory, manifest.artifacts[0].name), 'corruption');
  assert.throws(() => collectPackages('v0.1.6', directory, root), /checksum mismatch/);
});

test('release validation rejects Dev browser and macOS identities with a stable version', archiveTest, t => {
  const { root } = fixture(t);
  for (const name of ['passpine-0.1.6-chrome.zip', 'passpine-0.1.6-firefox.zip', 'Passpine-0.1.6-macos-arm64.zip']) {
    createPackage(root, name);
    const archive = path.join(root, 'packages', name);
    validatePackage(archive, '0.1.6');
    const browser = name.startsWith('passpine-');
    const entry = browser ? 'manifest.json' : 'Passpine.app/Contents/Info.plist';
    const source = path.join(root, 'input', entry);
    const contents = fs.readFileSync(source, 'utf8');
    if (browser) {
      const manifest = JSON.parse(contents);
      if (name.endsWith('firefox.zip')) manifest.browser_specific_settings.gecko.id = 'passpine-dev@knothhe.com';
      else manifest.key = 'dev-public-key';
      fs.writeFileSync(source, JSON.stringify(manifest));
    } else fs.writeFileSync(source, contents.replace('com.knothhe.passpine</string>', 'com.knothhe.passpine.dev</string>'));
    fs.rmSync(archive);
    execFileSync('zip', ['-qr', archive, '.'], { cwd: path.join(root, 'input') });
    assert.throws(() => validatePackage(archive, '0.1.6'), /release identity/);
  }
});

test('staging refuses a Dev native binary even if its version matches the tag', archiveTest, t => {
  const { root } = fixture(t);
  fs.mkdirSync(path.join(root, 'dist'));
  const binary = path.join(root, 'dist', 'passpine');
  fs.writeFileSync(binary, '#!/bin/sh\nif [ "$1" = "--build-channel" ]; then echo dev; else echo "Passpine 0.1.6"; fi\n', { mode: 0o755 });
  assert.throws(() => stage('v0.1.6', 'client-darwin-arm64', root), /Only release-channel/);
  assert.equal(fs.existsSync(path.join(root, 'dist/releases/0.1.6')), false);
});

test('browser staging validates archive versions before writing a release manifest', archiveTest, t => {
  const { root } = fixture(t);
  fs.mkdirSync(path.join(root, 'browser/.output'), { recursive: true });
  for (const name of packagesFor('0.1.6', 'browser')) {
    createPackage(root, name, '0.0.9');
    fs.copyFileSync(path.join(root, 'packages', name), path.join(root, 'browser/.output', name));
  }
  assert.throws(() => stage('v0.1.6', 'browser', root), /Archive version mismatch/);
  assert.equal(fs.existsSync(path.join(root, 'dist/releases/0.1.6/manifest-browser.json')), false);
});

test('signed publication reuses all ordinary packages and publishes exactly six attachments from the exact tag', { skip: archiveTest.skip || installerTest.skip }, t => {
  const { root, directory } = packageSet(t);
  assert.throws(() => collectUpdates('v0.1.6', directory, root), /Missing or mismatched/);
  createInstallerFixtures(root, '0.1.6');
  for (const target of ['client-darwin-arm64', 'client-linux-amd64', 'client-win32-amd64']) {
    const filename = path.join(directory, `manifest-${target}.json`), manifest = JSON.parse(fs.readFileSync(filename, 'utf8'));
    createOrdinaryInstaller(root, directory, '0.1.6', target);
    manifest.update = stageUpdate('0.1.6', target, root, directory);
    manifest.artifacts = [manifest.update.full]; fs.writeFileSync(filename, JSON.stringify(manifest));
  }
  const updates = collectUpdates('v0.1.6', directory, root);
  assert.equal(updates.length, 3);
  const signing = crypto.generateKeyPairSync('ed25519');
  const publicKey = signing.publicKey.export({ type: 'spki', format: 'der' }).subarray(-32).toString('base64');
  const privateKey = signing.privateKey.export({ type: 'pkcs8', format: 'pem' });
  const generated = createUpdateFeed({ version: '0.1.6', repo: 'knothhe/passpine', updates,
    output: path.join(root, 'published-feed'), publicKey, privateKey, legacyFeedBytes: legacyFeedBytes({ privateKey }, updates) });
  const attachments = [...collectPackages('v0.1.6', directory, root), ...generated];
  assert.equal(attachments.length, 6); assert.equal(new Set(attachments.map(item => item.name)).size, 6);
  assert.equal(attachments.filter(item => item.name.includes('-update-')).length, 0);
  assert.deepEqual(attachments.map(item => item.name).sort(), ['Passpine-0.1.6-macos-arm64.zip', 'Passpine-0.1.6-linux-amd64.tar.gz',
    'Passpine-0.1.6-windows-amd64-setup.exe', 'passpine-0.1.6-chrome.zip', 'passpine-0.1.6-firefox.zip', 'update-feed.json'].sort());
  const feed = verifyFeed(fs.readFileSync(generated[0].path), publicKey);
  assert.equal(feed.version, '0.1.5'); assert.equal(feed.installerUpdate.version, '0.1.6');
  assert.deepEqual(Object.keys(feed).sort(), ['assets', 'installerUpdate', 'schema', 'version']);
  assert.ok(feed.installerUpdate.assets.every(asset => Object.keys(asset).sort().join(',') === 'arch,files,format,full,os'));
  for (const asset of feed.installerUpdate.assets) {
    const ordinary = attachments.find(item => asset.full.url.endsWith(`/${item.name}`));
    assert.ok(ordinary); assert.equal(asset.full.sha256, ordinary.sha256); assert.equal(asset.full.size, ordinary.size);
  }
  const filename = path.join(directory, 'manifest-client-linux-amd64.json'), manifest = fs.readFileSync(filename, 'utf8');
  const changed = JSON.parse(manifest); changed.head = 'another-commit'; fs.writeFileSync(filename, JSON.stringify(changed));
  assert.throws(() => collectUpdates('v0.1.6', directory, root), /Invalid update provenance/);
  changed.head = JSON.parse(manifest).head; changed.artifacts[0].sha256 = '0'.repeat(64); fs.writeFileSync(filename, JSON.stringify(changed));
  assert.throws(() => collectUpdates('v0.1.6', directory, root), /mismatched ordinary/);
  fs.writeFileSync(filename, manifest);
  fs.appendFileSync(path.join(directory, JSON.parse(manifest).update.full.name), 'corruption');
  assert.throws(() => collectUpdates('v0.1.6', directory, root), /Installer checksum mismatch/);
});


test('production publication requires the pinned trust root and a matching private key', t => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-signing-config-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const signing = crypto.generateKeyPairSync('ed25519');
  const publicKey = signing.publicKey.export({ type: 'spki', format: 'der' }).subarray(-32).toString('base64');
  const privateKey = signing.privateKey.export({ type: 'pkcs8', format: 'pem' });
  const pinned = { schema: 1, algorithm: 'ed25519', repository: 'knothhe/passpine', publicKey, feedURL: 'https://github.com/knothhe/passpine/releases/latest/download/update-feed.json' };
  const filename = path.join(directory, 'update-signing.json'); fs.writeFileSync(filename, JSON.stringify(pinned));
  const env = { GITHUB_REPOSITORY: pinned.repository, PASSPINE_UPDATE_PRIVATE_KEY: privateKey };
  assert.deepEqual(updatePublicationConfig(env, filename), { key: publicKey, feed: pinned.feedURL });
  assert.deepEqual(updatePublicationConfig({ ...env, PASSPINE_UPDATE_PUBLIC_KEY: '', PASSPINE_UPDATE_FEED_URL: '' }, filename), { key: publicKey, feed: pinned.feedURL });
  assert.throws(() => updatePublicationConfig({ GITHUB_REPOSITORY: pinned.repository }, filename), /repository secret/);
  assert.throws(() => updatePublicationConfig({ ...env, GITHUB_REPOSITORY: 'another/project' }, filename), /checked-in repository/);
  assert.throws(() => updatePublicationConfig({ ...env, PASSPINE_UPDATE_FEED_URL: 'https://example.com/feed.json' }, filename), /checked-in repository/);
  const other = crypto.generateKeyPairSync('ed25519');
  const otherPublic = other.publicKey.export({ type: 'spki', format: 'der' }).subarray(-32).toString('base64');
  assert.throws(() => updatePublicationConfig({ ...env, PASSPINE_UPDATE_PUBLIC_KEY: otherPublic }, filename), /checked-in repository/);
  assert.throws(() => updatePublicationConfig({ ...env, PASSPINE_UPDATE_PRIVATE_KEY: other.privateKey.export({ type: 'pkcs8', format: 'pem' }) }, filename), /does not match/);
  assert.throws(() => updatePublicationConfig({ ...env, PASSPINE_UPDATE_PRIVATE_KEY: 'invalid sensitive diagnostic' }, filename), error => !error.message.includes('invalid sensitive diagnostic') && /PKCS#8/.test(error.message));
  assert.deepEqual(Object.keys(updatePublicationConfig(env, filename)).sort(), ['feed', 'key']);
});
