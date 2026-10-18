import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { artifactPlan } from './release-artifacts.mjs';

const scriptsDir = fileURLToPath(new URL('./', import.meta.url));
function command(cwd, executable, args, success = true, env = {}) {
  const result = spawnSync(executable, args, { cwd, encoding: 'utf8', env: { ...process.env, GIT_CONFIG_GLOBAL: '/dev/null', GIT_CONFIG_NOSYSTEM: '1', ...env } });
  if (result.error) throw result.error;
  if (success) assert.equal(result.status, 0, result.stderr + result.stdout);
  return result;
}
function fixture(t, checks = []) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-release-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.mkdirSync(path.join(root, 'scripts'));
  fs.mkdirSync(path.join(root, 'browser'));
  for (const name of ['release.mjs', 'version.mjs', 'release-artifacts.mjs']) fs.copyFileSync(path.join(scriptsDir, name), path.join(root, 'scripts', name));
  for (const name of ['package.json', 'browser/package.json']) fs.writeFileSync(path.join(root, name), '{"version":"0.1.0"}\n');
  for (const name of ['pnpm-lock.yaml', 'browser/pnpm-lock.yaml']) fs.writeFileSync(path.join(root, name), "lockfileVersion: '9.0'\n");
  fs.writeFileSync(path.join(root, '.release.json'), JSON.stringify({ version_file: 'package.json', branch: 'main', remote: 'origin', checks, github_release: false }));
  const git = (...args) => command(root, 'git', args).stdout.trim();
  git('init', '--initial-branch=main');
  git('config', 'user.name', 'Release Fixture'); git('config', 'user.email', 'fixture@example.test'); git('config', 'commit.gpgsign', 'false'); git('config', 'tag.gpgsign', 'false');
  git('add', '--', '.release.json', 'package.json', 'browser/package.json', 'pnpm-lock.yaml', 'browser/pnpm-lock.yaml', 'scripts');
  git('commit', '-m', 'test: create release fixture');
  const env = {};
  const release = (...args) => command(root, process.execPath, ['scripts/release.mjs', ...args], false, env);
  return { root, git, release, env };
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

test('first release tags the current version without rewriting files or adding a commit, and resumes from the original commit', t => {
  const { root, git, release } = fixture(t, [[process.execPath, '-e', "require('fs').appendFileSync('.git/check-runs', 'checked\\n')"]]);
  const original = git('rev-parse', 'HEAD');
  const before = fs.readFileSync(path.join(root, 'package.json'), 'utf8');
  const preview = release('0.1.0', '--no-push');
  assert.equal(preview.status, 0, preview.stderr);
  assert.deepEqual(JSON.parse(preview.stdout).files, []);
  assert.equal(fs.existsSync(path.join(root, '.git/check-runs')), false);
  const applied = release('0.1.0', '--no-push', '--apply');
  assert.equal(applied.status, 0, applied.stderr);
  assert.equal(git('rev-parse', 'HEAD'), original);
  assert.equal(git('rev-parse', 'v0.1.0^{commit}'), original);
  assert.equal(git('cat-file', '-t', 'refs/tags/v0.1.0'), 'tag');
  assert.equal(fs.readFileSync(path.join(root, 'package.json'), 'utf8'), before);
  assert.match(release('0.1.0', '--no-push').stderr, /Tag already exists/);
  assert.equal(release('0.1.0', '--no-push', '--resume', '--apply').status, 0);
  assert.equal(fs.readFileSync(path.join(root, '.git/check-runs'), 'utf8'), 'checked\n');
  assert.equal(git('status', '--porcelain'), '');
});

test('current-version release failures do not create a tag, and resume rejects a different HEAD or lightweight tag', t => {
  const { git, release } = fixture(t, [[process.execPath, '-e', 'process.exit(1)']]);
  assert.notEqual(release('0.1.0', '--apply', '--no-push').status, 0);
  assert.equal(git('tag'), '');
  assert.notEqual(release('0.0.9', '--no-push').status, 0);
  git('tag', 'v0.1.0');
  assert.match(release('0.1.0', '--resume', '--no-push').stderr, /annotated tag/);
  git('tag', '-a', 'v0.1.1', '-m', 'Release v0.1.1');
  git('commit', '--allow-empty', '-m', 'test: later commit');
  assert.match(release('0.1.0', '--resume', '--no-push').stderr, /must point to HEAD/);
});

