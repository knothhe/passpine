import { browser } from 'wxt/browser';
import type { ClientTarget } from './client-target';
import { PassPineError } from './errors';
import { associationKey } from './model';
import { PROTOCOL, type BrowserAssociations, type NativeRequest, type VaultState } from './types';

export const associationCacheStorageKey = 'associationCacheV1';
export const associationCapability = 'browser-associations-v1';
export const associationOperationsCapability = 'browser-association-operations-v1';
export const associationRefreshMilliseconds = 60_000;
type Links = Record<string, boolean>;
type State = Pick<VaultState, 'vaultId' | 'capabilities'>;
export interface AssociationSession {
  target: ClientTarget;
  assertCurrent(): void;
  request<T>(request: NativeRequest): Promise<T>;
}
export interface AssociationView { links: Links; vaultIds: string[]; revision: number }
export interface AssociationIO {
  read(key: string): Promise<Record<string, unknown>>;
  write(values: Record<string, unknown>): Promise<void>;
  now?(): number;
}
interface Batch { id: string; patch: Links }
interface Cache { links: Links; vaultIds: string[]; imported: boolean; pending?: Links; batch?: Batch }
interface Ledger {
  version: 1;
  legacyOwner?: ClientTarget;
  legacyStore?: string;
  bindings: Record<string, string>;
  caches: Record<string, Cache>;
}
const record = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const digest = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const supported = (state: State): boolean => Array.isArray(state.capabilities) && state.capabilities.includes(associationCapability);
const operationsSupported = (state: State): boolean => Array.isArray(state.capabilities) && state.capabilities.includes(associationOperationsCapability);
const operationId = (): string => Array.from(crypto.getRandomValues(new Uint8Array(32)), byte => byte.toString(16).padStart(2, '0')).join('');
function links(value: unknown): Links | undefined {
  if (!record(value) || Object.keys(value).length > 4096 || Object.entries(value).some(([key, enabled]) => !digest(key) || typeof enabled !== 'boolean')) return undefined;
  return { ...value } as Links;
}
function legacyLinks(value: unknown): Links {
  return record(value) ? Object.fromEntries(Object.entries(value).filter(([key, enabled]) => digest(key) && typeof enabled === 'boolean')) as Links : {};
}
function response(value: unknown): BrowserAssociations {
  if (!record(value) || !digest(value.associationStoreId) || !digest(value.associationScope) || !Array.isArray(value.associationVaultIds) || value.associationVaultIds.length > 256 || value.associationVaultIds.some(id => typeof id !== 'string' || !id || id.length > 128 || /[\x00-\x1f\x7f]/.test(id))) throw new PassPineError('invalid_response');
  const parsed = links(value.associations);
  if (!parsed) throw new PassPineError('invalid_response');
  return { associationStoreId: value.associationStoreId, associationScope: value.associationScope, associationVaultIds: [...value.associationVaultIds] as string[], associations: parsed };
}
function ledger(value: unknown): Ledger | undefined {
  if (value === undefined) return { version: 1, bindings: {}, caches: {} };
  if (!record(value) || value.version !== 1 || value.legacyOwner !== undefined && value.legacyOwner !== 'dev' && value.legacyOwner !== 'release' || value.legacyStore !== undefined && !digest(value.legacyStore) || !record(value.bindings) || !record(value.caches) || Object.keys(value.caches).length > 1024) return undefined;
  const caches: Record<string, Cache> = {}, bindings: Record<string, string> = {};
  for (const [key, entry] of Object.entries(value.caches)) {
    if (!/^(dev|release):[a-f0-9]{64}:[a-f0-9]{64}$/.test(key) || !record(entry) || typeof entry.imported !== 'boolean' || !Array.isArray(entry.vaultIds) || entry.vaultIds.length > 258 || entry.vaultIds.some(id => typeof id !== 'string' || !id || id.length > 128)) return undefined;
    const parsed = links(entry.links);
    const pending = entry.pending === undefined ? undefined : links(entry.pending);
    const batch = entry.batch;
    const patch = record(batch) ? links(batch.patch) : undefined;
    if (!parsed || entry.pending !== undefined && !pending || batch !== undefined && (!record(batch) || !digest(batch.id) || !patch || !Object.keys(patch).length)) return undefined;
    caches[key] = { links: parsed, vaultIds: [...entry.vaultIds] as string[], imported: entry.imported, ...(pending ? { pending } : {}), ...(record(batch) && patch ? { batch: { id: batch.id as string, patch } } : {}) };
  }
  for (const [key, bound] of Object.entries(value.bindings)) {
    if (!/^(dev|release):[^\x00-\x1f\x7f]{1,128}$/.test(key) || typeof bound !== 'string' || !caches[bound] || !bound.startsWith(key.slice(0, key.indexOf(':') + 1))) return undefined;
    bindings[key] = bound;
  }
  return { version: 1, ...(value.legacyOwner ? { legacyOwner: value.legacyOwner } : {}), ...(value.legacyStore ? { legacyStore: value.legacyStore as string } : {}), bindings, caches };
}

