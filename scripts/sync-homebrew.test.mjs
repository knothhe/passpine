import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { homebrewCask, homebrewRelease, publishHomebrew, syncHomebrew } from './sync-homebrew.mjs';

const bytes = Buffer.from('macOS release fixture');
const digest = createHash('sha256').update(bytes).digest('hex');
function release(version = '0.1.7') {
  return {
    tag_name: `v${version}`, draft: false, prerelease: false,
    assets: [{
      name: `Passpine-${version}-macos-arm64.zip`, state: 'uploaded', size: bytes.length,
      digest: `sha256:${digest}`,
      browser_download_url: `https://github.com/knothhe/passpine/releases/download/v${version}/Passpine-${version}-macos-arm64.zip`,
    }],
  };
}

test('synchronization and publication require explicit targets instead of defaulting to the source repository', async () => {
  await assert.rejects(syncHomebrew(), /explicit cask destination/);
  await assert.rejects(publishHomebrew({ tag: 'v0.1.7' }), /explicit tap checkout root/);
  for (const args of [[], ['--publish', 'v0.1.7']]) {
    assert.throws(() => execFileSync(process.execPath, [path.resolve('scripts/sync-homebrew.mjs'), ...args], {
      encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
    }), error => error.status === 1 && /Usage: .*--tap-root CHECKOUT/.test(error.stderr));
  }
});

test('casks accept only stable published releases with the exact arm64 asset and digest', () => {
  assert.equal(homebrewRelease(release()).sha256, digest);
  for (const change of [
    data => { data.draft = true; },
    data => { data.prerelease = true; },
    data => { data.tag_name = 'v0.1.8-rc.1'; },
    data => { data.assets = []; },
    data => { data.assets.push(data.assets[0]); },
    data => { data.assets[0].name = 'Passpine-0.1.7-macos-x86_64.zip'; },
    data => { data.assets[0].state = 'new'; },
    data => { data.assets[0].digest = undefined; },
    data => { data.assets[0].browser_download_url = 'https://example.test/package.zip'; },
  ]) {
    const data = release();
    change(data);
    assert.throws(() => homebrewRelease(data));
  }
});

test('sync verifies downloaded bytes before replacing an existing cask', async t => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'passpine-homebrew-'));
  t.after(() => fs.rm(directory, { recursive: true, force: true }));
  const destination = path.join(directory, 'passpine.rb');
  const previous = homebrewCask({ version: '0.1.6', sha256: digest });
  await fs.writeFile(destination, previous);
  for (const fetchImpl of [
    async () => new Response('unavailable', { status: 503 }),
    async () => new Response(Buffer.alloc(bytes.length)),
    async () => new Response(bytes.subarray(1)),
  ]) {
    await assert.rejects(syncHomebrew({ destination, loadRelease: () => release(), fetchImpl }));
    assert.equal(await fs.readFile(destination, 'utf8'), previous);
  }
  await assert.rejects(syncHomebrew({ destination, loadRelease: () => { throw new Error('lookup failed'); } }));
  assert.equal(await fs.readFile(destination, 'utf8'), previous);
  const version = await syncHomebrew({ destination, loadRelease: () => release(), fetchImpl: async url => {
    assert.equal(url, release().assets[0].browser_download_url);
    return new Response(bytes);
  } });
  assert.equal(version, '0.1.7');
  const cask = await fs.readFile(destination, 'utf8');
  assert.match(cask, /version "0\.1\.7"/);
  assert.ok(cask.includes(`sha256 "${digest}"`));
  assert.match(cask, /depends_on arch: :arm64/);
  assert.match(cask, /app "Passpine\.app"/);
  assert.deepEqual(await fs.readdir(directory), ['passpine.rb']);
});

