// @vitest-environment jsdom
import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ShortcutSettings } from './ShortcutSettings';
import { FILL_COMMAND, OPEN_COMMAND } from '../lib/shortcuts';

const extension = vi.hoisted(() => ({ commands: { getAll: vi.fn(), update: vi.fn(), reset: vi.fn(), openShortcutSettings: vi.fn() }, tabs: { create: vi.fn() } }));
vi.mock('wxt/browser', () => ({ browser: extension }));

let container: HTMLDivElement;
let root: Root;
let bindings: { name: string; shortcut: string }[];
const button = (label: string) => Array.from(container.querySelectorAll('button')).find(item => item.getAttribute('aria-label') === label || item.textContent === label)!;

beforeEach(() => {
  vi.clearAllMocks();
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement('div'); document.body.append(container); root = createRoot(container);
  bindings = [{ name: OPEN_COMMAND, shortcut: 'Alt+L' }, { name: FILL_COMMAND, shortcut: '' }];
  extension.commands.getAll.mockImplementation(async () => bindings.map(value => ({ ...value })));
  extension.commands.update.mockImplementation(async ({ name, shortcut }: { name: string; shortcut: string }) => { bindings = bindings.map(value => value.name === name ? { name, shortcut } : value); });
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); });

describe('shortcut settings interactions', () => {
  it('shows browser-assigned values, opens the customization page and refreshes after returning', async () => {
    await act(async () => root.render(createElement(ShortcutSettings, { firefox: false })));
    expect(container.textContent).toContain('Alt + L');
    expect(container.textContent).toContain('未设置');
    await act(async () => button('自定义快捷键').click());
    expect(extension.tabs.create).toHaveBeenCalledWith({ url: 'chrome://extensions/shortcuts' });
    bindings[0]!.shortcut = 'Alt+K';
    await act(async () => window.dispatchEvent(new Event('focus')));
    expect(container.textContent).toContain('Alt + K');
  });

  it('records and saves a Firefox command and keeps Tab available for keyboard navigation', async () => {
    await act(async () => root.render(createElement(ShortcutSettings, { firefox: true })));
    await act(async () => button('修改打开 Passpine Neo快捷键').click());
    const input = container.querySelector('input')!;
    const tab = new KeyboardEvent('keydown', { key: 'Tab', code: 'Tab', bubbles: true, cancelable: true });
    await act(async () => input.dispatchEvent(tab));
    expect(tab.defaultPrevented).toBe(false);
    await act(async () => input.dispatchEvent(new KeyboardEvent('keydown', { key: 'K', code: 'KeyK', altKey: true, bubbles: true, cancelable: true })));
    expect(input.value).toBe('Alt + K');
    await act(async () => button('保存').click());
    expect(extension.commands.update).toHaveBeenCalledWith({ name: OPEN_COMMAND, shortcut: 'Alt+K' });
    expect(container.querySelector('input')).toBeNull();
    expect(container.textContent).toContain('快捷键已保存。');
  });

  it('keeps an unsuccessful Firefox edit open and exposes a retryable error', async () => {
    extension.commands.update.mockRejectedValue(new Error('快捷键冲突'));
    await act(async () => root.render(createElement(ShortcutSettings, { firefox: true })));
    await act(async () => button('修改打开 Passpine Neo快捷键').click());
    await act(async () => button('保存').click());
    expect(container.querySelector('input')).not.toBeNull();
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('快捷键冲突');
    expect(container.textContent).not.toContain('快捷键已保存。');
  });
});
