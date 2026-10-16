import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

const scriptsDir = fileURLToPath(new URL('./', import.meta.url));
function command(cwd, executable, args, success = true) {
  const result = spawnSync(executable, args, { cwd, encoding: 'utf8', env: { ...process.env, GIT_CONFIG_GLOBAL: '/dev/null', GIT_CONFIG_NOSYSTEM: '1' } });
  if (result.error) throw result.error;
  if (success) assert.equal(result.status, 0, result.stderr + result.stdout);
  return result;
}
function fixture(t, checks = []) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-release-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.mkdirSync(path.join(root, 'scripts'));
  fs.mkdirSync(path.join(root, 'browser'));
  for (const name of ['release.mjs', 'version.mjs']) fs.copyFileSync(path.join(scriptsDir, name), path.join(root, 'scripts', name));
  for (const name of ['package.json', 'browser/package.json']) fs.writeFileSync(path.join(root, name), '{"version":"0.1.0"}\n');
  for (const name of ['pnpm-lock.yaml', 'browser/pnpm-lock.yaml']) fs.writeFileSync(path.join(root, name), "lockfileVersion: '9.0'\n");
  fs.writeFileSync(path.join(root, '.release.json'), JSON.stringify({ version_file: 'package.json', branch: 'main', remote: 'origin', checks, github_release: false }));
  const git = (...args) => command(root, 'git', args).stdout.trim();
  git('init', '--initial-branch=main');
  git('config', 'user.name', 'Release Fixture'); git('config', 'user.email', 'fixture@example.test'); git('config', 'commit.gpgsign', 'false'); git('config', 'tag.gpgsign', 'false');
  git('add', '--', '.release.json', 'package.json', 'browser/package.json', 'pnpm-lock.yaml', 'browser/pnpm-lock.yaml', 'scripts/release.mjs', 'scripts/version.mjs');
  git('commit', '-m', 'test: create release fixture');
  const release = (...args) => command(root, process.execPath, ['scripts/release.mjs', ...args], false);
  return { root, git, release };
}

test('preview is read-only, apply synchronizes both packages and creates an annotated tag, resume preserves it', t => {
  const { root, git, release } = fixture(t, [[process.execPath, 'scripts/version.mjs', 'check']]);
  const original = git('rev-parse', 'HEAD');
  const preview = release('minor', '--no-push');
  assert.equal(preview.status, 0, preview.stderr);
  const plan = JSON.parse(preview.stdout);
  assert.equal(plan.mode, 'preview'); assert.equal(plan.target, '0.2.0');
  assert.deepEqual(plan.files, ['package.json', 'browser/package.json']);
  assert.equal(git('rev-parse', 'HEAD'), original); assert.equal(git('status', '--porcelain'), ''); assert.equal(git('tag'), '');
  const applied = release('minor', '--apply', '--no-push');
  assert.equal(applied.status, 0, applied.stderr);
  for (const name of ['package.json', 'browser/package.json']) assert.equal(JSON.parse(fs.readFileSync(path.join(root, name), 'utf8')).version, '0.2.0');
  for (const name of ['pnpm-lock.yaml', 'browser/pnpm-lock.yaml']) assert.equal(fs.readFileSync(path.join(root, name), 'utf8'), "lockfileVersion: '9.0'\n");
  assert.equal(git('log', '-1', '--format=%s'), 'chore(release): v0.2.0');
  assert.equal(git('cat-file', '-t', 'refs/tags/v0.2.0'), 'tag'); assert.equal(git('status', '--porcelain'), '');
  const released = git('rev-parse', 'HEAD');
  assert.equal(release('0.2.0', '--resume', '--apply', '--no-push').status, 0);
  assert.equal(git('rev-parse', 'HEAD'), released);
});

test('failed checks preserve both source versions and create no release commit or tag', t => {
  const { root, git, release } = fixture(t, [[process.execPath, '-e', 'process.exit(1)']]);
  const original = git('rev-parse', 'HEAD');
  const failed = release('patch', '--apply', '--no-push');
  assert.notEqual(failed.status, 0); assert.match(failed.stderr, /Stopped before updating/);
  for (const name of ['package.json', 'browser/package.json']) assert.equal(JSON.parse(fs.readFileSync(path.join(root, name), 'utf8')).version, '0.1.0');
  assert.equal(git('rev-parse', 'HEAD'), original); assert.equal(git('tag'), ''); assert.equal(git('status', '--porcelain'), '');
});

test('release rejects side effects from checks, dirty trees, and prerelease manifest versions', t => {
  const { root, git, release } = fixture(t, [[process.execPath, '-e', "require('fs').writeFileSync('unexpected.txt', 'side effect')"]]);
  assert.notEqual(release('1.0.0-rc.1', '--no-push').status, 0);
  assert.equal(git('status', '--porcelain'), '');
  assert.match(release('patch', '--apply', '--no-push').stderr, /Checks changed the working tree/);
  assert.equal(git('tag'), ''); assert.ok(fs.existsSync(path.join(root, 'unexpected.txt')));
  assert.match(release('patch', '--no-push').stderr, /Working tree must be clean/);
});

test('atomic publishing uses only the configured branch and release tag in a local remote', t => {
  const { root, git, release } = fixture(t);
  const bare = path.join(os.tmpdir(), `passpine-release-remote-${path.basename(root)}.git`);
  t.after(() => fs.rmSync(bare, { recursive: true, force: true }));
  command(root, 'git', ['init', '--bare', bare]);
  git('remote', 'add', 'origin', bare); git('push', '-u', 'origin', 'main');
  const applied = release('patch', '--apply');
  assert.equal(applied.status, 0, applied.stderr);
  const remoteRefs = command(root, 'git', ['ls-remote', bare]).stdout;
  assert.match(remoteRefs, /refs\/heads\/main/); assert.match(remoteRefs, /refs\/tags\/v0\.1\.1\^\{\}/);
  assert.equal(git('rev-parse', 'HEAD'), command(root, 'git', ['--git-dir', bare, 'rev-parse', 'refs/heads/main']).stdout.trim());
});
