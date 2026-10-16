// @vitest-environment jsdom
import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AppearanceSettings } from './AppearanceSettings';
import { Provider } from './Provider';
import { Brand } from './Primitives';
import { Select } from '@radix-ui/themes';
import { appearanceStorageKey, defaultAppearancePreferences } from '../lib/appearance';

const extension = vi.hoisted(() => ({ storage: { local: { get: vi.fn(), set: vi.fn() }, onChanged: { addListener: vi.fn(), removeListener: vi.fn() } } }));
vi.mock('wxt/browser', () => ({ browser: extension }));
let container: HTMLDivElement, root: Root;
let stored: Record<string, unknown>, systemDark: boolean;
type ChangeListener = (changes: Record<string, { newValue?: unknown }>, area: string) => void;
const listeners = new Set<ChangeListener>(), systemListeners = new Set<() => void>();
const radio = (label: string) => container.querySelector<HTMLButtonElement>(`[role="radio"][aria-label="${label}"]`)!;
const mode = (label: string) => Array.from(container.querySelectorAll<HTMLButtonElement>('[role="radio"]')).find(element => element.textContent?.includes(label))!;
const rootTheme = () => container.querySelector<HTMLElement>('[data-is-root-theme="true"]')!;

beforeEach(() => {
  vi.clearAllMocks(); listeners.clear(); systemListeners.clear(); stored = {}; systemDark = false;
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  Element.prototype.scrollIntoView = vi.fn();
  vi.stubGlobal('matchMedia', () => ({ get matches() { return systemDark; }, addEventListener: (_: string, listener: () => void) => systemListeners.add(listener), removeEventListener: (_: string, listener: () => void) => systemListeners.delete(listener) }));
  extension.storage.local.get.mockImplementation(async () => stored);
  extension.storage.local.set.mockImplementation(async value => {
    stored = value;
    for (const listener of listeners) listener({ [appearanceStorageKey]: { newValue: value[appearanceStorageKey] } }, 'local');
  });
  extension.storage.onChanged.addListener.mockImplementation(listener => listeners.add(listener));
  extension.storage.onChanged.removeListener.mockImplementation(listener => listeners.delete(listener));
  container = document.createElement('div'); document.body.append(container); root = createRoot(container);
  const favicon = document.createElement('link'); favicon.rel = 'icon'; document.head.append(favicon);
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); document.head.querySelector('link[rel="icon"]')?.remove(); vi.unstubAllGlobals(); });
async function render() { await act(async () => root.render(createElement(Provider, { children: createElement('div', {}, createElement(Brand), createElement(AppearanceSettings), createElement(Select.Root, { defaultOpen: true, defaultValue: 'one' }, createElement(Select.Trigger, { 'aria-label': '测试菜单' }), createElement(Select.Content, {}, createElement(Select.Item, { value: 'one' }, '选项')))) }))); }

