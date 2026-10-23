// @vitest-environment jsdom
import { act, createElement, type ReactNode } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { Theme } from '@radix-ui/themes';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ClientTargetSettings, useClientTarget } from './ClientTargetSettings';
import { LanguageProvider } from './LanguageProvider';
import { Options } from './Options';
import { CLIENT_TARGET_STORAGE_KEY } from '../lib/client-target';
import { languageStorageKey } from '../lib/language';
import type { Client, Snapshot } from '../lib/types';

const extension = vi.hoisted(() => ({ management: { getSelf: vi.fn() }, storage: { local: { get: vi.fn(), set: vi.fn() }, onChanged: { addListener: vi.fn(), removeListener: vi.fn() } } }));
vi.mock('wxt/browser', () => ({ browser: extension }));
vi.mock('./AppearanceSettings', () => ({ AppearanceSettings: () => null }));
vi.mock('./LanguageSettings', () => ({ LanguageSettings: () => null }));
vi.mock('./ShortcutSettings', () => ({ ShortcutSettings: () => null }));
vi.mock('./Primitives', () => ({ Brand: () => null }));
vi.mock('./About', () => ({ About: () => null }));

type Listener = (changes: Record<string, { newValue?: unknown }>, area: string) => void;
const listeners = new Set<Listener>();
let container: HTMLDivElement, root: Root, stored: Record<string, unknown>;
const choice = (text: string) => Array.from(container.querySelectorAll<HTMLButtonElement>('[role="radio"]')).find(element => element.textContent?.includes(text))!;
const emit = (key: string, value: unknown) => { stored[key] = value; for (const listener of listeners) listener({ [key]: { newValue: value } }, 'local'); };
function Settings() { return createElement(ClientTargetSettings, { state: useClientTarget() }); }
async function render(child: ReactNode = createElement(Settings)) { await act(async () => root.render(createElement(LanguageProvider, { children: createElement(Theme, {}, child) }))); }
const state: Snapshot = { configured: true, vaultId: 'vault', vaultName: 'Personal', locked: false, entries: [], matches: [], capabilities: [] };
const makeClient = (): Client => ({ demo: false, request: vi.fn().mockResolvedValue(state), openOptions: vi.fn(), copy: vi.fn() });

beforeEach(() => {
  vi.clearAllMocks(); listeners.clear(); stored = {};
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  Element.prototype.scrollIntoView = vi.fn();
  extension.management.getSelf.mockResolvedValue({ installType: 'development' });
  extension.storage.local.get.mockImplementation(async () => ({ ...stored }));
  extension.storage.local.set.mockImplementation(async value => { for (const [key, next] of Object.entries(value)) emit(key, next); });
  extension.storage.onChanged.addListener.mockImplementation(listener => listeners.add(listener));
  extension.storage.onChanged.removeListener.mockImplementation(listener => listeners.delete(listener));
  container = document.createElement('div'); document.body.append(container); root = createRoot(container);
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.unstubAllGlobals(); });

