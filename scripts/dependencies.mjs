#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const projectRoot = fileURLToPath(new URL('../', import.meta.url));

export function resolveDependencies(root, catalog) {
  const go = new Map([...fs.readFileSync(path.join(root, 'client/go.mod'), 'utf8').matchAll(/^\s*(?:require\s+)?([\w.!/-]+)\s+(v\S+)([^\n]*)$/gm)]
    .filter(([, , , suffix]) => !suffix.includes('// indirect')).map(([, name, version]) => [name, version]));
  const expected = new Map([...go.keys()].map(name => [name, 'desktop']));
  if (catalog.length !== expected.size || new Set(catalog.map(dep => dep.module)).size !== catalog.length || new Set(catalog.map(dep => dep.id)).size !== catalog.length) {
    throw new Error('Dependency catalog must cover every direct runtime dependency exactly once.');
  }
  return catalog.map(dep => {
    if (expected.get(dep.module) !== dep.group) throw new Error(`Missing or incorrect dependency group: ${dep.module}`);
    const version = go.get(dep.module);
    if (!version) throw new Error(`Missing locked dependency version: ${dep.module}`);
    for (const link of [dep.url, dep.source]) if (new URL(link).protocol !== 'https:') throw new Error(`Invalid dependency link: ${link}`);
    return { ...dep, version };
  });
}

export function generateDependencies(root = projectRoot, check = false) {
  const file = path.join(root, 'client/about/dependencies.json');
  const source = fs.readFileSync(file, 'utf8');
  const output = JSON.stringify(resolveDependencies(root, JSON.parse(source)), null, 2) + '\n';
  if (check && output !== source) throw new Error('Dependency information is stale. Run node scripts/dependencies.mjs.');
  if (!check && output !== source) fs.writeFileSync(file, output);
  return JSON.parse(output);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.slice(2).some(arg => arg !== '--check')) throw new Error('Usage: node scripts/dependencies.mjs [--check]');
    console.log(`Verified ${generateDependencies(projectRoot, process.argv.includes('--check')).length} direct dependencies.`);
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
