import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
const hash = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const generator = path.join(root, 'scripts/dev_icon.py');
// Generated files are checked in so normal builds do not require Pillow.
for (const [style, source] of [['onevra', 'Passpine.icns'], ['monochrome', 'Passpine-monochrome.icns']]) {
  const directory = path.join(root, 'client/assets/dev', style);
  const input = path.join(root, 'client/assets', source);
  let current = false;
  try {
    const manifest = JSON.parse(fs.readFileSync(path.join(directory, '.dev-icon-manifest.json'), 'utf8'));
    current = manifest.source_sha256 === hash(input) && manifest.generator_sha256 === hash(generator) && manifest.formats?.join(',') === 'icns,png' && Object.entries(manifest.files).every(([name, digest]) => hash(path.join(directory, name)) === digest);
  } catch { /* Generate missing resources with the prescribed deterministic tool. */ }
  if (!current) execFileSync(process.env.PASSPINE_ICON_PYTHON || 'python3', [generator, '--input', input, '--output-dir', directory, '--formats', 'png,icns', '--sizes', '256', '--name', 'app-icon-dev', '--force'], { stdio: 'inherit' });
}