describe('client target settings', () => {
  it('defaults to release, persists a development selection and restores it after reopening', async () => {
    await render(); expect(choice('正式版').getAttribute('aria-checked')).toBe('true');
    await act(async () => choice('Dev 版').click());
    expect(stored[CLIENT_TARGET_STORAGE_KEY]).toBe('dev'); expect(choice('Dev 版').getAttribute('aria-checked')).toBe('true');
    await act(async () => root.unmount()); root = createRoot(container); await render();
    expect(choice('Dev 版').getAttribute('aria-checked')).toBe('true');
  });

  it('keeps the previous client while saving and after failure, then allows a successful retry', async () => {
    await render();
    let reject!: (error: Error) => void;
    extension.storage.local.set.mockImplementationOnce(() => new Promise<void>((_resolve, fail) => { reject = fail; }));
    await act(async () => choice('Dev 版').click());
    expect(choice('正式版').getAttribute('aria-checked')).toBe('true'); expect(choice('Dev 版').disabled).toBe(true);
    await act(async () => reject(new Error('storage failed')));
    expect(choice('正式版').getAttribute('aria-checked')).toBe('true'); expect(choice('Dev 版').disabled).toBe(false);
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('仍使用原来的客户端');
    await act(async () => choice('Dev 版').click()); expect(choice('Dev 版').getAttribute('aria-checked')).toBe('true'); expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it('keeps release selected when installation metadata fails during a save without a storage notification', async () => {
    const client = makeClient(); await render(createElement(Options, { client, extensionId: 'extension-id' }));
    extension.management.getSelf.mockRejectedValueOnce(new Error('temporarily unavailable'));
    extension.storage.local.set.mockResolvedValueOnce(undefined);
    await act(async () => choice('Dev 版').click());
    expect(extension.storage.local.set).toHaveBeenLastCalledWith({ [CLIENT_TARGET_STORAGE_KEY]: 'release' });
    expect(choice('正式版').getAttribute('aria-checked')).toBe('true');
    expect(container.textContent).not.toContain('com.knothhe.passpine.dev'); expect(client.request).toHaveBeenCalledTimes(1);
  });

  it('ignores stale initial reads and save completions after a cross-page selection', async () => {
    let initial!: (value: unknown) => void;
    extension.storage.local.get.mockImplementation(key => key === CLIENT_TARGET_STORAGE_KEY ? new Promise(resolve => { initial = resolve; }) : Promise.resolve({}));
    await render(); await act(async () => emit(CLIENT_TARGET_STORAGE_KEY, 'dev'));
    await act(async () => initial({ [CLIENT_TARGET_STORAGE_KEY]: 'release' }));
    expect(choice('Dev 版').getAttribute('aria-checked')).toBe('true');
    let finish!: () => void;
    extension.storage.local.set.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve; }));
    await act(async () => choice('正式版').click());
    await act(async () => emit(CLIENT_TARGET_STORAGE_KEY, 'dev'));
    await act(async () => finish()); expect(choice('Dev 版').getAttribute('aria-checked')).toBe('true');
  });

  it('shows a localized recoverable read error with the release fallback', async () => {
    extension.storage.local.get.mockImplementation(key => key === CLIENT_TARGET_STORAGE_KEY ? Promise.reject(new Error('read failed')) : Promise.resolve({}));
    await render(); expect(choice('正式版').getAttribute('aria-checked')).toBe('true');
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('无法读取客户端设置');
    await act(async () => emit(languageStorageKey, 'en'));
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Could not load client settings');
    await act(async () => choice('Dev').click()); expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it.each(['normal', 'admin'])('hides the client selector for a %s installation and rejects injected dev storage', async installType => {
    extension.management.getSelf.mockResolvedValue({ installType }); stored[CLIENT_TARGET_STORAGE_KEY] = 'dev';
    const client = makeClient(); await render(createElement(Options, { client, extensionId: 'extension-id' }));
    expect(container.querySelector('[role="radiogroup"]')).toBeNull(); expect(container.textContent).toContain('连接 Passpine 正式客户端');
    expect(container.textContent).toContain('com.knothhe.passpine'); expect(container.textContent).not.toContain('com.knothhe.passpine.dev');
    await act(async () => emit(CLIENT_TARGET_STORAGE_KEY, 'dev'));
    expect(container.textContent).not.toContain('com.knothhe.passpine.dev'); expect(client.request).toHaveBeenCalledTimes(1);
  });

  it('refreshes diagnostics and connection once when a saved target changes, without restarting on a language change', async () => {
    const client = makeClient(); await render(createElement(Options, { client, extensionId: 'extension-id' }));
    expect(client.request).toHaveBeenCalledTimes(1);
    await act(async () => choice('Dev 版').click());
    expect(container.textContent).toContain('com.knothhe.passpine.dev'); expect(client.request).toHaveBeenCalledTimes(2);
    await act(async () => emit(languageStorageKey, 'en'));
    expect(choice('Dev').getAttribute('aria-checked')).toBe('true'); expect(client.request).toHaveBeenCalledTimes(2);
    extension.storage.local.set.mockRejectedValueOnce(new Error('failed'));
    await act(async () => choice('Release').click()); expect(client.request).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain('com.knothhe.passpine.dev');
  });

  it('renews an invalidated connection even when a storage notification keeps the effective target unchanged', async () => {
    extension.management.getSelf.mockResolvedValue({ installType: 'normal' });
    let generation = 0;
    const request = vi.fn(async (captured: number) => {
      if (captured !== generation) throw new Error('native_connection_failed');
      return state;
    });
    const client = makeClient();
    client.scope = () => {
      const captured = generation;
      return { ...client, request: (() => request(captured)) as Client['request'] };
    };
    await render(createElement(Options, { client, extensionId: 'extension-id' }));
    await act(async () => { generation++; emit(CLIENT_TARGET_STORAGE_KEY, 'dev'); });
    const refresh = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(button => button.textContent?.includes('检查连接'))!;
    await act(async () => refresh.click());
    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(container.textContent).toContain('Personal');
    expect(request).toHaveBeenCalledTimes(3);
  });

  it('discards an old connection result after selecting a different client', async () => {
    const client = makeClient(); let finish!: (value: Snapshot) => void;
    vi.mocked(client.request).mockImplementationOnce(() => new Promise(resolve => { finish = resolve as (value: Snapshot) => void; }));
    await render(createElement(Options, { client, extensionId: 'extension-id' }));
    vi.mocked(client.request).mockResolvedValueOnce({ ...state, vaultName: 'Dev vault' });
    await act(async () => choice('Dev 版').click()); await act(async () => finish({ ...state, vaultName: 'Release vault' }));
    expect(container.textContent).toContain('Dev vault'); expect(container.textContent).not.toContain('Release vault');
  });
});
