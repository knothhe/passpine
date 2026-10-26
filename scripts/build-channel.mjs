#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { versionFiles } from './version.mjs';
import { tagVersion } from './release-notes.mjs';
import { readUpdateSigningConfig, updateBuildConfig } from './update-artifacts.mjs';

const projectRoot = fileURLToPath(new URL('../', import.meta.url));
const moduleName = 'github.com/knothhe/passpine/client/buildinfo';
const git = (root, ...args) => execFileSync('git', args, { cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();

export function buildChannel(env = process.env) {
  const channel = env.PASSPINE_BUILD_CHANNEL || 'dev';
  if (!['dev', 'release'].includes(channel)) throw new Error('PASSPINE_BUILD_CHANNEL must be dev or release.');
  return channel;
}

export function assertReleaseBuild(env = process.env, root = projectRoot) {
  if (buildChannel(env) !== 'release' || env.GITHUB_ACTIONS !== 'true' || env.GITHUB_REPOSITORY !== 'knothhe/passpine') {
    throw new Error('Release builds are available only in the knothhe/passpine GitHub Actions release workflow. Local build and install commands produce development builds.');
  }
  const tag = env.RELEASE_TAG;
  if (!tag) throw new Error('A verified RELEASE_TAG is required for release builds.');
  const version = tagVersion(tag, root);
  if (versionFiles(root)[0] !== version) throw new Error('Release tag differs from the synchronized package versions.');
  const head = git(root, 'rev-parse', 'HEAD');
  if (git(root, 'rev-parse', '--verify', `refs/tags/${tag}^{commit}`) !== head) throw new Error('Release checkout must point to the release tag commit.');
  if (git(root, 'status', '--porcelain', '--untracked-files=no') !== '') throw new Error('Release builds require an unchanged tracked source tree.');
  return { version, tag, head };
}

export function clientBuildConfig(env = process.env, root = projectRoot) {
  const channel = buildChannel(env);
  const baseVersion = versionFiles(root)[0];
  let version, updaterFlags = '';
  if (channel === 'release') {
    version = assertReleaseBuild(env, root).version;
    const signingConfigPath = path.join(root, 'config/update-signing.json');
    const pinned = readUpdateSigningConfig(signingConfigPath);
    const update = updateBuildConfig(env, signingConfigPath);
    if (update.key !== pinned.publicKey || update.feed !== pinned.feedURL) throw new Error('Release builds must use the repository-pinned update public key and feed.');
    updaterFlags = ` -X main.updateFeedURL=${update.feed} -X main.updatePublicKey=${update.key}`;
  } else {
    let revision = 'source';
    try { revision = git(root, 'rev-parse', '--short=12', 'HEAD'); } catch { /* Source archives are valid development inputs. */ }
    if (!/^[a-z0-9]+$/.test(revision)) throw new Error('Invalid development revision.');
    version = `${baseVersion}-dev.${revision}`;
  }
  const development = channel === 'dev';
  let commit = 'unknown', modified = 'unknown';
  try {
    commit = git(root, 'rev-parse', 'HEAD');
    modified = String(git(root, 'status', '--porcelain') !== '');
  } catch { /* Source archives have no verifiable VCS identity. */ }
  const executor = env.GITHUB_ACTIONS === 'true' ? 'github-actions' : 'local';
  const metadataFlags = ` -X ${moduleName}.Commit=${commit} -X ${moduleName}.Modified=${modified} -X ${moduleName}.Executor=${executor}`;
  return {
    schema: 1, channel, version, baseVersion, commit, modified, executor,
    outputDirectory: path.join(root, development ? 'dist/dev' : 'dist'),
    appName: development ? 'Passpine Dev' : 'Passpine',
    bundleID: development ? 'com.knothhe.passpine.dev' : 'com.knothhe.passpine',
    packageName: development ? 'Passpine-Dev' : 'Passpine',
    linuxBinary: development ? 'passpine-dev' : 'passpine',
    linuxHost: development ? 'passpine-dev-host' : 'passpine-host',
    ldflags: `-X main.version=${version} -X ${moduleName}.Channel=${channel}${metadataFlags}${updaterFlags}`,
    hostLdflags: `-X main.version=${version} -X ${moduleName}.Channel=${channel}${metadataFlags}`,
  };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const [command = 'json', ...rest] = process.argv.slice(2);
    if (rest.length) throw new Error('Unexpected build-channel arguments.');
    const config = clientBuildConfig();
    if (command === 'json') console.log(JSON.stringify(config));
    else if (command === 'shell') console.log(['channel', 'version', 'baseVersion', 'outputDirectory', 'appName', 'bundleID', 'packageName', 'ldflags', 'hostLdflags'].map(key => config[key]).join('\t'));
    else if (command === 'write') {
      fs.mkdirSync(config.outputDirectory, { recursive: true });
      fs.writeFileSync(path.join(config.outputDirectory, 'build-channel.json'), `${JSON.stringify(config, null, 2)}\n`);
    } else throw new Error('Usage: node scripts/build-channel.mjs [json|shell|write]');
  } catch (error) { console.error(`ERROR: ${error.message}`); process.exitCode = 1; }
}
