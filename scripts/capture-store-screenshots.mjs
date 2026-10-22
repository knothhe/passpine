#!/usr/bin/env node
// Build the extension first. Requires Go, Playwright, Sharp and a full Chromium
// supporting unpacked extensions. See docs/chrome-web-store/assets.md.
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';

const require = createRequire(import.meta.url);
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const modules = process.env.PASSPINE_ART_NODE_MODULES;
const resolve = name => require.resolve(name, { paths: modules ? [modules] : [root] });
const { chromium } = require(resolve('playwright'));
const sharp = require(resolve('sharp'));
const executablePath = process.env.PASSPINE_CAPTURE_BROWSER;
if (!executablePath) throw new Error('Set PASSPINE_CAPTURE_BROWSER to a full Chrome for Testing or Chromium executable.');
const extension = path.join(root, 'browser/.output/chrome-mv3');
const manifest = JSON.parse(await fs.readFile(path.join(extension, 'manifest.json'), 'utf8'));
const base = await fs.mkdtemp(path.join(os.tmpdir(), 'passpine-store-'));
const profile = path.join(base, 'profile');
const binary = path.join(base, 'demo-helper');
const data = path.join(base, 'data');
const output = path.join(root, 'docs/chrome-web-store');
let helper, context;

const run = (command, args, options = {}) => new Promise((resolve, reject) => {
  const child = spawn(command, args, { stdio: 'inherit', ...options });
  child.once('error', reject);
  child.once('exit', code => code === 0 ? resolve() : reject(new Error(`${command} exited ${code}`)));
});
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const escapeXML = value => value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
const copy = {
  en: {
    language: 'Language', details: 'Account details', view: 'View Email', back: 'Back to vault', create: 'New account', generate: 'Regenerate password', generator: 'Password generator', generated: 'Generated password',
    scenes: [
      ['01-accounts', ['Your vault.', 'In your browser.'], ['Find your accounts in a local,', 'pass-compatible password vault.']],
      ['02-details', ['Keep credentials', 'within reach.'], ['View account details and copy', 'the field you need.']],
      ['03-generator', ['Make room for', 'stronger passwords.'], ['Choose the length and character types.', 'Generate a password with the local client.']],
    ],
    footer: 'Desktop client required · Fictional demo accounts',
  },
  'zh-CN': {
    language: '语言', details: '账号详情', view: '查看 Email', back: '返回密码库', create: '新建账号', generate: '重新生成密码', generator: '密码生成器', generated: '生成的密码',
    scenes: [
      ['01-accounts', ['本地密码库，', '在浏览器中随手可用。'], ['查找账号，连接本机客户端。', '沿用兼容 pass 的密码库格式。']],
      ['02-details', ['需要的凭证，', '触手可及。'], ['查看账号详情，', '按需复制用户名与密码。']],
      ['03-generator', ['为每个账号，', '生成新的强密码。'], ['自定义密码长度与字符类型，', '通过本机客户端生成密码。']],
    ],
    footer: '需要运行桌面客户端 · 画面使用虚构演示账号',
  },
};
async function compose(raw, locale, scene) {
  const [name, headline, description] = scene;
  const icon = (await fs.readFile(path.join(root, 'client/assets/app-icon-monochrome.svg'))).toString('base64');
  const font = "'Avenir Next','PingFang SC','Helvetica Neue',Arial,sans-serif";
  const text = (lines, y, size, color, weight = 400) => lines.map((line, index) => `<text x="80" y="${y + index * size * 1.3}" font-family="${font}" font-size="${size}" font-weight="${weight}" fill="${color}">${escapeXML(line)}</text>`).join('');
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="800"><defs><linearGradient id="bg" x2="1" y2="1"><stop stop-color="#f8f8fa"/><stop offset="1" stop-color="#e8e8ed"/></linearGradient><filter id="shadow" x="-30%" y="-20%" width="160%" height="150%"><feDropShadow dx="0" dy="20" stdDeviation="24" flood-opacity=".12"/></filter></defs><rect width="1280" height="800" fill="url(#bg)"/><image x="74" y="64" width="72" height="72" href="data:image/svg+xml;base64,${icon}"/><text x="162" y="111" font-family="${font}" font-size="31" font-weight="600" fill="#18191b">Passpine</text>${text(headline, 305, locale === 'en' ? 46 : 42, '#18191b', 600)}${text(description, 462, 21, '#62636a')}<text x="80" y="727" font-family="${font}" font-size="15" fill="#787981">${escapeXML(copy[locale].footer)}</text><rect x="718" y="99" width="456" height="552" rx="8" fill="#ffffff" filter="url(#shadow)"/></svg>`;
  const ui = await sharp(raw).resize(456, 552).png().toBuffer();
  const destination = path.join(output, 'assets', locale, `${name}.png`);
  await fs.mkdir(path.dirname(destination), { recursive: true });
  await sharp(Buffer.from(svg)).composite([{ input: ui, left: 718, top: 99 }]).removeAlpha().png().toFile(destination);
}

try {
  await run('go', ['build', '-o', binary, '../scripts/store-demo/main.go'], { cwd: path.join(root, 'client') });
  await fs.mkdir(path.join(profile, 'NativeMessagingHosts'), { recursive: true });
  const id = 'obnlebeedcbbchadakacilopfdaifgjh'; // Native Host only authorizes this checked-in identity.
  const wrapper = path.join(base, 'host.sh');
  const quote = value => `'${value.replaceAll("'", "'\\''")}'`;
  await fs.writeFile(wrapper, `#!/bin/sh\nexec ${quote(binary)} host ${quote(data)} "$@"\n`, { mode: 0o700 });
  await fs.writeFile(path.join(profile, 'NativeMessagingHosts/com.knothhe.passpine.json'), JSON.stringify({ name: 'com.knothhe.passpine', description: 'Isolated Passpine screenshot fixture', path: wrapper, type: 'stdio', allowed_origins: [`chrome-extension://${id}/`] }));
  const ready = path.join(base, 'ready');
  helper = spawn(binary, ['serve', data, ready], { stdio: ['ignore', 'ignore', 'inherit'] });
  for (let i = 0; ; i++) {
    try { await fs.access(ready); break; }
    catch { if (i >= 100 || helper.exitCode !== null) throw new Error('Demo client failed to start'); await delay(100); }
  }
  context = await chromium.launchPersistentContext(profile, { executablePath, headless: true, viewport: { width: 1280, height: 800 }, locale: 'en-US', args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`] });
  const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker');
  if (new URL(worker.url()).hostname !== id) throw new Error('Built extension identity does not match the Native Host');
  const cdp = await context.browser().newBrowserCDPSession();
  const options = await context.newPage();
  await options.goto(`chrome-extension://${id}/options.html`);
  await options.getByRole('heading', { name: 'Connected', exact: true }).waitFor();
  await options.getByRole('radio', { name: 'Light', exact: true }).click();
  const records = [];
  for (const locale of ['en', 'zh-CN']) {
    const labels = copy[locale];
    await options.bringToFront();
    await options.getByRole('combobox', { name: 'Language', exact: true }).click();
    await options.getByRole('option', { name: locale === 'en' ? 'English' : '简体中文', exact: true }).click();
    await options.getByRole('heading', { name: labels.language, exact: true }).waitFor();
    await worker.evaluate(() => chrome.action.openPopup());
    const targets = await cdp.send('Target.getTargets');
    const target = targets.targetInfos.find(target => target.url === `chrome-extension://${id}/popup.html`);
    if (!target) throw new Error('Toolbar popup did not open');
    const { sessionId } = await cdp.send('Target.attachToTarget', { targetId: target.targetId, flatten: false });
    let sequence = 0;
    const pending = new Map();
    const received = event => {
      if (event.sessionId !== sessionId) return;
      const message = JSON.parse(event.message), waiter = pending.get(message.id);
      if (!waiter) return;
      pending.delete(message.id); clearTimeout(waiter.timer);
      message.error ? waiter.reject(new Error(message.error.message)) : waiter.resolve(message.result);
    };
    cdp.on('Target.receivedMessageFromTarget', received);
    const send = (method, params = {}) => new Promise((resolve, reject) => {
      const requestId = ++sequence;
      const timer = setTimeout(() => { pending.delete(requestId); reject(new Error(`Popup ${method} timed out`)); }, 10000);
      pending.set(requestId, { resolve, reject, timer });
      cdp.send('Target.sendMessageToTarget', { sessionId, message: JSON.stringify({ id: requestId, method, params }) }).catch(error => { clearTimeout(timer); pending.delete(requestId); reject(error); });
    });
    const evaluate = async expression => {
      const result = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });
      if (result.exceptionDetails) throw new Error(result.exceptionDetails.text);
      return result.result.value;
    };
    const wait = async expression => {
      for (let i = 0; i < 100; i++) { if (await evaluate(expression)) return; await delay(100); }
      throw new Error(`Popup condition did not become true: ${expression}`);
    };
    const click = async (value, attribute = 'aria-label') => {
      await evaluate(`(() => { const button = [...document.querySelectorAll('button')].find(el => ${attribute === 'text' ? 'el.textContent.trim()' : `el.getAttribute(${JSON.stringify(attribute)})`} === ${JSON.stringify(value)}); if (!button || button.disabled) throw new Error('Missing enabled control'); button.click(); })()`);
    };
    const capture = async index => {
      // Use real pointer input to dismiss navigation focus rings, then wait for
      // the product's transitions before capturing; never alter the UI markup.
      await send('Input.dispatchMouseEvent', { type: 'mousePressed', x: 376, y: 3, button: 'left', clickCount: 1 });
      await send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: 376, y: 3, button: 'left', clickCount: 1 });
      await evaluate(`(async () => {
        await document.fonts.ready;
        await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
        await Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})));
        await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      })()`);
      const shot = await send('Page.captureScreenshot', { format: 'png', clip: { x: 0, y: 0, width: 380, height: 460, scale: 1 } });
      const raw = Buffer.from(shot.data, 'base64');
      const name = labels.scenes[index][0];
      const rawPath = path.join(output, 'source-screenshots', locale, `${name}.png`);
      await fs.mkdir(path.dirname(rawPath), { recursive: true });
      await fs.writeFile(rawPath, raw);
      await compose(raw, locale, labels.scenes[index]);
      records.push({ locale, scene: name, source: 'Actual toolbar popup; production build; isolated real Go client with fictional credentials', viewport: '380x460', output: '1280x800' });
    };
    await wait(`document.querySelector('.footer-vault')?.textContent === 'Demo vault' && document.querySelectorAll('[data-entry-path]').length === 5 && document.documentElement.dataset.appearance === 'light'`);
    await capture(0);
    await click(labels.view);
    await wait(`document.querySelector('h1')?.textContent === ${JSON.stringify(labels.details)} && document.body.innerText.includes('alex@example.com')`);
    await capture(1);
    await click(labels.back);
    await wait(`!!document.querySelector('[data-entry-path]')`);
    await click(labels.create);
    await wait(`!!document.querySelector('#entry-password')`);
    await click(labels.generate, 'text');
    await wait(`document.querySelector('[aria-label=${JSON.stringify(labels.generated)}]')?.value?.length === 24 && !document.querySelector('.generator-screen [aria-busy="true"]')`);
    await capture(2);
    cdp.off('Target.receivedMessageFromTarget', received);
    await cdp.send('Target.closeTarget', { targetId: target.targetId });
  }
  await fs.writeFile(path.join(output, 'capture-metadata.json'), JSON.stringify({ version: manifest.version, capturedAt: new Date().toISOString(), browser: context.browser().version(), extensionId: id, fixtures: 'scripts/store-demo/main.go; all generated data is disposable; no personal vault/profile used', records }, null, 2) + '\n');
  console.log(`Captured ${records.length} genuine extension screenshots for Passpine ${manifest.version}`);
} finally {
  await context?.close();
  if (helper && helper.exitCode === null) {
    const stopped = new Promise(resolve => helper.once('exit', resolve));
    helper.kill('SIGTERM');
    await stopped;
  }
  await fs.rm(base, { recursive: true, force: true });
}