describe('appearance settings', () => {
  it('restores older style preferences with default text size and persists text changes across reopening', async () => {
    stored = { [appearanceStorageKey]: { style: 'onevra', appearance: 'dark' } };
    await render();
    expect(rootTheme().dataset.accentColor).toBe('orange');
    expect(rootTheme().classList.contains('dark')).toBe(true);
    expect(mode('100%').getAttribute('aria-checked')).toBe('true');
    expect(document.documentElement.style.getPropertyValue('--pine-font-scale')).toBe('1');
    await act(async () => mode('120%').click());
    expect(document.documentElement.style.getPropertyValue('--pine-font-scale')).toBe('1.2');
    expect(stored[appearanceStorageKey]).toEqual({ style: 'onevra', appearance: 'dark', fontScale: 120 });
    await act(async () => root.unmount()); root = createRoot(container); await render();
    expect(mode('120%').getAttribute('aria-checked')).toBe('true');
    expect(document.documentElement.style.getPropertyValue('--pine-font-scale')).toBe('1.2');
    await act(async () => { for (const listener of listeners) listener({ [appearanceStorageKey]: { newValue: { style: 'onevra', appearance: 'dark', fontScale: 80 } } }, 'local'); });
    expect(mode('80%').getAttribute('aria-checked')).toBe('true');
    expect(document.documentElement.style.getPropertyValue('--pine-font-scale')).toBe('0.8');
    // The global text scale also reaches body-level Radix portal themes.
    expect(document.body.querySelector('[role="listbox"]')?.closest('.radix-themes')).not.toBeNull();
  });

  it('applies text size before storage completes and restores the selection if saving fails', async () => {
    await render();
    let reject!: (error: Error) => void;
    extension.storage.local.set.mockImplementationOnce(() => new Promise<void>((_resolve, fail) => { reject = fail; }));
    await act(async () => mode('120%').click());
    expect(document.documentElement.style.getPropertyValue('--pine-font-scale')).toBe('1.2');
    expect(mode('120%').getAttribute('aria-checked')).toBe('true');
    await act(async () => reject(new Error('无法保存文字大小')));
    expect(document.documentElement.style.getPropertyValue('--pine-font-scale')).toBe('1');
    expect(mode('100%').getAttribute('aria-checked')).toBe('true');
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('无法保存文字大小');
  });

  it('preserves newer cross-page text settings when an older optimistic save fails', async () => {
    await render();
    let reject!: (error: Error) => void;
    extension.storage.local.set.mockImplementationOnce(() => new Promise<void>((_resolve, fail) => { reject = fail; }));
    await act(async () => mode('120%').click());
    await act(async () => { for (const listener of listeners) listener({ [appearanceStorageKey]: { newValue: { style: 'onevra', appearance: 'dark', fontScale: 90 } } }, 'local'); });
    await act(async () => reject(new Error('保存失败')));
    expect(rootTheme().dataset.accentColor).toBe('orange');
    expect(mode('90%').getAttribute('aria-checked')).toBe('true');
    expect(document.documentElement.style.getPropertyValue('--pine-font-scale')).toBe('0.9');
  });

  it('switches linked UI/icon style and explicit appearance, persists it and restores it after reopening', async () => {
    await render();
    expect(rootTheme().dataset.accentColor).toBe('gray'); expect(rootTheme().classList.contains('light')).toBe(true);
    expect(radio('黑白').getAttribute('aria-checked')).toBe('true');
    expect(container.querySelector<HTMLImageElement>('.brand-mark img')?.src).toContain('app-icon-monochrome.png');
    expect(document.head.querySelector<HTMLLinkElement>('link[rel="icon"]')?.href).toContain('app-icon-monochrome.png');
    await act(async () => radio('陶土橙').click());
    expect(rootTheme().dataset.accentColor).toBe('orange');
    expect(container.querySelector<HTMLImageElement>('.brand-mark img')?.src).not.toContain('monochrome');
    await act(async () => radio('黑白').click());
    expect(rootTheme().dataset.accentColor).toBe('gray');
    expect(container.querySelector<HTMLImageElement>('.brand-mark img')?.src).toContain('app-icon-monochrome.png');
    expect(document.head.querySelector<HTMLLinkElement>('link[rel="icon"]')?.href).toContain('app-icon-monochrome.png');
    await act(async () => mode('深色').click());
    expect(stored[appearanceStorageKey]).toEqual({ style: 'monochrome', appearance: 'dark', fontScale: 100 });
    expect(rootTheme().classList.contains('dark')).toBe(true);
    // Radix renders portals in their own inherited Theme wrapper.
    const portal = document.body.querySelector<HTMLElement>('[role="listbox"]')?.closest<HTMLElement>('.radix-themes');
    expect(portal?.dataset.accentColor).toBe('gray'); expect(portal?.classList.contains('dark')).toBe(true);
    await act(async () => root.unmount()); root = createRoot(container); await render();
    expect(rootTheme().dataset.accentColor).toBe('gray'); expect(rootTheme().classList.contains('dark')).toBe(true);
  });

  it('follows system changes only in system mode and updates an already open page from storage', async () => {
    await render();
    await act(async () => { systemDark = true; for (const listener of systemListeners) listener(); });
    expect(rootTheme().classList.contains('dark')).toBe(true);
    await act(async () => mode('浅色').click());
    expect(rootTheme().classList.contains('light')).toBe(true);
    await act(async () => { for (const listener of listeners) listener({ [appearanceStorageKey]: { newValue: { style: 'monochrome', appearance: 'system' } } }, 'local'); });
    expect(rootTheme().dataset.accentColor).toBe('gray'); expect(rootTheme().classList.contains('dark')).toBe(true);
    await act(async () => { for (const listener of listeners) listener({ [appearanceStorageKey]: {} }, 'local'); });
    expect(rootTheme().dataset.accentColor).toBe('gray'); expect(mode('跟随系统').getAttribute('aria-checked')).toBe('true');
  });

  it('keeps the current selection when saving fails and shows a retryable error', async () => {
    await render(); extension.storage.local.set.mockRejectedValueOnce(new Error('无法保存外观'));
    await act(async () => radio('陶土橙').click());
    expect(rootTheme().dataset.accentColor).toBe('gray'); expect(container.querySelector('[role="alert"]')?.textContent).toContain('无法保存外观');
    await act(async () => radio('陶土橙').click());
    expect(rootTheme().dataset.accentColor).toBe('orange'); expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it('ignores a stale initial storage response after a live preference change', async () => {
    let finish!: (value: unknown) => void;
    extension.storage.local.get.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    await render();
    await act(async () => { for (const listener of listeners) listener({ [appearanceStorageKey]: { newValue: { style: 'onevra', appearance: 'dark' } } }, 'local'); });
    await act(async () => finish({ [appearanceStorageKey]: defaultAppearancePreferences }));
    expect(rootTheme().dataset.accentColor).toBe('orange'); expect(rootTheme().classList.contains('dark')).toBe(true);
  });

  it('does not let an old save completion override a newer cross-context storage update', async () => {
    await render();
    let finish!: () => void;
    extension.storage.local.set.mockImplementationOnce(value => {
      stored = value;
      for (const listener of listeners) listener({ [appearanceStorageKey]: { newValue: value[appearanceStorageKey] } }, 'local');
      return new Promise<void>(resolve => { finish = resolve; });
    });
    await act(async () => radio('陶土橙').click());
    const newer = { style: 'monochrome', appearance: 'dark' };
    await act(async () => {
      stored = { [appearanceStorageKey]: newer };
      for (const listener of listeners) listener({ [appearanceStorageKey]: { newValue: newer } }, 'local');
    });
    await act(async () => finish());
    expect(rootTheme().dataset.accentColor).toBe('gray'); expect(rootTheme().classList.contains('dark')).toBe(true);
    expect(container.querySelector<HTMLImageElement>('.brand-mark img')?.src).toContain('app-icon-monochrome.png');
  });
});
