#!/usr/bin/env node
// Capture the website's product screenshots in both languages:
//   assets/website/<language>/desktop.png    real desktop UI, headless renderer
//   assets/website/<language>/extension.png real popup from the extension preview
// Requires Go, pnpm and PASSPINE_CAPTURE_BROWSER pointing at Chrome or Chromium.
// Every captured value is fictional demo data.
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const output = path.join(root, 'assets/website');
const languages = ['zh-CN', 'en'];
const port = Number(process.env.PASSPINE_CAPTURE_PORT || 5179);
const browser = process.env.PASSPINE_CAPTURE_BROWSER;
if (!browser) throw new Error('Set PASSPINE_CAPTURE_BROWSER to a Chrome or Chromium executable.');

const run = (command, args, options = {}) => new Promise((resolve, reject) => {
  const child = spawn(command, args, { stdio: 'inherit', ...options });
  child.once('error', reject);
  child.once('exit', code => code === 0 ? resolve() : reject(new Error(`${command} exited ${code}`)));
});
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

await run('go', ['test', './desktop', '-run', '^TestWebsiteScreenshots$', '-count=1'], {
  cwd: path.join(root, 'client'), env: { ...process.env, PASSPINE_WEBSITE_SCREENSHOT_DIR: output },
});

const server = spawn('pnpm', ['--dir', 'browser', 'exec', 'vite', '--config', 'vite.preview.config.ts', '--port', String(port), '--strictPort'], { cwd: root, stdio: 'ignore' });
const base = await fs.mkdtemp(path.join(os.tmpdir(), 'passpine-website-'));
try {
  const url = `http://127.0.0.1:${port}/`;
  for (let attempt = 0; ; attempt++) {
    try { if ((await fetch(url)).ok) break; } catch {}
    if (attempt > 100) throw new Error('The extension preview server did not start.');
    await delay(200);
  }
  for (const language of languages) {
    const destination = path.join(output, language, 'extension.png');
    await fs.mkdir(path.dirname(destination), { recursive: true });
    // A fresh profile has no saved language, so the preview follows --lang.
    await run(browser, [
      '--headless=new', '--disable-gpu', '--no-first-run', '--no-default-browser-check', '--hide-scrollbars',
      `--user-data-dir=${path.join(base, language)}`, `--lang=${language}`, '--window-size=380,460',
      '--force-device-scale-factor=2', '--virtual-time-budget=5000', `--screenshot=${destination}`,
      `${url}?view=popup&theme=light&style=monochrome`,
    ], { env: { ...process.env, LANGUAGE: language.replace('-', '_'), LANG: `${language.replace('-', '_')}.UTF-8` } });
  }
} finally {
  server.kill();
  await fs.rm(base, { recursive: true, force: true });
}
console.log(`Captured website screenshots for ${languages.join(', ')} in ${path.relative(root, output)}`);
