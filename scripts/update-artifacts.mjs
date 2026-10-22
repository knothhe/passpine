#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import zlib from 'node:zlib';
import { pathToFileURL } from 'node:url';
import { validateVersion } from './version.mjs';

export const defaultUpdateFeedURL = 'https://github.com/knothhe/passpine/releases/latest/download/update-feed.json';
const defaultSigningConfigURL = new URL('../config/update-signing.json', import.meta.url);
const officialUpdateRepository = 'knothhe/passpine';
const targetPlatforms = { 'client-darwin-arm64': ['darwin', 'arm64'], 'client-linux-amd64': ['linux', 'amd64'], 'client-win32-amd64': ['windows', 'amd64'] };
const digest = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const requireThat = (condition, message) => { if (!condition) throw new Error(message); };
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);

export function publicKey(value) {
  requireThat(typeof value === 'string' && /^[A-Za-z0-9+/]{43}=$/.test(value), 'PASSPINE_UPDATE_PUBLIC_KEY must be a base64-encoded raw Ed25519 public key (32 bytes).');
  const raw = Buffer.from(value, 'base64');
  requireThat(raw.length === 32 && raw.toString('base64') === value, 'Invalid Ed25519 public key.');
  return crypto.createPublicKey({ key: Buffer.concat([Buffer.from('302a300506032b6570032100', 'hex'), raw]), format: 'der', type: 'spki' });
}

