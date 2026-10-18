import { browser } from 'wxt/browser';
import type { LanguagePreference } from './i18n';

export const languageStorageKey = 'languagePreference';
const changedEvent = 'passpine-language-changed';
export const normalizeLanguage = (value: unknown): LanguagePreference => value === 'en' || value === 'zh-CN' ? value : 'system';

export async function readLanguage(demo = false): Promise<LanguagePreference> {
  return normalizeLanguage(demo ? localStorage.getItem(languageStorageKey) : (await browser.storage.local.get(languageStorageKey))[languageStorageKey]);
}

export async function writeLanguage(value: LanguagePreference, demo = false): Promise<void> {
  const language = normalizeLanguage(value);
  if (demo) {
    localStorage.setItem(languageStorageKey, language);
    window.dispatchEvent(new CustomEvent(changedEvent, { detail: language }));
  } else await browser.storage.local.set({ [languageStorageKey]: language });
}

export function watchLanguage(changed: (language: LanguagePreference) => void, demo = false): () => void {
  if (demo) {
    const storage = (event: StorageEvent) => {
      if (event.key === null || event.key === languageStorageKey) changed(normalizeLanguage(event.key === null ? null : event.newValue));
    };
    const local = (event: Event) => changed(normalizeLanguage((event as CustomEvent).detail));
    window.addEventListener('storage', storage); window.addEventListener(changedEvent, local);
    return () => { window.removeEventListener('storage', storage); window.removeEventListener(changedEvent, local); };
  }
  const listener = (changes: Record<string, { newValue?: unknown }>, area: string) => {
    if (area === 'local' && languageStorageKey in changes) changed(normalizeLanguage(changes[languageStorageKey]?.newValue));
  };
  browser.storage.onChanged.addListener(listener);
  return () => browser.storage.onChanged.removeListener(listener);
}
