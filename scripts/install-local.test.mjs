import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { installLocal } from './install-local.mjs';

const identities = JSON.parse(fs.readFileSync(new URL('../config/browser-identities.json', import.meta.url), 'utf8'));
const manifest = (channel = 'dev', browser = 'chrome') => ({
  manifest_version: 3, name: `Passpine${channel === 'dev' ? ' Dev' : ''}`, version: '0.1.3', key: identities[channel].key,
  ...(browser === 'firefox' ? { browser_specific_settings: { gecko: { id: identities[channel].firefoxID } } } : {}),
});

function fixture(t, config = {}) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-dev-extension-test-'));
  const root = path.join(directory, 'repo'), parent = path.join(directory, 'extensions');
  fs.mkdirSync(root); fs.mkdirSync(parent);
  fs.writeFileSync(path.join(root, 'package.json'), JSON.stringify({ name: 'passpine', packageManager: 'pnpm@12.11.2' }));
  fs.writeFileSync(path.join(root, '.install-local.json'), JSON.stringify({ name: 'passpine', directories: { chrome: parent, firefox: parent }, ...config }));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const runBuild = async (browser, _root, options) => {
    fs.mkdirSync(options.outputs[browser], { recursive: true });
    fs.writeFileSync(path.join(options.outputs[browser], 'manifest.json'), JSON.stringify(manifest('dev', browser)));
  };
  return { root, parent, runBuild };
}

test('public browser identities derive distinct Chromium IDs and native hosts', () => {
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

test('legacy passpine local configuration installs a separate dev folder and preserves release', async t => {
  const options = fixture(t), release = path.join(options.parent, 'passpine');
  fs.mkdirSync(release); fs.writeFileSync(path.join(release, 'manifest.json'), JSON.stringify(manifest('release')));
  fs.writeFileSync(path.join(release, 'keep.txt'), 'installed release');
  const before = fs.readFileSync(path.join(release, 'manifest.json'));
  const result = await installLocal([], { ...options, confirmReplacement: async () => assert.fail('New dev folder does not need confirmation') });
  assert.equal(result.destination, path.join(options.parent, 'passpine-dev'));
  assert.deepEqual(fs.readFileSync(path.join(release, 'manifest.json')), before);
  assert.equal(fs.readFileSync(path.join(release, 'keep.txt'), 'utf8'), 'installed release');
  const ownership = JSON.parse(fs.readFileSync(path.join(result.destination, '.h2-ext-local.json'), 'utf8'));
  assert.equal(ownership.channel, 'dev');
  assert.equal(ownership.schema, 2);
  await installLocal([], { ...options, confirmReplacement: async () => assert.fail('Managed dev update does not need confirmation') });
});

test('dev suffix is applied to explicit names once', async t => {
  const options = fixture(t);
  const first = await installLocal(['--name', 'custom'], options);
  const second = await installLocal(['--name', 'custom-dev'], options);
  assert.equal(first.destination, path.join(options.parent, 'custom-dev'));
  assert.equal(second.destination, first.destination);
});

test('release identity in the dev destination is protected before any build or confirmation', async t => {
  const options = fixture(t), destination = path.join(options.parent, 'passpine-dev');
  fs.mkdirSync(destination); fs.writeFileSync(path.join(destination, 'manifest.json'), JSON.stringify(manifest('release')));
  await assert.rejects(installLocal([], { ...options,
    runBuild: () => assert.fail('Protected destination must be rejected before building'),
    confirmReplacement: () => assert.fail('Release destination cannot be overwritten by confirmation'),
  }), /must not replace a release extension/);
  assert.equal(JSON.parse(fs.readFileSync(path.join(destination, 'manifest.json'))).key, identities.release.key);
});

test('custom output pointing to a release build is rejected and existing dev files survive', async t => {
  const options = fixture(t), destination = path.join(options.parent, 'passpine-dev');
  await installLocal([], options);
  fs.writeFileSync(path.join(destination, 'keep.txt'), 'previous dev');
  await assert.rejects(installLocal([], { ...options, runBuild: async (browser, _root, settings) => {
    fs.writeFileSync(path.join(settings.outputs[browser], 'manifest.json'), JSON.stringify(manifest('release')));
  } }), /requires a development extension build/);
  assert.equal(fs.readFileSync(path.join(destination, 'keep.txt'), 'utf8'), 'previous dev');
  assert.equal(JSON.parse(fs.readFileSync(path.join(destination, 'manifest.json'))).key, identities.dev.key);
});

test('Firefox local install checks its dev ID as well as the public key', async t => {
  const options = fixture(t);
  await assert.rejects(installLocal(['firefox'], { ...options, runBuild: async (browser, root, settings) => {
    await options.runBuild(browser, root, settings);
    const wrong = manifest('dev', browser); wrong.browser_specific_settings.gecko.id = identities.release.firefoxID;
    fs.writeFileSync(path.join(settings.outputs[browser], 'manifest.json'), JSON.stringify(wrong));
  } }), /development Firefox extension identity/);
  assert.equal(fs.existsSync(path.join(options.parent, 'passpine-dev')), false);
});
