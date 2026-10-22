#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import zlib from 'node:zlib';
import { pathToFileURL } from 'node:url';
import { validateVersion } from './version.mjs';

export const defaultUpdateFeedURL = 'https://github.com/knothhe/passpine/releases/latest/download/update-feed.json';
const defaultSigningConfigURL = new URL('../config/update-signing.json', import.meta.url);
const defaultLegacyFeedURL = new URL('../config/update-legacy-feed.json', import.meta.url);
const legacyVersion = '0.1.5';
const maxExpandedSize = 512 * 1024 * 1024;
const officialUpdateRepository = 'knothhe/passpine';
const targetPlatforms = { 'client-darwin-arm64': ['darwin', 'arm64'], 'client-linux-amd64': ['linux', 'amd64'], 'client-win32-amd64': ['windows', 'amd64'] };
const digest = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const requireThat = (condition, message) => { if (!condition) throw new Error(message); };
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const newerThanLegacy = version => {
  const parts = version.split('.').map(Number), baseline = legacyVersion.split('.').map(Number);
  const index = parts.findIndex((part, i) => part !== baseline[i]);
  return index >= 0 && parts[index] > baseline[index];
};

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

// Inspect ordinary ZIP archives without extracting files onto the filesystem.
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
  requireThat(typeof name === 'string' && name.length < 1024 && !/[\\\0\r\n:]/.test(name)
    && name.split('/').every(part => part && part !== '.' && part !== '..'), 'Unsafe update file path.');
  requireThat(os === 'darwin' ? name.startsWith('Contents/') : (os === 'windows' ? ['passpine.exe', 'passpine-host.exe'] : ['passpine', 'passpine-host']).includes(name), `Unexpected update file: ${name}`);
}
export function decodeZip(bytes, validateEntry = () => {}) {
  let end = bytes.length - 22;
  while (end >= Math.max(0, bytes.length - 65557) && !(bytes.readUInt32LE(end) === 0x06054b50 && end + 22 + bytes.readUInt16LE(end + 20) === bytes.length)) end--;
  requireThat(end >= Math.max(0, bytes.length - 65557), 'Invalid ZIP end record.');
  const count = bytes.readUInt16LE(end + 10), centralSize = bytes.readUInt32LE(end + 12), centralStart = bytes.readUInt32LE(end + 16);
  requireThat(bytes.readUInt16LE(end + 4) === 0 && bytes.readUInt16LE(end + 6) === 0 && count > 0 && count <= 16384
    && count === bytes.readUInt16LE(end + 8) && centralStart + centralSize === end, 'Unsupported ZIP structure.');
  const entries = [], names = new Set(), ranges = []; let cursor = centralStart, total = 0;
  for (let i = 0; i < count; i++) {
    requireThat(cursor + 46 <= end && bytes.readUInt32LE(cursor) === 0x02014b50, 'Invalid ZIP directory.');
    const flags = bytes.readUInt16LE(cursor + 8), method = bytes.readUInt16LE(cursor + 10), crc = bytes.readUInt32LE(cursor + 16), length = bytes.readUInt32LE(cursor + 20), size = bytes.readUInt32LE(cursor + 24);
    const nameLength = bytes.readUInt16LE(cursor + 28), extraLength = bytes.readUInt16LE(cursor + 30), commentLength = bytes.readUInt16LE(cursor + 32), attributes = bytes.readUInt32LE(cursor + 38), offset = bytes.readUInt32LE(cursor + 42);
    requireThat(cursor + 46 + nameLength + extraLength + commentLength <= end && offset + 30 <= centralStart && !(flags & ~0x80e) && [0, 8].includes(method), 'Invalid ZIP bounds or compression.');
    const filename = bytes.subarray(cursor + 46, cursor + 46 + nameLength), name = filename.toString('utf8');
    const directory = name.endsWith('/'), stripped = directory ? name.slice(0, -1) : name;
    requireThat(filename.equals(Buffer.from(name)) && stripped && !stripped.includes('\\') && !stripped.includes('\0')
      && stripped.split('/').every(part => part && part !== '.' && part !== '..'), 'Unsafe ZIP entry path.');
    const folded = name.toLowerCase(), type = (attributes >>> 16) & 0o170000;
    requireThat(!names.has(folded) && (directory ? [0, 0o040000].includes(type) : [0, 0o100000].includes(type)), 'Duplicate or non-regular ZIP entry.'); names.add(folded);
    validateEntry(name, directory);
    requireThat(bytes.readUInt32LE(offset) === 0x04034b50 && bytes.readUInt16LE(offset + 8) === method && bytes.readUInt16LE(offset + 6) === flags, 'Invalid ZIP local header.');
    const nameStart = offset + 30, localNameLength = bytes.readUInt16LE(offset + 26), dataStart = nameStart + localNameLength + bytes.readUInt16LE(offset + 28);
    requireThat(bytes.subarray(nameStart, nameStart + localNameLength).equals(filename) && dataStart + length <= centralStart && size <= maxExpandedSize, 'Invalid ZIP data bounds.');
    total += size; requireThat(total <= maxExpandedSize, 'ZIP exceeds the 512 MiB uncompressed size limit.');
    ranges.push([offset, dataStart + length]);
    const compressed = bytes.subarray(dataStart, dataStart + length), data = method === 0 ? Buffer.from(compressed) : zlib.inflateRawSync(compressed, { maxOutputLength: Math.max(size, 1) });
    requireThat(data.length === size && crc32(data) === crc && (!directory || size === 0), 'Corrupted ZIP entry.');
    if (!directory) entries.push({ path: name, mode: (attributes >>> 16) & 0o111 ? 0o755 : 0o644, data });
    cursor += 46 + nameLength + extraLength + commentLength;
  }
  ranges.sort((a, b) => a[0] - b[0]);
  requireThat(cursor === end && ranges.every((range, i) => i === 0 || range[0] >= ranges[i - 1][1]), 'Invalid ZIP directory size or overlapping entries.');
  return entries;
}

