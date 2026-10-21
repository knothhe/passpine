import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { test } from 'node:test';
import { artifactPlan, buildArtifacts, saveArtifacts, cachedArtifacts } from './release-artifacts.mjs';
import { publishRelease } from './release.mjs';

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-artifacts-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const plan = artifactPlan('0.1.0', 'darwin', 'arm64');
  const archive = (artifact, version = '0.1.0') => {
    const contents = artifact.entry.endsWith('.json') ? JSON.stringify({ version })
      : `<plist><dict><key>CFBundleShortVersionString</key><string>${version}</string><key>CFBundleVersion</key><string>${version}</string></dict></plist>`;
    const input = path.join(root, 'input');
    fs.mkdirSync(path.dirname(path.join(input, artifact.entry)), { recursive: true });
    fs.writeFileSync(path.join(input, artifact.entry), contents);
    const output = path.join(root, artifact.source);
    fs.mkdirSync(path.dirname(output), { recursive: true });
    execFileSync('zip', ['-q', output, artifact.entry], { cwd: input });
  };
  return { root, plan, archive };
}

test('artifact names specify the version and Mac architecture, with one Chromium package and no sources ZIP', () => {
  assert.deepEqual(artifactPlan('0.1.0', 'darwin', 'arm64').map(item => item.name), [
    'Passpine-0.1.0-macos-arm64.zip', 'passpine-0.1.0-chrome.zip', 'passpine-0.1.0-firefox.zip',
  ]);
  assert.match(artifactPlan('0.1.0', 'darwin', 'x86_64')[0].name, /x86_64/);
  assert.throws(() => artifactPlan('0.1.0', 'linux', 'x86_64'), /require macOS/);
});

test('packaging rejects stale archives and mismatched versions before staging any assets', { skip: process.platform === 'win32' && 'macOS release archive validation uses POSIX zip/unzip' }, t => {
  const { root, plan, archive } = fixture(t);
  plan.forEach(item => archive(item));
  assert.throws(() => buildArtifacts('0.1.0', plan, () => {}, root), /ENOENT/);
  assert.throws(() => buildArtifacts('0.1.0', plan, command => {
    if (command[0] === 'pnpm') plan.forEach(item => archive(item, '0.0.9'));
  }, root), /Archive version differs/);
  assert.equal(fs.existsSync(path.join(root, 'dist/releases/0.1.0')), false);
});

test('resume caches bind all artifact checksums to the tag commit and reject modified files', { skip: process.platform === 'win32' && 'macOS release archive validation uses POSIX zip/unzip' }, t => {
  const { root, plan, archive } = fixture(t);
  const commands = [];
  const artifacts = buildArtifacts('0.1.0', plan, command => {
    commands.push(command);
    if (command[0] === 'pnpm') plan.forEach(item => archive(item));
  }, root);
  assert.equal(commands.length, 2);
  assert.equal(cachedArtifacts('0.1.0', 'head', plan, root), null);
  saveArtifacts('0.1.0', 'head', artifacts, root);
  assert.deepEqual(cachedArtifacts('0.1.0', 'head', plan, root), artifacts);
  assert.throws(() => cachedArtifacts('0.1.0', 'different-head', plan, root), /does not match/);
  fs.appendFileSync(artifacts[0].path, 'corruption');
  assert.throws(() => cachedArtifacts('0.1.0', 'head', plan, root), /checksum mismatch/);
});

function github(artifacts, { published = false, present = false } = {}) {
  const calls = [];
  const state = { exists: present, draft: !published, assets: [], failUpload: false, corruptUpload: false };
  const invoke = (args, check, inherit, input) => {
    calls.push({ args, input });
    const action = args[2];
    if (action === 'view') return state.exists
      ? { status: 0, stdout: JSON.stringify({ databaseId: 1, tagName: 'v0.1.0', url: `https://github.com/fixture/passpine/releases/tag/${state.draft ? 'untagged-draft' : 'v0.1.0'}`, isDraft: state.draft }) }
      : { status: 1, stderr: 'release not found' };
    if (action === 'create') { state.exists = true; assert.ok(args.includes('--draft')); }
    if (args[1] === 'api') return { status: 0, stdout: JSON.stringify([state.assets.slice(0, 1), state.assets.slice(1)]) };
    if (action === 'upload') {
      for (const filename of args.slice(4, args.indexOf('--repo'))) {
        const artifact = artifacts.find(item => item.path === filename);
        assert.ok(artifact, `Unexpected asset: ${filename}`);
        state.assets.push({ name: artifact.name, size: artifact.size, digest: `sha256:${state.corruptUpload ? 'bad' : artifact.sha256}`, state: 'uploaded' });
        if (state.failUpload) throw new Error('Upload interrupted');
      }
    }
    if (action === 'edit') { assert.ok(args.includes('--draft=false')); state.draft = false; }
    return { status: 0, stdout: '' };
  };
  return { state, calls, invoke };
}
const packages = ['macos-arm64', 'chrome', 'firefox'].map((target, i) => ({ name: `passpine-0.1.0-${target}.zip`, path: `/packages/${target}.zip`, size: i + 1, sha256: String(i).repeat(64) }));
const options = { repo: 'fixture/passpine', tag: 'v0.1.0', notes: '**Full Changelog**: link\n\n- First release', artifacts: packages };

test('Release publication creates a draft, uploads only named packages, verifies hashes and publishes last', () => {
  const { state, calls, invoke } = github(packages);
  assert.match(publishRelease(options, invoke), /releases\/tag\/v0\.1\.0/);
  assert.equal(state.draft, false);
  assert.equal(state.assets.length, 3);
  assert.equal(calls.filter(call => ['create', 'upload', 'edit'].includes(call.args[2])).at(-1).args[2], 'edit');
  assert.equal(calls.find(call => call.args[2] === 'create').input, options.notes);
  assert.equal(calls.some(call => call.args.includes('--clobber')), false);
});

test('interrupted uploads remain drafts and resume sends only missing packages without recreating the Release', () => {
  const { state, calls, invoke } = github(packages);
  state.failUpload = true;
  assert.throws(() => publishRelease(options, invoke), /Upload interrupted/);
  assert.equal(state.draft, true);
  assert.equal(state.assets.length, 1);
  state.failUpload = false;
  publishRelease(options, invoke);
  assert.equal(state.draft, false);
  assert.equal(calls.filter(call => call.args[2] === 'create').length, 1);
  const upload = calls.filter(call => call.args[2] === 'upload').at(-1).args;
  assert.equal(upload.includes(packages[0].path), false);
  const count = calls.length;
  publishRelease(options, invoke);
  assert.equal(calls.slice(count).some(call => ['create', 'upload', 'edit'].includes(call.args[2])), false);
});

test('conflicting or corrupted assets and GitHub read failures cannot publish or overwrite a release', () => {
  const { state, calls, invoke } = github(packages);
  state.corruptUpload = true;
  assert.throws(() => publishRelease(options, invoke), /Refusing to overwrite/);
  assert.equal(state.draft, true);
  assert.equal(calls.some(call => call.args[2] === 'edit'), false);
  const networkCalls = [];
  assert.throws(() => publishRelease(options, args => { networkCalls.push(args); return { status: 1, stderr: 'HTTP 503 unavailable' }; }), /Cannot inspect/);
  assert.equal(networkCalls.length, 1);
  const existing = github(packages, { present: true, published: true });
  assert.throws(() => publishRelease(options, existing.invoke), /Published release is missing/);
  assert.equal(existing.calls.some(call => ['create', 'upload', 'edit'].includes(call.args[2])), false);
});
