import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const paths = process.argv.slice(2);
assert(paths.length > 0, 'Provide at least one built manifest path');

for (const path of paths) {
  const manifest = JSON.parse(await readFile(path, 'utf8'));
  assert.equal(manifest.manifest_version, 3, 'Expected an MV3 extension');
  assert.equal(manifest.action?.default_title, '__MSG_extensionName__', 'Toolbar title must use the localized extension name');
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
