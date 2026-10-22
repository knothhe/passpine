import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { assertReleaseBuild, buildChannel, clientBuildConfig } from './build-channel.mjs';
import { readUpdateSigningConfig } from './update-artifacts.mjs';

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-build-channel-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.mkdirSync(path.join(root, 'browser'));
  fs.mkdirSync(path.join(root, 'config'));
  for (const file of ['package.json', 'browser/package.json']) fs.writeFileSync(path.join(root, file), JSON.stringify({ version: '1.2.3' }));
  fs.writeFileSync(path.join(root, '.release.json'), JSON.stringify({ tag_prefix: 'v' }));
  fs.writeFileSync(path.join(root, 'config/update-signing.json'), JSON.stringify(readUpdateSigningConfig()));
  const git = (...args) => execFileSync('git', ['-c', 'commit.gpgsign=false', '-c', 'tag.gpgsign=false', '-c', 'core.hooksPath=/dev/null', ...args], { cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  git('init'); git('add', '.'); git('-c', 'user.name=Build Test', '-c', 'user.email=build-test@example.invalid', 'commit', '-m', 'Fixture'); git('tag', 'v1.2.3');
  const env = { PASSPINE_BUILD_CHANNEL: 'release', GITHUB_ACTIONS: 'true', GITHUB_REPOSITORY: 'knothhe/passpine', RELEASE_TAG: 'v1.2.3' };
  return { root, env, git };
}

test('source builds default to a distinct development identity and never embed production updater settings', t => {
  const { root, git } = fixture(t);
  const config = clientBuildConfig({ PASSPINE_UPDATE_PUBLIC_KEY: 'invalid', PASSPINE_UPDATE_FEED_URL: 'invalid', PASSPINE_UPDATE_PRIVATE_KEY: 'private material' }, root);
  assert.equal(config.channel, 'dev');
  assert.equal(config.version, `1.2.3-dev.${git('rev-parse', '--short=12', 'HEAD')}`);
  assert.equal(config.outputDirectory, path.join(root, 'dist/dev'));
  assert.equal(config.appName, 'Passpine Dev');
  assert.equal(config.bundleID, 'com.knothhe.passpine.dev');
  assert.equal(config.linuxBinary, 'passpine-dev');
  assert.equal(config.linuxHost, 'passpine-dev-host');
  assert.match(config.ldflags, /buildinfo\.Channel=dev/);
  assert.doesNotMatch(JSON.stringify(config), /updateFeedURL|updatePublicKey|private material/);
  assert.throws(() => buildChannel({ PASSPINE_BUILD_CHANNEL: 'production' }), /dev or release/);
});

test('formal builds require the GitHub repository, existing matching tag, clean source and pinned update trust', t => {
  const { root, env } = fixture(t);
  const config = clientBuildConfig(env, root), pinned = readUpdateSigningConfig();
  assert.equal(config.channel, 'release'); assert.equal(config.version, '1.2.3');
  assert.equal(config.outputDirectory, path.join(root, 'dist')); assert.equal(config.appName, 'Passpine');
  assert.ok(config.ldflags.includes(`main.updatePublicKey=${pinned.publicKey}`));
  assert.ok(config.ldflags.includes(`main.updateFeedURL=${pinned.feedURL}`));
  assert.match(config.hostLdflags, /buildinfo\.Channel=release/);
  for (const overrides of [{ GITHUB_ACTIONS: '' }, { GITHUB_ACTIONS: 'false' }, { GITHUB_REPOSITORY: 'fork/passpine' }, { PASSPINE_BUILD_CHANNEL: 'dev' }, { RELEASE_TAG: '' }]) {
    assert.throws(() => assertReleaseBuild({ ...env, ...overrides }, root));
  }
  assert.throws(() => clientBuildConfig({ ...env, PASSPINE_UPDATE_PUBLIC_KEY: Buffer.alloc(32).toString('base64') }, root), /repository-pinned/);
  assert.throws(() => clientBuildConfig({ ...env, PASSPINE_UPDATE_FEED_URL: 'https://example.invalid/feed.json' }, root), /repository-pinned/);
  fs.writeFileSync(path.join(root, 'package.json'), JSON.stringify({ version: '1.2.4' }));
  fs.writeFileSync(path.join(root, 'browser/package.json'), JSON.stringify({ version: '1.2.4' }));
  assert.throws(() => assertReleaseBuild(env, root), /package versions/);
});

test('moving off the release tag or editing tracked files refuses a formal build', t => {
  const { root, env, git } = fixture(t);
  fs.writeFileSync(path.join(root, '.release.json'), '{"tag_prefix":"v"}\n');
  assert.throws(() => assertReleaseBuild(env, root), /unchanged tracked/);
  git('add', '.'); git('-c', 'user.name=Build Test', '-c', 'user.email=build-test@example.invalid', 'commit', '-m', 'Other commit');
  assert.throws(() => assertReleaseBuild(env, root), /tag commit/);
});

test('local installer rejects a formal channel before changing installation directories', { skip: process.platform === 'win32' }, () => {
  const script = fileURLToPath(new URL('./install-client.sh', import.meta.url));
  const result = spawnSync('bash', [script, '/this-path-must-never-be-created'], {
    encoding: 'utf8', env: { ...process.env, PASSPINE_BUILD_CHANNEL: 'release', GITHUB_ACTIONS: '', GITHUB_REPOSITORY: '' },
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /Release builds are available only/);
});
