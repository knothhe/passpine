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
  const commandEnv = { ...process.env, GIT_CONFIG_GLOBAL: '/dev/null', GIT_CONFIG_NOSYSTEM: '1' };
  // Fixture Make invocations must use their own defaults when this suite runs
  // inside `make release`, without inheriting its flags or release arguments.
  for (const name of ['MAKEFLAGS', 'MFLAGS', 'MAKEOVERRIDES', 'MAKELEVEL', 'VERSION_BUMP', 'RELEASE_FLAGS']) delete commandEnv[name];
  Object.assign(commandEnv, env);
  const result = spawnSync(executable, args, { cwd, encoding: 'utf8', env: commandEnv });
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
  assert.equal(plan.release_type, 'upgrade'); assert.equal(plan.create_version_commit, true);
  assert.deepEqual(plan.files, ['package.json', 'browser/package.json']);
  assert.equal(git('rev-parse', 'HEAD'), original); assert.equal(git('status', '--porcelain'), ''); assert.equal(git('tag'), '');
  const applied = release('minor', '--apply', '--no-push');
  assert.equal(applied.status, 0, applied.stderr);
  for (const name of ['package.json', 'browser/package.json']) assert.equal(JSON.parse(fs.readFileSync(path.join(root, name), 'utf8')).version, '0.2.0');
  for (const name of ['pnpm-lock.yaml', 'browser/pnpm-lock.yaml']) assert.equal(fs.readFileSync(path.join(root, name), 'utf8'), "lockfileVersion: '9.0'\n");
  assert.equal(git('log', '-1', '--format=%s'), 'chore(release): v0.2.0');
  assert.equal(git('cat-file', '-t', 'refs/tags/v0.2.0'), 'tag'); assert.equal(git('status', '--porcelain'), '');
  const released = git('rev-parse', 'HEAD');
  const resumed = JSON.parse(release('0.2.0', '--resume', '--no-push').stdout);
  assert.equal(resumed.release_type, 'upgrade'); assert.equal(resumed.create_version_commit, false);
  assert.equal(release('0.2.0', '--resume', '--apply', '--no-push').status, 0);
  assert.equal(git('rev-parse', 'HEAD'), released);
});

test('CI release mode leaves packages and publication to Actions and rejects competing local publication', t => {
  const { root, git, release } = fixture(t);
  const config = JSON.parse(fs.readFileSync(path.join(root, '.release.json'), 'utf8'));
  fs.writeFileSync(path.join(root, '.release.json'), JSON.stringify({ ...config, ci_release: true, build_artifacts: false }));
  git('add', '.release.json'); git('commit', '-m', 'test: enable CI release');
  const preview = release('patch', '--no-push');
  assert.equal(preview.status, 0, preview.stderr);
  assert.deepEqual(JSON.parse(preview.stdout).builds, []);
  assert.equal(JSON.parse(preview.stdout).github_release, false);
  assert.match(release('patch', '--github-release').stderr, /CI owns release packages/);
  assert.equal(release('patch', '--apply', '--no-push').status, 0);
  assert.equal(git('rev-parse', 'v0.1.1^{commit}'), git('rev-parse', 'HEAD'));
});