export function installerPackageFor(version, target) {
  validateVersion(version);
  const platform = targetPlatforms[target]; requireThat(platform, `Unsupported update target: ${target}`);
  const [os, arch] = platform;
  const [format, suffix] = os === 'darwin' ? ['app-zip', `macos-${arch}.zip`]
    : os === 'linux' ? ['tar-gzip', `linux-${arch}.tar.gz`] : ['nsis', `windows-${arch}-setup.exe`];
  return { os, arch, format, name: `Passpine-${version}-${suffix}` };
}

function decodeGzip(bytes) {
  requireThat(bytes.length >= 18 && bytes[0] === 0x1f && bytes[1] === 0x8b && bytes[2] === 8 && !(bytes[3] & 0xe0), 'Invalid GZIP header.');
  const flags = bytes[3]; let offset = 10;
  if (flags & 4) {
    requireThat(offset + 2 <= bytes.length - 8, 'Invalid GZIP extra header.');
    offset += 2 + bytes.readUInt16LE(offset);
  }
  for (const flag of [8, 16]) if (flags & flag) {
    const end = bytes.indexOf(0, offset);
    requireThat(end >= offset && end < bytes.length - 8 && end < 128 * 1024, 'Invalid GZIP text header.'); offset = end + 1;
  }
  requireThat(offset <= bytes.length - 8 && offset <= 128 * 1024, 'GZIP header exceeds size limit.');
  if (flags & 2) {
    requireThat(offset + 2 <= bytes.length - 8 && (crc32(bytes.subarray(0, offset)) & 0xffff) === bytes.readUInt16LE(offset), 'Invalid GZIP header checksum.'); offset += 2;
  }
  const decoded = zlib.inflateRawSync(bytes.subarray(offset), { info: true, maxOutputLength: maxExpandedSize + 128 * 1024 });
  const trailer = offset + decoded.engine.bytesWritten, raw = decoded.buffer;
  requireThat(trailer + 8 === bytes.length && bytes.readUInt32LE(trailer) === crc32(raw)
    && bytes.readUInt32LE(trailer + 4) === raw.length, 'Invalid GZIP trailer or multiple GZIP streams.');
  return raw;
}

function decodeTar(bytes, version) {
  const raw = decodeGzip(bytes), entries = [], seen = new Set();
  const required = ['passpine', 'passpine-host', 'version.txt', 'install.sh', 'app-icon-monochrome.png'];
  const text = value => value.toString('utf8').replace(/\0.*$/s, '');
  const octal = value => { const str = text(value).trim(); requireThat(/^[0-7]+$/.test(str), 'Invalid TAR numeric field.'); return parseInt(str, 8); };
  let cursor = 0, ended = false;
  while (cursor + 512 <= raw.length) {
    const header = raw.subarray(cursor, cursor + 512);
    if (header.every(byte => byte === 0)) { requireThat(raw.subarray(cursor).every(byte => byte === 0), 'Unexpected trailing TAR data.'); ended = true; break; }
    const checksum = octal(header.subarray(148, 156)), checked = Buffer.from(header); checked.fill(32, 148, 156);
    requireThat(checked.reduce((sum, byte) => sum + byte, 0) === checksum, 'Invalid TAR checksum.');
    const name = text(header.subarray(0, 100)), prefix = text(header.subarray(345, 500)), size = octal(header.subarray(124, 136)), mode = octal(header.subarray(100, 108));
    requireThat(!prefix && required.includes(name) && !seen.has(name) && [0, 48].includes(header[156]) && size <= maxExpandedSize, 'Unsafe or unexpected TAR entry.');
    seen.add(name); const start = cursor + 512; requireThat(start + size <= raw.length, 'Invalid TAR file bounds.');
    entries.push({ path: name, mode: mode & 0o111 ? 0o755 : 0o644, data: Buffer.from(raw.subarray(start, start + size)) });
    cursor = start + Math.ceil(size / 512) * 512;
  }
  requireThat(ended && required.every(name => seen.has(name)), 'Linux package file list mismatch.');
  requireThat(entries.find(entry => entry.path === 'version.txt').data.toString('utf8').trim() === version, 'Linux package version mismatch.');
  return entries.filter(entry => ['passpine', 'passpine-host'].includes(entry.path));
}

