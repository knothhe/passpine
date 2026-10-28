import { describe, expect, it, vi } from 'vitest';
import { AssociationSync, associationCacheStorageKey, associationCapability, associationOperationsCapability, associationRefreshMilliseconds, type AssociationSession } from './associations';
import { associationKey } from './model';
import { PassPineError } from './errors';
import type { BrowserAssociations, NativeRequest, VaultState } from './types';

const store = 'a'.repeat(64), scope = 'b'.repeat(64), otherScope = 'c'.repeat(64);
const state: VaultState = { configured: true, vaultId: 'vault', vaultName: 'Personal', locked: false, entries: [], capabilities: [associationCapability, associationOperationsCapability] };
const basic = { ...state, capabilities: [associationCapability] };
const old = { ...state, capabilities: [] };
const entry = 'work/account', hostname = 'example.test';
const deferred = <T>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
};
function fixture(initial: Record<string, unknown> = {}) {
  let stored = structuredClone(initial), live = true, now = 0;
  let remote: BrowserAssociations = { associationStoreId: store, associationScope: scope, associationVaultIds: ['vault'], associations: {} };
  const receipts = new Map<string, string>();
  const read = vi.fn(async () => structuredClone(stored));
  const write = vi.fn(async (value: Record<string, unknown>) => { stored = structuredClone({ ...stored, ...value }); });
  const request = vi.fn(async (value: NativeRequest) => {
    if (value.associationStoreId && value.associationStoreId !== remote.associationStoreId) throw new PassPineError('vault_changed');
    if (value.associationScope && value.associationScope !== remote.associationScope) throw new Error('vault_changed');
    if (value.associationOperationId) {
      const receipt = `${remote.associationStoreId}:${value.associationOperationId}`;
      const payload = JSON.stringify(value.associationPatch);
      if (receipts.has(receipt)) {
        if (receipts.get(receipt) !== payload) throw new PassPineError('invalid_request');
        return structuredClone(remote);
      }
      receipts.set(receipt, payload);
    }
    for (const [key, enabled] of Object.entries(value.associationImport ?? {})) if (!Object.hasOwn(remote.associations, key)) remote.associations[key] = enabled;
    Object.assign(remote.associations, value.associationPatch);
    return structuredClone(remote);
  });
  const session: AssociationSession = { target: 'release', assertCurrent: () => { if (!live) throw new Error('stale'); }, request: request as AssociationSession['request'] };
  const create = () => new AssociationSync({ read, write, now: () => now });
  let sync = create();
  return {
    get sync() { return sync; }, session, read, write, request, stored: () => stored, remote: () => remote,
    advance: () => { now += associationRefreshMilliseconds; },
    setRemote: (value: Partial<BrowserAssociations>) => { remote = { ...remote, ...value }; },
    restart: () => { sync.invalidate(); sync = create(); },
    expire: () => { live = false; sync.invalidate(); },
  };
}
type StoredCache = { pending?: Record<string, boolean>; batch?: { id: string; patch: Record<string, boolean> } };
function firstCache(values: Record<string, unknown>): StoredCache | undefined {
  const ledger = values[associationCacheStorageKey] as { caches: Record<string, StoredCache> } | undefined;
  return ledger && Object.values(ledger.caches)[0];
}

