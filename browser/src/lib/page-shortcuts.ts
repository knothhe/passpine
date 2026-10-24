import { browser } from 'wxt/browser';
import { canonicalShortcut, captureShortcut, defaultShortcuts, FILL_COMMAND, OPEN_COMMAND } from './shortcuts';

export const pageShortcutKey = 'pageShortcuts';
export const pageShortcutAccessKey = 'pageShortcutAccess';
export const pageShortcutOrigins = ['http://*/*', 'https://*/*'];
export const pageShortcutScriptId = 'passpine-page-shortcuts';
export const pageShortcutFile = '/content-scripts/page-shortcuts.js';
export type PageShortcutCommand = typeof OPEN_COMMAND | typeof FILL_COMMAND;
export type PageShortcuts = Record<PageShortcutCommand, string>;
export const defaultPageShortcuts = (mac = /Mac/.test(navigator.userAgent)): PageShortcuts => defaultShortcuts(mac) as PageShortcuts;

export function normalizePageShortcuts(value: unknown, fallback = defaultPageShortcuts()): PageShortcuts {
  if (value === undefined || value === null) return { ...fallback };
  if (typeof value !== 'object' || Array.isArray(value)) throw new Error('invalid_shortcuts');
  const record = value as Record<string, unknown>;
  const result = { ...fallback };
  for (const name of [OPEN_COMMAND, FILL_COMMAND] as const) {
    if (record[name] === undefined) continue;
    if (typeof record[name] !== 'string' || record[name].length > 128) throw new Error('invalid_shortcuts');
    result[name] = record[name];
  }
  return result;
}

export async function readPageShortcuts(demo = false, fallback?: PageShortcuts): Promise<PageShortcuts> {
  const value = demo ? JSON.parse(localStorage.getItem(pageShortcutKey) ?? 'null') : (await browser.storage.local.get(pageShortcutKey))[pageShortcutKey];
  return normalizePageShortcuts(value, fallback);
}
export async function writePageShortcuts(value: PageShortcuts, demo = false): Promise<void> {
  if (demo) { localStorage.setItem(pageShortcutKey, JSON.stringify(value)); window.dispatchEvent(new Event('page-shortcuts-changed')); }
  else await browser.storage.local.set({ [pageShortcutKey]: value });
}
export const hasPageShortcutAccess = () => browser.permissions.contains({ origins: pageShortcutOrigins });

// Readomi's host hotkeys ignore text editing, including editors inside Shadow DOM.
export function pageShortcutForEvent(event: KeyboardEvent, shortcuts: PageShortcuts, mac = /Mac/.test(navigator.userAgent)): PageShortcutCommand | undefined {
  if (event.defaultPrevented || event.repeat || event.isComposing) return;
  if (event.composedPath().some(target => target instanceof Element && (target.matches('input,textarea,select,[role="textbox"],[role="combobox"]') || (target as HTMLElement).isContentEditable || target.getAttribute('contenteditable') === '' || target.getAttribute('contenteditable') === 'true'))) return;
  const captured = captureShortcut(event, mac, true);
  if (captured.type !== 'shortcut' || !captured.shortcut) return;
  return ([OPEN_COMMAND, FILL_COMMAND] as const).find(name => shortcuts[name] && canonicalShortcut(shortcuts[name]) === canonicalShortcut(captured.shortcut));
}
