import type { Copy } from '../constants/copy';
import { systemCopy } from './i18n';
export const OPEN_COMMAND = '_execute_action';
export const FILL_COMMAND = 'fill-best';

// Keep the command IDs stable so browser-managed custom bindings survive updates.
// Share the defaults between Chromium/Firefox manifests and page shortcuts.
export const extensionCommands = (firefox = false) => ({
  // Firefox reserves Command+Shift+P for opening a private window.
  [OPEN_COMMAND]: { suggested_key: { default: 'Ctrl+Shift+L', mac: firefox ? 'Command+Shift+L' : 'Command+Shift+P' }, description: '__MSG_openApp__' },
  [FILL_COMMAND]: { suggested_key: { default: 'Ctrl+Shift+F', mac: 'Command+Shift+F' }, description: '__MSG_fillMatch__' },
});

export const shortcutDefinitions = [
  { name: OPEN_COMMAND, labelKey: 'openApp' },
  { name: FILL_COMMAND, labelKey: 'fillMatch' },
] as const;

export interface ShortcutCommand { name?: string; shortcut?: string }
export interface ShortcutAPI {
  getAll(): Promise<ShortcutCommand[]>;
  update?(details: { name: string; shortcut: string }): Promise<void>;
  reset?(name: string): Promise<void>;
}

export function assignedShortcuts(commands: ShortcutCommand[]): Record<string, string> {
  return Object.fromEntries(shortcutDefinitions.map(({ name }) => [name, commands.find(command => command.name === name)?.shortcut ?? '']));
}

export function defaultShortcuts(mac: boolean, firefox = false) {
  const commands = extensionCommands(firefox), platform = mac ? 'mac' : 'default';
  return { [OPEN_COMMAND]: commands[OPEN_COMMAND].suggested_key[platform], [FILL_COMMAND]: commands[FILL_COMMAND].suggested_key[platform] };
}

export function formatShortcut(shortcut: string, copy: Copy = systemCopy()): string {
  return shortcut.replace(/Command/g, copy.shortcuts.keyNames.Command).replace(/MacCtrl/g, 'Control').split('+').join(' + ');
}

type ShortcutKeyEvent = Pick<KeyboardEvent, 'key' | 'code' | 'altKey' | 'ctrlKey' | 'metaKey' | 'shiftKey' | 'isComposing'>;
type ShortcutCapture = { type: 'shortcut'; shortcut: string } | { type: 'pending' | 'cancel' | 'invalid' };

export function captureShortcut(event: ShortcutKeyEvent, mac: boolean, popup = false): ShortcutCapture {
  if (event.isComposing || ['Control', 'Shift', 'Alt', 'Meta', 'AltGraph'].includes(event.key)) return { type: 'pending' };
  const modified = event.ctrlKey || event.altKey || event.metaKey || event.shiftKey;
  if (event.key === 'Escape' && !modified) return { type: 'cancel' };
  if (['Backspace', 'Delete'].includes(event.key) && !modified) return { type: 'shortcut', shortcut: '' };
  const namedKeys: Record<string, string> = { ArrowUp: 'Up', ArrowDown: 'Down', ArrowLeft: 'Left', ArrowRight: 'Right', ',': 'Comma', '.': 'Period', ' ': 'Space', Home: 'Home', End: 'End', PageUp: 'PageUp', PageDown: 'PageDown', Insert: 'Insert', Delete: 'Delete', ...(popup ? { Enter: 'Enter', '/': 'Slash', '\\': 'Backslash', ';': 'Semicolon', "'": 'Quote', '-': 'Minus', '=': 'Equal', '[': 'BracketLeft', ']': 'BracketRight', '`': 'Backquote' } : {}) };
  const punctuation = ['Comma', 'Period', ...(popup ? ['Slash', 'Backslash', 'Semicolon', 'Quote', 'Minus', 'Equal', 'BracketLeft', 'BracketRight', 'Backquote'] : [])];
  const key = /^Key[A-Z]$/.test(event.code) ? event.code.slice(3) : /^Digit[0-9]$/.test(event.code) ? event.code.slice(5) : punctuation.includes(event.code) ? event.code : /^F([1-9]|1[0-9])$/.test(event.key) ? event.key : namedKeys[event.key];
  const modifiers = [event.ctrlKey ? mac ? 'MacCtrl' : 'Ctrl' : '', event.altKey ? 'Alt' : '', event.metaKey && mac ? 'Command' : '', event.shiftKey ? 'Shift' : ''].filter(Boolean);
  if (!key || (event.metaKey && !mac) || !popup && modifiers.length > 2 || (!event.ctrlKey && !event.altKey && !event.metaKey && !/^F\d+$/.test(key))) return { type: 'invalid' };
  return { type: 'shortcut', shortcut: [...modifiers, key].join('+') };
}

export function canonicalShortcut(shortcut: string): string {
  return shortcut.replace(/MacCtrl|Control|⌃/g, 'Ctrl+').replace(/Command|⌘/g, 'Meta+').replace(/Shift|⇧/g, 'Shift+').replace(/Alt|⌥/g, 'Alt+').replace(/\s/g, '').split('+').filter(Boolean).sort().join('+').toLowerCase();
}

export async function saveShortcut(api: ShortcutAPI, name: string, shortcut: string, copy: Copy = systemCopy()): Promise<Record<string, string>> {
  if (!api.update) throw new Error(copy.shortcuts.editUnavailable);
  const commands = await api.getAll();
  if (shortcut && commands.some(command => command.name !== name && command.shortcut && canonicalShortcut(command.shortcut) === canonicalShortcut(shortcut))) throw new Error(copy.shortcuts.conflict);
  await api.update({ name, shortcut });
  return assignedShortcuts(await api.getAll());
}