const extractProtocol = 'passpine-update-extract-v1-a8e19eb9-3dc3-4ccf-8b58-97546f3051f0';
function validateNSISProtocol(bytes) {
  requireThat(bytes.length >= 64 && bytes.subarray(0, 2).toString() === 'MZ', 'Invalid Windows installer.');
  const pe = bytes.readUInt32LE(60);
  requireThat(pe >= 64 && pe + 24 <= bytes.length && bytes.readUInt32LE(pe) === 0x4550, 'Invalid Windows installer PE header.');
  const count = bytes.readUInt16LE(pe + 6), table = pe + 24 + bytes.readUInt16LE(pe + 20);
  requireThat(count > 0 && count <= 96 && table + count * 40 <= bytes.length, 'Invalid Windows installer PE sections.');
  const resources = [];
  for (let i = 0; i < count; i++) {
    const section = table + i * 40;
    if (bytes.subarray(section, section + 8).toString('ascii').replace(/\0.*$/, '') !== '.rsrc') continue;
    const start = bytes.readUInt32LE(section + 20), size = bytes.readUInt32LE(section + 16);
    requireThat(start >= table + count * 40 && size <= 4 * 1024 * 1024 && start + size <= bytes.length, 'Invalid Windows installer resource bounds.');
    resources.push(bytes.subarray(start, start + size));
  }
  requireThat(resources.length === 1 && resources[0].includes(Buffer.from('PasspineExtractProtocol\0', 'utf16le'))
    && resources[0].includes(Buffer.from(extractProtocol + '\0', 'utf16le')), 'Windows installer is missing the supported update extraction protocol.');
}

function nsisEntries(filename, expectedFiles) {
  // Static extraction only: never execute an installer to inspect release input.
  const run = (args, maxBuffer) => {
    for (const tool of ['7z', '7zz']) {
      try { return execFileSync(tool, args, { maxBuffer, timeout: 120_000, windowsHide: true }); }
      catch (error) { if (error.code !== 'ENOENT') throw new Error('NSIS package static extraction failed.'); }
    }
    throw new Error('NSIS package verification requires 7z or 7zz.');
  };
  const listing = run(['l', '-slt', '-ba', '-sccUTF-8', filename], 4 * 1024 * 1024).toString('utf8');
  const records = listing.trim().split(/\r?\n\s*\r?\n/).map(record => Object.fromEntries(record.split(/\r?\n/).map(line => line.split(' = '))));
  const names = ['passpine.exe', 'passpine-host.exe'], allowed = [...names, '$PLUGINSDIR/System.dll', 'Uninstall.exe'];
  const seen = new Set();
  for (const record of records) {
    if (typeof record.Path === 'string') record.Path = record.Path.replaceAll('\\', '/');
    requireThat(allowed.includes(record.Path) && !seen.has(record.Path.toLowerCase()), 'Unexpected or duplicate NSIS package entry.'); seen.add(record.Path.toLowerCase());
  }
  return names.map(name => {
    const files = records.filter(record => record.Path === name), expected = expectedFiles.find(file => file.path === name);
    requireThat(files.length === 1 && expected && Number.isSafeInteger(expected.size) && expected.size > 0 && expected.size <= maxExpandedSize
      && (files[0].Size === undefined || files[0].Size === '' || typeof files[0].Size === 'string' && /^\d+$/.test(files[0].Size) && Number(files[0].Size) === expected.size)
      && !/D/.test(files[0].Attributes || '') && !files[0]['Symbolic Link'] && !files[0]['Hard Link'], 'NSIS package file list mismatch.');
    // NSIS compressed blocks may omit their expanded size from the 7z listing.
    // Bound extraction using the signed file manifest, then compare exact size
    // here and hash/mode in validateUpdate regardless of listing metadata.
    const data = run(['x', '-so', '-y', filename, name], expected.size + 1);
    requireThat(data.length === expected.size, 'NSIS package file size mismatch.');
    return { path: name, mode: 0o755, data };
  });
}

