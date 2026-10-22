#!/usr/bin/env node
// Generate Chrome Web Store brand artwork from the existing Passpine assets.
// Requires sharp. If it is installed outside this repository, set
// PASSPINE_ART_NODE_MODULES to its node_modules directory before running:
//   PASSPINE_ART_NODE_MODULES=/path/to/node_modules node scripts/generate-store-art.mjs
// Typography uses Avenir Next when installed, with Helvetica Neue / Arial fallbacks.
import fs from 'node:fs/promises';
import path from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
let sharp;
try {
  const searchPaths = process.env.PASSPINE_ART_NODE_MODULES
    ? [process.env.PASSPINE_ART_NODE_MODULES]
    : [path.dirname(fileURLToPath(import.meta.url))];
  sharp = require(require.resolve('sharp', { paths: searchPaths }));
} catch (cause) {
  throw new Error('Store artwork generation requires sharp. Set PASSPINE_ART_NODE_MODULES to a node_modules directory containing sharp.', { cause });
}

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const output = path.join(root, 'docs/chrome-web-store/assets');
await fs.mkdir(output, { recursive: true });
const appIcon = await fs.readFile(path.join(root, 'client/assets/app-icon-monochrome.svg'));
const iconUrl = `data:image/svg+xml;base64,${appIcon.toString('base64')}`;
const font = "'Avenir Next', 'Helvetica Neue', Arial, sans-serif";
const title = 'Passpine';
const positioning = 'Local password manager';

function canvas(width, height, body) {
  return Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
    <defs>
      <linearGradient id="background" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#fbfbfc"/><stop offset="1" stop-color="#eeeef2"/></linearGradient>
      <radialGradient id="glow"><stop stop-color="#ffffff"/><stop offset="1" stop-color="#ffffff" stop-opacity="0"/></radialGradient>
      <filter id="shadow" x="-30%" y="-25%" width="160%" height="170%"><feDropShadow dx="0" dy="9" stdDeviation="11" flood-color="#171918" flood-opacity="0.08"/></filter>
    </defs>
    <rect width="${width}" height="${height}" fill="url(#background)"/>
    ${body}
  </svg>`);
}

const small = canvas(440, 280, `
  <ellipse cx="220" cy="80" rx="210" ry="170" fill="url(#glow)"/>
  <image x="170" y="23" width="100" height="100" xlink:href="${iconUrl}" filter="url(#shadow)"/>
  <text x="220" y="174" text-anchor="middle" font-family="${font}" font-size="44" font-weight="600" letter-spacing="-1.4" fill="#171918">${title}</text>
  <text x="220" y="209" text-anchor="middle" font-family="${font}" font-size="16" font-weight="400" fill="#63636b">${positioning}</text>
`);

const marquee = canvas(1400, 560, `
  <ellipse cx="380" cy="200" rx="570" ry="430" fill="url(#glow)"/>
  <circle cx="1465" cy="390" r="355" fill="none" stroke="#ffffff" stroke-opacity="0.55" stroke-width="1.5"/>
  <circle cx="1465" cy="390" r="455" fill="none" stroke="#ffffff" stroke-opacity="0.35" stroke-width="1.5"/>
  <image x="126" y="125" width="310" height="310" xlink:href="${iconUrl}" filter="url(#shadow)"/>
  <text x="487" y="284" font-family="${font}" font-size="94" font-weight="600" letter-spacing="-3" fill="#171918">${title}</text>
  <text x="492" y="345" font-family="${font}" font-size="31" font-weight="400" fill="#63636b">${positioning}</text>
`);

await fs.copyFile(path.join(root, 'browser/public/icon128-monochrome.png'), path.join(output, 'icon-128.png'));
for (const [filename, input] of [
  ['promo-440x280.png', small],
  ['marquee-1400x560.png', marquee],
]) {
  await sharp(input, { density: 144 })
    .resize(filename.startsWith('promo') ? 440 : 1400, filename.startsWith('promo') ? 280 : 560)
    .flatten({ background: '#f2f2f5' })
    .toColourspace('srgb')
    .removeAlpha()
    .png({ palette: false, compressionLevel: 9 })
    .toFile(path.join(output, filename));
}

for (const filename of ['icon-128.png', 'promo-440x280.png', 'marquee-1400x560.png']) {
  const metadata = await sharp(path.join(output, filename)).metadata();
  console.log(`${path.relative(root, path.join(output, filename))}: ${metadata.width}x${metadata.height}, ${metadata.channels} channels, ${metadata.space}`);
}
