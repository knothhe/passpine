import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { assertReleaseBuild, buildChannel, clientBuildConfig } from './build-channel.mjs';
import { readUpdateSigningConfig } from './update-artifacts.mjs';

test('native macOS builds require the SDK Keychain backend before any build side effects', { skip: process.platform === 'win32' }, t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine cgo build-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const scripts = path.join(root, 'scripts'), bin = path.join(root, 'bin');
  fs.mkdirSync(scripts); fs.mkdirSync(bin);
  fs.copyFileSync(new URL('./build-client.sh', import.meta.url), path.join(scripts, 'build-client.sh'));
  fs.writeFileSync(path.join(bin, 'uname'), '#!/bin/sh\nprintf "%s\\n" "$PASSPINE_TEST_UNAME"\n', { mode: 0o755 });
  fs.writeFileSync(path.join(bin, 'node'), '#!/bin/sh\nprintf "%s" "${CGO_ENABLED-unset}" > "$PASSPINE_TEST_CGO_RECORD"\nexit 73\n', { mode: 0o755 });
  const run = (platform, cgo, channel) => {
    const record = path.join(root, `record-${platform}-${String(cgo)}-${channel}`);
    const env = { ...process.env, PATH: `${bin}${path.delimiter}${process.env.PATH}`, PASSPINE_TEST_UNAME: platform, PASSPINE_TEST_CGO_RECORD: record, PASSPINE_BUILD_CHANNEL: channel };
    if (cgo === undefined) delete env.CGO_ENABLED;
    else env.CGO_ENABLED = cgo;
    const result = spawnSync('bash', [path.join(scripts, 'build-client.sh')], { env, encoding: 'utf8' });
    assert.notEqual(result.status, 0, 'the fixture must stop before any real build');
    return { result, value: fs.existsSync(record) ? fs.readFileSync(record, 'utf8') : undefined };
  };
  for (const channel of ['dev', 'release']) {
    for (const value of ['0', '', 'invalid']) {
      const rejected = run('Darwin', value, channel);
      assert.match(rejected.result.stderr, /require CGO_ENABLED=1 for R2 Keychain storage/);
      assert.equal(rejected.value, undefined, 'rejected builds must not run dependency generation or packaging');
    }
    for (const value of [undefined, '1']) assert.equal(run('Darwin', value, channel).value, '1');
  }
  assert.equal(run('Linux', '0', 'dev').value, '0', 'the macOS requirement must not rewrite Linux build flags');
});

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
  assert.equal(config.buildTags, '');
  assert.doesNotMatch(config.ldflags, /github\.com\/egoist\/mygo\.production=/);
  assert.equal(config.commit,git('rev-parse','HEAD'));
  assert.equal(config.modified,'false'); assert.equal(config.executor,'local');
  fs.writeFileSync(path.join(root,'package.json'), JSON.stringify({version:'1.2.3',description:'uncommitted'}));
  const modified = clientBuildConfig({},root);
  assert.equal(modified.modified,'true');
  assert.match(modified.hostLdflags,/buildinfo\.Modified=true/);
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
  assert.equal(config.buildTags, 'mygo_noinspector');
  assert.match(config.ldflags, /(?:^| )-X github\.com\/egoist\/mygo\.production=1(?: |$)/);
  assert.doesNotMatch(config.hostLdflags, /github\.com\/egoist\/mygo\.production=/);
  assert.equal(config.executor,'github-actions'); assert.equal(config.modified,'false');
  for (const overrides of [{ GITHUB_ACTIONS: '' }, { GITHUB_ACTIONS: 'false' }, { GITHUB_REPOSITORY: 'fork/passpine' }, { PASSPINE_BUILD_CHANNEL: 'dev' }, { RELEASE_TAG: '' }]) {
    assert.throws(() => assertReleaseBuild({ ...env, ...overrides }, root));
  }
  assert.throws(() => clientBuildConfig({ ...env, PASSPINE_UPDATE_PUBLIC_KEY: Buffer.alloc(32).toString('base64') }, root), /repository-pinned/);
  assert.throws(() => clientBuildConfig({ ...env, PASSPINE_UPDATE_FEED_URL: 'https://example.invalid/feed.json' }, root), /repository-pinned/);
  fs.writeFileSync(path.join(root, 'package.json'), JSON.stringify({ version: '1.2.4' }));
  fs.writeFileSync(path.join(root, 'browser/package.json'), JSON.stringify({ version: '1.2.4' }));
  assert.throws(() => assertReleaseBuild(env, root), /package versions/);
});

test('channel flags retain the native inspector in Dev and remove it in production', t => {
  const { root, env } = fixture(t);
  const cwd = fileURLToPath(new URL('../client/', import.meta.url));
  const source = path.join(root, 'probe.go');
  fs.writeFileSync(source, `package main
import (
  "fmt"
  "github.com/egoist/mygo"
  "github.com/knothhe/passpine/client/buildinfo"
)
var version string
func main() { fmt.Printf("%s %t", buildinfo.ChannelName(), mygo.IsDev()) }
`);
  for (const channel of ['dev', 'release']) {
    const config = clientBuildConfig({ ...env, PASSPINE_BUILD_CHANNEL: channel }, root);
    const tags = `-tags=${config.buildTags}`;
    const ui = JSON.parse(execFileSync('go', ['list', '-json', tags, 'github.com/egoist/mygo/ui'], { cwd, encoding: 'utf8' }));
    assert.equal(ui.GoFiles.includes('inspector.go'), channel === 'dev');
    assert.equal(ui.GoFiles.includes('inspector_off.go'), channel === 'release');
    const binary = path.join(root, `probe-${channel}${process.platform === 'win32' ? '.exe' : ''}`);
    execFileSync('go', ['build', tags, `-ldflags=${config.ldflags}`, '-o', binary, source], { cwd, stdio: 'pipe' });
    // A release binary must keep developer tools off even in a development environment.
    const actual = execFileSync(binary, [], { encoding: 'utf8', env: { ...process.env, MYGO_ENV: 'development' } });
    assert.equal(actual, `${channel} ${channel === 'dev'}`);
  }
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
