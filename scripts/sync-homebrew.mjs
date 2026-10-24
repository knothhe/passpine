import { createHash, randomUUID } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { validateVersion } from './version.mjs';

const repository = 'knothhe/passpine';
const projectRoot = fileURLToPath(new URL('../', import.meta.url));
const caskPath = 'Casks/passpine.rb';
const destinationPath = path.join(projectRoot, caskPath);

function tagVersion(tag) {
  if (typeof tag !== 'string' || !tag.startsWith('v')) throw new Error('Expected a stable vMAJOR.MINOR.PATCH release tag.');
  return validateVersion(tag.slice(1));
}

export function homebrewRelease(release, expectedTag) {
  if (!release || release.draft !== false || release.prerelease !== false) {
    throw new Error('Homebrew requires a published stable release.');
  }
  const version = tagVersion(release.tag_name);
  if (expectedTag !== undefined && release.tag_name !== expectedTag) throw new Error('Published release differs from the requested tag.');
  const name = `Passpine-${version}-macos-arm64.zip`;
  const matches = release.assets?.filter(asset => asset.name === name) ?? [];
  const asset = matches[0];
  const url = `https://github.com/${repository}/releases/download/v${version}/${name}`;
  if (matches.length !== 1 || asset.state !== 'uploaded' || asset.browser_download_url !== url
    || !Number.isSafeInteger(asset.size) || asset.size <= 0 || !/^sha256:[a-f0-9]{64}$/.test(asset.digest)) {
    throw new Error('Release must contain one uploaded macOS arm64 package with a SHA-256 digest.');
  }
  return { version, url, size: asset.size, sha256: asset.digest.slice(7) };
}

export function homebrewCask({ version, sha256 }) {
  validateVersion(version);
  if (!/^[a-f0-9]{64}$/.test(sha256)) throw new Error('Invalid cask SHA-256.');
  return `cask "passpine" do
  version "${version}"
  sha256 "${sha256}"

  url "https://github.com/${repository}/releases/download/v#{version}/Passpine-#{version}-macos-arm64.zip"
  name "Passpine"
  desc "Local password manager compatible with pass"
  homepage "https://github.com/${repository}"

  livecheck do
    url :url
    strategy :github_latest
  end

  auto_updates true
  depends_on arch: :arm64
  depends_on macos: :ventura

  app "Passpine.app"
  binary "#{appdir}/Passpine.app/Contents/MacOS/passpine"

  uninstall quit: "com.knothhe.passpine"
end
`;
}

export async function syncHomebrew({
  destination = destinationPath,
  tag,
  loadRelease = requestedTag => JSON.parse(execFileSync('gh', ['api', `repos/${repository}/releases/${requestedTag ? `tags/${requestedTag}` : 'latest'}`], {
    encoding: 'utf8', timeout: 30_000,
  })),
  fetchImpl = fetch,
} = {}) {
  if (tag !== undefined) tagVersion(tag);
  const release = homebrewRelease(await loadRelease(tag), tag);
  let previous;
  try { previous = await fs.readFile(destination, 'utf8'); }
  catch (error) { if (error.code !== 'ENOENT') throw error; }
  if (previous !== undefined) {
    const current = validateVersion(/^  version "([^"]+)"$/m.exec(previous)?.[1]);
    const parts = release.version.split('.').map(Number);
    const difference = current.split('.').map((part, i) => Number(part) - parts[i]).find(value => value !== 0) ?? 0;
    // Release jobs can finish out of order; retrying an old tag must not roll back the tap.
    if (difference > 0) return current;
    if (difference === 0 && /^  sha256 "([^"]+)"$/m.exec(previous)?.[1] !== release.sha256) {
      throw new Error('Published checksum changed for the existing cask version.');
    }
  }
  const download = await fetchImpl(release.url, { signal: AbortSignal.timeout(30_000) });
  if (!download.ok) throw new Error(`Package download failed: HTTP ${download.status}.`);
  const bytes = Buffer.from(await download.arrayBuffer());
  if (bytes.length !== release.size || createHash('sha256').update(bytes).digest('hex') !== release.sha256) {
    throw new Error('Downloaded macOS package does not match the published size and SHA-256.');
  }
  const source = homebrewCask(release);
  if (source === previous) return release.version;
  await fs.mkdir(path.dirname(destination), { recursive: true });
  const temporary = `${destination}.${randomUUID()}.tmp`;
  try {
    await fs.writeFile(temporary, source, { flag: 'wx' });
    await fs.rename(temporary, destination);
  } finally {
    await fs.rm(temporary, { force: true });
  }
  return release.version;
}

export async function publishHomebrew({ tag, root = projectRoot, gitEnv = process.env, ...options } = {}) {
  tagVersion(tag);
  const git = (...args) => execFileSync('git', args, {
    cwd: root, env: gitEnv, encoding: 'utf8', timeout: 30_000, stdio: ['ignore', 'pipe', 'pipe'],
  }).trim();
  if (await fs.realpath(root) !== await fs.realpath(git('rev-parse', '--show-toplevel'))) {
    throw new Error('Homebrew publication requires the tap checkout root.');
  }
  if (git('branch', '--show-current') !== 'main' || git('status', '--porcelain', '--untracked-files=all')) {
    throw new Error('Homebrew publication requires a clean main checkout.');
  }
  git('fetch', '--no-tags', 'origin', 'refs/heads/main');
  git('merge', '--ff-only', 'FETCH_HEAD');
  if (git('rev-parse', 'HEAD') !== git('rev-parse', 'FETCH_HEAD')) {
    throw new Error('Refusing to push unrelated local commits to main.');
  }
  const version = await syncHomebrew({ ...options, tag, destination: path.join(root, caskPath) });
  git('add', '--', caskPath);
  const staged = git('diff', '--cached', '--name-only');
  if (!staged) return { version, changed: false };
  if (staged !== caskPath) throw new Error('Homebrew publication may commit only the cask.');
  git('-c', 'user.name=github-actions[bot]', '-c', 'user.email=41898282+github-actions[bot]@users.noreply.github.com',
    '-c', 'commit.gpgsign=false', 'commit', '-m', `chore(homebrew): update Passpine to ${version}`, '--', caskPath);
  // A concurrent main push is rejected normally; a fresh job retry starts from the new main.
  git('push', 'origin', 'HEAD:refs/heads/main');
  return { version, changed: true, commit: git('rev-parse', 'HEAD') };
}

if (process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url) {
  try {
    const args = process.argv.slice(2);
    if (args[0] === '--publish' && (args.length === 2 || (args.length === 4 && args[2] === '--tap-root'))) {
      if (process.env.GITHUB_ACTIONS !== 'true' || process.env.GITHUB_REPOSITORY !== repository) {
        throw new Error('Automatic cask publication is reserved for this repository\'s GitHub Actions.');
      }
      console.log(JSON.stringify(await publishHomebrew({ tag: args[1], ...(args[3] && { root: path.resolve(args[3]) }) })));
    } else if (args.length) {
      throw new Error('Usage: sync-homebrew.mjs [--publish vMAJOR.MINOR.PATCH [--tap-root CHECKOUT]]');
    } else {
      console.log(`Casks/passpine.rb is synchronized at ${await syncHomebrew()}.`);
    }
  } catch (error) {
    console.error(`Homebrew synchronization failed: ${error.message}`);
    process.exitCode = 1;
  }
}
