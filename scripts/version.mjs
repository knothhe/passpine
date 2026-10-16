#!/usr/bin/env node
// Passpine shares one stable version across the desktop and browser packages.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const projectRoot = fileURLToPath(new URL('../', import.meta.url));
const filenames = ['package.json', 'browser/package.json'];

export function validateVersion(version) {
  if (typeof version !== 'string' || !/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(version)
    || version.split('.').some(part => BigInt(part) > 65535n) || version === '0.0.0') {
    throw new Error('Version must be stable MAJOR.MINOR.PATCH, with components 0–65535 and at least one nonzero component (Chrome/Edge manifest compatibility).');
  }
  return version;
}

export function versionFiles(root = projectRoot) {
  const files = new Map();
  for (const filename of filenames) {
    const fullPath = path.join(root, filename);
    if (!fs.lstatSync(fullPath).isFile()) throw new Error(`Missing or symlinked version file: ${filename}`);
    const data = JSON.parse(fs.readFileSync(fullPath, 'utf8'));
    validateVersion(data.version);
    files.set(filename, data);
  }
  const current = files.get('package.json').version;
  if (files.get('browser/package.json').version !== current) throw new Error('browser/package.json version differs from package.json.');
  return [current, files];
}

export function nextVersion(current, bump) {
  validateVersion(current);
  const parts = current.split('.').map(Number);
  const index = ['major', 'minor', 'patch'].indexOf(bump);
  if (index !== -1) {
    parts[index]++;
    parts.fill(0, index + 1);
    bump = parts.join('.');
  }
  validateVersion(bump);
  const targetParts = bump.split('.').map(Number);
  const firstChanged = partsFor(current).findIndex((part, i) => part !== targetParts[i]);
  if (firstChanged === -1 || targetParts[firstChanged] < partsFor(current)[firstChanged]) throw new Error('New version must be greater than the current version.');
  return bump;
}

function partsFor(version) { return version.split('.').map(Number); }

export function writeVersions(files, target, root = projectRoot) {
  validateVersion(target);
  // Validate all inputs before writing either package; never re-resolve dependencies.
  const updates = [...files].map(([filename, data]) => {
    const fullPath = path.join(root, filename);
    if (!fs.lstatSync(fullPath).isFile()) throw new Error(`Missing or symlinked version file: ${filename}`);
    const source = fs.readFileSync(fullPath, 'utf8');
    const indent = /\n([ \t]+)"/.exec(source)?.[1] ?? 2;
    return [fullPath, `${JSON.stringify({ ...data, version: target }, null, indent)}\n`];
  });
  for (const [filename, content] of updates) fs.writeFileSync(filename, content);
}

export function manageVersion(argv, root = projectRoot) {
  const [command = 'show', value, ...rest] = argv;
  if (command === '--help' || command === '-h') {
    return 'Usage: node scripts/version.mjs [show|check|patch|minor|major|set <MAJOR.MINOR.PATCH>]\nVersion changes only update package.json and browser/package.json; no commit, tag, push, or dependency resolution.';
  }
  if (rest.length || (command !== 'set' && value !== undefined) || (command === 'set' && value === undefined)) throw new Error('Unexpected arguments. Use --help for usage.');
  const [current, files] = versionFiles(root);
  if (command === 'show') return current;
  if (command === 'check') return `Version ${current} is synchronized across desktop and browser build sources.`;
  if (!['patch', 'minor', 'major', 'set'].includes(command)) throw new Error(`Unknown version command: ${command}`);
  const target = nextVersion(current, command === 'set' ? value : command);
  writeVersions(files, target, root);
  return `${current} → ${target}`;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { console.log(manageVersion(process.argv.slice(2))); }
  catch (error) { console.error(`ERROR: ${error.message}`); process.exitCode = 1; }
}