function installerEntries(filename, format, version, files) {
  if (format === 'app-zip') return decodeZip(fs.readFileSync(filename), (name, directory) => {
    const allowed = directory ? files.some(file => `Passpine.app/${file.path}`.startsWith(name)) : name.startsWith('Passpine.app/Contents/');
    requireThat(allowed, 'Unexpected macOS package entry.');
  }).map(entry => ({ ...entry, path: entry.path.slice('Passpine.app/'.length) }));
  if (format === 'tar-gzip') return decodeTar(fs.readFileSync(filename), version);
  validateNSISProtocol(fs.readFileSync(filename));
  return nsisEntries(filename, files);
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
  const stat = fs.lstatSync(filename);
  requireThat(stat.isFile() && !stat.isSymbolicLink() && stat.size > 0, `Invalid update artifact: ${filename}`);
  const bytes = fs.readFileSync(filename);
  return { name: path.basename(filename), path: filename, size: bytes.length, sha256: digest(bytes) };
}
function validateFiles(files, os) {
  requireThat(Array.isArray(files) && files.length > 0 && files.length <= 4096, 'Invalid update files manifest.');
  const seen = new Set(); let total = 0;
  for (const file of files) {
    safePath(file.path, os); const folded = file.path.toLowerCase(); requireThat(!seen.has(folded), 'Duplicate update file.'); seen.add(folded);
    requireThat(/^[0-9a-f]{64}$/.test(file.sha256) && Number.isSafeInteger(file.size) && file.size >= 0 && [0o755, 0o644].includes(file.mode), 'Invalid update file metadata.');
    total += file.size; requireThat(total <= 512 * 1024 * 1024, 'Update bundle exceeds the 512 MiB uncompressed size limit.');
  }
  for (const name of os === 'darwin' ? ['Contents/MacOS/passpine', 'Contents/MacOS/passpine-host'] : os === 'windows' ? ['passpine.exe', 'passpine-host.exe'] : ['passpine', 'passpine-host']) requireThat(seen.has(name.toLowerCase()), `Update is missing ${name}`);
}
export function stageUpdate(version, target, root, destination) {
  const { os, arch, format, name } = installerPackageFor(version, target), input = path.join(root, 'dist', os === 'darwin' ? 'Passpine.app' : '');
  const files = fileEntries(input, os).map(({ path, mode, data }) => ({ path, mode, size: data.length, sha256: digest(data) }));
  validateFiles(files, os);
  const { path: ignored, ...full } = artifact(path.join(destination, name));
  const update = { os, arch, format, full, files };
  validateUpdate(update, target, destination, version);
  return update;
}
export function validateUpdate(update, target, directory, expectedVersion) {
  const version = expectedVersion || /^Passpine-([0-9]+\.[0-9]+\.[0-9]+)-/.exec(update?.full?.name || '')?.[1];
  const expected = installerPackageFor(version, target);
  requireThat(update?.os === expected.os && update?.arch === expected.arch && update?.format === expected.format, 'Update target or format mismatch.');
  validateFiles(update.files, update.os);
  requireThat(update.full?.name === expected.name, 'Invalid ordinary installer artifact name.');
  const full = artifact(path.join(directory, expected.name));
  requireThat(full.size <= 1024 * 1024 * 1024 && full.size === update.full.size && full.sha256 === update.full.sha256, 'Installer checksum mismatch.');
  const entries = installerEntries(full.path, update.format, version, update.files);
  requireThat(entries.length === update.files.length, 'Installer file list mismatch.');
  for (const entry of entries) {
    const file = update.files.find(file => file.path === entry.path);
    requireThat(file && file.size === entry.data.length && file.sha256 === digest(entry.data) && file.mode === entry.mode, 'Installer file checksum mismatch.');
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
  if (feed.installerUpdate !== undefined) {
    validateLegacyFeed({ schema: feed.schema, version: feed.version, assets: feed.assets });
    const installer = feed.installerUpdate;
    requireThat(object(installer) && installer.schema === 2 && Array.isArray(installer.assets) && installer.assets.length === 3, 'Invalid installer update payload.');
    validateVersion(installer.version); requireThat(newerThanLegacy(installer.version), 'Invalid installer update version.');
    const seen = new Set();
    for (const asset of installer.assets) {
      const target = Object.keys(targetPlatforms).find(target => targetPlatforms[target][0] === asset.os && targetPlatforms[target][1] === asset.arch);
      requireThat(target && !seen.has(target), 'Invalid or duplicate installer update target.'); seen.add(target);
      const expected = installerPackageFor(installer.version, target); validateFiles(asset.files, asset.os);
      requireThat(asset.format === expected.format && asset.full?.url === `https://github.com/${officialUpdateRepository}/releases/download/v${installer.version}/${expected.name}`
        && Number.isSafeInteger(asset.full.size) && asset.full.size > 0 && asset.full.size <= 1024 * 1024 * 1024 && /^[0-9a-f]{64}$/.test(asset.full.sha256), 'Invalid installer update package.');
    }
  }
  return feed;
}

function validateLegacyFeed(feed) {
  requireThat(feed.schema === 1 && feed.version === legacyVersion && Object.keys(feed).sort().join(',') === 'assets,schema,version', 'Legacy compatibility feed must be the published v0.1.5 schema 1 feed.');
  requireThat(feed.assets.length === 3, 'Legacy compatibility target set is incomplete.');
  const seen = new Set();
  for (const asset of feed.assets) {
    const target = Object.values(targetPlatforms).find(([os, arch]) => os === asset.os && arch === asset.arch);
    const id = `${asset.os}-${asset.arch}`;
    requireThat(target && !seen.has(id) && Object.keys(asset).sort().join(',') === 'arch,files,full,os', 'Invalid legacy compatibility target.'); seen.add(id);
    requireThat(asset.full?.url === `https://github.com/${officialUpdateRepository}/releases/download/v${legacyVersion}/Passpine-${legacyVersion}-update-${id}.zip`
      && Number.isSafeInteger(asset.full.size) && asset.full.size > 0 && asset.full.size <= 1024 * 1024 * 1024 && /^[0-9a-f]{64}$/.test(asset.full.sha256), 'Invalid legacy compatibility package.');
  }
}

export function readLegacyUpdateFeed(key, bytes = fs.readFileSync(defaultLegacyFeedURL)) {
  const feed = verifyFeed(bytes, key); validateLegacyFeed(feed);
  return feed;
}

export function createUpdateFeed({ version, repo, updates, output, publicKey: key, privateKey, legacyFeedBytes }) {
  validateVersion(version);
  requireThat(repo === officialUpdateRepository, 'Invalid update release repository.');
  requireThat(newerThanLegacy(version), 'Installer updates must be newer than v0.1.5.');
  const verificationKey = publicKey(key), signingKey = crypto.createPrivateKey(privateKey);
  requireThat(signingKey.asymmetricKeyType === 'ed25519' && crypto.createPublicKey(signingKey).export({ type: 'spki', format: 'der' }).equals(verificationKey.export({ type: 'spki', format: 'der' })), 'Update signing key does not match the embedded public key.');
  const legacy = readLegacyUpdateFeed(key, legacyFeedBytes);
  requireThat(updates.length === Object.keys(targetPlatforms).length && new Set(updates.map(({ target }) => target)).size === updates.length, 'Update target set is incomplete.');
  fs.mkdirSync(output, { recursive: true });
  const baseURL = `https://github.com/${repo}/releases/download/v${version}/`;
  const assets = [...updates].sort((a, b) => a.target < b.target ? -1 : a.target > b.target ? 1 : 0).map(({ target, update, directory }) => {
    const full = validateUpdate(update, target, directory, version);
    return { os: update.os, arch: update.arch, format: update.format, full: { url: baseURL + full.name, size: full.size, sha256: full.sha256 }, files: update.files };
  });
  const payload = Buffer.from(`${JSON.stringify({ ...legacy, installerUpdate: { schema: 2, version, assets } })}\n`), signature = crypto.sign(null, payload, signingKey);
  const filename = path.join(output, 'update-feed.json');
  const envelope = `${JSON.stringify({ payload: payload.toString('base64'), signature: signature.toString('base64') })}\n`;
  requireThat(Buffer.byteLength(envelope) <= 4 * 1024 * 1024, 'Signed update feed exceeds the client size limit.');
  fs.writeFileSync(filename, envelope);
  return [artifact(filename)];
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const [action] = process.argv.slice(2), config = updateBuildConfig();
    if (action === 'ldflags') console.log(`-X main.updateFeedURL=${config.feed} -X main.updatePublicKey=${config.key}`);
    else if (action === 'public-key') console.log(config.key);
    else throw new Error('Usage: update-artifacts.mjs ldflags|public-key');
  } catch (error) { console.error(`ERROR: ${error.message}`); process.exitCode = 1; }
}
