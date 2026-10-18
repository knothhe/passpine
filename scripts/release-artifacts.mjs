import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { validateVersion } from './version.mjs';

export const artifactBuilds = [['make', 'client:build'], ['pnpm', 'run', 'package:browser']];

export function artifactPlan(version, platform = process.platform, architecture = os.machine()) {
  validateVersion(version);
  if (platform !== 'darwin' || !['arm64', 'x86_64'].includes(architecture)) {
    throw new Error('Client release packages currently require macOS arm64 or x86_64. Build on the target Mac architecture.');
  }
  return [
    { name: `Passpine-${version}-macos-${architecture}.zip`, source: `dist/Passpine-${version}-macos-${architecture}.zip`, entry: 'Passpine.app/Contents/Info.plist' },
    ...['chrome', 'firefox'].map(browser => ({ name: `passpine-${version}-${browser}.zip`, source: `browser/.output/passpine-${version}-${browser}.zip`, entry: 'manifest.json' })),
  ];
}

function fingerprint(filename) {
  const stat = fs.lstatSync(filename);
  if (!stat.isFile() || stat.size === 0) throw new Error(`Missing, empty or symlinked release artifact: ${filename}`);
  return { size: stat.size, sha256: crypto.createHash('sha256').update(fs.readFileSync(filename)).digest('hex') };
}

function directory(version, root) { return path.join(root, 'dist', 'releases', version); }

export function buildArtifacts(version, plan, runBuild, root = process.cwd()) {
  // Remove only the exact expected archives so a successful no-op cannot reuse an old build.
  for (const artifact of plan) fs.rmSync(path.join(root, artifact.source), { force: true });
  for (const command of artifactBuilds) runBuild(command);
  for (const artifact of plan) {
    const source = path.join(root, artifact.source);
    fingerprint(source);
    execFileSync('unzip', ['-tq', source], { encoding: 'utf8' });
    const contents = execFileSync('unzip', ['-p', source, artifact.entry], { encoding: 'utf8' });
    const versions = artifact.entry.endsWith('.json')
      ? [JSON.parse(contents).version]
      : ['CFBundleShortVersionString', 'CFBundleVersion'].map(key => new RegExp(`<key>${key}</key>\\s*<string>([^<]+)</string>`).exec(contents)?.[1]);
    if (versions.some(value => value !== version)) throw new Error(`Archive version differs from ${version}: ${artifact.name}`);
  }
  fs.mkdirSync(directory(version, root), { recursive: true });
  return plan.map(artifact => {
    const filename = path.join(directory(version, root), artifact.name);
    fs.copyFileSync(path.join(root, artifact.source), filename);
    return { name: artifact.name, path: filename, ...fingerprint(filename) };
  });
}

export function saveArtifacts(version, head, artifacts, root = process.cwd()) {
  const filename = path.join(directory(version, root), 'manifest.json');
  const manifest = { schema: 1, version, head, artifacts: artifacts.map(({ name, size, sha256 }) => ({ name, size, sha256 })) };
  fs.writeFileSync(`${filename}.tmp`, `${JSON.stringify(manifest, null, 2)}\n`);
  fs.renameSync(`${filename}.tmp`, filename);
}

export function cachedArtifacts(version, head, plan, root = process.cwd()) {
  const filename = path.join(directory(version, root), 'manifest.json');
  if (!fs.existsSync(filename)) return null;
  const manifest = JSON.parse(fs.readFileSync(filename, 'utf8'));
  if (manifest.schema !== 1 || manifest.version !== version || manifest.head !== head
    || !Array.isArray(manifest.artifacts) || manifest.artifacts.length !== plan.length) {
    throw new Error(`Artifact cache does not match this release HEAD: ${filename}. Inspect it before retrying.`);
  }
  return plan.map(artifact => {
    const saved = manifest.artifacts.find(item => item.name === artifact.name);
    const file = path.join(directory(version, root), artifact.name);
    const actual = fingerprint(file);
    if (!saved || saved.size !== actual.size || saved.sha256 !== actual.sha256) {
      throw new Error(`Artifact cache checksum mismatch: ${file}. Inspect it before retrying.`);
    }
    return { name: artifact.name, path: file, ...actual };
  });
}
