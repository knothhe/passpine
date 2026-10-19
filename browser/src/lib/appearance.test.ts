// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { appearanceStorageKey, defaultAppearancePreferences, initializeToolbarAppearance, normalizeAppearancePreferences, readAppearancePreferences, toolbarIconPaths, watchAppearancePreferences, writeAppearancePreferences } from './appearance';

const extension = vi.hoisted(() => ({
  storage: { local: { get: vi.fn(), set: vi.fn() }, onChanged: { addListener: vi.fn(), removeListener: vi.fn() } },
  action: { setIcon: vi.fn() },
}));
vi.mock('wxt/browser', () => ({ browser: extension }));
type ChangeListener = (changes: Record<string, { newValue?: unknown }>, area: string) => void;
const listeners = new Set<ChangeListener>();
const flush = () => new Promise(resolve => setTimeout(resolve, 0));
function change(value: unknown, area = 'local') { for (const listener of listeners) listener({ [appearanceStorageKey]: { newValue: value } }, area); }

beforeEach(() => {
  vi.clearAllMocks(); listeners.clear(); localStorage.clear();
  extension.storage.local.get.mockResolvedValue({}); extension.storage.local.set.mockResolvedValue(undefined);
  extension.storage.onChanged.addListener.mockImplementation(listener => listeners.add(listener));
  extension.storage.onChanged.removeListener.mockImplementation(listener => listeners.delete(listener));
  extension.action.setIcon.mockResolvedValue(undefined);
});
afterEach(() => listeners.clear());

describe('appearance preferences', () => {
  it('preserves valid fields and recovers independently from unknown stored values', () => {
    expect(normalizeAppearancePreferences(undefined)).toEqual(defaultAppearancePreferences);
    expect(normalizeAppearancePreferences('monochrome')).toEqual(defaultAppearancePreferences);
    expect(defaultAppearancePreferences.style).toBe('monochrome');
    expect(normalizeAppearancePreferences({ style: 'unknown', appearance: 'dark' })).toEqual({ style: 'monochrome', appearance: 'dark', fontScale: 100 });
    expect(normalizeAppearancePreferences({ style: 'onevra', appearance: 'light' })).toEqual({ style: 'onevra', appearance: 'light', fontScale: 100 });
    expect(normalizeAppearancePreferences({ style: 'monochrome', appearance: 'unknown' })).toEqual({ style: 'monochrome', appearance: 'system', fontScale: 100 });
  });

  it.each([80, 90, 100, 110, 120])('preserves supported font scale %s independently of the stored theme', fontScale => {
    expect(normalizeAppearancePreferences({ style: 'onevra', appearance: 'dark', fontScale })).toEqual({ style: 'onevra', appearance: 'dark', fontScale });
  });

  it.each([undefined, 0, 70, 130, 95, '120', null])('defaults missing or invalid font scale %j without changing older preferences', fontScale => {
    expect(normalizeAppearancePreferences({ style: 'onevra', appearance: 'light', fontScale })).toEqual({ style: 'onevra', appearance: 'light', fontScale: 100 });
  });

  it('persists preferences in extension storage and delivers changes across UI contexts', async () => {
    let stored: Record<string, unknown> = {};
    extension.storage.local.get.mockImplementation(async () => stored);
    extension.storage.local.set.mockImplementation(async value => { stored = value; change(value[appearanceStorageKey]); });
    const popup = vi.fn(), options = vi.fn();
    const stopPopup = watchAppearancePreferences(popup), stopOptions = watchAppearancePreferences(options);
    const next = { style: 'monochrome', appearance: 'dark', fontScale: 120 } as const;
    await writeAppearancePreferences(next);
    expect(await readAppearancePreferences()).toEqual(next);
    expect(popup).toHaveBeenLastCalledWith(next); expect(options).toHaveBeenLastCalledWith(next);
    change({ style: 'onevra', appearance: 'light' }, 'sync');
    expect(popup).toHaveBeenCalledTimes(1);
    stopPopup(); stopOptions(); expect(listeners.size).toBe(0);
  });

  it('isolates demo settings from extension storage, survives reopening, and recovers invalid JSON', async () => {
    const changed = vi.fn(), stop = watchAppearancePreferences(changed, true);
    await writeAppearancePreferences({ style: 'monochrome', appearance: 'light', fontScale: 80 }, true); await flush();
    expect(await readAppearancePreferences(true)).toEqual({ style: 'monochrome', appearance: 'light', fontScale: 80 });
    expect(changed).toHaveBeenLastCalledWith({ style: 'monochrome', appearance: 'light', fontScale: 80 });
    expect(extension.storage.local.set).not.toHaveBeenCalled();
    localStorage.setItem(appearanceStorageKey, '{broken');
    window.dispatchEvent(new StorageEvent('storage', { key: appearanceStorageKey })); await flush();
    expect(changed).toHaveBeenLastCalledWith(defaultAppearancePreferences);
    stop();
  });
});

