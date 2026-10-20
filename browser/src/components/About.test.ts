// @vitest-environment jsdom
import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { About } from './About';
import { Options } from './Options';
import { chineseCopy, type Copy } from '../constants/copy';
import { englishCopy } from '../i18n/en';
import { version } from '../../package.json';
import type { Client } from '../lib/types';

let copy: Copy, container: HTMLDivElement, root: Root;
vi.mock('./LanguageProvider', () => ({ useCopy: () => copy, useLanguage: () => ({ locale: copy === englishCopy ? 'en' : 'zh-CN' }) }));
vi.mock('./Primitives', () => ({ Brand: () => 'Passpine', ApplicationIcon: () => null }));
vi.mock('./ShortcutSettings', () => ({ ShortcutSettings: () => null }));
vi.mock('./AppearanceSettings', () => ({ AppearanceSettings: () => null }));
vi.mock('./LanguageSettings', () => ({ LanguageSettings: () => null }));
beforeEach(() => {
  copy = chineseCopy;
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  container = document.createElement('div'); document.body.append(container); root = createRoot(container);
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); });

it('shows only extension version and browser identity, with no dependency information', async () => {
  const extensionId = 'test-extension-用户标识';
  await act(async () => root.render(createElement(About, { extensionId })));
  expect(container.textContent).toContain(chineseCopy.about.version(version));
  expect(container.textContent).toContain(chineseCopy.about.chromium);
  expect(container.textContent).toContain(extensionId);
  expect(container.querySelectorAll('details')).toHaveLength(0);
  expect(container.textContent).not.toContain('go-crypto');
  expect(container.querySelector('a')?.href).toBe('https://github.com/knothhe/passpine');
  expect(container.querySelector('a')?.rel).toContain('noreferrer');
  copy = englishCopy;
  await act(async () => root.render(createElement(About, { extensionId, firefox: true })));
  expect(container.querySelector('h1')?.textContent).toBe('About');
  expect(container.textContent).toContain('Firefox');
  expect(container.textContent).toContain(extensionId);
});

it('switches between settings and About without restarting the client request', async () => {
  const request = vi.fn(async () => ({ configured: false, locked: true, vaultId: '', vaultName: '', entries: [], matches: [], capabilities: [] }));
  const client = { demo: true, request, copy: vi.fn() } as unknown as Client;
  await act(async () => root.render(createElement(Options, { client, extensionId: 'test-extension' })));
  const button = (label: string) => Array.from(container.querySelectorAll('nav button')).find(item => item.textContent === label) as HTMLButtonElement;
  await act(async () => button('关于').click());
  expect(document.title).toBe(chineseCopy.about.pageTitle);
  expect(container.textContent).toContain(chineseCopy.about.name);
  expect(container.querySelectorAll('details')).toHaveLength(0);
  await act(async () => button('设置').click());
  expect(container.querySelector('h1')?.textContent).toBe(chineseCopy.options.title);
  expect(request).toHaveBeenCalledTimes(1);
});

it('copies the public extension ID with the browser even when the native client is unavailable', async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
  const client = { demo: false, request: vi.fn().mockRejectedValue(new Error('unavailable')), copy: vi.fn().mockRejectedValue(new Error('must not copy public IDs as passwords')) } as unknown as Client;
  await act(async () => root.render(createElement(Options, { client, extensionId: 'test-extension' })));
  const button = Array.from(container.querySelectorAll('button')).find(item => item.textContent?.includes(chineseCopy.options.copyExtensionID)) as HTMLButtonElement;
  await act(async () => button.click());
  expect(writeText).toHaveBeenCalledExactlyOnceWith('test-extension');
  expect(client.copy).not.toHaveBeenCalled();
  expect(button.textContent).toContain(chineseCopy.common.copied);
});
