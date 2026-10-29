import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { test } from 'node:test';

// Exercise the actual acceptance entry point while stopping at external command
// boundaries. Platform selection tests command orchestration, not native signing
// or installation; those still require the corresponding CI runner.
function fixture(t, { platform, failBuild = false }) {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'passpine acceptance environment ')));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const source = path.join(root, 'source repository');
  const runner = path.join(root, 'runner temp');
  const runnerHome = path.join(root, 'runner home');
  const eventsFile = path.join(root, 'command events.jsonl');
  // URL delimiters expose unencoded --import paths even on Unix runners.
  const preload = path.join(root, 'command #100% boundaries.mjs');
  fs.mkdirSync(source); fs.mkdirSync(runner); fs.mkdirSync(runnerHome);
  // Keep every script dependency available; only process creation is replaced.
  fs.cpSync(fileURLToPath(new URL('./', import.meta.url)), path.join(source, 'scripts'), { recursive: true });
  const sentinel = path.join(runnerHome, 'preserve-runner-home');
  fs.writeFileSync(sentinel, 'original runner home');
  fs.writeFileSync(preload, `
import fs from 'node:fs';
import path from 'node:path';
import childProcess from 'node:child_process';
import { EventEmitter } from 'node:events';
import { syncBuiltinESMExports } from 'node:module';
Object.defineProperty(process, 'platform', { value: process.env.PASSPINE_TEST_PLATFORM });
const selected = env => Object.fromEntries([
  'HOME', 'USERPROFILE', 'APPDATA', 'LOCALAPPDATA', 'XDG_CONFIG_HOME',
  'XDG_DATA_HOME', 'XDG_RUNTIME_DIR', 'PASSPINE_DEV_HOME',
  'PASSPINE_BUILD_CHANNEL', 'PASSPINE_SKIP_TEST_VAULT',
  'PASSPINE_MACOS_SIGNING_IDENTITY', 'PASSPINE_MACOS_SIGNING_KEYCHAIN',
].map(key => [key, env?.[key]]));
const log = event => fs.appendFileSync(process.env.PASSPINE_TEST_EVENTS, JSON.stringify(event) + '\\n');
const scriptName = value => String(value).replaceAll('\\\\', '/').split('/').at(-1);
childProcess.execFileSync = (file, args, options) => {
  const names = args.map(scriptName);
  const build = names.includes('build-client.sh') || names.includes('build-client.ps1');
  const install = names.includes('install-client.sh') || names.includes('install-client.ps1');
  if (!build && !install) throw new Error('Unexpected external command: ' + file);
  log({ kind: build ? 'build' : 'install', file, args, env: selected(options.env) });
  if (build) {
    if (process.env.PASSPINE_TEST_FAIL_BUILD === '1') throw new Error('fixture: Dev build failed');
    return Buffer.alloc(0);
  }
  // The real script has reached installation with its final environment. Do
  // not execute an installer, registration command or native application.
  throw new Error('fixture: reached isolated installation');
};
childProcess.spawn = (file, args, options) => {
  if (file !== 'reg.exe' || args.join(' ') !== 'query HKCU\\\\Software\\\\Passpine Dev') {
    throw new Error('Unexpected spawned command: ' + file);
  }
  log({ kind: 'registration-check', file, args, env: selected(options.env) });
  const child = new EventEmitter();
  process.nextTick(() => child.emit('exit', 1));
  return child;
};
syncBuiltinESMExports();
`);
  const env = {
    ...Object.fromEntries(Object.entries(process.env).filter(([key]) => !/^(PASSPINE_|XDG_)/.test(key))),
    HOME: runnerHome, USERPROFILE: runnerHome,
    GITHUB_ACTIONS: 'true', RUNNER_TEMP: runner,
    PASSPINE_BUILD_CHANNEL: 'release',
    PASSPINE_MACOS_SIGNING_IDENTITY: 'A'.repeat(40),
    PASSPINE_MACOS_SIGNING_KEYCHAIN: path.join(root, 'temporary signing.keychain-db'),
    PASSPINE_TEST_PLATFORM: platform,
    PASSPINE_TEST_FAIL_BUILD: failBuild ? '1' : '0',
    PASSPINE_TEST_EVENTS: eventsFile,
  };
  const result = spawnSync(process.execPath, ['--import', pathToFileURL(preload).href, path.join(source, 'scripts/verify-dev-installation.mjs'), '--run'], {
    cwd: source, env, encoding: 'utf8', timeout: 30000,
  });
  const events = fs.existsSync(eventsFile) ? fs.readFileSync(eventsFile, 'utf8').trim().split('\n').filter(Boolean).map(line => JSON.parse(line)) : [];
  assert.equal(fs.readFileSync(sentinel, 'utf8'), 'original runner home');
  return { root, source, runner, runnerHome, env, result, events };
}

