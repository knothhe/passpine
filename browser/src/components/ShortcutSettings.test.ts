// @vitest-environment jsdom
import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ShortcutSettings } from './ShortcutSettings';
import { FILL_COMMAND, OPEN_COMMAND } from '../lib/shortcuts';
import { Theme } from '@radix-ui/themes';

const extension = vi.hoisted(() => ({ commands: { getAll: vi.fn(), update: vi.fn(), reset: vi.fn(), openShortcutSettings: vi.fn() }, tabs: { create: vi.fn() }, storage: { local: { get: vi.fn(), set: vi.fn() } } }));
vi.mock('wxt/browser', () => ({ browser: extension }));

let container: HTMLDivElement;
let root: Root;
let bindings: { name: string; shortcut: string }[];
const button = (label: string) => Array.from(container.querySelectorAll('button')).find(item => item.getAttribute('aria-label') === label || item.textContent === label)!;

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement('div'); document.body.append(container); root = createRoot(container);
  bindings = [{ name: OPEN_COMMAND, shortcut: 'Alt+L' }, { name: FILL_COMMAND, shortcut: '' }];
  extension.commands.getAll.mockImplementation(async () => bindings.map(value => ({ ...value })));
  extension.storage.local.get.mockResolvedValue({});
  extension.storage.local.set.mockResolvedValue(undefined);
  extension.commands.update.mockImplementation(async ({ name, shortcut }: { name: string; shortcut: string }) => { bindings = bindings.map(value => value.name === name ? { name, shortcut } : value); });
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.unstubAllGlobals(); });

async function renderSettings(firefox: boolean) { await act(async () => root.render(createElement(Theme, {}, createElement(ShortcutSettings, { firefox })))); }

describe('shortcut settings interactions', () => {
  it('shows browser-assigned values, opens the customization page and refreshes after returning', async () => {
    await renderSettings(false);
    expect(button('修改打开 Passpine Dev快捷键').textContent).toBe('AltL');
    expect(container.textContent).toContain('未设置');
    await act(async () => button('修改打开 Passpine Dev快捷键').click());
    expect(extension.tabs.create).toHaveBeenCalledWith({ url: 'chrome://extensions/shortcuts' });
    bindings[0]!.shortcut = 'Alt+K';
    await act(async () => window.dispatchEvent(new Event('focus')));
    expect(button('修改打开 Passpine Dev快捷键').textContent).toBe('AltK');
  });

  it('records and saves a Firefox command and keeps Tab available for keyboard navigation', async () => {
    await renderSettings(true);
    await act(async () => button('修改打开 Passpine Dev快捷键').click());
    const input = button('修改打开 Passpine Dev快捷键');
    const tab = new KeyboardEvent('keydown', { key: 'Tab', code: 'Tab', bubbles: true, cancelable: true });
    await act(async () => input.dispatchEvent(tab));
    expect(tab.defaultPrevented).toBe(false);
    expect(input.getAttribute('aria-pressed')).toBe('false');
    await act(async () => input.click());
    await act(async () => input.dispatchEvent(new KeyboardEvent('keydown', { key: 'K', code: 'KeyK', altKey: true, bubbles: true, cancelable: true })));
    expect(extension.commands.update).toHaveBeenCalledWith({ name: OPEN_COMMAND, shortcut: 'Alt+K' });
    expect(container.querySelector('input')).toBeNull();
    expect(container.textContent).toContain('快捷键已保存');
  });

  it('keeps an unsuccessful Firefox edit open and exposes a retryable error', async () => {
    extension.commands.update.mockRejectedValue(new Error('快捷键冲突'));
    await renderSettings(true);
    await act(async () => button('修改打开 Passpine Dev快捷键').click());
    await act(async () => button('修改打开 Passpine Dev快捷键').dispatchEvent(new KeyboardEvent('keydown', { key: 'K', code: 'KeyK', altKey: true, bubbles: true, cancelable: true })));
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('快捷键冲突');
    expect(container.textContent).not.toContain('快捷键已保存');
  });

  it('adds and records a popup action on Chromium without modifying browser commands', async () => {
    await renderSettings(false);
    await act(async () => button('添加快捷键').click());
    const recorder = button('修改新建账号快捷键');
    await act(async () => recorder.click());
    await act(async () => recorder.dispatchEvent(new KeyboardEvent('keydown', { key: 'N', code: 'KeyN', altKey: true, bubbles: true, cancelable: true })));
    expect(extension.storage.local.set).toHaveBeenLastCalledWith({ popupShortcuts: [expect.objectContaining({ action: 'search', shortcut: 'Ctrl+K' }), expect.objectContaining({ action: 'new', shortcut: 'Alt+N' })] });
    expect(extension.commands.update).not.toHaveBeenCalled();
    await act(async () => button('删除新建账号快捷键').click());
    expect(extension.storage.local.set).toHaveBeenLastCalledWith({ popupShortcuts: [expect.objectContaining({ action: 'search' })] });
  });

  it('rejects a popup binding that conflicts with a browser command and permits retry', async () => {
    await renderSettings(false);
    const recorder = button('修改搜索账号快捷键');
    await act(async () => recorder.click());
    await act(async () => recorder.dispatchEvent(new KeyboardEvent('keydown', { key: 'L', code: 'KeyL', altKey: true, bubbles: true, cancelable: true })));
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('此快捷键已用于另一项操作');
    expect(extension.storage.local.set).not.toHaveBeenCalled();
    await act(async () => recorder.click());
    await act(async () => recorder.dispatchEvent(new KeyboardEvent('keydown', { key: 'Backspace', code: 'Backspace', bubbles: true, cancelable: true })));
    expect(extension.storage.local.set).toHaveBeenCalledWith({ popupShortcuts: [expect.objectContaining({ action: 'search', shortcut: '' })] });
  });
});