async function gitFixture(t, version = '0.1.6') {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'passpine-homebrew-git-'));
  t.after(() => fs.rm(directory, { recursive: true, force: true }));
  const gitConfig = path.join(directory, 'gitconfig');
  await fs.writeFile(gitConfig, '');
  const gitEnv = { ...process.env, GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: gitConfig };
  const git = (cwd, ...args) => execFileSync('git', args, { cwd, env: gitEnv, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  const remote = path.join(directory, 'remote.git');
  const root = path.join(directory, 'tap checkout');
  git(directory, 'init', '--bare', '--initial-branch=main', remote);
  git(directory, 'clone', remote, root);
  git(root, 'config', 'user.name', 'Homebrew Fixture');
  git(root, 'config', 'user.email', 'homebrew@example.test');
  if (version !== null) {
    await fs.mkdir(path.join(root, 'Casks'));
    await fs.writeFile(path.join(root, 'Casks/passpine.rb'), homebrewCask({ version, sha256: digest }));
  }
  await fs.mkdir(path.join(root, 'Formula'));
  await fs.writeFile(path.join(root, 'Formula/scrollsplit.rb'), '# Existing formula\n');
  await fs.writeFile(path.join(root, 'README.md'), 'unrelated content\n');
  git(root, 'add', '--', ...(version !== null ? ['Casks/passpine.rb'] : []), 'Formula/scrollsplit.rb', 'README.md');
  git(root, 'commit', '-m', 'Initial tap');
  git(root, 'push', 'origin', 'main');
  const advanceMain = async () => {
    const other = path.join(directory, 'other-checkout');
    git(directory, 'clone', remote, other);
    git(other, 'config', 'user.name', 'Other Maintainer');
    git(other, 'config', 'user.email', 'other@example.test');
    await fs.writeFile(path.join(other, 'README.md'), 'concurrent main change\n');
    git(other, 'add', '--', 'README.md');
    git(other, 'commit', '-m', 'Update unrelated documentation');
    git(other, 'push', 'origin', 'main');
  };
  return {
    root, remote, git, gitEnv, advanceMain,
    options: { root, gitEnv, tag: 'v0.1.7', loadRelease: () => release(), fetchImpl: async () => new Response(bytes) },
  };
}

test('published-tag sync pushes exactly one cask commit and reruns without another commit', async t => {
  const { options, root, remote, git } = await gitFixture(t);
  let requestedTag;
  const result = await publishHomebrew({ ...options, loadRelease: tag => { requestedTag = tag; return release(); } });
  assert.equal(requestedTag, 'v0.1.7');
  assert.equal(result.changed, true);
  assert.equal(git(remote, 'rev-parse', 'main'), result.commit);
  assert.equal(git(root, 'show', '--format=', '--name-only', 'HEAD'), 'Casks/passpine.rb');
  assert.equal(git(root, 'log', '-1', '--format=%an <%ae>'), 'github-actions[bot] <41898282+github-actions[bot]@users.noreply.github.com>');
  assert.equal(git(remote, 'show', 'main:Casks/passpine.rb'), homebrewCask({ version: '0.1.7', sha256: digest }).trim());
  assert.equal(git(remote, 'show', 'main:README.md'), 'unrelated content');
  assert.equal(git(root, 'status', '--porcelain'), '');
  assert.deepEqual(await publishHomebrew(options), { version: '0.1.7', changed: false });
  assert.equal(git(remote, 'rev-parse', 'main'), result.commit);
});

test('retrying an older release does not downgrade a newer cask', async t => {
  const { options, remote, git } = await gitFixture(t, '0.1.10');
  const head = git(remote, 'rev-parse', 'main');
  assert.deepEqual(await publishHomebrew({ ...options, fetchImpl: () => { throw new Error('old package must not be downloaded'); } }),
    { version: '0.1.10', changed: false });
  assert.equal(git(remote, 'rev-parse', 'main'), head);
});

test('publication adds a missing cask to a separate tap without touching its formulae or source checkout', async t => {
  const source = await gitFixture(t, '0.1.6');
  const tap = await gitFixture(t, null);
  const sourceHead = source.git(source.root, 'rev-parse', 'HEAD');
  const result = await publishHomebrew(tap.options);
  assert.equal(tap.git(tap.remote, 'rev-parse', 'main'), result.commit);
  assert.equal(tap.git(tap.remote, 'show', '--format=', '--name-only', 'main'), 'Casks/passpine.rb');
  assert.equal(tap.git(tap.remote, 'show', 'main:Formula/scrollsplit.rb'), '# Existing formula');
  assert.equal(source.git(source.root, 'rev-parse', 'HEAD'), sourceHead);
  assert.equal(source.git(source.root, 'status', '--porcelain'), '');
  assert.deepEqual(await publishHomebrew(tap.options), { version: '0.1.7', changed: false });
});

test('a nested directory cannot publish into its parent repository', async t => {
  const { options, root, remote, git } = await gitFixture(t);
  const head = git(remote, 'rev-parse', 'main');
  await assert.rejects(publishHomebrew({ ...options, root: path.join(root, 'Formula') }), /tap checkout root/);
  assert.equal(git(remote, 'rev-parse', 'main'), head);
  assert.equal(git(root, 'status', '--porcelain'), '');
});

test('a missing, draft, mismatched, or corrupted published package leaves main unchanged', async t => {
  const { options, root, remote, git } = await gitFixture(t);
  const head = git(remote, 'rev-parse', 'main');
  for (const override of [
    { loadRelease: () => { throw new Error('release is not public'); } },
    { loadRelease: () => ({ ...release(), draft: true }) },
    { loadRelease: () => release('0.1.8') },
    { fetchImpl: async () => new Response(Buffer.alloc(bytes.length)) },
  ]) {
    await assert.rejects(publishHomebrew({ ...options, ...override }));
    assert.equal(git(remote, 'rev-parse', 'main'), head);
    assert.equal(git(root, 'status', '--porcelain'), '');
  }
});

test('publication catches up with main before committing and preserves its other changes', async t => {
  const { options, remote, git, advanceMain } = await gitFixture(t);
  await advanceMain();
  const result = await publishHomebrew(options);
  assert.equal(git(remote, 'rev-parse', 'main'), result.commit);
  assert.equal(git(remote, 'show', 'main:README.md'), 'concurrent main change');
  assert.equal(git(remote, 'show', '--format=', '--name-only', 'main'), 'Casks/passpine.rb');
});

test('a concurrent push is rejected without forcing over main', async t => {
  const { options, remote, git, advanceMain } = await gitFixture(t);
  await assert.rejects(publishHomebrew({ ...options, fetchImpl: async () => {
    await advanceMain();
    return new Response(bytes);
  } }));
  assert.equal(git(remote, 'show', 'main:README.md'), 'concurrent main change');
  assert.equal(git(remote, 'show', 'main:Casks/passpine.rb'), homebrewCask({ version: '0.1.6', sha256: digest }).trim());
});

test('dirty checkouts and unpushed unrelated commits cannot be published', async t => {
  const { options, root, remote, git } = await gitFixture(t);
  const head = git(remote, 'rev-parse', 'main');
  await fs.writeFile(path.join(root, 'README.md'), 'local work\n');
  git(root, 'add', '--', 'README.md');
  await assert.rejects(publishHomebrew(options), /clean main checkout/);
  assert.equal(await fs.readFile(path.join(root, 'README.md'), 'utf8'), 'local work\n');
  git(root, 'commit', '-m', 'Local unrelated work');
  await assert.rejects(publishHomebrew(options), /unrelated local commits/);
  assert.equal(git(remote, 'rev-parse', 'main'), head);
});

test('a changed checksum for the same release version is rejected', async t => {
  const { options, root, remote, git } = await gitFixture(t, '0.1.7');
  const head = git(remote, 'rev-parse', 'main');
  await assert.rejects(publishHomebrew({ ...options, loadRelease: () => {
    const data = release();
    data.assets[0].digest = `sha256:${'a'.repeat(64)}`;
    return data;
  } }), /checksum changed/);
  assert.equal(git(remote, 'rev-parse', 'main'), head);
  assert.equal(git(root, 'status', '--porcelain'), '');
});
