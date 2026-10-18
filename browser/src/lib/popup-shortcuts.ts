import { useEffect, useState } from 'react';
import { browser } from 'wxt/browser';
import { canonicalShortcut, captureShortcut } from './shortcuts';

export const popupShortcutActions = [
  { action: 'search' },
  { action: 'new' },
  { action: 'refresh' },
  { action: 'lock' },
  { action: 'settings' },
  { action: 'copy' },
  { action: 'fill' },
  { action: 'edit' },
] as const;
export type PopupShortcutAction = typeof popupShortcutActions[number]['action'];
export interface PopupShortcut { id: string; action: PopupShortcutAction; shortcut: string }
const storageKey = 'popupShortcuts';
export const isMac = () => /Mac/.test(navigator.userAgent);
export const defaultPopupShortcuts = (): PopupShortcut[] => [{ id: 'search', action: 'search', shortcut: `${isMac() ? 'Command' : 'Ctrl'}+K` }];

export function normalizePopupShortcuts(value: unknown): PopupShortcut[] {
  if (!Array.isArray(value)) return defaultPopupShortcuts();
  const ids = new Set<string>(), keys = new Set<string>();
  return value.filter((item): item is PopupShortcut => {
    if (!item || typeof item !== 'object' || typeof item.id !== 'string' || !item.id || typeof item.shortcut !== 'string' || !popupShortcutActions.some(({ action }) => action === item.action)) return false;
    const key = canonicalShortcut(item.shortcut);
    if (ids.has(item.id) || key && keys.has(key)) return false;
    ids.add(item.id); if (key) keys.add(key);
    return true;
  }).map(({ id, action, shortcut }) => ({ id, action, shortcut }));
}

export async function readPopupShortcuts(demo = false): Promise<PopupShortcut[]> {
  if (demo) return normalizePopupShortcuts(JSON.parse(localStorage.getItem(storageKey) ?? 'null'));
  return normalizePopupShortcuts((await browser.storage.local.get(storageKey))[storageKey]);
}
export async function writePopupShortcuts(values: PopupShortcut[], demo = false): Promise<void> {
  if (demo) { localStorage.setItem(storageKey, JSON.stringify(values)); window.dispatchEvent(new Event('popup-shortcuts-changed')); }
  else await browser.storage.local.set({ [storageKey]: values });
}
export function usePopupShortcuts(demo = false): PopupShortcut[] {
  const [values, setValues] = useState(defaultPopupShortcuts);
  useEffect(() => {
    let active = true, sequence = 0;
    const read = () => { const request = ++sequence; void readPopupShortcuts(demo).then(next => { if (active && request === sequence) setValues(next); }).catch(() => {}); };
    const changed = (changes: Record<string, unknown>, area: string) => { if (area === 'local' && storageKey in changes) read(); };
    read();
    if (demo) { window.addEventListener('storage', read); window.addEventListener('popup-shortcuts-changed', read); }
    else browser.storage.onChanged.addListener(changed);
    return () => { active = false; if (demo) { window.removeEventListener('storage', read); window.removeEventListener('popup-shortcuts-changed', read); } else browser.storage.onChanged.removeListener(changed); };
  }, [demo]);
  return values;
}
export function matchesPopupShortcut(event: KeyboardEvent, shortcut: string): boolean {
  if (!shortcut || event.repeat || event.defaultPrevented) return false;
  const captured = captureShortcut(event, isMac(), true);
  return captured.type === 'shortcut' && !!captured.shortcut && canonicalShortcut(captured.shortcut) === canonicalShortcut(shortcut);
}
