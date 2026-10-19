import { browser } from 'wxt/browser';

export const appearanceStorageKey = 'appearancePreferences';
export const themeStyles = [
  { id: 'monochrome' },
  { id: 'onevra' },
] as const;
export type ThemeStyle = typeof themeStyles[number]['id'];
export type Appearance = 'system' | 'light' | 'dark';
export const fontScales = [80, 90, 100, 110, 120] as const;
export type FontScale = typeof fontScales[number];
export interface AppearancePreferences { style: ThemeStyle; appearance: Appearance; fontScale: FontScale }
export const defaultAppearancePreferences: AppearancePreferences = { style: 'monochrome', appearance: 'system', fontScale: 100 };

export function normalizeAppearancePreferences(value: unknown): AppearancePreferences {
  const candidate = value && typeof value === 'object' ? value as Record<string, unknown> : {};
  return {
    style: candidate.style === 'onevra' ? 'onevra' : 'monochrome',
    appearance: candidate.appearance === 'light' || candidate.appearance === 'dark' ? candidate.appearance : 'system',
    fontScale: fontScales.includes(candidate.fontScale as FontScale) ? candidate.fontScale as FontScale : 100,
  };
}

export async function readAppearancePreferences(demo = false): Promise<AppearancePreferences> {
  if (!demo) return normalizeAppearancePreferences((await browser.storage.local.get(appearanceStorageKey))[appearanceStorageKey]);
  try { return normalizeAppearancePreferences(JSON.parse(localStorage.getItem(appearanceStorageKey) ?? 'null')); }
  catch { return { ...defaultAppearancePreferences }; }
}

export async function writeAppearancePreferences(value: AppearancePreferences, demo = false): Promise<void> {
  const normalized = normalizeAppearancePreferences(value);
  if (demo) {
    localStorage.setItem(appearanceStorageKey, JSON.stringify(normalized));
    window.dispatchEvent(new Event('passpine-appearance-changed'));
  } else await browser.storage.local.set({ [appearanceStorageKey]: normalized });
}

export function watchAppearancePreferences(changed: (preferences: AppearancePreferences) => void, demo = false): () => void {
  if (demo) {
    const refresh = () => { void readAppearancePreferences(true).then(changed); };
    const storageChanged = (event: StorageEvent) => { if (event.key === null || event.key === appearanceStorageKey) refresh(); };
    window.addEventListener('storage', storageChanged);
    window.addEventListener('passpine-appearance-changed', refresh);
    return () => { window.removeEventListener('storage', storageChanged); window.removeEventListener('passpine-appearance-changed', refresh); };
  }
  const storageChanged = (changes: Record<string, { newValue?: unknown }>, area: string) => {
    if (area === 'local' && appearanceStorageKey in changes) changed(normalizeAppearancePreferences(changes[appearanceStorageKey]?.newValue));
  };
  browser.storage.onChanged.addListener(storageChanged);
  return () => browser.storage.onChanged.removeListener(storageChanged);
}

export function toolbarIconPaths(style: ThemeStyle): Record<number, string> {
  const suffix = style === 'monochrome' ? '-monochrome' : '';
  return Object.fromEntries([16, 24, 32, 48, 128].map(size => [size, `icon${size}${suffix}.png`]));
}

/** Register before loading storage so a late initial read cannot undo a setting change. */
export function initializeToolbarAppearance(): () => void {
  let revision = 0;
  let pending = Promise.resolve();
  const apply = (preferences: AppearancePreferences) => {
    const request = ++revision;
    pending = pending.then(async () => {
      if (request === revision) await browser.action.setIcon({ path: toolbarIconPaths(preferences.style) });
    }).catch(() => {});
  };
  const unwatch = watchAppearancePreferences(apply);
  const initialRevision = revision;
  void readAppearancePreferences().then(preferences => { if (revision === initialRevision) apply(preferences); }).catch(() => { if (revision === initialRevision) apply(defaultAppearancePreferences); });
  return unwatch;
}
