import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { manageVersion, nextVersion, validateVersion, versionFiles } from './version.mjs';

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-version-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.mkdirSync(path.join(root, 'browser'));
  for (const name of ['package.json', 'browser/package.json']) fs.writeFileSync(path.join(root, name), JSON.stringify({ name, version: '0.1.0', custom: { keep: true } }, null, 2) + '\n');
  for (const name of ['pnpm-lock.yaml', 'browser/pnpm-lock.yaml']) fs.writeFileSync(path.join(root, name), "lockfileVersion: '9.0'\n");
  return root;
}

test('bump rules reset lower components and reject invalid extension versions', () => {
  assert.equal(nextVersion('1.2.3', 'major'), '2.0.0');
  assert.equal(nextVersion('1.2.3', 'minor'), '1.3.0');
  assert.equal(nextVersion('1.2.3', 'patch'), '1.2.4');
  assert.equal(nextVersion('0.1.0', '0.2.0'), '0.2.0');
  for (const value of ['0.0.0', '01.2.3', '1.2', '1.2.3-rc.1', '1.2.3+build', '65536.0.0', '1.2.3\n', undefined]) assert.throws(() => validateVersion(value));
  for (const value of ['1.2.3', '1.2.2', '1.1.99', '0.9.9']) assert.throws(() => nextVersion('1.2.3', value));
  assert.throws(() => nextVersion('1.2.65535', 'patch'));
});

test('all version entry points synchronize packages without touching the lock', t => {
  const root = fixture(t);
  assert.equal(manageVersion(['show'], root), '0.1.0');
  assert.match(manageVersion(['check'], root), /synchronized/);
  assert.equal(manageVersion(['minor'], root), '0.1.0 → 0.2.0');
  assert.equal(manageVersion(['set', '1.0.0'], root), '0.2.0 → 1.0.0');
  const [current, files] = versionFiles(root);
  assert.equal(current, '1.0.0');
  for (const data of files.values()) { assert.equal(data.version, current); assert.deepEqual(data.custom, { keep: true }); }
  for (const name of ['pnpm-lock.yaml', 'browser/pnpm-lock.yaml']) assert.equal(fs.readFileSync(path.join(root, name), 'utf8'), "lockfileVersion: '9.0'\n");
});

test('mismatched sources, symlinks and unexpected arguments fail before writes', t => {
  const root = fixture(t);
  const browser = path.join(root, 'browser/package.json');
  const before = fs.readFileSync(path.join(root, 'package.json'), 'utf8');
  fs.writeFileSync(browser, '{"version":"0.2.0"}');
  assert.throws(() => manageVersion(['patch'], root), /differs/);
  assert.equal(fs.readFileSync(path.join(root, 'package.json'), 'utf8'), before);
  fs.unlinkSync(browser);
  fs.symlinkSync(path.join(root, 'package.json'), browser);
  assert.throws(() => manageVersion(['patch'], root), /symlinked/);
  for (const argv of [['set'], ['show', 'extra'], ['patch', 'extra'], ['set', '0.2.0', '--apply']]) assert.throws(() => manageVersion(argv, root), /Unexpected/);
});