test('first release tags the current version without rewriting files or adding a commit, and resumes from the original commit', t => {
  const { root, git, release } = fixture(t, [[process.execPath, '-e', "require('fs').appendFileSync('.git/check-runs', 'checked\\n')"]]);
  const sources = ['package.json', 'browser/package.json', 'pnpm-lock.yaml', 'browser/pnpm-lock.yaml'];
  const before = sources.map(name => fs.readFileSync(path.join(root, name), 'utf8'));
  const config = JSON.parse(fs.readFileSync(path.join(root, '.release.json'), 'utf8'));
  fs.writeFileSync(path.join(root, '.release.json'), JSON.stringify({ ...config, check_phase: 'after-bump' }));
  git('add', '.release.json'); git('commit', '-m', 'test: configure checks after bump');
  const releaseHead = git('rev-parse', 'HEAD');
  const preview = release('initial', '--no-push');
  assert.equal(preview.status, 0, preview.stderr);
  const plan = JSON.parse(preview.stdout);
  assert.deepEqual(plan.files, []);
  assert.equal(plan.release_type, 'initial'); assert.equal(plan.create_version_commit, false);
  assert.equal(plan.current, '0.1.0'); assert.equal(plan.target, '0.1.0');
  assert.equal(plan.check_phase, 'current-version');
  assert.equal(fs.existsSync(path.join(root, '.git/check-runs')), false);
  const applied = release('initial', '--no-push', '--apply');
  assert.equal(applied.status, 0, applied.stderr);
  assert.equal(git('rev-parse', 'HEAD'), releaseHead);
  assert.equal(git('rev-parse', 'v0.1.0^{commit}'), releaseHead);
  assert.equal(git('cat-file', '-t', 'refs/tags/v0.1.0'), 'tag');
  assert.equal(git('for-each-ref', '--format=%(contents)', 'refs/tags/v0.1.0'), 'Release v0.1.0\n\nRelease-Type: initial');
  assert.deepEqual(sources.map(name => fs.readFileSync(path.join(root, name), 'utf8')), before);
  assert.match(release('initial', '--no-push').stderr, /Tag already exists/);
  const resumed = JSON.parse(release('0.1.0', '--no-push', '--resume').stdout);
  assert.equal(resumed.release_type, 'initial'); assert.equal(resumed.check_phase, null);
  assert.equal(release('0.1.0', '--no-push', '--resume', '--apply').status, 0);
  assert.equal(fs.readFileSync(path.join(root, '.git/check-runs'), 'utf8'), 'checked\n');
  assert.equal(git('status', '--porcelain'), '');
});

test('initial release failures do not create a tag, and resume rejects a different HEAD, unmarked initial or lightweight tag', t => {
  const { git, release } = fixture(t, [[process.execPath, '-e', 'process.exit(1)']]);
  assert.match(release('0.1.0', '--no-push').stderr, /make release-initial/);
  assert.match(release('initial', '--resume', '--no-push').stderr, /requires an exact version/);
  const failed = release('initial', '--apply', '--no-push');
  assert.notEqual(failed.status, 0); assert.match(failed.stderr, /checking the current version/);
  assert.equal(git('tag'), '');
  assert.notEqual(release('0.0.9', '--no-push').status, 0);
  git('tag', 'v0.1.0');
  assert.match(release('0.1.0', '--resume', '--no-push').stderr, /annotated tag/);
  git('tag', '-d', 'v0.1.0');
  git('tag', '-a', 'v0.1.0', '-m', 'Release v0.1.0');
  assert.match(release('0.1.0', '--resume', '--no-push').stderr, /tag has no initial-release marker.*do not move or rewrite/);
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
  assert.equal(release('initial', '--apply', '--no-push').status, 0);
  assert.equal(release('0.1.0', '--resume', '--no-push', '--apply').status, 0);
  assert.equal(notes(), before);
});

test('initial release resumes from a signed annotated tag without rewriting it', t => {
  const { root, git, release } = fixture(t);
  const signingKey = path.join(root, '.git/release-signing-key');
  command(root, 'ssh-keygen', ['-t', 'ed25519', '-N', '', '-f', signingKey]);
  git('config', 'gpg.format', 'ssh');
  git('config', 'user.signingkey', signingKey);
  git('config', 'tag.gpgsign', 'true');
  const applied = release('initial', '--apply', '--no-push');
  assert.equal(applied.status, 0, applied.stderr);
  assert.match(git('cat-file', 'tag', 'v0.1.0'), /BEGIN SSH SIGNATURE/);
  const tag = git('rev-parse', 'v0.1.0');
  const resumed = release('0.1.0', '--resume', '--apply', '--no-push');
  assert.equal(resumed.status, 0, resumed.stderr);
  assert.equal(git('rev-parse', 'v0.1.0'), tag);
  assert.equal(git('status', '--porcelain'), '');
});

