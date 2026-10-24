// @vitest-environment jsdom
import { beforeEach, expect, it, vi } from 'vitest';
import { defaultPageShortcuts, normalizePageShortcuts, pageShortcutForEvent, readPageShortcuts } from './page-shortcuts';
import { FILL_COMMAND, OPEN_COMMAND } from './shortcuts';

const storage = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('wxt/browser', () => ({ browser: { storage: { local: storage } } }));
beforeEach(() => { document.body.replaceChildren(); });

it('keeps explicitly cleared and customized page shortcuts when filling defaults', () => {
  expect(normalizePageShortcuts({ [OPEN_COMMAND]: '', [FILL_COMMAND]: 'Alt+F' }, defaultPageShortcuts(false))).toEqual({ [OPEN_COMMAND]: '', [FILL_COMMAND]: 'Alt+F' });
  expect(normalizePageShortcuts(undefined, defaultPageShortcuts(false))[OPEN_COMMAND]).toBe('Ctrl+Shift+L');
});

it('reports invalid or unreadable storage rather than silently overwriting it', async () => {
  storage.get.mockResolvedValueOnce({ pageShortcuts: { [OPEN_COMMAND]: 42 } });
  await expect(readPageShortcuts()).rejects.toThrow();
  storage.get.mockRejectedValueOnce(new Error('unavailable'));
  await expect(readPageShortcuts()).rejects.toThrow('unavailable');
});

it('matches physical macOS Option keys and ignores cleared bindings, repeats and composition', () => {
  const shortcuts = { [OPEN_COMMAND]: '', [FILL_COMMAND]: 'Alt+F' };
  const event = new KeyboardEvent('keydown', { key: 'ƒ', code: 'KeyF', altKey: true });
  expect(pageShortcutForEvent(event, shortcuts, true)).toBe(FILL_COMMAND);
  expect(pageShortcutForEvent(new KeyboardEvent('keydown', { key: 'F', code: 'KeyF', altKey: true, repeat: true }), shortcuts, true)).toBeUndefined();
  expect(pageShortcutForEvent(new KeyboardEvent('keydown', { key: 'F', code: 'KeyF', altKey: true, isComposing: true }), shortcuts, true)).toBeUndefined();
});

it.each(['input', 'textarea', 'select', '[contenteditable]', '[role="textbox"]'])('does not trigger while typing in %s', selector => {
  const element = document.createElement(selector.startsWith('[') ? 'div' : selector);
  if (selector === '[contenteditable]') element.setAttribute('contenteditable', 'true');
  if (selector === '[role="textbox"]') element.setAttribute('role', 'textbox');
  document.body.append(element);
  let result: unknown = 'unhandled';
  element.addEventListener('keydown', event => { result = pageShortcutForEvent(event as KeyboardEvent, { [OPEN_COMMAND]: 'Alt+L', [FILL_COMMAND]: '' }, true); });
  element.dispatchEvent(new KeyboardEvent('keydown', { key: 'L', code: 'KeyL', altKey: true, bubbles: true, composed: true }));
  expect(result).toBeUndefined();
});

it('recognizes an editable field through a shadow root', () => {
  const host = document.createElement('div'); document.body.append(host);
  const input = document.createElement('input'); host.attachShadow({ mode: 'open' }).append(input);
  let result: unknown = 'unhandled';
  document.addEventListener('keydown', event => { result = pageShortcutForEvent(event, { [OPEN_COMMAND]: 'Alt+L', [FILL_COMMAND]: '' }, true); }, { once: true });
  input.dispatchEvent(new KeyboardEvent('keydown', { key: 'L', code: 'KeyL', altKey: true, bubbles: true, composed: true }));
  expect(result).toBeUndefined();
});
