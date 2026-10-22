import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';
import { execFileSync, spawnSync } from 'node:child_process';
import { installerPackageFor } from '../update-artifacts.mjs';

// ZIP writer belongs only to fixtures; production publishes the existing installer.
const requireThat = (condition, message) => { if (!condition) throw new Error(message); };
const crc32 = data => {
  let crc = 0xffffffff;
  for (const byte of data) {
    crc ^= byte;
    for (let bit = 0; bit < 8; bit++) crc = (crc >>> 1) ^ ((crc & 1) ? 0xedb88320 : 0);
  }
  return (crc ^ 0xffffffff) >>> 0;
};
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

export const targets = ['client-darwin-arm64', 'client-linux-amd64', 'client-win32-amd64'];
export const signingKeys = () => {
  const { publicKey, privateKey } = crypto.generateKeyPairSync('ed25519');
  return { publicKey: publicKey.export({ format: 'der', type: 'spki' }).subarray(-32).toString('base64'), privateKey: privateKey.export({ format: 'pem', type: 'pkcs8' }) };
};
const nsisCandidates = ['makensis', ...(process.platform === 'win32' ? [path.join(process.env['ProgramFiles(x86)'] || '', 'NSIS', 'makensis.exe')] : [])];
export const nsisCompiler = nsisCandidates.find(tool => !spawnSync(tool, [process.platform === 'win32' ? '/VERSION' : '-VERSION'], { stdio: 'ignore' }).error);
export const installerToolsAvailable = !!nsisCompiler && ['7z', '7zz'].some(tool => !spawnSync(tool, ['i'], { stdio: 'ignore' }).error);
export const installerTest = { skip: !installerToolsAvailable && 'Static NSIS fixture verification requires makensis and 7z/7zz (no installer is executed).' };

export function createInstallerFixtures(root, version, changed = false) {
  const dist = path.join(root, 'dist');
  const write = (name, data, mode = 0o755) => {
    const filename = path.join(dist, name); fs.mkdirSync(path.dirname(filename), { recursive: true }); fs.writeFileSync(filename, data, { mode });
  };
  const host = Buffer.concat(Array.from({ length: 256 }, (_, i) => crypto.createHash('sha256').update(`host ${i}`).digest()));
  for (const suffix of ['', '.exe']) { write(`passpine${suffix}`, changed ? 'new client binary' : 'old client binary'); write(`passpine-host${suffix}`, host); }
  write('Passpine.app/Contents/MacOS/passpine', changed ? 'new mac client' : 'old mac client');
  write('Passpine.app/Contents/MacOS/passpine-host', host);
  write('Passpine.app/Contents/Info.plist', `<plist><key>CFBundleIdentifier</key><string>com.knothhe.passpine</string><key>CFBundleShortVersionString</key><string>${version}</string><key>CFBundleVersion</key><string>${version}</string></plist>`, 0o644);
  write('Passpine.app/Contents/Resources/icon.icns', host, 0o644);
  write('version.txt', `${version}\n`, 0o644); write('install.sh', '#!/bin/sh\nexit 1\n'); write('app-icon-monochrome.png', host, 0o644);
  return dist;
}

export function createOrdinaryInstaller(root, directory, version, target) {
  fs.mkdirSync(directory, { recursive: true });
  const { name, format } = installerPackageFor(version, target), filename = path.join(directory, name), dist = path.join(root, 'dist');
  if (format === 'app-zip') {
    const entries = [];
    const walk = (relative) => {
      for (const item of fs.readdirSync(path.join(dist, relative), { withFileTypes: true })) {
        const entry = `${relative}/${item.name}`, fullPath = path.join(dist, entry);
        if (item.isDirectory()) walk(entry);
        else entries.push({ path: entry, mode: fs.statSync(fullPath).mode & 0o111 ? 0o755 : 0o644, data: fs.readFileSync(fullPath) });
      }
    };
    walk('Passpine.app'); fs.writeFileSync(filename, encodeZip(entries));
  } else if (format === 'tar-gzip') {
    execFileSync('tar', ['--format=ustar', '-czf', filename, '-C', dist, 'passpine', 'passpine-host', 'version.txt', 'install.sh', 'app-icon-monochrome.png'], { env: { ...process.env, COPYFILE_DISABLE: '1' } });
  } else {
    if (!installerToolsAvailable) throw new Error('NSIS fixture tools are unavailable.');
    const quote = value => value.replaceAll('$', '$$').replaceAll('"', '$\\"');
    const source = path.join(directory, 'fixture.nsi');
    fs.writeFileSync(source, `Unicode true\nName "Passpine fixture"\nOutFile "${quote(filename)}"\nRequestExecutionLevel user\nVIProductVersion "${version}.0"\nVIAddVersionKey /LANG=1033 "PasspineExtractProtocol" "passpine-update-extract-v1-a8e19eb9-3dc3-4ccf-8b58-97546f3051f0"\nSection\nSetOutPath "$INSTDIR"\nFile "${quote(path.join(dist, 'passpine.exe'))}"\nFile "${quote(path.join(dist, 'passpine-host.exe'))}"\nSectionEnd\n`);
    execFileSync(nsisCompiler, [process.platform === 'win32' ? '/V1' : '-V1', source], { stdio: 'pipe' });
    fs.rmSync(source);
  }
  return filename;
}

export function legacyFeedBytes(keys, updates) {
  const payload = Buffer.from(JSON.stringify({ schema: 1, version: '0.1.5', assets: updates.map(({ update }) => ({
    os: update.os, arch: update.arch, full: { url: `https://github.com/knothhe/passpine/releases/download/v0.1.5/Passpine-0.1.5-update-${update.os}-${update.arch}.zip`, size: 1234, sha256: '1'.repeat(64) }, files: update.files,
  })) }));
  return Buffer.from(JSON.stringify({ payload: payload.toString('base64'), signature: crypto.sign(null, payload, keys.privateKey).toString('base64') }));
}
