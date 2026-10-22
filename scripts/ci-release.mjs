#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';
import { versionFiles } from './version.mjs';
import { tagVersion, notesForTag } from './release-notes.mjs';
import { publishRelease } from './release.mjs';

const targets = ['client-darwin-arm64', 'client-darwin-x86_64', 'client-linux-amd64', 'client-win32-amd64', 'browser'];
const command = (name, args, root) => execFileSync(name, args, { cwd: root, encoding: 'utf8' }).trim();

export function verifyTag(tag, root = process.cwd()) {
  const version = tagVersion(tag, root);
  if (versionFiles(root)[0] !== version) throw new Error(`Tag ${tag} differs from the package versions.`);
  const head = command('git', ['rev-parse', 'HEAD'], root);
  if (command('git', ['rev-parse', '--verify', `refs/tags/${tag}^{commit}`], root) !== head) {
    throw new Error('Checkout must point to the release tag commit.');
  }
  return { version, tag, head };
}

export function packagesFor(version, target) {
  const names = {
    'client-darwin-arm64': [`Passpine-${version}-macos-arm64.zip`],
    'client-darwin-x86_64': [`Passpine-${version}-macos-x86_64.zip`],
    'client-linux-amd64': [`Passpine-${version}-linux-amd64.tar.gz`],
    'client-win32-amd64': [`Passpine-${version}-windows-amd64-setup.exe`],
    browser: ['chrome', 'firefox'].map(browser => `passpine-${version}-${browser}.zip`),
  };
  if (!(target in names)) throw new Error(`Unsupported release target: ${target}`);
  return names[target];
}

function fingerprint(filename) {
  const stat = fs.lstatSync(filename);
  if (!stat.isFile() || stat.size === 0) throw new Error(`Missing, empty or symlinked package: ${filename}`);
  return { size: stat.size, sha256: crypto.createHash('sha256').update(fs.readFileSync(filename)).digest('hex') };
}

export function validatePackage(filename, version) {
  fingerprint(filename);
  if (filename.endsWith('.exe')) {
    if (fs.readFileSync(filename).subarray(0, 2).toString() !== 'MZ') throw new Error('Invalid Windows installer.');
  } else if (filename.endsWith('.tar.gz')) {
    const entries = command('tar', ['-tzf', filename]).split('\n');
    for (const entry of ['passpine', 'passpine-host', 'install.sh', 'app-icon-monochrome.png', 'version.txt']) {
      if (!entries.includes(entry)) throw new Error(`Linux package is missing ${entry}.`);
    }
    if (command('tar', ['-xOzf', filename, 'version.txt']) !== version) throw new Error('Linux package version mismatch.');
  } else {
    // The Windows job stages only its EXE. ZIP validation runs on macOS/Linux.
    command('unzip', ['-tq', filename]);
    const browser = path.basename(filename).startsWith('passpine-');
    const entry = browser ? 'manifest.json' : 'Passpine.app/Contents/Info.plist';
    const contents = command('unzip', ['-p', filename, entry]);
    const versions = browser ? [JSON.parse(contents).version]
      : ['CFBundleShortVersionString', 'CFBundleVersion'].map(key => new RegExp(`<key>${key}</key>\\s*<string>([^<]+)</string>`).exec(contents)?.[1]);
    if (versions.some(value => value !== version)) throw new Error(`Archive version mismatch: ${filename}`);
  }
}