function validateFeedURL(feed) {
  const url = new URL(feed);
  requireThat(url.protocol === 'https:' && !url.username && !url.password && !/[\s'"\\]/.test(feed), 'Update feed must be a credential-free HTTPS URL without spaces or quotes.');
}

export function readUpdateSigningConfig(filename = defaultSigningConfigURL) {
  const config = JSON.parse(fs.readFileSync(filename, 'utf8'));
  requireThat(object(config) && Object.keys(config).sort().join(',') === 'algorithm,feedURL,publicKey,repository,schema', 'Update signing configuration must contain only the five public configuration fields.');
  requireThat(config.schema === 1 && config.algorithm === 'ed25519', 'Unsupported update signing configuration schema or algorithm.');
  requireThat(config.repository === officialUpdateRepository, 'Update signing configuration repository must be knothhe/passpine.');
  publicKey(config.publicKey);
  validateFeedURL(config.feedURL);
  requireThat(config.feedURL === defaultUpdateFeedURL, 'Update signing configuration feed must use the official release repository.');
  return config;
}

export function updateBuildConfig(env = process.env, signingConfigPath = defaultSigningConfigURL) {
  // Always validate the checked-in trust root, including when a developer uses
  // an explicit key/feed override for an isolated update test.
  const pinned = readUpdateSigningConfig(signingConfigPath);
  const key = env.PASSPINE_UPDATE_PUBLIC_KEY || pinned.publicKey;
  publicKey(key);
  const feed = env.PASSPINE_UPDATE_FEED_URL || pinned.feedURL;
  validateFeedURL(feed);
  return { feed, key };
}

// ZIP32, deflate, UTF-8 names and explicit Unix file modes. There are no directory
// entries, symlinks, host timestamps or external archive dependencies.
const crcTable = Array.from({ length: 256 }, (_, value) => {
  for (let i = 0; i < 8; i++) value = (value >>> 1) ^ ((value & 1) ? 0xedb88320 : 0);
  return value >>> 0;
});
function crc32(bytes) {
  let crc = 0xffffffff;
  for (const byte of bytes) crc = (crc >>> 8) ^ crcTable[(crc ^ byte) & 255];
  return (crc ^ 0xffffffff) >>> 0;
}
function safePath(name, os) {
  requireThat(typeof name === 'string' && name.length < 1024 && !name.includes('\\') && !name.includes('\0')
    && name.split('/').every(part => part && part !== '.' && part !== '..'), 'Unsafe update file path.');
  requireThat(os === 'darwin' ? name.startsWith('Contents/') : (os === 'windows' ? ['passpine.exe', 'passpine-host.exe'] : ['passpine', 'passpine-host']).includes(name), `Unexpected update file: ${name}`);
}
export function encodeZip(entries) {
  requireThat(entries.length > 0 && entries.length < 65535, 'Invalid update ZIP entry count.');
  const localParts = [], centralParts = [];
  let offset = 0;
  const seen = new Set();
  for (const { path: name, data, mode } of entries) {
    requireThat(!seen.has(name), 'Duplicate ZIP entry.'); seen.add(name);
    const filename = Buffer.from(name), compressed = zlib.deflateRawSync(data, { level: 9 });
    requireThat(filename.length <= 65535 && data.length < 0xffffffff && compressed.length < 0xffffffff, 'Update ZIP exceeds ZIP32 limits.');
    const crc = crc32(data), local = Buffer.alloc(30), central = Buffer.alloc(46);
    local.writeUInt32LE(0x04034b50); local.writeUInt16LE(20, 4); local.writeUInt16LE(0x800, 6); local.writeUInt16LE(8, 8);
    local.writeUInt16LE(33, 12); local.writeUInt32LE(crc, 14); local.writeUInt32LE(compressed.length, 18); local.writeUInt32LE(data.length, 22); local.writeUInt16LE(filename.length, 26);
    central.writeUInt32LE(0x02014b50); central.writeUInt16LE(0x0314, 4); central.writeUInt16LE(20, 6); central.writeUInt16LE(0x800, 8); central.writeUInt16LE(8, 10);
    central.writeUInt16LE(33, 14); central.writeUInt32LE(crc, 16); central.writeUInt32LE(compressed.length, 20); central.writeUInt32LE(data.length, 24); central.writeUInt16LE(filename.length, 28);
    central.writeUInt32LE(((0o100000 | mode) << 16) >>> 0, 38); central.writeUInt32LE(offset, 42);
    localParts.push(local, filename, compressed); centralParts.push(central, filename); offset += local.length + filename.length + compressed.length;
  }
  const central = Buffer.concat(centralParts), end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50); end.writeUInt16LE(entries.length, 8); end.writeUInt16LE(entries.length, 10); end.writeUInt32LE(central.length, 12); end.writeUInt32LE(offset, 16);
  return Buffer.concat([...localParts, central, end]);
}

export function decodeZip(bytes) {
  requireThat(bytes.length >= 22 && bytes.readUInt32LE(bytes.length - 22) === 0x06054b50, 'Invalid update ZIP end record.');
  const end = bytes.length - 22, count = bytes.readUInt16LE(end + 10), centralSize = bytes.readUInt32LE(end + 12), centralStart = bytes.readUInt32LE(end + 16);
  requireThat(bytes.readUInt16LE(end + 4) === 0 && bytes.readUInt16LE(end + 6) === 0 && count === bytes.readUInt16LE(end + 8)
    && centralStart + centralSize === end, 'Unsupported update ZIP structure.');
  const entries = [], names = new Set(); let cursor = centralStart;
  for (let i = 0; i < count; i++) {
    requireThat(cursor + 46 <= end && bytes.readUInt32LE(cursor) === 0x02014b50, 'Invalid ZIP directory.');
    const method = bytes.readUInt16LE(cursor + 10), crc = bytes.readUInt32LE(cursor + 16), length = bytes.readUInt32LE(cursor + 20), size = bytes.readUInt32LE(cursor + 24);
    const nameLength = bytes.readUInt16LE(cursor + 28), extraLength = bytes.readUInt16LE(cursor + 30), commentLength = bytes.readUInt16LE(cursor + 32), attributes = bytes.readUInt32LE(cursor + 38), offset = bytes.readUInt32LE(cursor + 42);
    requireThat(cursor + 46 + nameLength + extraLength + commentLength <= end && offset + 30 <= centralStart, 'Invalid ZIP bounds.');
    const name = bytes.subarray(cursor + 46, cursor + 46 + nameLength).toString('utf8');
    requireThat(!names.has(name) && ((attributes >>> 16) & 0o170000) === 0o100000, 'Duplicate or non-regular ZIP entry.'); names.add(name);
    requireThat(bytes.readUInt32LE(offset) === 0x04034b50 && bytes.readUInt16LE(offset + 8) === method && bytes.readUInt16LE(offset + 6) === 0x800, 'Invalid ZIP local header.');
    const nameStart = offset + 30, dataStart = nameStart + bytes.readUInt16LE(offset + 26) + bytes.readUInt16LE(offset + 28);
    requireThat(bytes.subarray(nameStart, nameStart + bytes.readUInt16LE(offset + 26)).toString('utf8') === name && dataStart + length <= centralStart && size <= 512 * 1024 * 1024, 'Invalid ZIP data bounds.');
    const compressed = bytes.subarray(dataStart, dataStart + length);
    requireThat(method === 8, 'Unsupported update ZIP compression.');
    const data = zlib.inflateRawSync(compressed, { maxOutputLength: Math.max(size, 1) });
    requireThat(data.length === size && crc32(data) === crc, 'Corrupted update ZIP entry.');
    entries.push({ path: name, mode: (attributes >>> 16) & 0o777, data }); cursor += 46 + nameLength + extraLength + commentLength;
  }
  requireThat(cursor === end, 'Invalid update ZIP directory size.');
  return entries;
}

function fileEntries(root, os) {
  const entries = [];
  function walk(directory, prefix = '') {
    for (const item of fs.readdirSync(directory, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      const relative = prefix + item.name, filename = path.join(directory, item.name);
      requireThat(!item.isSymbolicLink(), `Symlink in update bundle: ${relative}`);
      if (item.isDirectory()) walk(filename, `${relative}/`);
      else {
        requireThat(item.isFile(), `Non-regular update bundle file: ${relative}`); safePath(relative, os);
        const stat = fs.statSync(filename); entries.push({ path: relative, mode: stat.mode & 0o111 ? 0o755 : 0o644, data: fs.readFileSync(filename) });
      }
    }
  }
  if (os === 'darwin') walk(root);
  else for (const name of os === 'windows' ? ['passpine.exe', 'passpine-host.exe'] : ['passpine', 'passpine-host']) {
    const filename = path.join(root, name), stat = fs.lstatSync(filename);
    requireThat(stat.isFile() && !stat.isSymbolicLink(), `Missing update binary: ${name}`);
    entries.push({ path: name, mode: 0o755, data: fs.readFileSync(filename) });
  }
  return entries.sort((a, b) => a.path.localeCompare(b.path));
}
function artifact(filename) {
  const bytes = fs.readFileSync(filename), stat = fs.lstatSync(filename);
  requireThat(stat.isFile() && !stat.isSymbolicLink() && bytes.length > 0, `Invalid update artifact: ${filename}`);
  return { name: path.basename(filename), path: filename, size: bytes.length, sha256: digest(bytes) };
}
function validateFiles(files, os) {
  requireThat(Array.isArray(files) && files.length > 0 && files.length <= 4096, 'Invalid update files manifest.');
  const seen = new Set(); let total = 0;
  for (const file of files) {
    safePath(file.path, os); requireThat(!seen.has(file.path), 'Duplicate update file.'); seen.add(file.path);
    requireThat(/^[0-9a-f]{64}$/.test(file.sha256) && Number.isSafeInteger(file.size) && file.size >= 0 && [0o755, 0o644].includes(file.mode), 'Invalid update file metadata.');
    total += file.size; requireThat(total <= 512 * 1024 * 1024, 'Update bundle exceeds the 512 MiB uncompressed size limit.');
  }
  for (const name of os === 'darwin' ? ['Contents/MacOS/passpine', 'Contents/MacOS/passpine-host'] : os === 'windows' ? ['passpine.exe', 'passpine-host.exe'] : ['passpine', 'passpine-host']) requireThat(seen.has(name), `Update is missing ${name}`);
}
export function stageUpdate(version, target, root, destination) {
  validateVersion(version);
  const platform = targetPlatforms[target]; requireThat(platform, `Unsupported update target: ${target}`);
  const [os, arch] = platform, input = path.join(root, 'dist', os === 'darwin' ? 'Passpine.app' : '');
  const entries = fileEntries(input, os), files = entries.map(({ path, mode, data }) => ({ path, mode, size: data.length, sha256: digest(data) }));
  validateFiles(files, os);
  const name = `Passpine-${version}-update-${os}-${arch}.zip`, filename = path.join(destination, name);
  fs.mkdirSync(destination, { recursive: true }); fs.writeFileSync(filename, encodeZip(entries));
  const { path: ignored, ...full } = artifact(filename);
  return { os, arch, full, files };
}
export function validateUpdate(update, target, directory) {
  const platform = targetPlatforms[target]; requireThat(platform && update?.os === platform[0] && update?.arch === platform[1], 'Update target mismatch.');
  validateFiles(update.files, update.os);
  requireThat(typeof update.full?.name === 'string' && /^Passpine-[0-9.]+-update-(darwin|linux|windows)-(amd64|arm64)\.zip$/.test(update.full.name), 'Invalid update artifact name.');
  const full = artifact(path.join(directory, update.full.name));
  requireThat(full.size === update.full.size && full.sha256 === update.full.sha256, 'Update ZIP checksum mismatch.');
  const entries = decodeZip(fs.readFileSync(full.path));
  requireThat(entries.length === update.files.length, 'Update ZIP file list mismatch.');
  for (const entry of entries) {
    const file = update.files.find(file => file.path === entry.path);
    requireThat(file && file.size === entry.data.length && file.sha256 === digest(entry.data) && file.mode === entry.mode, 'Update ZIP file checksum mismatch.');
  }
  return full;
}

export function verifyFeed(bytes, key) {
  publicKey(key);
  requireThat(bytes.length <= 4 * 1024 * 1024, 'Signed update feed exceeds the client size limit.');
  const envelope = JSON.parse(bytes.toString('utf8'));
  requireThat(object(envelope) && typeof envelope.payload === 'string' && typeof envelope.signature === 'string', 'Invalid signed update feed.');
  const payload = Buffer.from(envelope.payload, 'base64'), signature = Buffer.from(envelope.signature, 'base64');
  requireThat(signature.length === 64 && crypto.verify(null, payload, publicKey(key), signature), 'Update feed signature verification failed.');
  const feed = JSON.parse(payload.toString('utf8'));
  validateVersion(feed.version); requireThat(feed.schema === 1 && Array.isArray(feed.assets), 'Invalid update feed payload.');
  for (const asset of feed.assets) validateFiles(asset.files, asset.os);
  return feed;
}

export function createUpdateFeed({ version, repo, updates, output, publicKey: key, privateKey }) {
  validateVersion(version);
  requireThat(/^[\w.-]+\/[\w.-]+$/.test(repo), 'Invalid update release repository.');
  const verificationKey = publicKey(key), signingKey = crypto.createPrivateKey(privateKey);
  requireThat(signingKey.asymmetricKeyType === 'ed25519' && crypto.createPublicKey(signingKey).export({ type: 'spki', format: 'der' }).equals(verificationKey.export({ type: 'spki', format: 'der' })), 'Update signing key does not match the embedded public key.');
  requireThat(updates.length === Object.keys(targetPlatforms).length && new Set(updates.map(({ target }) => target)).size === updates.length, 'Update target set is incomplete.');
  fs.mkdirSync(output, { recursive: true });
  const artifacts = [], baseURL = `https://github.com/${repo}/releases/download/v${version}/`;
  const assets = [...updates].sort((a, b) => a.target < b.target ? -1 : a.target > b.target ? 1 : 0).map(({ target, update, directory }) => {
    const full = validateUpdate(update, target, directory); artifacts.push(full);
    return { os: update.os, arch: update.arch, full: { url: baseURL + full.name, size: full.size, sha256: full.sha256 }, files: update.files };
  });
  const payload = Buffer.from(`${JSON.stringify({ schema: 1, version, assets })}\n`), signature = crypto.sign(null, payload, signingKey);
  const filename = path.join(output, 'update-feed.json');
  const envelope = `${JSON.stringify({ payload: payload.toString('base64'), signature: signature.toString('base64') })}\n`;
  requireThat(Buffer.byteLength(envelope) <= 4 * 1024 * 1024, 'Signed update feed exceeds the client size limit.');
  fs.writeFileSync(filename, envelope);
  artifacts.push(artifact(filename));
  return artifacts;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const [action] = process.argv.slice(2), config = updateBuildConfig();
    if (action === 'ldflags') console.log(`-X main.updateFeedURL=${config.feed} -X main.updatePublicKey=${config.key}`);
    else if (action === 'public-key') console.log(config.key);
    else throw new Error('Usage: update-artifacts.mjs ldflags|public-key');
  } catch (error) { console.error(`ERROR: ${error.message}`); process.exitCode = 1; }
}
