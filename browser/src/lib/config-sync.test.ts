import { describe, expect, it, vi } from 'vitest';
import { ConfigurationSync, configurationSyncStorageKey, type ConfigurationSyncSession } from './config-sync';
import type { NativeRequest, SyncedBrowserPreferences } from './types';
import type { ClientTarget } from './client-target';

const remote: SyncedBrowserPreferences = { themeStyle: 'onevra', theme: 'dark', fontScale: 110, language: 'en' };
const spaceId = 'a'.repeat(64);
const otherSpaceId = 'b'.repeat(64);
const enabled = { capabilities: ['config-sync'], configSync: { enabled: true, ready: true, spaceId } };
const deferred = <T>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
};
function fixture(initial: Record<string, unknown> = {}) {
  let stored = structuredClone(initial), current = true;
  let selectedTarget: ClientTarget = 'release', selectedSpace = spaceId;
  const read = vi.fn(async () => structuredClone(stored));
  const write = vi.fn(async (values: Record<string, unknown>) => {
    const changes = Object.fromEntries(Object.entries(values).map(([key, newValue]) => [key, { oldValue: stored[key], newValue }]));
    stored = structuredClone({ ...stored, ...values });
    sync.storageChanged(changes, 'local');
  });
  let nativePreferences: SyncedBrowserPreferences | undefined = structuredClone(remote);
  const request = vi.fn(async (value: NativeRequest) => {
    if (value.syncPreferences) {
      if (value.syncSpaceId !== selectedSpace) throw new Error('not_ready');
      nativePreferences = { ...(nativePreferences ?? remote), ...value.syncPreferences };
    }
    return { ...enabled, configSync: { ...enabled.configSync, spaceId: selectedSpace }, syncedBrowserPreferences: nativePreferences };
  });
  const session: ConfigurationSyncSession = { id: 1, target: 'release', assertCurrent: () => { if (!current) throw new Error('stale'); }, request: request as ConfigurationSyncSession['request'] };
  const create = () => new ConfigurationSync({ read, write, target: async () => selectedTarget });
  let sync = create();
  const user = async (values: Record<string, unknown>) => {
    const changes = Object.fromEntries(Object.entries(values).map(([key, newValue]) => [key, { oldValue: stored[key], newValue }]));
    stored = structuredClone({ ...stored, ...values });
    sync.storageChanged(changes, 'local');
    await sync.settled();
  };
  return {
    get sync() { return sync; }, session, request, read, write, user, stored: () => stored,
    setRemote: (value: SyncedBrowserPreferences | undefined) => { nativePreferences = value; },
    setSpace: (space: string, value: SyncedBrowserPreferences | undefined) => { selectedSpace = space; nativePreferences = value; },
    setTarget: (target: ClientTarget) => { selectedTarget = target; },
    restart: () => { sync.invalidate(); sync = create(); },
    expire: () => { current = false; sync.invalidate(); },
  };
}