test('Make release entry points pass the default bump, initial operation and flags without dirtying the checkout', t => {
  const { root, git } = fixture(t);
  const make = 'make';
  fs.copyFileSync(path.join(scriptsDir, '../Makefile'), path.join(root, 'Makefile'));
  git('add', 'Makefile'); git('commit', '-m', 'test: add Make release entry points');
  const original = git('rev-parse', 'HEAD');
  const preview = command(root, make, ['--silent', 'release', 'RELEASE_FLAGS=--no-push']);
  assert.equal(JSON.parse(preview.stdout).target, '0.1.1');
  const initial = command(root, make, ['--silent', 'release-initial', 'RELEASE_FLAGS=--no-push']);
  const plan = JSON.parse(initial.stdout);
  assert.equal(plan.target, '0.1.0'); assert.equal(plan.release_type, 'initial');
  assert.equal(plan.mode, 'preview'); assert.equal(plan.push, false);
  assert.equal(git('tag'), ''); assert.equal(git('rev-parse', 'HEAD'), original);
  command(root, make, ['--silent', 'release-initial', 'RELEASE_FLAGS=--apply --no-push']);
  assert.equal(git('rev-parse', 'v0.1.0^{commit}'), original);
  assert.equal(git('rev-parse', 'HEAD'), original);
  assert.equal(git('status', '--porcelain'), '');
});

