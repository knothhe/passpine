// @vitest-environment jsdom
import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ShortcutSettings } from './ShortcutSettings';
import { FILL_COMMAND, OPEN_COMMAND } from '../lib/shortcuts';
import { chineseCopy } from '../constants/copy';
import { Theme } from '@radix-ui/themes';

const extension = vi.hoisted(() => ({ commands: { getAll: vi.fn(), update: vi.fn(), reset: vi.fn() }, tabs: { create: vi.fn() }, storage: { local: { get: vi.fn(), set: vi.fn() }, onChanged: { addListener: vi.fn(), removeListener: vi.fn() } }, permissions: { contains: vi.fn(), request: vi.fn(), remove: vi.fn(), onAdded: { addListener: vi.fn(), removeListener: vi.fn() }, onRemoved: { addListener: vi.fn(), removeListener: vi.fn() } } }));
vi.mock('wxt/browser', () => ({ browser: extension }));

let container: HTMLDivElement;
let root: Root;
let bindings: { name: string; shortcut: string }[];
const button = (label: string) => Array.from(container.querySelectorAll('button')).find(item => item.getAttribute('aria-label') === label || item.textContent === label)!;

beforeEach(() => {
  vi.clearAllMocks();
  extension.permissions.contains.mockResolvedValue(false); extension.permissions.request.mockResolvedValue(true); extension.permissions.remove.mockResolvedValue(true);
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement('div'); document.body.append(container); root = createRoot(container);
  bindings = [{ name: OPEN_COMMAND, shortcut: 'Alt+L' }, { name: FILL_COMMAND, shortcut: '' }];
  extension.commands.getAll.mockImplementation(async () => bindings.map(value => ({ ...value })));
  extension.storage.local.get.mockResolvedValue({});
  extension.storage.local.set.mockResolvedValue(undefined);
  extension.commands.update.mockImplementation(async ({ name, shortcut }: { name: string; shortcut: string }) => { bindings = bindings.map(value => value.name === name ? { name, shortcut } : value); });
  extension.commands.reset.mockImplementation(async (name: string) => { bindings = bindings.map(value => value.name === name ? { name, shortcut: name === OPEN_COMMAND ? 'Ctrl+Shift+L' : 'Ctrl+Shift+F' } : value); });
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

async function renderSettings(firefox: boolean) { await act(async () => root.render(createElement(Theme, {}, createElement(ShortcutSettings, { firefox })))); }

describe('shortcut settings interactions', () => {
  it('records page shortcuts on Chromium directly without opening browser settings or changing browser commands', async () => {
    await renderSettings(false);
    const recorder = button('修改打开 Passpine快捷键');
    expect(recorder.querySelector('kbd:last-child')?.textContent).toBe('L');
    await act(async () => recorder.click());
    await act(async () => recorder.dispatchEvent(new KeyboardEvent('keydown', { key: 'K', code: 'KeyK', altKey: true, bubbles: true, cancelable: true })));
    expect(extension.storage.local.set).toHaveBeenCalledWith({ pageShortcuts: expect.objectContaining({ [OPEN_COMMAND]: 'Alt+K' }) });
    expect(recorder.querySelector('kbd:last-child')?.textContent).toBe('K');
    expect(extension.tabs.create).not.toHaveBeenCalled();
    expect(extension.commands.update).not.toHaveBeenCalled();
  });

  it('records and saves a Firefox command and keeps Tab available for keyboard navigation', async () => {
    await renderSettings(true);
    await act(async () => button('修改打开 Passpine快捷键').click());
    const input = button('修改打开 Passpine快捷键');
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
    await act(async () => button('修改打开 Passpine快捷键').click());
    await act(async () => button('修改打开 Passpine快捷键').dispatchEvent(new KeyboardEvent('keydown', { key: 'K', code: 'KeyK', altKey: true, bubbles: true, cancelable: true })));
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

  it('releases a retained hidden recorder without intercepting keys on another page', async () => {
    await act(async () => root.render(createElement(Theme, {}, createElement('section', { id: 'page' }, createElement(ShortcutSettings, { firefox: false })))));
    const recorder = button('修改搜索账号快捷键');
    await act(async () => recorder.click());
    expect(recorder.getAttribute('aria-pressed')).toBe('true');
    container.querySelector<HTMLElement>('#page')!.hidden = true;
    const event = new KeyboardEvent('keydown', { key: 'N', code: 'KeyN', altKey: true, bubbles: true, cancelable: true });
    await act(async () => document.dispatchEvent(event));
    expect(event.defaultPrevented).toBe(false);
    expect(extension.storage.local.set).not.toHaveBeenCalled();
    expect(recorder.getAttribute('aria-pressed')).toBe('false');
  });

  it('retains the saved binding after a failed save and retries from the same recorder', async () => {
    await renderSettings(false);
    const recorder = button('修改搜索账号快捷键');
    extension.storage.local.set.mockRejectedValueOnce(new Error('Storage unavailable'));
    await act(async () => recorder.click());
    await act(async () => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'N', code: 'KeyN', altKey: true, bubbles: true, cancelable: true })));
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Storage unavailable');
    expect(recorder.querySelector('kbd:last-child')?.textContent).toBe('K');
    await act(async () => recorder.click());
    await act(async () => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'N', code: 'KeyN', altKey: true, bubbles: true, cancelable: true })));
    expect(recorder.querySelector('kbd:last-child')?.textContent).toBe('N');
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it('prevents overwriting unread popup shortcuts and allows a failed read to be retried', async () => {
    extension.storage.local.get.mockRejectedValueOnce(new Error('unavailable'));
    await renderSettings(false);
    expect((button('添加快捷键') as HTMLButtonElement).disabled).toBe(true);
    expect((button('修改搜索账号快捷键') as HTMLButtonElement).disabled).toBe(true);
    expect(extension.storage.local.set).not.toHaveBeenCalled();
    await act(async () => button('重新读取').click());
    expect((button('添加快捷键') as HTMLButtonElement).disabled).toBe(false);
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it('cancels recording when restoring defaults and keeps existing values if restoration fails', async () => {
    extension.storage.local.get.mockResolvedValue({ popupShortcuts: [{ id: 'search', action: 'search', shortcut: 'Alt+N' }] });
    await renderSettings(false);
    await act(async () => button('修改搜索账号快捷键').click());
    extension.storage.local.set.mockRejectedValueOnce(new Error('Restore failed'));
    await act(async () => button('恢复弹窗默认').click());
    expect(button('修改搜索账号快捷键').getAttribute('aria-pressed')).toBe('false');
    expect(button('修改搜索账号快捷键').querySelector('kbd:last-child')?.textContent).toBe('N');
    await act(async () => button('恢复弹窗默认').click());
    expect(button('修改搜索账号快捷键').querySelector('kbd:last-child')?.textContent).toBe('K');
  });

  it('restores Firefox commands through the browser API and rereads actual assignments', async () => {
    await renderSettings(true);
    await act(async () => button('恢复浏览器默认').click());
    expect(extension.commands.reset).toHaveBeenCalledWith(OPEN_COMMAND);
    expect(extension.commands.reset).toHaveBeenCalledWith(FILL_COMMAND);
    expect(container.textContent).toContain('快捷键已保存');
  });

  it.each(['L', 'P'])('checks the Firefox macOS reset defaults against a popup binding ending in %s', async key => {
    vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)');
    extension.storage.local.get.mockResolvedValue({ popupShortcuts: [{ id: 'search', action: 'search', shortcut: `Command+Shift+${key}` }] });
    extension.commands.reset.mockImplementation(async (name: string) => { bindings = bindings.map(value => value.name === name ? { name, shortcut: name === OPEN_COMMAND ? 'Command+Shift+L' : 'Command+Shift+F' } : value); });
    await renderSettings(true);
    await act(async () => button('恢复浏览器默认').click());
    if (key === 'L') {
      expect(container.querySelector('[role="alert"]')?.textContent).toContain(chineseCopy.shortcuts.popupConflict);
      expect(extension.commands.reset).not.toHaveBeenCalled();
    } else {
      expect(container.querySelector('[role="alert"]')).toBeNull();
      expect(extension.commands.reset).toHaveBeenCalledWith(OPEN_COMMAND);
      expect(button('修改打开 Passpine快捷键').querySelector('kbd:last-child')?.textContent).toBe('L');
    }
  });

  it.each(['edit', 'reset'])('reuses a Firefox command key released by %s without a stale conflict', async operation => {
    await renderSettings(true);
    if (operation === 'reset') {
      await act(async () => button('恢复浏览器默认').click());
    } else {
      await act(async () => button('修改打开 Passpine快捷键').click());
      await act(async () => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'K', code: 'KeyK', altKey: true, bubbles: true, cancelable: true })));
    }
    await act(async () => button(chineseCopy.shortcuts.modify(chineseCopy.shortcuts.fillMatch)).click());
    await act(async () => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'L', code: 'KeyL', altKey: true, bubbles: true, cancelable: true })));
    expect(extension.commands.update).toHaveBeenCalledWith({ name: FILL_COMMAND, shortcut: 'Alt+L' });
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it('rejects restoring a page key reserved by another browser command', async () => {
    bindings = [{ name: OPEN_COMMAND, shortcut: '' }, { name: FILL_COMMAND, shortcut: 'Ctrl+Shift+L' }];
    await renderSettings(false);
    await act(async () => button('恢复网页快捷键默认').click());
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('此快捷键已用于另一项操作');
    expect(extension.storage.local.set).not.toHaveBeenCalled();
  });

  it('keeps recorded page keys after permission is denied and enables them without opening another page', async () => {
    extension.storage.local.get.mockImplementation(async () => ({ pageShortcuts: { [OPEN_COMMAND]: 'Alt+O', [FILL_COMMAND]: 'Alt+F' } }));
    extension.permissions.request.mockResolvedValueOnce(false);
    await renderSettings(false);
    await act(async () => button('启用网页快捷键').click());
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('未获得网站访问权限');
    expect(container.querySelector('[role="status"]')?.textContent).not.toContain('已保存');
    expect(button('修改打开 Passpine快捷键').querySelector('kbd:last-child')?.textContent).toBe('O');
    expect(extension.storage.local.set).not.toHaveBeenCalled();
    await act(async () => button('启用网页快捷键').click());
    expect(extension.permissions.request).toHaveBeenCalledWith({ origins: ['http://*/*', 'https://*/*'] });
    expect(container.textContent).toContain('网页快捷键已启用');
    expect(extension.storage.local.set).not.toHaveBeenCalled();
    expect(extension.tabs.create).not.toHaveBeenCalled();
  });

  it('applies permission revocation received while a shortcut save is pending', async () => {
    extension.permissions.contains.mockResolvedValue(true);
    await renderSettings(false);
    let finish!: () => void;
    extension.storage.local.set.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve; }));
    await act(async () => button('修改搜索账号快捷键').click());
    await act(async () => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'N', code: 'KeyN', altKey: true, bubbles: true, cancelable: true })));
    extension.permissions.contains.mockResolvedValue(false);
    await act(async () => extension.permissions.onRemoved.addListener.mock.calls[0]![0]());
    await act(async () => finish());
    expect(button('启用网页快捷键')).toBeDefined();
    expect(button('关闭网页快捷键')).toBeUndefined();
  });

  it('restores page defaults in storage without changing Chromium managed commands', async () => {
    await renderSettings(false);
    await act(async () => button('恢复网页快捷键默认').click());
    expect(extension.storage.local.set).toHaveBeenCalledWith({ pageShortcuts: expect.objectContaining({ [FILL_COMMAND]: 'Ctrl+Shift+F' }) });
    expect(extension.commands.update).not.toHaveBeenCalled();
    expect(extension.commands.reset).not.toHaveBeenCalled();
  });
});