/** One background owner serializes native pulls, migration and explicit edits. */
export class AssociationSync {
  private state?: Ledger;
  private writable = true;
  private generation = 0;
  private work: Promise<unknown> = Promise.resolve();
  private attempts = new Map<string, { at: number; error?: unknown }>();
  private revision = 0;
  private fingerprints = new Map<string, { value: string; revision: number }>();
  constructor(private io: AssociationIO) {}

  invalidate(): void { this.generation++; this.attempts.clear(); }

  read(session: AssociationSession, state: State, force = false): Promise<AssociationView> {
    return this.enqueue(session, async current => {
      if (!state.vaultId) return this.view(session, state, {}, []);
      if (!supported(state)) {
        const stored = await this.io.read('associations'); current();
        return this.view(session, state, legacyLinks(stored.associations), [state.vaultId]);
      }
      await this.load(); current();
      try { await this.pull(session, state, current, force); }
      catch (error) {
        current();
        if (!this.cached(session, state)) throw error;
      }
      const cached = this.cached(session, state)!;
      return this.view(session, state, { ...cached.links, ...cached.batch?.patch, ...cached.pending }, cached.vaultIds);
    });
  }

  set(session: AssociationSession, state: State, entry: string, hostname: string, associated: boolean, beforeWrite?: () => Promise<void>): Promise<void> {
    return this.enqueue(session, async current => {
      if (!supported(state)) {
        await this.load(); current();
        const stored = await this.io.read('associations'); current();
        const key = await associationKey(state.vaultId, entry, hostname); current();
        const next = { ...legacyLinks(stored.associations), [key]: associated };
        const bound = this.state!.bindings[this.binding(session, state)];
        const cache = bound && this.state!.caches[bound];
        let updated: Ledger | undefined;
        if (cache && this.writable) {
          const scope = bound.split(':')[2]!;
          const canonical = await associationKey(scope, entry, hostname); current();
          updated = structuredClone(this.state!);
          updated.caches[bound]!.pending = { ...cache.pending, [canonical]: associated };
        }
        await beforeWrite?.(); current();
        await this.io.write({ associations: next, ...(updated ? { [associationCacheStorageKey]: updated } : {}) }); current();
        if (updated) this.state = updated;
        this.attempts.delete(this.binding(session, state));
        return;
      }
      await this.load(); current();
      // Revalidate store and vault identity before every mutation. A failed pull
      // may serve cached reads, but never turns a write into a local-only edit.
      const identity = await this.pull(session, state, current, true);
      const key = await associationKey(identity.associationScope, entry, hostname); current();
      const bound = this.state!.bindings[this.binding(session, state)]!;
      const cached = this.state!.caches[bound]!;
      // A client without operation receipts cannot safely retire a batch that
      // might already have reached another version of the client.
      if (cached.batch) throw new PassPineError('unsupported_action');
      if (cached.pending && Object.hasOwn(cached.pending, key)) {
        // A new explicit edit supersedes the older unsent intent. Record that
        // supersession before the native write so a worker restart cannot
        // replay the old intent over an acknowledged new edit.
        const next = structuredClone(this.state!);
        delete next.caches[bound]!.pending![key];
        await this.io.write({ [associationCacheStorageKey]: next }); current();
        this.state = next;
      }
      await beforeWrite?.(); current();
      const next = response(await session.request({ protocol: PROTOCOL, action: 'browser-associations', vaultId: state.vaultId, associationScope: identity.associationScope, ...(operationsSupported(state) ? { associationStoreId: identity.associationStoreId } : {}), associationPatch: { [key]: associated } }));
      current();
      if (next.associationStoreId !== identity.associationStoreId || next.associationScope !== identity.associationScope) throw new PassPineError('vault_changed');
      await this.cache(session, state, next, false, current);
    });
  }

