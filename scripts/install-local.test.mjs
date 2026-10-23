import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { installLocal } from './install-local.mjs';

const identities = JSON.parse(fs.readFileSync(new URL('../config/browser-identities.json', import.meta.url), 'utf8'));
const manifest = (channel = 'release', browser = 'chrome') => ({
  manifest_version: 3, name: `Passpine${channel === 'dev' ? ' Dev' : ''}`, version: '0.1.3', key: identities[channel].key,
  ...(browser === 'firefox' ? { browser_specific_settings: { gecko: { id: identities[channel].firefoxID } } } : {}),
});

function fixture(t, config = {}) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-extension-test-'));
  const root = path.join(directory, 'repo'), parent = path.join(directory, 'extensions');
  fs.mkdirSync(root); fs.mkdirSync(parent);
  fs.writeFileSync(path.join(root, 'package.json'), JSON.stringify({ name: 'passpine', packageManager: 'pnpm@12.11.2' }));
  fs.writeFileSync(path.join(root, '.install-local.json'), JSON.stringify({ name: 'passpine', directories: { chrome: parent, firefox: parent }, ...config }));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const runBuild = async (browser, _root, options) => {
    fs.mkdirSync(options.outputs[browser], { recursive: true });
    fs.writeFileSync(path.join(options.outputs[browser], 'manifest.json'), JSON.stringify(manifest('release', browser)));
  };
  return { root, parent, runBuild };
}

test('current and legacy browser identities retain their distinct Chromium IDs and native hosts', () => {
  for (const channel of ['dev', 'release']) {
    const identity = identities[channel];
    const hash = crypto.createHash('sha256').update(Buffer.from(identity.key, 'base64')).digest('hex').slice(0, 32);
    assert.equal(identity.chromiumID, hash.replace(/[0-9a-f]/g, character => String.fromCharCode(97 + Number.parseInt(character, 16))));
    assert.equal(crypto.createPublicKey({ key: Buffer.from(identity.key, 'base64'), format: 'der', type: 'spki' }).asymmetricKeyType, 'rsa');
  }
  assert.notEqual(identities.dev.chromiumID, identities.release.chromiumID);
  assert.notEqual(identities.dev.firefoxID, identities.release.firefoxID);
  assert.notEqual(identities.dev.host, identities.release.host);
});

test('local configuration installs the exact name with the unified identity and preserves adjacent files', async t => {
  const options = fixture(t), adjacent = path.join(options.parent, 'other-extension');
  fs.mkdirSync(adjacent); fs.writeFileSync(path.join(adjacent, 'keep.txt'), 'other extension');
  fs.writeFileSync(path.join(options.parent, 'keep.txt'), 'parent file');
  const result = await installLocal([], { ...options, confirmReplacement: async () => assert.fail('New folder does not need confirmation') });
  assert.equal(result.destination, path.join(options.parent, 'passpine'));
  assert.equal(fs.readFileSync(path.join(adjacent, 'keep.txt'), 'utf8'), 'other extension');
  assert.equal(fs.readFileSync(path.join(options.parent, 'keep.txt'), 'utf8'), 'parent file');
  assert.equal(JSON.parse(fs.readFileSync(path.join(result.destination, 'manifest.json'))).key, identities.release.key);
  const ownership = JSON.parse(fs.readFileSync(path.join(result.destination, '.h2-ext-local.json'), 'utf8'));
  assert.equal(ownership.channel, undefined);
  assert.equal(ownership.schema, 2);
  await installLocal([], { ...options, confirmReplacement: async () => assert.fail('Managed update does not need confirmation') });
});

test('explicit names are preserved exactly, including any configured dev suffix', async t => {
  const options = fixture(t);
  const first = await installLocal(['--name', 'custom'], options);
  const second = await installLocal(['--name', 'custom-dev'], options);
  assert.equal(first.destination, path.join(options.parent, 'custom'));
  assert.equal(second.destination, path.join(options.parent, 'custom-dev'));
});