const contained = (parent, child) => {
  const relative = path.relative(parent, child);
  return relative !== '' && relative !== '..' && !relative.startsWith(`..${path.sep}`) && !path.isAbsolute(relative);
};

function assertIsolatedEnvironment(env, fixture) {
  assert.notEqual(env.HOME, fixture.runnerHome);
  for (const key of ['HOME', 'USERPROFILE', 'APPDATA', 'LOCALAPPDATA', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_RUNTIME_DIR', 'PASSPINE_DEV_HOME']) {
    assert.ok(contained(fixture.runner, env[key]), `${key} must stay inside the isolated runner fixture`);
    assert.equal(contained(fixture.source, env[key]), false, `${key} must not overlap the source repository`);
    assert.ok(fs.statSync(env[key]).isDirectory(), `${key} must exist before external commands run`);
  }
  assert.equal(env.PASSPINE_BUILD_CHANNEL, 'dev');
  assert.equal(env.PASSPINE_SKIP_TEST_VAULT, '1');
}

test('macOS Dev signing keeps the runner home while installation retains isolated data paths', t => {
  const f = fixture(t, { platform: 'darwin' });
  assert.notEqual(f.result.status, 0);
  assert.match(f.result.stderr, /fixture: reached isolated installation/);
  assert.deepEqual(f.events.map(event => event.kind), ['build', 'install']);
  const [build, install] = f.events;
  assert.equal(build.env.HOME, f.runnerHome, 'macOS signing must retain the home used to import the temporary keychain');
  assertIsolatedEnvironment(install.env, f);
  for (const key of ['USERPROFILE', 'APPDATA', 'LOCALAPPDATA', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_RUNTIME_DIR', 'PASSPINE_DEV_HOME']) {
    assert.equal(build.env[key], install.env[key], `${key} must remain isolated during the macOS build`);
  }
  for (const event of [build, install]) {
    assert.equal(event.env.PASSPINE_MACOS_SIGNING_IDENTITY, f.env.PASSPINE_MACOS_SIGNING_IDENTITY);
    assert.equal(event.env.PASSPINE_MACOS_SIGNING_KEYCHAIN, f.env.PASSPINE_MACOS_SIGNING_KEYCHAIN);
    assert.equal(event.env.PASSPINE_BUILD_CHANNEL, 'dev');
    assert.equal(event.env.PASSPINE_SKIP_TEST_VAULT, '1');
  }
});

test('Linux and Windows builds and installation keep the isolated environment', async t => {
  for (const platform of ['linux', 'win32']) await t.test(platform, t => {
    const f = fixture(t, { platform });
    assert.notEqual(f.result.status, 0);
    assert.match(f.result.stderr, /fixture: reached isolated installation/);
    assert.deepEqual(f.events.map(event => event.kind), platform === 'win32' ? ['registration-check', 'build', 'install'] : ['build', 'install']);
    for (const event of f.events) assertIsolatedEnvironment(event.env, f);
  });
});

test('a failed Dev build prevents installation on every platform', async t => {
  for (const platform of ['darwin', 'linux', 'win32']) await t.test(platform, t => {
    const f = fixture(t, { platform, failBuild: true });
    assert.notEqual(f.result.status, 0);
    assert.match(f.result.stderr, /fixture: Dev build failed/);
    assert.equal(f.events.filter(event => event.kind === 'build').length, 1);
    assert.equal(f.events.some(event => event.kind === 'install'), false);
  });
});
