import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { test } from 'node:test';
import { collectPackages, packagesFor, stage, verifyTag } from './ci-release.mjs';
import { notesForTag } from './release-notes.mjs';

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
  for (const name of ['package.json', 'browser/package.json']) fs.writeFileSync(path.join(root, name), '{"version":"0.1.0"}\n');
  git('init', '--initial-branch=main');
  git('config', 'user.name', 'CI Fixture'); git('config', 'user.email', 'ci@example.test');
  git('config', 'commit.gpgsign', 'false'); git('config', 'tag.gpgsign', 'false');
  git('add', '.'); git('commit', '-m', 'feat: initial release'); git('tag', '-a', 'v0.1.0', '-m', 'Release v0.1.0');
  return { root, git };
}

test('CI verifies the exact tag checkout and synchronized package versions', t => {
  const { root, git } = fixture(t);
  assert.equal(verifyTag('v0.1.0', root).head, git('rev-parse', 'HEAD'));
  assert.throws(() => verifyTag('v0.2.0', root), /differs from the package versions/);
  assert.throws(() => verifyTag('main', root), /release tag/);
  fs.writeFileSync(path.join(root, 'browser/package.json'), '{"version":"0.2.0"}\n');
  assert.throws(() => verifyTag('v0.1.0', root), /differs from package.json/);
  git('add', '.'); git('commit', '-m', 'feat: next version');
  fs.writeFileSync(path.join(root, 'browser/package.json'), '{"version":"0.1.0"}\n');
  assert.throws(() => verifyTag('v0.1.0', root), /Checkout must point/);
});

test('standalone notes use the specified tag and exclude later commits', t => {
  const { root, git } = fixture(t);
  git('commit', '--allow-empty', '-m', 'feat: after release');
  const notes = notesForTag('fixture/passpine', 'v0.1.0', root);
  assert.match(notes, /Full Changelog/);
  assert.match(notes, /initial release/);
  assert.doesNotMatch(notes, /after release/);
});

const archiveTest = { skip: process.platform === 'win32' && 'ZIP archive fixtures require POSIX zip/unzip' };
function createPackage(root, name, version = '0.1.0') {
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
    fs.writeFileSync(path.join(input, entry), browser ? JSON.stringify({ version }) : `<plist><key>CFBundleShortVersionString</key>\n<string>${version}</string><key>CFBundleVersion</key>\n<string>${version}</string></plist>`);
    execFileSync('zip', ['-q', filename, entry], { cwd: input });
  }
  const bytes = fs.readFileSync(filename);
  return { name, size: bytes.length, sha256: crypto.createHash('sha256').update(bytes).digest('hex') };
}

function packageSet(t) {
  const { root, git } = fixture(t);
  const metadata = verifyTag('v0.1.0', root);
  for (const target of ['client-darwin-arm64', 'client-linux-amd64', 'client-win32-amd64', 'browser']) {
    const artifacts = packagesFor(metadata.version, target).map(name => createPackage(root, name));
    fs.writeFileSync(path.join(root, 'packages', `manifest-${target}.json`), JSON.stringify({ schema: 2, ...metadata, target, artifacts }));
  }
  return { root, git, directory: path.join(root, 'packages') };
}

test('publication requires exactly Mac arm64, Linux, Windows and both browser packages', archiveTest, t => {
  const { root, directory } = packageSet(t);
  const packages = collectPackages('v0.1.0', directory, root);
  assert.deepEqual(packages.map(({ name }) => name).sort(), [
    'Passpine-0.1.0-macos-arm64.zip',
    'Passpine-0.1.0-linux-amd64.tar.gz',
    'Passpine-0.1.0-windows-amd64-setup.exe',
    'passpine-0.1.0-chrome.zip',
    'passpine-0.1.0-firefox.zip',
  ].sort());
  for (const target of ['client-darwin-arm64', 'client-linux-amd64', 'client-win32-amd64', 'browser']) {
    const filename = path.join(directory, `manifest-${target}.json`);
    const manifest = fs.readFileSync(filename, 'utf8');
    fs.rmSync(filename);
    assert.throws(() => collectPackages('v0.1.0', directory, root), new RegExp(`incomplete.*${target}`));
    fs.writeFileSync(filename, manifest);
  }
  const filename = path.join(directory, 'manifest-browser.json');
  const manifest = JSON.parse(fs.readFileSync(filename, 'utf8'));
  manifest.artifacts.pop();
  fs.writeFileSync(filename, JSON.stringify(manifest));
  assert.throws(() => collectPackages('v0.1.0', directory, root), /Unexpected release package/);
});

test('CI release targets reject Intel Mac packages and provenance', archiveTest, t => {
  const { root, directory } = packageSet(t);
  assert.throws(() => packagesFor('0.1.0', 'client-darwin-x86_64'), /Unsupported release target/);
  const filename = path.join(directory, 'manifest-client-darwin-arm64.json');
  const manifest = JSON.parse(fs.readFileSync(filename, 'utf8'));
  const intelArtifact = createPackage(root, 'Passpine-0.1.0-macos-x86_64.zip');
  fs.writeFileSync(filename, JSON.stringify({ ...manifest, artifacts: [intelArtifact] }));
  assert.throws(() => collectPackages('v0.1.0', directory, root), /Unexpected release package/);
  fs.writeFileSync(filename, JSON.stringify(manifest));
  fs.writeFileSync(path.join(directory, 'manifest-client-darwin-x86_64.json'), JSON.stringify({
    ...manifest, target: 'client-darwin-x86_64', artifacts: [intelArtifact],
  }));
  assert.throws(() => collectPackages('v0.1.0', directory, root), /Invalid or duplicate/);
});

test('publication rejects changed packages, another commit and duplicate or unexpected manifest entries', archiveTest, t => {
  const { root, directory } = packageSet(t);
  const filename = path.join(directory, 'manifest-client-win32-amd64.json');
  const original = fs.readFileSync(filename, 'utf8');
  const manifest = JSON.parse(original);
  fs.writeFileSync(filename, JSON.stringify({ ...manifest, head: 'different-commit' }));
  assert.throws(() => collectPackages('v0.1.0', directory, root), /Invalid or duplicate/);
  fs.writeFileSync(filename, original);
  fs.writeFileSync(path.join(directory, 'manifest-duplicate.json'), original);
  assert.throws(() => collectPackages('v0.1.0', directory, root), /Invalid or duplicate/);
  fs.rmSync(path.join(directory, 'manifest-duplicate.json'));
  fs.writeFileSync(filename, JSON.stringify({ ...manifest, artifacts: [{ ...manifest.artifacts[0], name: '../unexpected.exe' }] }));
  assert.throws(() => collectPackages('v0.1.0', directory, root), /Unexpected release package/);
  fs.writeFileSync(filename, original);
  fs.appendFileSync(path.join(directory, manifest.artifacts[0].name), 'corruption');
  assert.throws(() => collectPackages('v0.1.0', directory, root), /checksum mismatch/);
});

test('browser staging validates archive versions before writing a release manifest', archiveTest, t => {
  const { root } = fixture(t);
  fs.mkdirSync(path.join(root, 'browser/.output'), { recursive: true });
  for (const name of packagesFor('0.1.0', 'browser')) {
    createPackage(root, name, '0.0.9');
    fs.copyFileSync(path.join(root, 'packages', name), path.join(root, 'browser/.output', name));
  }
  assert.throws(() => stage('v0.1.0', 'browser', root), /Archive version mismatch/);
  assert.equal(fs.existsSync(path.join(root, 'dist/releases/0.1.0/manifest-browser.json')), false);
});
