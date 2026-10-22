import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { buildChannel } from '../../scripts/build-channel.mjs';

const channel = buildChannel();
const identities = JSON.parse(await readFile(new URL('../../config/browser-identities.json', import.meta.url), 'utf8'));
const identity = identities[channel];
const paths = process.argv.slice(2).map(value => ['chrome', 'firefox'].includes(value) ? path.join(channel === 'release' ? '.output' : '.output-dev', `${value}-mv3`, 'manifest.json') : value);
assert(paths.length > 0, 'Provide at least one built manifest path');

for (const path of paths) {
  const manifest = JSON.parse(await readFile(path, 'utf8'));
  assert.equal(manifest.manifest_version, 3, 'Expected an MV3 extension');
  assert.equal(manifest.action?.default_title, channel === 'release' ? '__MSG_extensionName__' : '__MSG_extensionNameDev__', 'Toolbar title must use the localized channel name');
  assert.equal(manifest.key, identity.key, 'Manifest must use the public key for this build channel');
  if (manifest.browser_specific_settings?.gecko) assert.equal(manifest.browser_specific_settings.gecko.id, identity.firefoxID, 'Firefox identity must match the build channel');
  if (channel === 'dev') assert(Object.values(manifest.commands ?? {}).every(command => !command.suggested_key), 'Development extensions must not reserve the release shortcuts');
  assert(!JSON.stringify(manifest).includes('%COPY_'), 'Manifest contains an unresolved UI copy token');
  const policy = manifest.content_security_policy?.extension_pages;
  assert.equal(typeof policy, 'string', 'Extension pages require an explicit CSP');
  const directives = new Map();
  for (const directive of policy.split(';').map(value => value.trim()).filter(Boolean)) {
    const [name, ...sources] = directive.split(/\s+/);
    assert(!directives.has(name), `Duplicate CSP directive: ${name}`);
    directives.set(name, sources);
  }
  assert.deepEqual(directives.get('style-src'), ["'self'", "'unsafe-inline'"], 'Radix inline component styles must be allowed without remote CSS');
  assert.deepEqual(directives.get('script-src'), ["'self'"], 'Scripts must remain local: no inline, eval, or remote code');
  assert(![...directives.keys()].some(name => name.startsWith('script-src-')), 'Do not override the strict script-src directive');
  assert.deepEqual(directives.get('object-src'), ["'none'"]);
  assert.deepEqual(directives.get('connect-src'), ["'none'"]);
  console.log(`Manifest CSP verified: ${path}`);
}
