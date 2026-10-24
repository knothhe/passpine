import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';

const identities = JSON.parse(await readFile(new URL('../../config/browser-identities.json', import.meta.url), 'utf8'));
const identity = identities.release;
const args = process.argv.slice(2);
const store = args[0] === '--store';
if (store) args.shift();
const paths = args.map(value => ['chrome', 'firefox'].includes(value) ? path.join('.output', `${value}-mv3${store ? '-store' : ''}`, 'manifest.json') : value);
assert(paths.length > 0, 'Provide at least one built manifest path');

for (const path of paths) {
  const manifest = JSON.parse(await readFile(path, 'utf8'));
  assert.equal(manifest.manifest_version, 3, 'Expected an MV3 extension');
  assert.equal(manifest.name, '__MSG_extensionName__', 'Extension name must use the unified localized name');
  assert.equal(manifest.description, '__MSG_extensionDescription__', 'Extension description must use the unified localized description');
  assert.equal(manifest.action?.default_title, '__MSG_extensionName__', 'Toolbar title must use the unified localized name');
  if (store) assert(!Object.hasOwn(manifest, 'key'), 'Chrome Web Store manifest must omit the key field');
  else assert.equal(manifest.key, identity.key, 'Unpacked manifest must use the unified extension public key');
  assert.deepEqual(manifest.host_permissions ?? [], [], 'Website access must remain optional');
  assert.deepEqual(manifest.optional_host_permissions, ['http://*/*', 'https://*/*'], 'Page shortcuts request optional website access');
  assert(!manifest.content_scripts?.length, 'Page shortcut scripts must be registered only after permission is granted');
  if (manifest.browser_specific_settings?.gecko) assert.equal(manifest.browser_specific_settings.gecko.id, identity.firefoxID, 'Firefox identity must match the unified extension identity');
  assert.deepEqual(manifest.commands?.['_execute_action']?.suggested_key, { default: 'Ctrl+Shift+L', mac: 'Command+Shift+L' }, 'Opening Passpine must retain its default shortcuts');
  assert.deepEqual(manifest.commands?.['fill-best']?.suggested_key, { default: 'Ctrl+Shift+F', mac: 'Command+Shift+F' }, 'Filling an account must retain its default shortcuts');
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
