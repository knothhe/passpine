#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync, spawnSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';
import { versionFiles } from './version.mjs';
import { tagVersion, notesForTag } from './release-notes.mjs';
import { publishRelease } from './release.mjs';
import { createUpdateFeed, publicKey, readUpdateSigningConfig, stageUpdate, updateBuildConfig, validateUpdate } from './update-artifacts.mjs';
import { assertReleaseBuild } from './build-channel.mjs';

const targets = ['client-darwin-arm64', 'client-linux-amd64', 'client-win32-amd64', 'browser'];
const command = (name, args, root) => execFileSync(name, args, { cwd: root, encoding: 'utf8' }).trim();
const browserIdentity = JSON.parse(fs.readFileSync(new URL('../config/browser-identities.json', import.meta.url), 'utf8')).release;

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
    if (browser) {
      const manifest = JSON.parse(contents);
      const firefox = path.basename(filename).endsWith('-firefox.zip');
      if (manifest.key !== browserIdentity.key || (firefox && manifest.browser_specific_settings?.gecko?.id !== browserIdentity.firefoxID)
        || /dev/i.test(manifest.version_name || '')) throw new Error(`Browser archive must use the release identity: ${filename}`);
    } else if (!/<key>CFBundleIdentifier<\/key>\s*<string>com\.knothhe\.passpine<\/string>/.test(contents)) {
      throw new Error(`macOS archive must use the release identity: ${filename}`);
    }
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
      if (command(path.join(root, 'dist', `${binary}${suffix}`), ['--build-channel'], root) !== 'release') {
        throw new Error(`Only release-channel binaries can be staged: ${binary}`);
      }
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
  const update = target.startsWith('client-') ? stageUpdate(metadata.version, target, root, destination) : undefined;
  fs.writeFileSync(path.join(destination, `manifest-${target}.json`), `${JSON.stringify({ schema: 2, channel: 'release', ...metadata, target, artifacts, update }, null, 2)}\n`);
  return destination;
}