describe('client-owned website association synchronization', () => {
  it('keeps old clients on the existing local map without any new native requests', async () => {
    const key = await associationKey('vault', entry, hostname);
    const f = fixture({ associations: { [key]: true, invalid: true } });
    expect((await f.sync.read(f.session, old)).links).toEqual({ [key]: true });
    await f.sync.set(f.session, old, entry, hostname, false);
    expect(f.stored()).toEqual({ associations: { [key]: false } });
    expect(f.request).not.toHaveBeenCalled();
  });

  it('imports old true and false records once, with native values winning and no legacy cache writes', async () => {
    const key = await associationKey('vault', entry, hostname), removed = 'd'.repeat(64);
    const f = fixture({ associations: { [key]: true, [removed]: false } });
    f.setRemote({ associations: { [key]: false } });
    const first = await f.sync.read(f.session, state);
    expect(first.links).toEqual({ [key]: false, [removed]: false });
    expect(first.vaultIds).toEqual([scope, 'vault']);
    expect(f.request.mock.calls[1]![0]).toEqual({ protocol: 1, action: 'browser-associations', vaultId: 'vault', associationScope: scope, associationStoreId: store, associationImport: { [key]: true, [removed]: false } });
    f.advance(); await f.sync.read(f.session, state);
    f.restart(); await f.sync.read(f.session, state);
    expect(f.request.mock.calls.filter(([request]) => request.associationImport)).toHaveLength(1);
    expect(f.stored().associations).toEqual({ [key]: true, [removed]: false });
  });

  it('writes canonical scope keys and reflects acknowledged native data immediately', async () => {
    const f = fixture();
    await f.sync.set(f.session, state, entry, hostname, false);
    const key = await associationKey(scope, entry, hostname);
    expect(f.remote().associations).toEqual({ [key]: false });
    expect((await f.sync.read(f.session, state)).links).toEqual({ [key]: false });
    expect(f.stored()).not.toHaveProperty('associations');
  });

  it('retains edits made with a downgraded client and replays them after the same client supports storage again', async () => {
    const f = fixture();
    await f.sync.set(f.session, state, entry, hostname, true);
    await f.sync.set(f.session, old, entry, hostname, false);
    const legacyKey = await associationKey('vault', entry, hostname), canonical = await associationKey(scope, entry, hostname);
    expect(f.stored().associations).toEqual({ [legacyKey]: false });
    expect(f.remote().associations).toEqual({ [canonical]: true });
    f.restart();
    expect((await f.sync.read(f.session, state)).links).toEqual({ [canonical]: false });
    expect(f.remote().associations).toEqual({ [canonical]: false });
    f.advance(); await f.sync.read(f.session, state);
    expect(f.request.mock.calls.filter(([request]) => request.associationPatch?.[canonical] === false)).toHaveLength(1);
  });

  it('keeps downgraded edits pending when replay fails and retries without replacing the desired value', async () => {
    const f = fixture(), canonical = await associationKey(scope, entry, hostname);
    await f.sync.set(f.session, state, entry, hostname, true);
    await f.sync.set(f.session, old, entry, hostname, false);
    const original = f.request.getMockImplementation()!;
    f.request.mockImplementationOnce(original).mockRejectedValueOnce(new Error('write failed'));
    expect((await f.sync.read(f.session, state)).links[canonical]).toBe(false);
    expect(f.remote().associations[canonical]).toBe(true);
    f.advance();
    expect((await f.sync.read(f.session, state)).links[canonical]).toBe(false);
    expect(f.remote().associations[canonical]).toBe(false);
  });

  it('reuses the durable operation id after acknowledgement cleanup fails instead of overwriting a later browser edit', async () => {
    const f = fixture(), canonical = await associationKey(scope, entry, hostname);
    await f.sync.set(f.session, state, entry, hostname, true);
    await f.sync.set(f.session, old, entry, hostname, false);
    const write = f.write.getMockImplementation()!;
    let failCleanup = true;
    f.write.mockImplementation(async values => {
      const cached = firstCache(values);
      if (failCleanup && cached?.pending && !cached.batch && !Object.keys(cached.pending).length) {
        failCleanup = false;
        throw new Error('acknowledgement cache cleanup failed');
      }
      await write(values);
    });
    expect((await f.sync.read(f.session, state)).links[canonical]).toBe(false);
    const durableBatch = firstCache(f.stored())!.batch!;
    expect(durableBatch.patch).toEqual({ [canonical]: false });
    f.setRemote({ associations: { [canonical]: true } });
    f.restart();
    expect((await f.sync.read(f.session, state)).links[canonical]).toBe(true);
    expect(f.remote().associations[canonical]).toBe(true);
    const replay = f.request.mock.calls.map(([request]) => request).filter(request => request.associationOperationId);
    expect(replay).toHaveLength(2);
    expect(replay.map(request => request.associationOperationId)).toEqual([durableBatch.id, durableBatch.id]);
    expect(firstCache(f.stored())?.batch).toBeUndefined();
  });

  it('does not send a pending operation until its frozen payload and id have been persisted', async () => {
    const f = fixture(), canonical = await associationKey(scope, entry, hostname);
    await f.sync.set(f.session, state, entry, hostname, true);
    await f.sync.set(f.session, old, entry, hostname, false);
    const write = f.write.getMockImplementation()!;
    f.write.mockImplementation(async values => {
      if (firstCache(values)?.batch) throw new Error('cannot persist operation');
      await write(values);
    });
    expect((await f.sync.read(f.session, state)).links[canonical]).toBe(false);
    expect(f.remote().associations[canonical]).toBe(true);
    expect(f.request.mock.calls.some(([request]) => request.associationOperationId)).toBe(false);
    expect(firstCache(f.stored())?.batch).toBeUndefined();
    f.write.mockImplementation(write);
    f.advance();
    await f.sync.read(f.session, state);
    expect(f.remote().associations[canonical]).toBe(false);
  });

  it('keeps an interrupted batch immutable when a later downgraded edit changes the same association', async () => {
    const f = fixture(), canonical = await associationKey(scope, entry, hostname);
    await f.sync.set(f.session, state, entry, hostname, true);
    await f.sync.set(f.session, old, entry, hostname, false);
    const request = f.request.getMockImplementation()!;
    f.request.mockImplementationOnce(request).mockRejectedValueOnce(new Error('reply unavailable'));
    await f.sync.read(f.session, state);
    const batch = structuredClone(firstCache(f.stored())!.batch!);
    await f.sync.set(f.session, old, entry, hostname, true);
    expect(firstCache(f.stored())!.batch).toEqual(batch);
    f.advance();
    expect((await f.sync.read(f.session, state)).links[canonical]).toBe(true);
    const replay = f.request.mock.calls.map(([request]) => request).filter(request => request.associationOperationId);
    expect(replay.map(request => request.associationPatch)).toEqual([{ [canonical]: false }, { [canonical]: false }, { [canonical]: true }]);
    expect(replay[0]!.associationOperationId).toBe(replay[1]!.associationOperationId);
    expect(replay[2]!.associationOperationId).not.toBe(replay[1]!.associationOperationId);
    expect(f.remote().associations[canonical]).toBe(true);
  });

  it('preserves a newer same-value intent after an older batch was acknowledged without a reply', async () => {
    const f = fixture(), canonical = await associationKey(scope, entry, hostname);
    await f.sync.set(f.session, state, entry, hostname, true);
    await f.sync.set(f.session, old, entry, hostname, false);
    const request = f.request.getMockImplementation()!;
    let loseReply = true;
    f.request.mockImplementation(async value => {
      const reply = await request(value);
      if (loseReply && value.associationOperationId) {
        loseReply = false;
        throw new Error('acknowledgement reply lost');
      }
      return reply;
    });
    expect((await f.sync.read(f.session, state)).links[canonical]).toBe(false);
    const oldBatch = structuredClone(firstCache(f.stored())!.batch!);
    expect(firstCache(f.stored())!.pending).toEqual({});
    await f.sync.set(f.session, old, entry, hostname, true);
    await f.sync.set(f.session, old, entry, hostname, false);
    f.setRemote({ associations: { [canonical]: true } });
    f.restart();
    expect((await f.sync.read(f.session, state)).links[canonical]).toBe(false);
    expect(f.remote().associations[canonical]).toBe(false);
    const operations = f.request.mock.calls.map(([request]) => request).filter(request => request.associationOperationId);
    expect(operations.map(request => request.associationOperationId)).toEqual([oldBatch.id, oldBatch.id, expect.any(String)]);
    expect(operations[2]!.associationOperationId).not.toBe(oldBatch.id);
    expect(firstCache(f.stored())!.batch).toBeUndefined();
    expect(firstCache(f.stored())!.pending).toEqual({});
  });

  it('retains unsent pending edits on basic clients without adding unsupported protocol fields', async () => {
    const f = fixture(), canonical = await associationKey(scope, entry, hostname);
    await f.sync.set(f.session, state, entry, hostname, true);
    await f.sync.set(f.session, old, entry, hostname, false);
    f.request.mockClear();
    expect((await f.sync.read(f.session, basic)).links[canonical]).toBe(false);
    expect(f.remote().associations[canonical]).toBe(true);
    expect(f.request.mock.calls.some(([request]) => request.associationPatch)).toBe(false);
    await f.sync.set(f.session, basic, entry, hostname, true);
    expect(f.request.mock.calls.every(([request]) => request.associationOperationId === undefined && request.associationStoreId === undefined)).toBe(true);
    f.restart();
    expect((await f.sync.read(f.session, state)).links[canonical]).toBe(true);
    expect(f.remote().associations[canonical]).toBe(true);
  });

  it('rejects basic-client edits if superseding an old intent cannot be made durable', async () => {
    const f = fixture(), canonical = await associationKey(scope, entry, hostname);
    await f.sync.set(f.session, state, entry, hostname, true);
    await f.sync.set(f.session, old, entry, hostname, false);
    const write = f.write.getMockImplementation()!;
    f.write.mockImplementation(async values => {
      const cached = firstCache(values);
      if (cached?.pending && !Object.keys(cached.pending).length) throw new Error('cannot retire old intent');
      await write(values);
    });
    f.request.mockClear();
    await expect(f.sync.set(f.session, basic, entry, hostname, true)).rejects.toThrow('cannot retire old intent');
    expect(f.request.mock.calls.some(([request]) => request.associationPatch)).toBe(false);
    expect(firstCache(f.stored())!.pending).toEqual({ [canonical]: false });
  });

  it('does not alter or resend a frozen batch to a client without operation receipts', async () => {
    const f = fixture();
    await f.sync.set(f.session, state, entry, hostname, true);
    await f.sync.set(f.session, old, entry, hostname, false);
    const request = f.request.getMockImplementation()!;
    f.request.mockImplementationOnce(request).mockRejectedValueOnce(new Error('reply unavailable'));
    await f.sync.read(f.session, state);
    const batch = structuredClone(firstCache(f.stored())!.batch!);
    f.request.mockClear();
    await expect(f.sync.set(f.session, basic, entry, hostname, true)).rejects.toMatchObject({ code: 'unsupported_action' });
    expect(f.request.mock.calls.some(([request]) => request.associationPatch)).toBe(false);
    expect(firstCache(f.stored())!.batch).toEqual(batch);
  });

  it('rejects a replacement store with the same vault scope before it can receive an explicit edit', async () => {
    const f = fixture();
    await f.sync.read(f.session, state);
    await expect(f.sync.set(f.session, state, entry, hostname, true, async () => {
      f.setRemote({ associationStoreId: 'e'.repeat(64), associations: {} });
    })).rejects.toMatchObject({ code: 'vault_changed' });
    expect(f.remote().associations).toEqual({});
  });

  it('never replays downgraded edits or reimports their legacy copy into a replacement client store', async () => {
    const f = fixture(), canonical = await associationKey(scope, entry, hostname);
    await f.sync.set(f.session, state, entry, hostname, true);
    await f.sync.set(f.session, old, entry, hostname, false);
    f.setRemote({ associationStoreId: 'e'.repeat(64), associations: {} });
    f.restart();
    expect((await f.sync.read(f.session, state)).links).toEqual({});
    expect(f.remote().associations).toEqual({});
    f.setRemote({ associationStoreId: store, associations: { [canonical]: true } });
    f.advance();
    expect((await f.sync.read(f.session, state)).links).toEqual({ [canonical]: false });
  });

  it('rejects failed downgraded saves without recording a pending edit that could later overwrite the client', async () => {
    const f = fixture(), canonical = await associationKey(scope, entry, hostname);
    await f.sync.set(f.session, state, entry, hostname, true);
    f.write.mockRejectedValueOnce(new Error('local storage failed'));
    await expect(f.sync.set(f.session, old, entry, hostname, false)).rejects.toThrow('local storage failed');
    f.advance();
    expect((await f.sync.read(f.session, state)).links[canonical]).toBe(true);
    expect(f.remote().associations[canonical]).toBe(true);
  });

  it('allows explicit native edits when legacy import cannot be accepted and retries migration later', async () => {
    const f = fixture({ associations: { ['d'.repeat(64)]: false } });
    const original = f.request.getMockImplementation()!;
    f.request.mockImplementationOnce(original).mockRejectedValueOnce(new Error('legacy import too large'));
    await f.sync.set(f.session, state, entry, hostname, false);
    expect(f.remote().associations).toEqual({ [await associationKey(scope, entry, hostname)]: false });
    expect(f.stored().associations).toEqual({ ['d'.repeat(64)]: false });
    f.advance(); await f.sync.read(f.session, state);
    expect(f.remote().associations).toEqual({ ['d'.repeat(64)]: false, [await associationKey(scope, entry, hostname)]: false });
  });

  it('pulls periodically, changes its revision when data changes and limits repeated reads', async () => {
    const f = fixture();
    const first = await f.sync.read(f.session, state);
    f.setRemote({ associations: { ['d'.repeat(64)]: false }, associationVaultIds: ['vault', 'another-path'] });
    expect(await f.sync.read(f.session, state)).toEqual(first);
    expect(f.request).toHaveBeenCalledTimes(1);
    f.advance();
    const next = await f.sync.read(f.session, state);
    expect(next.links).toEqual({ ['d'.repeat(64)]: false });
    expect(next.vaultIds).toEqual([scope, 'vault', 'another-path']);
    expect(next.revision).not.toBe(first.revision);
    expect((await f.sync.read(f.session, state)).revision).toBe(next.revision);
    expect(f.request).toHaveBeenCalledTimes(2);
  });

  it('uses the existing cache after a pull failure, throttles failures and retries after the interval', async () => {
    const f = fixture();
    const first = await f.sync.read(f.session, state);
    f.advance(); f.request.mockRejectedValueOnce(new Error('offline'));
    expect(await f.sync.read(f.session, state)).toEqual(first);
    expect(await f.sync.read(f.session, state)).toEqual(first);
    expect(f.request).toHaveBeenCalledTimes(2);
    f.advance(); await f.sync.read(f.session, state);
    expect(f.request).toHaveBeenCalledTimes(3);
  });

  it('rejects unsupported-looking malformed replies and never falls back to legacy for a supported client', async () => {
    const key = await associationKey('vault', entry, hostname);
    const f = fixture({ associations: { [key]: true } });
    f.request.mockResolvedValueOnce({ ...f.remote(), associations: { invalid: true } });
    await expect(f.sync.read(f.session, state)).rejects.toMatchObject({ code: 'invalid_response' });
    expect(f.stored()).toEqual({ associations: { [key]: true } });
  });

  it('reports native write failures while retaining the last acknowledged cache and allowing retry', async () => {
    const f = fixture();
    await f.sync.read(f.session, state);
    f.request.mockRejectedValueOnce(new Error('offline'));
    await expect(f.sync.set(f.session, state, entry, hostname, true)).rejects.toThrow('offline');
    expect((await f.sync.read(f.session, state)).links).toEqual({});
    await f.sync.set(f.session, state, entry, hostname, true);
    expect((await f.sync.read(f.session, state)).links).toEqual({ [await associationKey(scope, entry, hostname)]: true });
  });

  it('does not turn an acknowledged native mutation into failure when cache persistence is unavailable', async () => {
    const f = fixture();
    f.write.mockRejectedValue(new Error('cache unavailable'));
    await f.sync.set(f.session, state, entry, hostname, true);
    expect((await f.sync.read(f.session, state)).links).toEqual({ [await associationKey(scope, entry, hostname)]: true });
  });

  it('revalidates navigation immediately before writes in both old and new modes', async () => {
    for (const mode of [old, state]) {
      const f = fixture(), check = vi.fn(async () => { throw new Error('tab_changed'); });
      await expect(f.sync.set(f.session, mode, entry, hostname, true, check)).rejects.toThrow('tab_changed');
      expect(f.remote().associations).toEqual({});
      expect(f.stored()).not.toHaveProperty('associations');
      expect(f.request.mock.calls.some(([request]) => request.associationPatch)).toBe(false);
    }
  });

  it('isolates supported caches, migration ownership and explicit edits between Dev and release', async () => {
    const legacy = 'd'.repeat(64), canonical = await associationKey(scope, entry, hostname);
    const f = fixture({ associations: { [legacy]: false } });
    await f.sync.read(f.session, state);
    await f.sync.set(f.session, state, entry, hostname, true);
    f.sync.invalidate(); f.setRemote({ associations: {} });
    const dev = { ...f.session, target: 'dev' as const };
    expect((await f.sync.read(dev, state)).links).toEqual({});
    expect(f.request.mock.calls.filter(([request]) => request.associationImport)).toHaveLength(1);
    f.request.mockRejectedValueOnce(new Error('offline')); f.advance();
    expect((await f.sync.read(dev, state)).links).toEqual({});
    f.sync.invalidate(); f.request.mockRejectedValueOnce(new Error('offline'));
    expect((await f.sync.read(f.session, state)).links).toEqual({ [legacy]: false, [canonical]: true });
  });

  it('replaces the binding when the client store or vault scope changes instead of uploading cached records', async () => {
    const f = fixture();
    await f.sync.set(f.session, state, entry, hostname, true);
    f.advance(); f.setRemote({ associationStoreId: 'e'.repeat(64), associationScope: otherScope, associations: {} });
    expect((await f.sync.read(f.session, state)).links).toEqual({});
    expect(f.request.mock.calls.filter(([request]) => request.associationImport)).toHaveLength(0);
    await f.sync.set(f.session, state, entry, hostname, false);
    expect(f.remote().associations).toEqual({ [await associationKey(otherScope, entry, hostname)]: false });
  });

  it('serializes a slow pull with a later edit so an earlier response cannot replace the new value', async () => {
    const f = fixture();
    await f.sync.read(f.session, state); f.advance();
    const pending = deferred<BrowserAssociations>();
    f.request.mockImplementationOnce(() => pending.promise);
    const pull = f.sync.read(f.session, state);
    const edit = f.sync.set(f.session, state, entry, hostname, false);
    await vi.waitFor(() => expect(f.request).toHaveBeenCalledTimes(2));
    pending.resolve({ ...f.remote(), associations: { [await associationKey(scope, entry, hostname)]: true } });
    await pull; await edit;
    expect((await f.sync.read(f.session, state)).links).toEqual({ [await associationKey(scope, entry, hostname)]: false });
  });

  it('discards late native responses and queued writes after the selected client changes', async () => {
    const f = fixture(), pending = deferred<BrowserAssociations>();
    f.request.mockImplementationOnce(() => pending.promise);
    const pull = f.sync.read(f.session, state);
    const edit = f.sync.set(f.session, state, entry, hostname, true);
    const expectedPull = expect(pull).rejects.toThrow('stale'), expectedEdit = expect(edit).rejects.toThrow('stale');
    await vi.waitFor(() => expect(f.request).toHaveBeenCalledTimes(1));
    f.expire(); pending.resolve(f.remote());
    await expectedPull; await expectedEdit;
    expect(f.write).not.toHaveBeenCalled();
    expect(f.request).toHaveBeenCalledTimes(1);
  });

  it('preserves a malformed cache ledger and continues native operations without migrating unknown ownership', async () => {
    const broken = { version: 99, bindings: {}, caches: {} }, legacy = 'd'.repeat(64);
    const f = fixture({ [associationCacheStorageKey]: broken, associations: { [legacy]: true } });
    expect((await f.sync.read(f.session, state)).links).toEqual({});
    await f.sync.set(f.session, state, entry, hostname, false);
    expect(f.stored()).toEqual({ [associationCacheStorageKey]: broken, associations: { [legacy]: true } });
    expect(f.request.mock.calls.some(([request]) => request.associationImport)).toBe(false);
    expect(f.remote().associations).toEqual({ [await associationKey(scope, entry, hostname)]: false });
  });
});