test('Make extension installation ignores desktop BROWSER commands and accepts explicit targets', () => {
  const root = path.join(scriptsDir, '..');
  const env = { BROWSER: 'omarchy-launch-browser' };
  const configured = command(root, 'make', ['--just-print', 'browser:install'], true, env);
  assert.doesNotMatch(configured.stdout, /--browser|omarchy-launch-browser/);
  const explicit = command(root, 'make', ['--just-print', 'browser:install', 'BROWSER=firefox'], true, env);
  assert.match(explicit.stdout, /--browser "firefox"/);
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

test('initial release rejects a remote tag and local-only preview does not query that remote', t => {
  const { root, git, release } = fixture(t);
  const bare = path.join(root, '.git/remote.git');
  command(root, 'git', ['init', '--bare', bare]);
  git('remote', 'add', 'origin', bare); git('push', '-u', 'origin', 'main');
  git('tag', '-a', 'v0.1.0', '-m', 'Release v0.1.0');
  git('push', 'origin', 'refs/tags/v0.1.0'); git('tag', '-d', 'v0.1.0');
  const failed = release('initial');
  assert.notEqual(failed.status, 0); assert.match(failed.stderr, /Remote tag already exists/);
  assert.equal(release('initial', '--no-push').status, 0);
  assert.equal(git('tag'), ''); assert.equal(git('status', '--porcelain'), '');
});

test('initial GitHub preflight checks visible releases with pagination even when creation is disabled, and fails closed', { skip: process.platform === 'win32' && 'POSIX executable fixtures require Unix shebang support' }, t => {
  const { root, git, release, env } = fixture(t);
  const bare = path.join(root, '.git/remote.git');
  command(root, 'git', ['init', '--bare', bare]);
  git('remote', 'add', 'origin', bare); git('push', '-u', 'origin', 'main');
  const original = git('rev-parse', 'HEAD');
  const realGit = command(root, 'which', ['git']).stdout.trim();
  const bin = path.join(root, '.git/bin');
  fs.mkdirSync(bin);
  fs.writeFileSync(path.join(bin, 'git'), `#!${process.execPath}\n
    const { spawnSync } = require('node:child_process');
    const args = process.argv.slice(2);
    if (args.join(' ') === 'remote get-url --push --all origin') console.log('https://github.com/fixture/passpine.git');
    else {
      if (args[0] === 'ls-remote' && args[1] === 'https://github.com/fixture/passpine.git') args[1] = ${JSON.stringify(bare)};
      const result = spawnSync(${JSON.stringify(realGit)}, args, { stdio: 'inherit' });
      process.exit(result.status ?? 1);
    }
  `, { mode: 0o755 });
  fs.writeFileSync(path.join(bin, 'gh'), `#!${process.execPath}\n
    const fs = require('node:fs');
    fs.appendFileSync('.git/github-calls', JSON.stringify(process.argv.slice(2)) + '\\n');
    if (fs.existsSync('.git/query-error')) { console.error('Simulated authentication or network failure'); process.exit(1); }
    if (fs.existsSync('.git/github-tags')) process.stdout.write(fs.readFileSync('.git/github-tags'));
  `, { mode: 0o755 });
  env.PATH = `${bin}${path.delimiter}${process.env.PATH}`;
  const preview = release('initial');
  assert.equal(preview.status, 0, preview.stderr);
  assert.equal(JSON.parse(preview.stdout).github_release, false);
  const calls = () => fs.readFileSync(path.join(root, '.git/github-calls'), 'utf8').trim().split('\n').map(JSON.parse);
  assert.deepEqual(calls(), [['api', '--hostname', 'github.com', '--paginate', 'repos/fixture/passpine/releases', '--jq', '.[].tag_name']]);
  fs.writeFileSync(path.join(root, '.git/github-tags'), 'v9.0.0\nv0.1.0\n');
  const existing = release('initial', '--no-github-release', '--apply');
  assert.notEqual(existing.status, 0); assert.match(existing.stderr, /GitHub Release already exists: v0\.1\.0/);
  fs.writeFileSync(path.join(root, '.git/query-error'), 'authentication or network failure');
  const failed = release('initial');
  assert.notEqual(failed.status, 0); assert.match(failed.stderr, /Command failed: gh api.*\nSimulated authentication or network failure/);
  const before = calls().length;
  assert.equal(release('initial', '--no-push').status, 0);
  assert.equal(calls().length, before);
  assert.equal(git('rev-parse', 'HEAD'), original);
  assert.equal(git('tag'), ''); assert.equal(git('status', '--porcelain'), '');
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
    if (args[0] === 'api' && args.includes('repos/fixture/passpine/releases')) {
      console.log(Object.keys(state).join('\\n')); process.exit(0);
    }
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
  const preview = release('initial');
  assert.equal(preview.status, 0, preview.stderr);
  const plan = JSON.parse(preview.stdout);
  assert.equal(plan.github_release, true);
  assert.equal(plan.artifacts.length, 3);
  assert.equal(fs.existsSync(path.join(root, '.git/build-runs')), false);
  const previewCalls = fs.readFileSync(path.join(root, '.git/github-calls'), 'utf8').trim().split('\n').map(JSON.parse);
  assert.equal(previewCalls.length, 1); assert.equal(previewCalls[0][0], 'api'); assert.ok(previewCalls[0].includes('--paginate'));
  fs.writeFileSync(path.join(root, '.git/fail-build'), 'interrupt');
  const failedBuild = release('initial', '--apply');
  assert.notEqual(failedBuild.status, 0);
  assert.match(failedBuild.stderr, /Build failed/);
  assert.equal(git('tag'), '');
  assert.equal(git('rev-parse', 'HEAD'), original);
  assert.equal(command(root, 'git', ['--git-dir', bare, 'tag', '--list']).stdout.trim(), '');
  fs.unlinkSync(path.join(root, '.git/fail-build'));
  fs.writeFileSync(path.join(root, '.git/fail-upload'), 'interrupt');
  const partial = release('initial', '--apply');
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

test('Linux development installer writes a desktop entry with escaped executable paths', { skip: process.platform !== 'linux' }, t => {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-install-')));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const dir of ['scripts', 'dist/dev', 'client/assets', 'browser', 'tools']) fs.mkdirSync(path.join(root, dir), { recursive: true });
  for (const script of ['install-client.sh', 'build-channel.mjs', 'version.mjs', 'release-notes.mjs', 'release.mjs', 'release-artifacts.mjs', 'update-artifacts.mjs']) {
    fs.copyFileSync(path.join(scriptsDir, script), path.join(root, 'scripts', script));
  }
  for (const name of ['package.json', 'browser/package.json']) fs.writeFileSync(path.join(root, name), '{"version":"0.1.0"}\n');
  for (const binary of ['passpine', 'passpine-host']) fs.writeFileSync(path.join(root, 'dist/dev', binary), '#!/bin/sh\ncase "$1" in\n  --build-channel) printf "dev\\n" ;;\n  --register) printf "%s\\n" "$1" > "$PASSPINE_TEST_REGISTER_LOG" ;;\n  *) exit 99 ;;\nesac\n', { mode: 0o755 });
  // Every executable used by installation is a fixture; no user host or
  // desktop database is registered, including during a simulated Linux run.
  fs.writeFileSync(path.join(root, 'tools/uname'), '#!/bin/sh\nprintf "Linux\\n"\n', { mode: 0o755 });
  fs.writeFileSync(path.join(root, 'tools/update-desktop-database'), '#!/bin/sh\nprintf "%s\\n" "$1" > "$PASSPINE_TEST_DESKTOP_LOG"\n', { mode: 0o755 });
  fs.writeFileSync(path.join(root, 'client/assets/app-icon-monochrome.png'), 'fixture');
  const installDir = path.join(root, '..', path.basename(root) + '-bin\t "\\$`%');
  t.after(() => fs.rmSync(installDir, { recursive: true, force: true }));
  const dataDir = path.join(root, 'data');
  const registrationLog = path.join(root, 'registered'), desktopLog = path.join(root, 'desktop-updated');
  command(root, 'bash', ['scripts/install-client.sh', installDir], true, {
    PASSPINE_BUILD_CHANNEL: 'dev', XDG_DATA_HOME: dataDir,
    PATH: `${path.join(root, 'tools')}${path.delimiter}${process.env.PATH}`,
    PASSPINE_TEST_REGISTER_LOG: registrationLog, PASSPINE_TEST_DESKTOP_LOG: desktopLog,
  });
  const entry = fs.readFileSync(path.join(dataDir, 'applications/com.knothhe.passpine.dev.desktop'), 'utf8');
  assert.equal(entry.split('\n')[0], '[Desktop Entry]');
  assert.match(entry, /\nName=Passpine Dev\n/);
  assert.match(entry, /\nIcon=com\.knothhe\.passpine\.dev\n/);
  assert.equal(fs.readFileSync(registrationLog, 'utf8'), '--register\n');
  assert.equal(fs.readFileSync(desktopLog, 'utf8'), path.join(dataDir, 'applications') + '\n');
  for (const binary of ['passpine-dev', 'passpine-dev-host']) assert.equal(fs.statSync(path.join(installDir, binary)).mode & 0o777, 0o755);
  for (const binary of ['passpine', 'passpine-host']) assert.equal(fs.existsSync(path.join(installDir, binary)), false);
  const exec = entry.split('\n').find(line => line.startsWith('Exec=')).slice(5);
  // Decode Desktop Entry string escapes, then the quoted Exec argument.
  const decoded = exec.replace(/\\([snrt\\])/g, (_, c) => ({ s: ' ', n: '\n', r: '\r', t: '\t', '\\': '\\' })[c]);
  assert.equal(decoded[0], '"');
  assert.equal(decoded.at(-1), '"');
  assert.equal(decoded.slice(1, -1).replace(/\\(["`$\\])/g, '$1').replace(/%%/g, '%'), path.join(installDir, 'passpine-dev'));
});
