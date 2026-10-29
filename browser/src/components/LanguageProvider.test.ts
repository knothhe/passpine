// @vitest-environment jsdom
import { act, createElement, Fragment } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { Theme } from '@radix-ui/themes';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { LanguageProvider, useLanguage } from './LanguageProvider';
import { LanguageSettings } from './LanguageSettings';
import { App } from './App';
import { EditorForm } from './EditorForm';
import { Generator } from './Generator';
import { languageStorageKey, normalizeLanguage, readLanguage, watchLanguage, writeLanguage } from '../lib/language';
import type { Client, Snapshot, UIRequest } from '../lib/types';

const extension = vi.hoisted(() => ({ storage: { local: { get: vi.fn(), set: vi.fn() }, onChanged: { addListener: vi.fn(), removeListener: vi.fn() } } }));
vi.mock('wxt/browser', () => ({ browser: extension }));
type ChangeListener = (changes: Record<string, { newValue?: unknown }>, area: string) => void;
const listeners = new Set<ChangeListener>();
let container: HTMLDivElement, root: Root, stored: Record<string, unknown>;
function externalLanguage(value: unknown) {
  stored[languageStorageKey] = value;
  for (const listener of listeners) listener({ [languageStorageKey]: { newValue: value } }, 'local');
}
function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (error: Error) => void;
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail; });
  return { promise, resolve, reject };
}
function Preference() {
  return createElement('output', { 'data-language': true }, useLanguage().language);
}
beforeEach(() => {
  vi.clearAllMocks(); listeners.clear(); localStorage.clear();
  stored = { [languageStorageKey]: 'zh-CN', appearancePreferences: { style: 'onevra', appearance: 'dark', fontScale: 110 } };
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  Element.prototype.scrollIntoView = vi.fn();
  extension.storage.local.get.mockImplementation(async () => ({ ...stored }));
  extension.storage.local.set.mockImplementation(async value => {
    stored = { ...stored, ...value };
    if (languageStorageKey in value) externalLanguage(value[languageStorageKey]);
  });
  extension.storage.onChanged.addListener.mockImplementation(listener => listeners.add(listener));
  extension.storage.onChanged.removeListener.mockImplementation(listener => listeners.delete(listener));
  container = document.createElement('div'); document.body.append(container); root = createRoot(container);
  document.documentElement.dataset.copyTitle = 'options';
});
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); delete document.documentElement.dataset.copyTitle;
  expect(listeners.size).toBe(0); vi.restoreAllMocks(); vi.unstubAllGlobals();
});
async function render(content?: React.ReactNode) {
  const settings = createElement(LanguageProvider, { children: createElement(Theme, {}, createElement(LanguageSettings), createElement(Preference)) });
  const popup = content && createElement(LanguageProvider, { children: createElement(Theme, {}, content) });
  await act(async () => root.render(createElement(Fragment, {}, settings, popup)));
}
async function choose(label: string) {
  const trigger = container.querySelector<HTMLButtonElement>('[role="combobox"]')!;
  await act(async () => trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true })));
  const option = Array.from(document.querySelectorAll<HTMLElement>('[role="option"]')).find(element => element.textContent === label)!;
  expect(option).toBeDefined();
  await act(async () => option.click());
}
function clientFixture() {
  const state: Snapshot = { configured: true, locked: false, vaultId: 'vault', vaultName: '中文密码库', entriesVersion: 1, capabilities: [], matches: [], entries: [{ path: '中文/账号', name: '账号', folder: '中文' }] };
  const request = vi.fn(async (request: UIRequest) => request.action === 'generate' ? { password: 'generated-unchanged' } : state);
  const client: Client = { demo: false, request: async <T>(value: UIRequest) => await request(value) as T, copy: async () => {}, openOptions: async () => {} };
  return { client, request };
}

