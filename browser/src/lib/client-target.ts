import { browser } from 'wxt/browser';

export type ClientTarget = 'release' | 'dev';
export const CLIENT_TARGET_STORAGE_KEY = 'clientTarget';
const changedEvent = 'passpine-client-target-changed';

export const normalizeClientTarget = (value: unknown): ClientTarget => value === 'dev' ? 'dev' : 'release';
export const hostForTarget = (target: ClientTarget): string => normalizeClientTarget(target) === 'dev' ? 'com.knothhe.passpine.dev' : 'com.knothhe.passpine';

export async function canSelectClientTarget(demo = false): Promise<boolean> {
  if (demo) return true;
  try { return (await browser.management.getSelf()).installType === 'development'; }
  catch { return false; }
}

export async function loadClientTarget(demo = false): Promise<ClientTarget> {
  if (!(await canSelectClientTarget(demo))) return 'release';
  return normalizeClientTarget(demo ? localStorage.getItem(CLIENT_TARGET_STORAGE_KEY) : (await browser.storage.local.get(CLIENT_TARGET_STORAGE_KEY))[CLIENT_TARGET_STORAGE_KEY]);
}

export async function saveClientTarget(value: ClientTarget, demo = false): Promise<ClientTarget> {
  const target = (await canSelectClientTarget(demo)) ? normalizeClientTarget(value) : 'release';
  if (demo) {
    localStorage.setItem(CLIENT_TARGET_STORAGE_KEY, target);
    window.dispatchEvent(new CustomEvent(changedEvent, { detail: target }));
  } else await browser.storage.local.set({ [CLIENT_TARGET_STORAGE_KEY]: target });
  return target;
}

export function subscribeClientTarget(changed: (target: ClientTarget) => void, demo = false, onInvalidated?: () => void): () => void {
  if (demo) {
    const storage = (event: StorageEvent) => {
      if (event.key === null || event.key === CLIENT_TARGET_STORAGE_KEY) { onInvalidated?.(); changed(normalizeClientTarget(event.key === null ? null : event.newValue)); }
    };
    const local = (event: Event) => { onInvalidated?.(); changed(normalizeClientTarget((event as CustomEvent).detail)); };
    window.addEventListener('storage', storage); window.addEventListener(changedEvent, local);
    return () => { window.removeEventListener('storage', storage); window.removeEventListener(changedEvent, local); };
  }
  let active = true, revision = 0;
  const allowed = canSelectClientTarget();
  const listener = (changes: Record<string, { newValue?: unknown }>, area: string) => {
    if (area !== 'local' || !(CLIENT_TARGET_STORAGE_KEY in changes)) return;
    onInvalidated?.();
    const request = ++revision, next = normalizeClientTarget(changes[CLIENT_TARGET_STORAGE_KEY]?.newValue);
    void allowed.then(canSelect => { if (active && revision === request) changed(canSelect ? next : 'release'); });
  };
  browser.storage.onChanged.addListener(listener);
  return () => { active = false; browser.storage.onChanged.removeListener(listener); };
}