test('first-release notes include the root commit and remain identical when resumed', t => {
  const { root, git, release } = fixture(t);
  const notes = () => command(root, process.execPath, ['--input-type=module', '-e', "import { releaseNotes } from './scripts/release.mjs'; console.log(releaseNotes('fixture/passpine', 'v', '0.1.0', 'HEAD', 'v0.1.0')); "]).stdout;
  const before = notes();
  assert.match(before, /Full Changelog.*commits\/v0\.1\.0/);
  assert.ok(before.includes(git('rev-parse', '--short', 'HEAD')));
  assert.match(before, /create release fixture/);
  assert.equal(release('0.1.0', '--apply', '--no-push').status, 0);
  assert.equal(notes(), before);
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

test('one-command release builds before tagging, resumes a partial draft upload and then builds the bumped version', { skip: process.platform !== 'darwin' }, t => {
  const { root, git, release, env } = fixture(t, [[process.execPath, '-e', "require('fs').appendFileSync('.git/check-runs', 'checked\\n')"]]);
  const bare = path.join(root, '.git/remote.git');
  command(root, 'git', ['init', '--bare', bare]);
  git('remote', 'add', 'origin', bare);
  const config = JSON.parse(fs.readFileSync(path.join(root, '.release.json'), 'utf8'));
  fs.writeFileSync(path.join(root, '.release.json'), JSON.stringify({ ...config, github_release: true, build_artifacts: true }));
  fs.writeFileSync(path.join(root, '.gitignore'), 'dist/\nbrowser/.output/\n');
  git('add', '.release.json', '.gitignore'); git('commit', '-m', 'test: configure full release');
  git('push', '-u', 'origin', 'main');
  const original = git('rev-parse', 'HEAD');
  const realGit = command(root, 'which', ['git']).stdout.trim();
  const bin = path.join(root, '.git/bin');
  fs.mkdirSync(bin);
  const executable = (name, source) => fs.writeFileSync(path.join(bin, name), `#!${process.execPath}\n${source}\n`, { mode: 0o755 });
  executable('git', `
    const { spawnSync } = require('node:child_process');
    const args = process.argv.slice(2);
    if (args.join(' ') === 'remote get-url --push --all origin') {
      console.log('https://github.com/fixture/passpine.git');
    } else {
      if (args[0] === 'ls-remote' && args[1] === 'https://github.com/fixture/passpine.git') args[1] = ${JSON.stringify(bare)};
      const result = spawnSync(${JSON.stringify(realGit)}, args, { stdio: 'inherit' });
      process.exit(result.status ?? 1);
    }
  `);
  const builder = `
    const fs = require('node:fs'), path = require('node:path'), { execFileSync } = require('node:child_process');
    const version = JSON.parse(fs.readFileSync('package.json')).version;
    const architecture = require('node:os').machine();
    const tag = execFileSync(${JSON.stringify(realGit)}, ['tag', '--list', 'v' + version], { encoding: 'utf8' }).trim();
    if (tag) throw new Error('Build must happen before the release tag is created');
    fs.appendFileSync('.git/build-runs', version + ':' + path.basename(process.argv[1]) + '\\n');
    if (fs.existsSync('.git/fail-build')) process.exit(1);
    const files = path.basename(process.argv[1]) === 'make'
      ? [{ output: 'dist/Passpine-' + version + '-macos-' + architecture + '.zip', entry: 'Passpine.app/Contents/Info.plist', content: '<plist><dict><key>CFBundleShortVersionString</key><string>' + version + '</string><key>CFBundleVersion</key><string>' + version + '</string></dict></plist>' }]
      : ['chrome', 'firefox'].map(browser => ({ output: 'browser/.output/passpine-' + version + '-' + browser + '.zip', entry: 'manifest.json', content: JSON.stringify({ version }) }));
    for (const file of files) {
      const input = path.resolve('.git/zip-input');
      fs.mkdirSync(path.dirname(path.join(input, file.entry)), { recursive: true });
      fs.writeFileSync(path.join(input, file.entry), file.content);
      fs.mkdirSync(path.dirname(file.output), { recursive: true });
      execFileSync('zip', ['-q', path.resolve(file.output), file.entry], { cwd: input });
    }
  `;
  executable('make', builder); executable('pnpm', builder);
  executable('gh', `
    const fs = require('node:fs'), path = require('node:path'), crypto = require('node:crypto');
    const args = process.argv.slice(2), file = '.git/github-state.json';
    const state = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file)) : {};
    fs.appendFileSync('.git/github-calls', JSON.stringify(args) + '\\n');
    let tag = args[2], release = state[tag];
    if (args[0] === 'api') { tag = Object.keys(state).find(key => args[1].includes('/' + state[key].databaseId + '/assets')); release = state[tag]; }
    if (args[1] === 'view') {
      if (!release) { console.error('release not found'); process.exit(1); }
      console.log(JSON.stringify(release));
    }
    if (args[1] === 'create') state[tag] = { databaseId: Object.keys(state).length + 1, tagName: tag, isDraft: true, url: 'https://github.com/fixture/passpine/releases/tag/' + tag, assets: [], body: fs.readFileSync(0, 'utf8') };
    if (args[0] === 'api') console.log(JSON.stringify([release.assets]));
    if (args[1] === 'upload') {
      for (const filename of args.slice(3, args.indexOf('--repo'))) {
        const bytes = fs.readFileSync(filename);
        release.assets.push({ name: path.basename(filename), size: bytes.length, digest: 'sha256:' + crypto.createHash('sha256').update(bytes).digest('hex'), state: 'uploaded' });
        fs.writeFileSync(file, JSON.stringify(state));
        if (fs.existsSync('.git/fail-upload')) { console.error('Simulated upload interruption'); process.exit(1); }
      }
    }
    if (args[1] === 'edit') release.isDraft = false;
    fs.writeFileSync(file, JSON.stringify(state));
  `);
  env.PATH = `${bin}${path.delimiter}${process.env.PATH}`;
  const preview = release('0.1.0');
  assert.equal(preview.status, 0, preview.stderr);
  const plan = JSON.parse(preview.stdout);
  assert.equal(plan.github_release, true);
  assert.equal(plan.artifacts.length, 3);
  assert.equal(fs.existsSync(path.join(root, '.git/build-runs')), false);
  assert.equal(fs.existsSync(path.join(root, '.git/github-calls')), false);
  fs.writeFileSync(path.join(root, '.git/fail-build'), 'interrupt');
  const failedBuild = release('0.1.0', '--apply');
  assert.notEqual(failedBuild.status, 0);
  assert.match(failedBuild.stderr, /Build failed/);
  assert.equal(git('tag'), '');
  assert.equal(git('rev-parse', 'HEAD'), original);
  assert.equal(command(root, 'git', ['--git-dir', bare, 'tag', '--list']).stdout.trim(), '');
  fs.unlinkSync(path.join(root, '.git/fail-build'));
  fs.writeFileSync(path.join(root, '.git/fail-upload'), 'interrupt');
  const partial = release('0.1.0', '--apply');
  assert.notEqual(partial.status, 0);
  assert.match(partial.stderr, /Simulated upload interruption/);
  assert.equal(git('rev-parse', 'HEAD'), original);
  assert.equal(git('cat-file', '-t', 'refs/tags/v0.1.0'), 'tag');
  assert.equal(command(root, 'git', ['--git-dir', bare, 'rev-parse', 'v0.1.0^{commit}']).stdout.trim(), original);
  const readState = () => JSON.parse(fs.readFileSync(path.join(root, '.git/github-state.json'), 'utf8'));
  assert.equal(readState()['v0.1.0'].isDraft, true);
  assert.equal(readState()['v0.1.0'].assets.length, 1);
  const builds = fs.readFileSync(path.join(root, '.git/build-runs'), 'utf8');
  fs.unlinkSync(path.join(root, '.git/fail-upload'));
  const resumed = release('0.1.0', '--resume', '--apply');
  assert.equal(resumed.status, 0, resumed.stderr);
  assert.equal(readState()['v0.1.0'].isDraft, false);
  assert.deepEqual(readState()['v0.1.0'].assets.map(item => item.name), artifactPlan('0.1.0').map(item => item.name));
  assert.equal(fs.readFileSync(path.join(root, '.git/build-runs'), 'utf8'), builds);
  assert.equal(fs.readFileSync(path.join(root, '.git/check-runs'), 'utf8'), 'checked\nchecked\n');
  assert.match(readState()['v0.1.0'].body, /Full Changelog/);
  fs.writeFileSync(path.join(root, '.git/fail-build'), 'interrupt');
  const failedBump = release('patch', '--apply');
  assert.notEqual(failedBump.status, 0);
  assert.equal(JSON.parse(fs.readFileSync(path.join(root, 'package.json'))).version, '0.1.1');
  assert.equal(git('rev-parse', 'HEAD'), original);
  assert.equal(git('tag', '--list', 'v0.1.1'), '');
  assert.equal(readState()['v0.1.1'], undefined);
  fs.unlinkSync(path.join(root, '.git/fail-build'));
  git('restore', '--', 'package.json', 'browser/package.json');
  const bumped = release('patch', '--apply');
  assert.equal(bumped.status, 0, bumped.stderr);
  assert.equal(git('log', '-1', '--format=%s'), 'chore(release): v0.1.1');
  assert.equal(readState()['v0.1.1'].isDraft, false);
  assert.ok(readState()['v0.1.1'].assets.every(item => item.name.includes('0.1.1')));
  assert.equal(git('status', '--porcelain'), '');
});
