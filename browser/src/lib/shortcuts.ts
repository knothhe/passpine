export const OPEN_COMMAND = '_execute_action';
export const FILL_COMMAND = 'fill-best';

// Keep the command IDs stable so browser-managed custom bindings survive updates.
// The defaults match browserpasspine-extension's Chromium and Firefox manifests.
export const extensionCommands = {
  [OPEN_COMMAND]: { suggested_key: { default: 'Ctrl+Shift+L', mac: 'Command+Shift+L' }, description: '打开 Passpine Neo' },
  [FILL_COMMAND]: { suggested_key: { default: 'Ctrl+Shift+F', mac: 'Command+Shift+F' }, description: '填充匹配账号' },
};

export const shortcutDefinitions = [
  { name: OPEN_COMMAND, label: '打开 Passpine Neo' },
  { name: FILL_COMMAND, label: '填充匹配账号' },
] as const;

export interface ShortcutCommand { name?: string; shortcut?: string }
export interface ShortcutAPI {
  getAll(): Promise<ShortcutCommand[]>;
  update?(details: { name: string; shortcut: string }): Promise<void>;
  reset?(name: string): Promise<void>;
  openShortcutSettings?(): Promise<void>;
}

export function assignedShortcuts(commands: ShortcutCommand[]): Record<string, string> {
  return Object.fromEntries(shortcutDefinitions.map(({ name }) => [name, commands.find(command => command.name === name)?.shortcut ?? '']));
}

export function defaultShortcuts(mac: boolean): Record<string, string> {
  return Object.fromEntries(Object.entries(extensionCommands).map(([name, command]) => [name, command.suggested_key[mac ? 'mac' : 'default']]));
}

export function formatShortcut(shortcut: string): string {
  return shortcut.replace(/Command/g, '⌘').replace(/MacCtrl/g, 'Control').split('+').join(' + ');
}

export function shortcutsPageURL(userAgent: string): string {
  return /Edg\//.test(userAgent) ? 'edge://extensions/shortcuts' : 'chrome://extensions/shortcuts';
}

type ShortcutKeyEvent = Pick<KeyboardEvent, 'key' | 'code' | 'altKey' | 'ctrlKey' | 'metaKey' | 'shiftKey' | 'isComposing'>;
type ShortcutCapture = { type: 'shortcut'; shortcut: string } | { type: 'pending' | 'cancel' | 'invalid' };

export function captureShortcut(event: ShortcutKeyEvent, mac: boolean): ShortcutCapture {
  if (event.isComposing || ['Control', 'Shift', 'Alt', 'Meta', 'AltGraph'].includes(event.key)) return { type: 'pending' };
  const modified = event.ctrlKey || event.altKey || event.metaKey || event.shiftKey;
  if (event.key === 'Escape' && !modified) return { type: 'cancel' };
  if (['Backspace', 'Delete'].includes(event.key) && !modified) return { type: 'shortcut', shortcut: '' };
  const namedKeys: Record<string, string> = { ArrowUp: 'Up', ArrowDown: 'Down', ArrowLeft: 'Left', ArrowRight: 'Right', ',': 'Comma', '.': 'Period', ' ': 'Space', Home: 'Home', End: 'End', PageUp: 'PageUp', PageDown: 'PageDown', Insert: 'Insert', Delete: 'Delete' };
  const key = /^Key[A-Z]$/.test(event.code) ? event.code.slice(3) : /^Digit[0-9]$/.test(event.code) ? event.code.slice(5) : ['Comma', 'Period'].includes(event.code) ? event.code : /^F([1-9]|1[0-9])$/.test(event.key) ? event.key : namedKeys[event.key];
  const modifiers = [event.ctrlKey ? mac ? 'MacCtrl' : 'Ctrl' : '', event.altKey ? 'Alt' : '', event.metaKey && mac ? 'Command' : '', event.shiftKey ? 'Shift' : ''].filter(Boolean);
  if (!key || (event.metaKey && !mac) || modifiers.length > 2 || (!event.ctrlKey && !event.altKey && !event.metaKey && !/^F\d+$/.test(key))) return { type: 'invalid' };
  return { type: 'shortcut', shortcut: [...modifiers, key].join('+') };
}

function canonicalShortcut(shortcut: string): string {
  return shortcut.replace(/MacCtrl|Control|⌃/g, 'Ctrl+').replace(/Command|⌘/g, 'Meta+').replace(/Shift|⇧/g, 'Shift+').replace(/Alt|⌥/g, 'Alt+').replace(/\s/g, '').split('+').filter(Boolean).sort().join('+').toLowerCase();
}

export async function saveShortcut(api: ShortcutAPI, name: string, shortcut: string): Promise<Record<string, string>> {
  if (!api.update) throw new Error('请在浏览器的扩展快捷键页面修改。');
  const commands = await api.getAll();
  if (shortcut && commands.some(command => command.name !== name && command.shortcut && canonicalShortcut(command.shortcut) === canonicalShortcut(shortcut))) throw new Error('此快捷键已用于另一项操作，请选择其他组合。');
  await api.update({ name, shortcut });
  return assignedShortcuts(await api.getAll());
}