describe('configuration synchronization bridge', () => {
  it('fetches cloud preferences before publishing local defaults, and does not echo imported values', async () => {
    const f = fixture();
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    expect(f.request).toHaveBeenCalledExactlyOnceWith({ protocol: 1, action: 'config-sync-browser' });
    expect(f.stored()).toMatchObject({ appearancePreferences: { style: 'onevra', appearance: 'dark', fontScale: 110 }, languagePreference: 'en' });
  });

  it('retains persisted local preferences only when the completed first pull has no browser configuration', async () => {
    const f = fixture({ languagePreference: 'zh-CN', associations: { opaque: true }, pageShortcuts: { fill: 'Alt+F' } });
    f.setRemote(undefined);
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    expect(f.request.mock.calls.map(([request]) => request.syncPreferences)).toEqual([undefined, { language: 'zh-CN' }]);
    expect(f.stored()).toMatchObject({ associations: { opaque: true }, pageShortcuts: { fill: 'Alt+F' }, languagePreference: 'zh-CN' });
  });

  it('does not connect, upload or write when disabled, old native versions or the initial cloud pull is still pending', async () => {
    const f = fixture({ languagePreference: 'zh-CN' });
    f.sync.observe(f.session, {});
    f.sync.observe(f.session, { ...enabled, configSync: { enabled: false, ready: false } });
    f.sync.observe(f.session, { ...enabled, configSync: { enabled: true, ready: false } });
    await f.sync.settled();
    expect(f.request).not.toHaveBeenCalled();
    expect(f.write.mock.calls.filter(([value]) => 'appearancePreferences' in value || 'languagePreference' in value)).toHaveLength(0);
  });

  it('keeps a local edit during the initial cloud pull and patches only that field', async () => {
    const f = fixture(), pending = deferred<Record<string, unknown>>();
    f.request.mockImplementationOnce(() => pending.promise as any);
    f.sync.observe(f.session, enabled);
    await vi.waitFor(() => expect(f.request).toHaveBeenCalledTimes(1));
    const editing = f.user({ languagePreference: 'zh-CN' });
    pending.resolve({ ...enabled, syncedBrowserPreferences: remote });
    await editing;
    expect(f.request).toHaveBeenLastCalledWith({ protocol: 1, action: 'config-sync-browser', syncSpaceId: spaceId, syncPreferences: { language: 'zh-CN' } });
    expect(f.stored()).toMatchObject({ appearancePreferences: { style: 'onevra', appearance: 'dark', fontScale: 110 }, languagePreference: 'zh-CN' });
  });

  it('tracks edits while the first pull is pending without uploading an older appearance snapshot', async () => {
    const f = fixture();
    f.sync.observe(f.session, { ...enabled, configSync: { enabled: true, ready: false } });
    await f.user({ languagePreference: 'zh-CN' });
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    expect(f.request).toHaveBeenLastCalledWith({ protocol: 1, action: 'config-sync-browser', syncSpaceId: spaceId, syncPreferences: { language: 'zh-CN' } });
  });

  it('serializes different-field local changes and preserves edits made while an earlier patch is pending', async () => {
    const f = fixture();
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    const patch = deferred<Record<string, unknown>>();
    f.request.mockImplementationOnce(() => patch.promise as any);
    const one = f.user({ languagePreference: 'zh-CN' });
    await vi.waitFor(() => expect(f.request).toHaveBeenCalledTimes(2));
    const two = f.user({ appearancePreferences: { style: 'onevra', appearance: 'light', fontScale: 110 } });
    f.setRemote({ ...remote, language: 'zh-CN' });
    patch.resolve({ ...enabled, syncedBrowserPreferences: { ...remote, language: 'zh-CN' } });
    await Promise.all([one, two]);
    expect(f.request.mock.calls.map(([request]) => request.syncPreferences)).toEqual([undefined, { language: 'zh-CN' }, { theme: 'light' }]);
    expect(f.stored()).toMatchObject({ appearancePreferences: { appearance: 'light' }, languagePreference: 'zh-CN' });
  });

  it('preserves unknown local appearance attributes and ignores invalid remote data', async () => {
    const f = fixture({ appearancePreferences: { style: 'monochrome', appearance: 'light', fontScale: 100, futureOption: true } });
    f.sync.observe(f.session, { ...enabled, syncedBrowserPreferences: { ...remote, fontScale: 999 } });
    await f.sync.settled();
    expect(f.request).not.toHaveBeenCalled();
    expect(f.write.mock.calls.filter(([value]) => 'appearancePreferences' in value || 'languagePreference' in value)).toHaveLength(0);
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    expect(f.stored()).toMatchObject({ appearancePreferences: { style: 'onevra', futureOption: true } });
  });

  it('does not replace corrupt/newer local records with normalized defaults', async () => {
    const f = fixture({ appearancePreferences: { style: 'future-style', appearance: 'dark', fontScale: 100 } });
    f.setRemote(undefined);
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    expect(f.request).toHaveBeenCalledTimes(1);
    expect(f.write.mock.calls.filter(([value]) => 'appearancePreferences' in value || 'languagePreference' in value)).toHaveLength(0);
    expect(f.stored()).toMatchObject({ appearancePreferences: { style: 'future-style' } });
  });

  it('never applies or replays a previous client target response after switching targets', async () => {
    const f = fixture(), pending = deferred<Record<string, unknown>>();
    f.request.mockImplementationOnce(() => pending.promise as any);
    f.sync.observe(f.session, enabled);
    await vi.waitFor(() => expect(f.request).toHaveBeenCalledTimes(1));
    f.expire();
    const next = vi.fn(async () => ({ ...enabled, syncedBrowserPreferences: { ...remote, language: 'zh-CN' } }));
    f.sync.observe({ id: 2, target: 'dev', assertCurrent: () => {}, request: next as ConfigurationSyncSession['request'] }, enabled);
    pending.resolve({ ...enabled, syncedBrowserPreferences: remote });
    await f.sync.settled();
    expect(f.request).toHaveBeenCalledTimes(1);
    expect(next).toHaveBeenCalledExactlyOnceWith({ protocol: 1, action: 'config-sync-browser' });
    expect(f.write.mock.calls.filter(([value]) => 'appearancePreferences' in value || 'languagePreference' in value)).toHaveLength(1);
    expect(f.stored()).toMatchObject({ languagePreference: 'zh-CN' });
  });

  it('retains failed local patches and retries only on a new state or edit', async () => {
    const f = fixture();
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    f.request.mockRejectedValueOnce(new Error('app unavailable'));
    await f.user({ languagePreference: 'zh-CN' });
    expect(f.request).toHaveBeenCalledTimes(2);
    f.sync.observe(f.session, { ...enabled, syncedBrowserPreferences: remote });
    await f.sync.settled();
    expect(f.request).toHaveBeenCalledTimes(3);
    expect(f.request).toHaveBeenLastCalledWith({ protocol: 1, action: 'config-sync-browser', syncSpaceId: spaceId, syncPreferences: { language: 'zh-CN' } });
    expect(f.stored()).toMatchObject({ languagePreference: 'zh-CN' });
  });

  it('applies a new remote value on subsequent status responses without uploading it', async () => {
    const f = fixture();
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    f.sync.observe(f.session, { ...enabled, syncedBrowserPreferences: { ...remote, theme: 'light' } });
    await f.sync.settled();
    expect(f.stored()).toMatchObject({ appearancePreferences: { appearance: 'light' } });
    expect(f.request).toHaveBeenCalledTimes(1);
  });

  it('persists offline edits before a native session and survives another worker restart', async () => {
    const f = fixture();
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    f.restart();
    await f.user({ languagePreference: 'zh-CN' });
    expect(f.request).toHaveBeenCalledTimes(1);
    expect(f.stored()[configurationSyncStorageKey]).toMatchObject({ scopes: { [`release:${spaceId}`]: { pending: { language: 'zh-CN' } } } });
    f.restart();
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    expect(f.request).toHaveBeenLastCalledWith({ protocol: 1, action: 'config-sync-browser', syncSpaceId: spaceId, syncPreferences: { language: 'zh-CN' } });
    expect(f.stored()).toMatchObject({ languagePreference: 'zh-CN' });
    expect(f.stored()[configurationSyncStorageKey]).toMatchObject({ scopes: { [`release:${spaceId}`]: { baseline: { language: 'zh-CN' }, pending: {} } } });
  });

  it('keeps an explicit local edit made before the very first native read', async () => {
    const f = fixture();
    await f.user({ languagePreference: 'zh-CN' });
    expect(f.request).not.toHaveBeenCalled();
    expect(f.stored()[configurationSyncStorageKey]).toMatchObject({ scopes: { 'release:unbound': { pending: { language: 'zh-CN' } } } });
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    expect(f.request.mock.calls.map(([request]) => request.syncPreferences)).toEqual([undefined, { language: 'zh-CN' }]);
    expect(f.stored()).toMatchObject({ languagePreference: 'zh-CN', appearancePreferences: { style: 'onevra' } });
  });

  it('retains failed patches through pause and resume', async () => {
    const f = fixture();
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    f.request.mockRejectedValueOnce(new Error('offline'));
    await f.user({ languagePreference: 'zh-CN' });
    f.sync.observe(f.session, { ...enabled, configSync: { enabled: false, ready: true, spaceId } });
    await f.user({ appearancePreferences: { style: 'onevra', appearance: 'light', fontScale: 110 } });
    expect(f.request).toHaveBeenCalledTimes(2);
    f.restart();
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    expect(f.request).toHaveBeenLastCalledWith({ protocol: 1, action: 'config-sync-browser', syncSpaceId: spaceId, syncPreferences: { language: 'zh-CN', theme: 'light' } });
    expect(f.stored()).toMatchObject({ languagePreference: 'zh-CN', appearancePreferences: { appearance: 'light' } });
  });

  it('does not seed a new native target from another target and retains the first target pending edits', async () => {
    const f = fixture();
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    f.request.mockRejectedValueOnce(new Error('offline'));
    await f.user({ languagePreference: 'zh-CN' });
    f.sync.invalidate();
    f.setTarget('dev');
    const devRequest = vi.fn(async () => ({ ...enabled }));
    const dev: ConfigurationSyncSession = { id: 2, target: 'dev', assertCurrent() {}, request: devRequest as ConfigurationSyncSession['request'] };
    f.sync.observe(dev, enabled);
    await f.sync.settled();
    expect(devRequest).toHaveBeenCalledExactlyOnceWith({ protocol: 1, action: 'config-sync-browser' });
    expect(f.stored()[configurationSyncStorageKey]).toMatchObject({ scopes: { [`release:${spaceId}`]: { pending: { language: 'zh-CN' } } } });
    f.sync.invalidate();
    f.setTarget('release');
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    expect(f.request).toHaveBeenLastCalledWith({ protocol: 1, action: 'config-sync-browser', syncSpaceId: spaceId, syncPreferences: { language: 'zh-CN' } });
  });

  it('does not upload old-space edits when the same native client changes buckets or prefix', async () => {
    const f = fixture();
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    f.request.mockRejectedValueOnce(new Error('offline'));
    await f.user({ languagePreference: 'zh-CN' });
    f.setSpace(otherSpaceId, undefined);
    f.sync.observe(f.session, { ...enabled, configSync: { ...enabled.configSync, spaceId: otherSpaceId } });
    await f.sync.settled();
    expect(f.request).toHaveBeenCalledTimes(3);
    expect(f.request.mock.calls[2]![0]).toEqual({ protocol: 1, action: 'config-sync-browser' });
    expect(f.stored()[configurationSyncStorageKey]).toMatchObject({ scopes: { [`release:${spaceId}`]: { pending: { language: 'zh-CN' } }, [`release:${otherSpaceId}`]: { pending: {} } } });
    f.setSpace(spaceId, remote);
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    expect(f.request).toHaveBeenLastCalledWith({ protocol: 1, action: 'config-sync-browser', syncSpaceId: spaceId, syncPreferences: { language: 'zh-CN' } });
  });

  it('uses the authoritative initial native read when space selection changed after the observed status', async () => {
    const f = fixture();
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    f.restart();
    await f.user({ languagePreference: 'zh-CN' });
    f.setSpace(otherSpaceId, { ...remote, language: 'system' });
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    expect(f.request.mock.calls.slice(1).every(([request]) => request.syncPreferences === undefined)).toBe(true);
    expect(f.stored()).toMatchObject({ languagePreference: 'system' });
    expect(f.stored()[configurationSyncStorageKey]).toMatchObject({ scopes: { [`release:${spaceId}`]: { pending: { language: 'zh-CN' } } } });
  });

  it('keeps old-space pending intent if its in-flight patch is rejected after a space switch', async () => {
    const f = fixture();
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    const pending = deferred<Record<string, unknown>>();
    f.request.mockImplementationOnce(() => pending.promise as any);
    const editing = f.user({ languagePreference: 'zh-CN' });
    await vi.waitFor(() => expect(f.request).toHaveBeenCalledTimes(2));
    f.setSpace(otherSpaceId, { ...remote, language: 'system' });
    f.sync.observe(f.session, { ...enabled, configSync: { ...enabled.configSync, spaceId: otherSpaceId } });
    pending.resolve({ ...enabled, syncedBrowserPreferences: { ...remote, language: 'zh-CN' } });
    await editing;
    expect(f.request.mock.calls[1]![0].syncSpaceId).toBe(spaceId);
    expect(f.request.mock.calls.slice(2).every(([request]) => request.syncPreferences === undefined)).toBe(true);
    expect(f.stored()).toMatchObject({ languagePreference: 'system' });
    expect(f.stored()[configurationSyncStorageKey]).toMatchObject({ scopes: { [`release:${spaceId}`]: { pending: { language: 'zh-CN' } } } });
  });

  it('preserves an invalid or future durable ledger without overwriting it', async () => {
    const original = { version: 99, scopes: { privateFutureSetting: true } };
    const f = fixture({ [configurationSyncStorageKey]: original, languagePreference: 'zh-CN' });
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    await f.user({ languagePreference: 'en' });
    expect(f.stored()[configurationSyncStorageKey]).toEqual(original);
    expect(f.request.mock.calls.every(([request]) => request.syncPreferences === undefined)).toBe(true);
    expect(f.write).not.toHaveBeenCalled();
  });

  it.each([
    { target: 'dev' as const, space: spaceId },
    { target: 'release' as const, space: otherSpaceId },
  ])('does not relabel $target/$space edits as changes to the previously displayed scope', async ({ target, space }) => {
    const f = fixture();
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    const next: ConfigurationSyncSession = { ...f.session, id: target === 'dev' ? 2 : 1, target };
    f.setTarget(target);
    f.setSpace(space, remote);
    f.sync.observe(next, { ...enabled, configSync: { enabled: true, ready: false, spaceId: space } });
    await f.user({ languagePreference: 'zh-CN' });
    f.sync.invalidate();
    f.setTarget('release');
    f.setSpace(spaceId, remote);
    f.sync.observe(f.session, enabled);
    await f.sync.settled();
    expect(f.request.mock.calls.every(([request]) => request.syncPreferences === undefined)).toBe(true);
    expect(f.stored()[configurationSyncStorageKey]).toMatchObject({ scopes: { [`${target}:${space}`]: { pending: { language: 'zh-CN' } } } });
    f.sync.invalidate();
    f.setTarget(target);
    f.setSpace(space, remote);
    f.sync.observe(next, { ...enabled, configSync: { ...enabled.configSync, spaceId: space } });
    await f.sync.settled();
    expect(f.request).toHaveBeenLastCalledWith({ protocol: 1, action: 'config-sync-browser', syncSpaceId: space, syncPreferences: { language: 'zh-CN' } });
  });
});
