import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { generateDependencies, resolveDependencies } from './dependencies.mjs';

test('only direct client modules are included and stale versions cannot silently reach About', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-dependencies-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.mkdirSync(path.join(root, 'client/about'), { recursive: true });
  fs.writeFileSync(path.join(root, 'client/go.mod'), 'module example.test/app\nrequire example.test/core v1.2.3\nrequire example.test/indirect v2.0.0 // indirect\n');
  const catalog = [{ id: 'core', module: 'example.test/core', group: 'desktop', version: 'v1.0.0', url: 'https://example.test/core', source: 'https://example.test/core' }];
  fs.writeFileSync(path.join(root, 'client/about/dependencies.json'), JSON.stringify(catalog));
  assert.throws(() => generateDependencies(root, true), /stale/);
  assert.deepEqual(generateDependencies(root).map(dep => dep.version), ['v1.2.3']);
  assert.doesNotThrow(() => generateDependencies(root, true));
  const file = path.join(root, 'client/about/dependencies.json');
  fs.writeFileSync(file, fs.readFileSync(file, 'utf8').replace(/\n/g, '\r\n'));
  assert.doesNotThrow(() => generateDependencies(root, true));
  assert.throws(() => resolveDependencies(root, []), /exactly once/);
  assert.throws(() => resolveDependencies(root, [{ ...catalog[0], url: 'javascript:alert(1)' }]), /Invalid dependency link/);
});

test('the project catalog contains only the current direct client dependencies', () => {
  const deps = generateDependencies(undefined, true);
  assert.ok(deps.every(dep => dep.group === 'desktop' && dep.license && dep.name && dep.version));
});