export function stage(tag, target, root = process.cwd()) {
  const metadata = verifyTag(tag, root);
  const destination = path.join(root, 'dist', 'releases', metadata.version);
  const names = packagesFor(metadata.version, target);
  const sourceDir = path.join(root, target === 'browser' ? 'browser/.output' : 'dist');
  // Check the native binaries too, before recording the installer's provenance.
  if (target.startsWith('client-')) {
    const suffix = target.includes('win32') ? '.exe' : '';
    for (const [binary, title] of [['passpine', 'Passpine'], ['passpine-host', 'Passpine Host']]) {
      if (command(path.join(root, 'dist', `${binary}${suffix}`), ['--version'], root) !== `${title} ${metadata.version}`) {
        throw new Error(`Built binary version mismatch: ${binary}`);
      }
    }
  }
  for (const name of names) validatePackage(path.join(sourceDir, name), metadata.version);
  fs.mkdirSync(destination, { recursive: true });
  const artifacts = names.map(name => {
    const filename = path.join(destination, name);
    fs.copyFileSync(path.join(sourceDir, name), filename);
    return { name, ...fingerprint(filename) };
  });
  fs.writeFileSync(path.join(destination, `manifest-${target}.json`), `${JSON.stringify({ schema: 2, ...metadata, target, artifacts }, null, 2)}\n`);
  return destination;
}

function manifestsIn(directory) {
  const files = [];
  for (const item of fs.readdirSync(directory, { withFileTypes: true })) {
    const filename = path.join(directory, item.name);
    if (item.isSymbolicLink()) throw new Error(`Symlink in release artifacts: ${filename}`);
    if (item.isDirectory()) files.push(...manifestsIn(filename));
    else if (/^manifest-.*\.json$/.test(item.name)) files.push(filename);
  }
  return files;
}

export function collectPackages(tag, directory, root = process.cwd()) {
  const { version, head } = verifyTag(tag, root);
  const seen = new Set();
  const artifacts = [];
  for (const filename of manifestsIn(directory)) {
    const data = JSON.parse(fs.readFileSync(filename, 'utf8'));
    if (data.schema !== 2 || data.tag !== tag || data.version !== version || data.head !== head
      || !targets.includes(data.target) || seen.has(data.target)) throw new Error(`Invalid or duplicate release manifest: ${filename}`);
    seen.add(data.target);
    const expected = packagesFor(version, data.target);
    if (!Array.isArray(data.artifacts) || data.artifacts.length !== expected.length
      || data.artifacts.some((item, i) => item.name !== expected[i])) throw new Error(`Unexpected release package list: ${filename}`);
    for (const artifact of data.artifacts) {
      const file = path.join(path.dirname(filename), artifact.name);
      const actual = fingerprint(file);
      if (actual.size !== artifact.size || actual.sha256 !== artifact.sha256) throw new Error(`Package checksum mismatch: ${file}`);
      validatePackage(file, version);
      artifacts.push({ name: artifact.name, path: file, ...actual });
    }
  }
  if (seen.size !== targets.length) throw new Error(`Release packages are incomplete. Missing: ${targets.filter(target => !seen.has(target)).join(', ')}`);
  return artifacts;
}

function main() {
  const [action, tag, directory, ...rest] = process.argv.slice(2);
  if (rest.length || !tag || (action === 'publish' ? !directory : directory !== undefined)) throw new Error('Usage: ci-release.mjs verify|stage-client|stage-browser TAG, or publish TAG DIRECTORY');
  const metadata = verifyTag(tag);
  if (action === 'verify') console.log(JSON.stringify(metadata));
  else if (action === 'stage-client') {
    const arch = process.arch === 'x64' ? (process.platform === 'darwin' ? 'x86_64' : 'amd64') : process.arch;
    console.log(stage(tag, `client-${process.platform}-${arch}`));
  } else if (action === 'stage-browser') console.log(stage(tag, 'browser'));
  else if (action === 'publish') {
    const artifacts = collectPackages(tag, path.resolve(directory));
    const repo = process.env.GITHUB_REPOSITORY;
    const notes = notesForTag(repo, tag);
    console.log(publishRelease({ repo, tag, notes, artifacts }));
  } else throw new Error(`Unknown CI release action: ${action}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { main(); }
  catch (error) { console.error(`ERROR: ${error.message}`); process.exitCode = 1; }
}