test('unmanaged unified installation requires confirmation before build or replacement', async t => {
  const options = fixture(t), destination = path.join(options.parent, 'passpine');
  fs.mkdirSync(destination); fs.writeFileSync(path.join(destination, 'manifest.json'), JSON.stringify(manifest('release')));
  fs.writeFileSync(path.join(destination, 'keep.txt'), 'previous installation');
  let confirmations = 0;
  const result = await installLocal([], { ...options,
    runBuild: () => assert.fail('Cancelled overwrite must not build'),
    confirmReplacement: async target => { assert.equal(target, destination); confirmations++; return false; },
  });
  assert.equal(result, null);
  assert.equal(confirmations, 1);
  assert.equal(fs.readFileSync(path.join(destination, 'keep.txt'), 'utf8'), 'previous installation');
  await installLocal([], { ...options, confirmReplacement: async () => true });
  assert.equal(fs.existsSync(path.join(destination, 'keep.txt')), false);
  assert.equal(JSON.parse(fs.readFileSync(path.join(destination, 'manifest.json'))).key, identities.release.key);
});

test('custom output with the legacy dev identity is rejected and existing files survive', async t => {
  const options = fixture(t), destination = path.join(options.parent, 'passpine');
  await installLocal([], options);
  fs.writeFileSync(path.join(destination, 'keep.txt'), 'previous extension');
  await assert.rejects(installLocal([], { ...options, runBuild: async (browser, _root, settings) => {
    fs.writeFileSync(path.join(settings.outputs[browser], 'manifest.json'), JSON.stringify(manifest('dev')));
  } }), /requires the unified extension identity/);
  assert.equal(fs.readFileSync(path.join(destination, 'keep.txt'), 'utf8'), 'previous extension');
  assert.equal(JSON.parse(fs.readFileSync(path.join(destination, 'manifest.json'))).key, identities.release.key);
});

test('Firefox local install checks its unified ID as well as the public key', async t => {
  const options = fixture(t);
  await assert.rejects(installLocal(['firefox'], { ...options, runBuild: async (browser, root, settings) => {
    await options.runBuild(browser, root, settings);
    const wrong = manifest('release', browser); wrong.browser_specific_settings.gecko.id = identities.dev.firefoxID;
    fs.writeFileSync(path.join(settings.outputs[browser], 'manifest.json'), JSON.stringify(wrong));
  } }), /unified Firefox extension identity/);
  assert.equal(fs.existsSync(path.join(options.parent, 'passpine')), false);
});

test('old same-project schema 2 dev installations update without another confirmation', async t => {
  const options = fixture(t, { name: 'passpine-dev' }), destination = path.join(options.parent, 'passpine-dev');
  fs.mkdirSync(destination);
  fs.writeFileSync(path.join(destination, 'manifest.json'), JSON.stringify(manifest('dev')));
  fs.writeFileSync(path.join(destination, 'old.js'), 'previous build');
  fs.writeFileSync(path.join(destination, '.h2-ext-local.json'), JSON.stringify({ installer: 'h2-ext-local', schema: 2, project: fs.realpathSync(options.root), browser: 'chrome', channel: 'dev' }));
  await installLocal([], { ...options, confirmReplacement: async () => assert.fail('Same-project schema 2 installation is already managed') });
  assert.equal(JSON.parse(fs.readFileSync(path.join(destination, 'manifest.json'))).key, identities.release.key);
  assert.equal(fs.existsSync(path.join(destination, 'old.js')), false);
});

test('a managed directory belonging to another browser still requires confirmation', async t => {
  const options = fixture(t);
  await installLocal([], options);
  const result = await installLocal(['firefox'], { ...options,
    runBuild: () => assert.fail('Cancelled replacement must not build'),
    confirmReplacement: async () => false,
  });
  assert.equal(result, null);
});

test('a directory populated during build cannot be silently replaced', async t => {
  const options = fixture(t), destination = path.join(options.parent, 'passpine');
  await assert.rejects(installLocal([], { ...options, runBuild: async (...args) => {
    await options.runBuild(...args);
    fs.mkdirSync(destination);
    fs.writeFileSync(path.join(destination, 'keep.txt'), 'new unmanaged files');
  } }), /changed during the build.*overwrite confirmation/);
  assert.equal(fs.readFileSync(path.join(destination, 'keep.txt'), 'utf8'), 'new unmanaged files');
});

test('overlap and symlink destinations are rejected before confirmation or build', async t => {
  const options = fixture(t);
  const forbidden = { ...options,
    runBuild: () => assert.fail('Invalid destinations must fail before build'),
    confirmReplacement: () => assert.fail('Invalid destinations cannot be confirmed'),
  };
  await assert.rejects(installLocal(['--dir', options.root], forbidden), /must not overlap the repository/);
  fs.symlinkSync(options.root, path.join(options.parent, 'passpine'), 'dir');
  await assert.rejects(installLocal([], forbidden), /must not overlap the repository|must be a real directory/);
});