function manifestsIn(directory) {
  const files = [];
  for (const item of fs.readdirSync(directory, { withFileTypes: true }).sort((a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : 0)) {
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
    if (data.schema !== 2 || data.channel !== 'release' || data.tag !== tag || data.version !== version || data.head !== head
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

export function collectUpdates(tag, directory, root = process.cwd()) {
  const { version, head } = verifyTag(tag, root), updates = [], seen = new Set();
  for (const filename of manifestsIn(directory)) {
    const manifest = JSON.parse(fs.readFileSync(filename, 'utf8'));
    if (!manifest.target?.startsWith('client-')) continue;
    if (manifest.schema !== 2 || manifest.channel !== 'release' || manifest.tag !== tag || manifest.version !== version || manifest.head !== head
      || !targets.includes(manifest.target) || seen.has(manifest.target)) throw new Error(`Invalid update provenance: ${filename}`);
    seen.add(manifest.target);
    const { update } = manifest;
    if (!update?.full || update.full.name !== `Passpine-${version}-update-${update.os}-${update.arch}.zip`) throw new Error(`Missing or mismatched update bundle: ${filename}`);
    validateUpdate(update, manifest.target, path.dirname(filename));
    updates.push({ target: manifest.target, update, directory: path.dirname(filename) });
  }
  if (seen.size !== 3) throw new Error('Update bundles are incomplete.');
  return updates;
}

// Only a genuine missing release/asset means that no incremental baseline is
// available. Authentication and network errors stop publication.
export function previousUpdateFeed(repo, version, invoke = spawnSync) {
  const call = args => {
    const result = invoke('gh', args, { encoding: 'utf8', maxBuffer: 8 * 1024 * 1024 });
    if (result.error) throw result.error;
    return result;
  };
  const result = call(['api', '--hostname', 'github.com', '--paginate', '--slurp', `repos/${repo}/releases?per_page=100`]);
  if (result.status !== 0) {
    if (/HTTP 404/.test(result.stderr || '')) return null;
    throw new Error(`Cannot read the previous release: ${result.stderr}`);
  }
  // Select the same older baseline on retries after the current release has
  // become public. Using /latest would choose our own feed and break resume.
  const compare = (a, b) => {
    const left = a.split('.').map(Number), right = b.split('.').map(Number);
    for (let i = 0; i < 3; i++) if (left[i] !== right[i]) return left[i] - right[i];
    return 0;
  };
  const releases = JSON.parse(result.stdout).flat().filter(item => !item.draft && !item.prerelease
    && /^v\d+\.\d+\.\d+$/.test(item.tag_name) && compare(item.tag_name.slice(1), version) < 0);
  releases.sort((a, b) => compare(b.tag_name.slice(1), a.tag_name.slice(1)));
  const asset = releases[0]?.assets?.find(item => item.name === 'update-feed.json');
  if (!asset) return null;
  if (!Number.isSafeInteger(asset.id) || asset.size > 8 * 1024 * 1024) throw new Error('Invalid previous update feed metadata.');
  const download = call(['api', '--hostname', 'github.com', '-H', 'Accept: application/octet-stream', `repos/${repo}/releases/assets/${asset.id}`]);
  if (download.status !== 0) throw new Error(`Cannot download the previous update feed: ${download.stderr}`);
  return Buffer.from(download.stdout);
}

export function updateNotes(version, generatedNotes, root = process.cwd()) {
  const filename = path.join(root, 'docs', 'releases', `${version}.json`);
  if (fs.existsSync(filename)) {
    const notes = JSON.parse(fs.readFileSync(filename, 'utf8'));
    if (typeof notes.en !== 'string' || !notes.en.trim() || typeof notes['zh-CN'] !== 'string' || !notes['zh-CN'].trim()) throw new Error('Release notes require English and Simplified Chinese.');
    return notes;
  }
  return { en: `Release changes\n\n${generatedNotes}`, 'zh-CN': `版本变更（提交标题保留原文）\n\n${generatedNotes}` };
}

// Publication uses the trust anchor checked into the same release sources as
// CI builds. Development overrides cannot redirect or rotate a release.
// Return only public configuration; never log or persist the private input.
export function updatePublicationConfig(env = process.env, signingConfigPath) {
  const pinned = readUpdateSigningConfig(signingConfigPath);
  const config = updateBuildConfig(env, signingConfigPath);
  if (env.GITHUB_REPOSITORY !== pinned.repository || config.key !== pinned.publicKey || config.feed !== pinned.feedURL) {
    throw new Error('Update publication must match the checked-in repository, public key and feed URL.');
  }
  if (!env.PASSPINE_UPDATE_PRIVATE_KEY) throw new Error('Signed update publication requires the PASSPINE_UPDATE_PRIVATE_KEY repository secret.');
  let signingKey;
  try { signingKey = crypto.createPrivateKey(env.PASSPINE_UPDATE_PRIVATE_KEY); }
  catch { throw new Error('PASSPINE_UPDATE_PRIVATE_KEY must be an Ed25519 PKCS#8 PEM private key.'); }
  if (signingKey.asymmetricKeyType !== 'ed25519' || !crypto.createPublicKey(signingKey).export({ type: 'spki', format: 'der' }).equals(publicKey(config.key).export({ type: 'spki', format: 'der' }))) {
    throw new Error('Release signing private key does not match the checked-in public key.');
  }
  return config;
}

function main() {
  const [action, tag, directory, ...rest] = process.argv.slice(2);
  if (rest.length || !tag || (action === 'publish' ? !directory : directory !== undefined)) throw new Error('Usage: ci-release.mjs verify|stage-client|stage-browser TAG, or publish TAG DIRECTORY');
  const metadata = verifyTag(tag);
  if (action !== 'verify') {
    const context = assertReleaseBuild();
    if (context.tag !== tag) throw new Error('Release command tag must match RELEASE_TAG.');
  }
  if (action === 'verify') console.log(JSON.stringify(metadata));
  else if (action === 'stage-client') {
    const arch = process.arch === 'x64' ? 'amd64' : process.arch;
    console.log(stage(tag, `client-${process.platform}-${arch}`));
  } else if (action === 'stage-browser') console.log(stage(tag, 'browser'));
  else if (action === 'publish') {
    const config = updatePublicationConfig();
    const artifacts = collectPackages(tag, path.resolve(directory));
    const repo = process.env.GITHUB_REPOSITORY;
    const notes = notesForTag(repo, tag);
    artifacts.push(...createUpdateFeed({
      version: metadata.version, repo, notes: updateNotes(metadata.version, notes),
      updates: collectUpdates(tag, path.resolve(directory)), output: path.join('dist', 'updates', metadata.version),
      publicKey: config.key, privateKey: process.env.PASSPINE_UPDATE_PRIVATE_KEY, previousFeed: previousUpdateFeed(repo, metadata.version),
    }));
    console.log(publishRelease({ repo, tag, notes, artifacts }));
  } else throw new Error(`Unknown CI release action: ${action}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { main(); }
  catch (error) { console.error(`ERROR: ${error.message}`); process.exitCode = 1; }
}