  private enqueue<T>(session: AssociationSession, operation: (current: () => void) => Promise<T>): Promise<T> {
    const generation = this.generation;
    const current = () => { session.assertCurrent(); if (this.generation !== generation) throw new PassPineError('native_connection_failed'); };
    const result = this.work.then(async () => { current(); return operation(current); });
    this.work = result.catch(() => {});
    return result;
  }
  private async load(): Promise<void> {
    if (this.state) return;
    const stored = await this.io.read(associationCacheStorageKey);
    const parsed = ledger(stored[associationCacheStorageKey]);
    // Invalid cache files are preserved. The client can still serve reads and
    // writes; legacy migration waits until the durable ownership is repaired.
    this.writable = parsed !== undefined;
    this.state = parsed ?? { version: 1, bindings: {}, caches: {} };
  }
  private binding(session: AssociationSession, state: State): string { return `${session.target}:${state.vaultId}`; }
  private cached(session: AssociationSession, state: State): Cache | undefined {
    return this.state?.caches[this.state.bindings[this.binding(session, state)] ?? ''];
  }
  private async persist(current: () => void): Promise<void> {
    current();
    if (this.writable) await this.io.write({ [associationCacheStorageKey]: structuredClone(this.state!) });
    current();
  }
  private async cache(session: AssociationSession, state: State, value: BrowserAssociations, imported: boolean, current: () => void): Promise<void> {
    current();
    const key = `${session.target}:${value.associationStoreId}:${value.associationScope}`;
    const previous = this.state!.caches[key];
    this.state!.bindings[this.binding(session, state)] = key;
    this.state!.caches[key] = { links: value.associations, vaultIds: [...new Set([value.associationScope, state.vaultId, ...value.associationVaultIds])], imported: imported || previous?.imported === true, ...(previous?.pending ? { pending: previous.pending } : {}), ...(previous?.batch ? { batch: previous.batch } : {}) };
    // The native acknowledgement is authoritative even if its disposable cache
    // cannot be persisted. Keep the acknowledgement in memory for this worker.
    try { await this.persist(current); } catch { current(); }
  }
  private async pull(session: AssociationSession, state: State, current: () => void, force: boolean): Promise<BrowserAssociations> {
    const binding = this.binding(session, state), attempt = this.attempts.get(binding);
    const now = this.io.now?.() ?? Date.now();
    const cached = this.cached(session, state);
    if (!force && attempt && now - attempt.at < associationRefreshMilliseconds) {
      if (cached) {
        const [, storeId, scope] = this.state!.bindings[binding]!.split(':');
        return { associationStoreId: storeId!, associationScope: scope!, associationVaultIds: cached.vaultIds, associations: cached.links };
      }
      if (attempt.error) throw attempt.error;
    }
    this.attempts.set(binding, { at: now });
    try {
      let value = response(await session.request({ protocol: PROTOCOL, action: 'browser-associations', vaultId: state.vaultId })); current();
      const key = `${session.target}:${value.associationStoreId}:${value.associationScope}`;
      const existing = this.state!.caches[key];
      await this.cache(session, state, value, existing?.imported === true, current);
      if (this.writable && !existing?.imported && (this.state!.legacyOwner === undefined || this.state!.legacyOwner === session.target && (this.state!.legacyStore === undefined || this.state!.legacyStore === value.associationStoreId))) {
        // Claim migration ownership durably before sending any legacy values.
        this.state!.legacyOwner = session.target;
        this.state!.legacyStore = value.associationStoreId;
        try {
          await this.persist(current);
          const stored = await this.io.read('associations'); current();
          const imported = legacyLinks(stored.associations);
          if (Object.keys(imported).length) {
            const next = response(await session.request({ protocol: PROTOCOL, action: 'browser-associations', vaultId: state.vaultId, associationScope: value.associationScope, ...(operationsSupported(state) ? { associationStoreId: value.associationStoreId } : {}), associationImport: imported })); current();
            if (next.associationStoreId !== value.associationStoreId || next.associationScope !== value.associationScope) throw new PassPineError('vault_changed');
            value = next;
          }
          await this.cache(session, state, value, true, current);
        } catch (error) { current(); if (error instanceof PassPineError && error.code === 'vault_changed') throw error; }
      }
      while (operationsSupported(state)) {
        let batch = this.state!.caches[key]?.batch;
        if (!batch) {
          const pending = { ...this.state!.caches[key]?.pending };
          if (!Object.keys(pending).length) break;
          const nextState = structuredClone(this.state!);
          batch = { id: operationId(), patch: pending };
          nextState.caches[key]!.batch = batch;
          nextState.caches[key]!.pending = {};
          // The frozen payload and its id must survive before sending. If
          // acknowledgement cleanup later fails, a restart resends this exact
          // id and the client returns current data without applying it twice.
          await this.io.write({ [associationCacheStorageKey]: nextState }); current();
          this.state = nextState;
        }
        const next = response(await session.request({ protocol: PROTOCOL, action: 'browser-associations', vaultId: state.vaultId, associationScope: value.associationScope, associationStoreId: value.associationStoreId, associationOperationId: batch.id, associationPatch: batch.patch })); current();
        if (next.associationStoreId !== value.associationStoreId || next.associationScope !== value.associationScope) throw new PassPineError('vault_changed');
        // New intents live only in pending after the batch is frozen. Even a
        // later edit to the same boolean value is a separate operation and
        // must not be retired by this older acknowledgement.
        delete this.state!.caches[key]!.batch;
        await this.cache(session, state, next, false, current);
        value = next;
      }
      return value;
    } catch (error) {
      current(); this.attempts.set(binding, { at: now, error }); throw error;
    }
  }
  private view(session: AssociationSession, state: State, value: Links, vaultIds: string[]): AssociationView {
    const key = this.binding(session, state);
    const fingerprint = JSON.stringify([vaultIds, Object.keys(value).sort().map(key => [key, value[key]])]);
    const existing = this.fingerprints.get(key);
    const revision = existing?.value === fingerprint ? existing.revision : ++this.revision;
    this.fingerprints.set(key, { value: fingerprint, revision });
    return { links: { ...value }, vaultIds: [...vaultIds], revision };
  }
}

export function initializeAssociationSync(): AssociationSync {
  return new AssociationSync({ read: key => browser.storage.local.get(key), write: values => browser.storage.local.set(values) });
}
