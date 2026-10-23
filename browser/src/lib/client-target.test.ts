// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { CLIENT_TARGET_STORAGE_KEY, canSelectClientTarget, hostForTarget, loadClientTarget, normalizeClientTarget, saveClientTarget, subscribeClientTarget } from './client-target';

const extension = vi.hoisted(() => ({ management: { getSelf: vi.fn() }, storage: { local: { get: vi.fn(), set: vi.fn() }, onChanged: { addListener: vi.fn(), removeListener: vi.fn() } } }));
vi.mock('wxt/browser', () => ({ browser: extension }));
type Listener = (changes: Record<string, { newValue?: unknown }>, area: string) => void;
const listeners = new Set<Listener>();
const emit = (value: unknown, area = 'local') => { for (const listener of listeners) listener({ [CLIENT_TARGET_STORAGE_KEY]: { newValue: value } }, area); };

beforeEach(() => {
  vi.clearAllMocks(); listeners.clear(); localStorage.clear();
  extension.management.getSelf.mockResolvedValue({ installType: 'development' });
  extension.storage.local.get.mockResolvedValue({}); extension.storage.local.set.mockResolvedValue(undefined);
  extension.storage.onChanged.addListener.mockImplementation(listener => listeners.add(listener));
  extension.storage.onChanged.removeListener.mockImplementation(listener => listeners.delete(listener));
});

describe('desktop client target', () => {
  it('defaults absent, deleted and unknown settings to the release host', async () => {
    for (const value of [undefined, null, '', 'production', 'DEV', {}, false]) expect(normalizeClientTarget(value)).toBe('release');
    expect(normalizeClientTarget('dev')).toBe('dev');
    expect(await loadClientTarget()).toBe('release');
    expect(hostForTarget('release')).toBe('com.knothhe.passpine'); expect(hostForTarget('dev')).toBe('com.knothhe.passpine.dev');
  });

  it('persists unpacked client selections and notifies other extension contexts', async () => {
    let stored: Record<string, unknown> = {};
    extension.storage.local.get.mockImplementation(async () => stored);
    extension.storage.local.set.mockImplementation(async value => { stored = value; emit(value[CLIENT_TARGET_STORAGE_KEY]); });
    const changed = vi.fn(), invalidated = vi.fn(), stop = subscribeClientTarget(changed, false, invalidated);
    await saveClientTarget('dev');
    expect(await loadClientTarget()).toBe('dev'); expect(changed).toHaveBeenLastCalledWith('dev');
    emit('release'); expect(invalidated).toHaveBeenCalledTimes(2); // Invalidation precedes the asynchronous policy check.
    await Promise.resolve(); expect(changed).toHaveBeenLastCalledWith('release');
    emit('dev', 'sync'); await Promise.resolve(); expect(changed).toHaveBeenCalledTimes(2);
    emit(undefined); await Promise.resolve(); expect(changed).toHaveBeenLastCalledWith('release');
    stop(); expect(listeners.size).toBe(0);
  });

  it.each(['normal', 'sideload', 'admin', 'other'])('locks %s installations to release despite a stored or injected dev choice', async installType => {
    extension.management.getSelf.mockResolvedValue({ installType }); extension.storage.local.get.mockResolvedValue({ [CLIENT_TARGET_STORAGE_KEY]: 'dev' });
    expect(await canSelectClientTarget()).toBe(false); expect(await loadClientTarget()).toBe('release');
    await saveClientTarget('dev'); expect(extension.storage.local.set).toHaveBeenCalledWith({ [CLIENT_TARGET_STORAGE_KEY]: 'release' });
    const changed = vi.fn(), invalidated = vi.fn(), stop = subscribeClientTarget(changed, false, invalidated);
    emit('dev'); expect(invalidated).toHaveBeenCalledOnce(); expect(changed).not.toHaveBeenCalled();
    await Promise.resolve(); await Promise.resolve(); expect(changed).toHaveBeenLastCalledWith('release'); stop();
  });

  it('fails closed when installation metadata is unavailable', async () => {
    extension.management.getSelf.mockRejectedValue(new Error('unavailable'));
    expect(await canSelectClientTarget()).toBe(false); expect(await loadClientTarget()).toBe('release');
    await saveClientTarget('dev'); expect(extension.storage.local.set).toHaveBeenCalledWith({ [CLIENT_TARGET_STORAGE_KEY]: 'release' });
    extension.management.getSelf.mockResolvedValue(undefined);
    expect(await canSelectClientTarget()).toBe(false);
  });

  it('does not deliver stale policy resolutions or callbacks after unsubscribing', async () => {
    let resolve!: (value: unknown) => void;
    extension.management.getSelf.mockImplementation(() => new Promise(done => { resolve = done; }));
    const changed = vi.fn(), stop = subscribeClientTarget(changed);
    emit('dev'); emit('release'); resolve({ installType: 'development' }); await Promise.resolve(); await Promise.resolve();
    expect(changed).toHaveBeenCalledOnce(); expect(changed).toHaveBeenCalledWith('release');
    emit('dev'); stop(); await Promise.resolve(); expect(changed).toHaveBeenCalledOnce();
  });

  it('keeps demo preferences separate and invalidates local and cross-page demo sessions immediately', async () => {
    const changed = vi.fn(), invalidated = vi.fn(), stop = subscribeClientTarget(changed, true, invalidated);
    await saveClientTarget('dev', true);
    expect(changed).toHaveBeenLastCalledWith('dev'); expect(invalidated).toHaveBeenCalledOnce();
    expect(await loadClientTarget(true)).toBe('dev'); expect(await canSelectClientTarget(true)).toBe(true);
    expect(extension.management.getSelf).not.toHaveBeenCalled(); expect(extension.storage.local.set).not.toHaveBeenCalled();
    window.dispatchEvent(new StorageEvent('storage', { key: CLIENT_TARGET_STORAGE_KEY, newValue: 'release' }));
    expect(changed).toHaveBeenLastCalledWith('release'); expect(invalidated).toHaveBeenCalledTimes(2);
    window.dispatchEvent(new StorageEvent('storage', { key: null })); expect(changed).toHaveBeenLastCalledWith('release');
    stop(); window.dispatchEvent(new StorageEvent('storage', { key: CLIENT_TARGET_STORAGE_KEY, newValue: 'dev' })); expect(changed).toHaveBeenCalledTimes(3);
  });
});