describe('extension language settings', () => {
  it('waits for the saved language before enabling edits and rolls a failed save back to that value', async () => {
    const initial = deferred<Record<string, unknown>>();
    extension.storage.local.get.mockReturnValueOnce(initial.promise);
    await render();
    expect(container.querySelector<HTMLButtonElement>('[role="combobox"]')?.disabled).toBe(true);
    expect(extension.storage.local.set).not.toHaveBeenCalled();
    await act(async () => initial.resolve({ [languageStorageKey]: 'en' }));
    expect(container.querySelector<HTMLButtonElement>('[role="combobox"]')?.disabled).toBe(false);
    expect(container.querySelector('[data-language]')?.textContent).toBe('en');
    extension.storage.local.set.mockRejectedValueOnce(new Error('Storage unavailable'));
    await choose('简体中文');
    expect(container.querySelector('[data-language]')?.textContent).toBe('en');
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Storage unavailable');
  });

  it('switches both open contexts, persists only the language and restores it after reopening', async () => {
    const { client, request } = clientFixture();
    await render(createElement(App, { client }));
    expect(container.querySelector('input[aria-label="搜索密码库"]')).not.toBeNull();
    const requests = request.mock.calls.length;
    await choose('English');
    expect(container.querySelector('input[aria-label="Search vault"]')).not.toBeNull();
    expect(container.textContent).toContain('No website open'); expect(container.textContent).toContain('中文密码库');
    expect(container.textContent).toContain('账号');
    expect(request.mock.calls.length).toBe(requests);
    expect(document.documentElement.lang).toBe('en'); expect(document.title).toBe('Passpine · Browser settings');
    expect(stored).toEqual({ [languageStorageKey]: 'en', appearancePreferences: { style: 'onevra', appearance: 'dark', fontScale: 110 } });
    await act(async () => root.unmount()); root = createRoot(container); await render(createElement(App, { client }));
    expect(container.querySelector('input[aria-label="Search vault"]')).not.toBeNull();
    await choose('简体中文');
    expect(container.querySelector('input[aria-label="搜索密码库"]')).not.toBeNull();
  });

  it('keeps unsaved entry inputs and custom field labels when the language changes', async () => {
    const { client, request } = clientFixture();
    await render(createElement(EditorForm, { client, value: { vaultId: 'vault' }, defaultURL: 'https://例子.test', close: () => {}, saved: async () => {} }));
    const input = container.querySelector<HTMLInputElement>('input[placeholder="用户名或邮箱"]')!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, '中文 user@example.test');
      input.dispatchEvent(new Event('input', { bubbles: true }));
      Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(button => button.textContent === '添加字段')!.click();
    });
    await choose('English');
    expect(container.querySelector<HTMLInputElement>('input[placeholder="Username or email"]')?.value).toBe('中文 user@example.test');
    expect(container.querySelector('input[aria-label="Field 1 name"]')).not.toBeNull();
    expect(container.querySelector<HTMLInputElement>('input[autocomplete="url"]')?.value).toBe('https://例子.test');
    expect(request).not.toHaveBeenCalled();
  });

  it('translates the generator without regenerating or changing the displayed password', async () => {
    const { client, request } = clientFixture();
    await render(createElement(Generator, { client, open: true, close: () => {}, usePassword: () => {}, canSave: true }));
    await choose('English');
    expect(container.querySelector<HTMLInputElement>('input[aria-label="Generated password"]')?.value).toBe('generated-unchanged');
    expect(container.querySelector('[aria-label="Lowercase letters"]')).not.toBeNull();
    expect(request).toHaveBeenCalledTimes(1);
  });

  it('translates an open unsaved-changes dialog without clearing inputs or requesting credentials', async () => {
    const { client, request } = clientFixture();
    await render(createElement(EditorForm, { client, value: { vaultId: 'vault' }, defaultURL: 'https://例子.test', close: () => {}, saved: async () => {} }));
    const input = container.querySelector<HTMLInputElement>('input[placeholder="用户名或邮箱"]')!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, '中文 user@example.test');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await act(async () => container.querySelector<HTMLButtonElement>('form button[aria-label="返回密码库"]')!.click());
    expect(document.querySelector('[role="alertdialog"]')?.textContent).toContain('放弃未保存的修改？');
    await act(async () => externalLanguage('en'));
    expect(document.querySelector('[role="alertdialog"]')?.textContent).toContain('Discard unsaved changes?');
    expect(container.querySelector<HTMLInputElement>('input[placeholder="Username or email"]')?.value).toBe('中文 user@example.test');
    expect(request).not.toHaveBeenCalled();
  });

  it('rolls back an unsuccessful save and lets the user retry without changing appearance settings', async () => {
    await render();
    extension.storage.local.set.mockRejectedValueOnce(new Error('Storage unavailable'));
    await choose('English');
    expect(container.querySelector('[data-language]')?.textContent).toBe('zh-CN');
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Storage unavailable');
    expect(stored[languageStorageKey]).toBe('zh-CN');
    await choose('English'); expect(stored[languageStorageKey]).toBe('en');
    expect(stored.appearancePreferences).toEqual({ style: 'onevra', appearance: 'dark', fontScale: 110 });
  });

  it('does not roll back a newer cross-page choice when an older save fails', async () => {
    await render(); const pending = deferred<void>();
    extension.storage.local.set.mockReturnValueOnce(pending.promise);
    await choose('English');
    await act(async () => externalLanguage('system'));
    await act(async () => pending.reject(new Error('Old write failed')));
    expect(container.querySelector('[data-language]')?.textContent).toBe('system');
    expect(stored[languageStorageKey]).toBe('system');
  });

  it('ignores an initial read that arrives after a storage change', async () => {
    const initial = deferred<Record<string, unknown>>(); extension.storage.local.get.mockReturnValueOnce(initial.promise);
    await render();
    await act(async () => externalLanguage('en'));
    await act(async () => initial.resolve({ [languageStorageKey]: 'zh-CN' }));
    expect(container.querySelector('[data-language]')?.textContent).toBe('en');
    expect(container.textContent).toContain('Language');
  });

  it('follows browser language changes only in system mode and reports failed reads without writing defaults', async () => {
    vi.spyOn(navigator, 'language', 'get').mockReturnValue('fr-FR');
    extension.storage.local.get.mockRejectedValueOnce(new Error('Read failed'));
    await render();
    expect(container.textContent).toContain('Could not load language settings');
    expect(document.documentElement.lang).toBe('en'); expect(extension.storage.local.set).not.toHaveBeenCalled();
    vi.spyOn(navigator, 'language', 'get').mockReturnValue('zh-Hans');
    await act(async () => window.dispatchEvent(new Event('languagechange')));
    expect(document.documentElement.lang).toBe('zh-CN');
    await choose('English');
    await act(async () => window.dispatchEvent(new Event('languagechange')));
    expect(document.documentElement.lang).toBe('en');
  });

  it('isolates preview language preferences and recovers missing or invalid values', async () => {
    expect(normalizeLanguage(undefined)).toBe('system'); expect(normalizeLanguage('fr')).toBe('system');
    const changed = vi.fn(), unwatch = watchLanguage(changed, true);
    try {
      await writeLanguage('en', true);
      expect(await readLanguage(true)).toBe('en'); expect(changed).toHaveBeenLastCalledWith('en');
      expect(extension.storage.local.set).not.toHaveBeenCalled();
      localStorage.setItem(languageStorageKey, 'invalid'); expect(await readLanguage(true)).toBe('system');
    } finally { unwatch(); }
  });
});