describe('toolbar appearance', () => {
  it.each(['missing', 'unavailable'])('uses monochrome icons when stored preferences are %s', async source => {
    if (source === 'unavailable') extension.storage.local.get.mockRejectedValueOnce(new Error('storage unavailable'));
    const stop = initializeToolbarAppearance(); await flush();
    expect(extension.action.setIcon).toHaveBeenLastCalledWith({ path: toolbarIconPaths('monochrome') }); stop();
  });

  it('loads the persisted style at worker startup and switches every toolbar icon size', async () => {
    extension.storage.local.get.mockResolvedValue({ [appearanceStorageKey]: { style: 'monochrome', appearance: 'dark' } });
    const stop = initializeToolbarAppearance(); await flush();
    expect(extension.action.setIcon).toHaveBeenLastCalledWith({ path: { 16: 'icon16-monochrome.png', 24: 'icon24-monochrome.png', 32: 'icon32-monochrome.png', 48: 'icon48-monochrome.png', 128: 'icon128-monochrome.png' } });
    change({ style: 'onevra', appearance: 'light' }); await flush();
    expect(extension.action.setIcon).toHaveBeenLastCalledWith({ path: toolbarIconPaths('onevra') }); stop();
  });

  it.each(['resolve', 'reject'] as const)('does not let a late initial storage %s replace a newer style', async completion => {
    let resolve!: (value: unknown) => void, reject!: (reason: unknown) => void;
    extension.storage.local.get.mockImplementation(() => new Promise((done, fail) => { resolve = done; reject = fail; }));
    const stop = initializeToolbarAppearance();
    change({ style: 'monochrome', appearance: 'system' }); await flush();
    if (completion === 'resolve') resolve({ [appearanceStorageKey]: defaultAppearancePreferences }); else reject(new Error('storage unavailable'));
    await flush();
    expect(extension.action.setIcon).toHaveBeenCalledTimes(1);
    expect(extension.action.setIcon).toHaveBeenLastCalledWith({ path: toolbarIconPaths('monochrome') }); stop();
  });

  it('orders pending icon updates and recovers after a failed action call', async () => {
    let finish!: () => void;
    extension.action.setIcon.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve; }));
    const stop = initializeToolbarAppearance(); await flush();
    change({ style: 'monochrome', appearance: 'light' });
    change({ style: 'onevra', appearance: 'light' });
    change({ style: 'monochrome', appearance: 'dark' });
    finish(); await flush();
    expect(extension.action.setIcon).toHaveBeenCalledTimes(2);
    expect(extension.action.setIcon).toHaveBeenLastCalledWith({ path: toolbarIconPaths('monochrome') });
    extension.action.setIcon.mockRejectedValueOnce(new Error('icon unavailable'));
    change(defaultAppearancePreferences); await flush();
    change({ style: 'monochrome', appearance: 'system' }); await flush();
    expect(extension.action.setIcon).toHaveBeenLastCalledWith({ path: toolbarIconPaths('monochrome') }); stop();
  });
});
