import { browser } from 'wxt/browser';
import { loadClientTarget, type ClientTarget } from './client-target';
import { PROTOCOL, type NativeRequest, type SyncedBrowserPreferences } from './types';

const appearanceKey = 'appearancePreferences';
const languageKey = 'languagePreference';
export const configurationSyncStorageKey = 'configurationSyncStateV1';
const fields = ['themeStyle', 'theme', 'fontScale', 'language'] as const;
type Field = typeof fields[number];
type Patch = Partial<SyncedBrowserPreferences>;
type Stored = Record<string, unknown>;
export interface ConfigurationSyncSession {
  readonly id: number;
  readonly target: ClientTarget;
  assertCurrent(): void;
  request<T>(request: NativeRequest): Promise<T>;
}
interface SyncData {
  capabilities?: string[];
  configSync?: { enabled?: boolean; ready?: boolean; spaceId?: string; fields?: unknown };
  syncedBrowserPreferences?: unknown;
}
export interface ConfigurationSyncIO {
  read(): Promise<Stored>;
  write(value: Stored): Promise<void>;
  target?(): Promise<ClientTarget>;
}
interface TargetState {
  baseline?: Patch;
  pending: Patch;
}
interface SyncState {
  version: 1;
  owners: Partial<Record<typeof fields[number], string>>;
  selected: Partial<Record<ClientTarget, string>>;
  scopes: Record<string, TargetState>;
}

function preferences(value: unknown): Patch | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const v = value as Record<string, unknown>, result: Patch = {};
  if (v.themeStyle !== undefined) {
    if (v.themeStyle !== 'monochrome' && v.themeStyle !== 'onevra') return undefined;
    result.themeStyle = v.themeStyle;
  }
  if (v.theme !== undefined) {
    if (v.theme !== 'system' && v.theme !== 'light' && v.theme !== 'dark') return undefined;
    result.theme = v.theme;
  }
  if (v.fontScale !== undefined) {
    if (![80, 90, 100, 110, 120].includes(v.fontScale as number)) return undefined;
    result.fontScale = v.fontScale as SyncedBrowserPreferences['fontScale'];
  }
  if (v.language !== undefined) {
    if (v.language !== 'system' && v.language !== 'en' && v.language !== 'zh-CN') return undefined;
    result.language = v.language;
  }
  return result;
}
function selectedFields(value: unknown): Field[] | undefined {
  if (value === undefined) return [...fields];
  if (!Array.isArray(value) || value.some(field => typeof field !== 'string' || !fields.includes(field as Field))) return undefined;
  return fields.filter(field => value.includes(field));
}
function selectPreferences(value: Patch, selected: readonly Field[]): Patch {
  return Object.fromEntries(selected.filter(field => value[field] !== undefined).map(field => [field, value[field]])) as Patch;
}
function fromStorage(value: Stored, selected: readonly Field[] = fields): Patch | undefined {
  const appearance = value[appearanceKey];
  const appearanceSelected = selected.some(field => field !== 'language');
  if (appearanceSelected && appearance !== undefined && (!appearance || typeof appearance !== 'object' || Array.isArray(appearance))) return undefined;
  const v = appearanceSelected ? appearance as Record<string, unknown> | undefined : undefined;
  const raw = { themeStyle: v?.style, theme: v?.appearance, fontScale: v?.fontScale, language: value[languageKey] };
  return preferences(Object.fromEntries(selected.map(field => [field, raw[field]])));
}
const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
const targetName = (value: unknown): value is ClientTarget => value === 'release' || value === 'dev';
const spaceName = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const scopeKey = (target: ClientTarget, space?: string) => `${target}:${space ?? 'unbound'}`;
const validScope = (value: string) => /^(release|dev):(?:[a-f0-9]{64}|unbound)$/.test(value);
function syncState(value: unknown): SyncState | undefined {
  if (value === undefined) return { version: 1, owners: {}, selected: {}, scopes: {} };
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const v = value as Record<string, unknown>;
  if (v.version !== 1 || !v.owners || typeof v.owners !== 'object' || Array.isArray(v.owners)) return undefined;
  if (!v.selected || typeof v.selected !== 'object' || Array.isArray(v.selected) || !v.scopes || typeof v.scopes !== 'object' || Array.isArray(v.scopes)) return undefined;
  if (Object.keys(v).some(key => !['version', 'owners', 'selected', 'scopes'].includes(key))) return undefined;
  const result: SyncState = { version: 1, owners: {}, selected: {}, scopes: {} };
  for (const [field, owner] of Object.entries(v.owners)) {
    if (!fields.includes(field as typeof fields[number]) || typeof owner !== 'string' || !validScope(owner)) return undefined;
    result.owners[field as typeof fields[number]] = owner;
  }
  for (const [target, space] of Object.entries(v.selected)) {
    if (!targetName(target) || !spaceName(space)) return undefined;
    result.selected[target] = space;
  }
  if (Object.keys(v.scopes).length > 1024) return undefined;
  for (const [scope, entry] of Object.entries(v.scopes)) {
    if (!validScope(scope) || !entry || typeof entry !== 'object' || Array.isArray(entry)) return undefined;
    const record = entry as Record<string, unknown>;
    if (Object.keys(record).some(key => !['baseline', 'pending'].includes(key))) return undefined;
    const pending = preferences(record.pending);
    if (!pending || Object.keys(record.pending as object).some(key => !fields.includes(key as typeof fields[number]))) return undefined;
    const baseline = record.baseline === undefined ? undefined : preferences(record.baseline);
    if (record.baseline !== undefined && !baseline) return undefined;
    result.scopes[scope] = { pending, ...(baseline ? { baseline } : {}) };
  }
  return result;
}
function cloneState(value: SyncState): SyncState { return structuredClone(value); }

/** One coordinator per background worker. The ledger contains preferences only. */
export class ConfigurationSync {
  private active?: ConfigurationSyncSession;
  private selectedTarget?: ClientTarget;
  private selectedSpace?: string;
  private spaceId?: string;
  private initialized = false;
  private ready = false;
  private running?: Promise<void>;
  private remote?: Patch;
  private selectedFields: readonly Field[] = fields;
  private generation = 0;
  private expected = new Map<string, unknown[]>();
  private state?: SyncState;
  private loading?: Promise<void>;
  private work: Promise<void> = Promise.resolve();

  constructor(private io: ConfigurationSyncIO) {}

  invalidate(): void {
    this.generation++;
    this.active = undefined;
    this.selectedTarget = undefined;
    this.selectedSpace = undefined;
    this.spaceId = undefined;
    this.initialized = false;
    this.ready = false;
    this.remote = undefined;
    this.selectedFields = fields;
    // The durable target ledger survives MV3 suspension and target switches.
    // In-flight work retains generation guards and cannot issue a new request.
  }

  observe(session: ConfigurationSyncSession, value: unknown): void {
    if (!value || typeof value !== 'object') return;
    const data = value as SyncData, space = data.configSync?.spaceId;
    if (spaceName(space)) this.rememberSelection(session.target, space);
    if (data.configSync?.enabled !== true) {
      if (data.configSync?.enabled === false) {
        this.invalidate();
        this.selectedTarget = session.target;
        if (spaceName(space)) this.selectedSpace = space;
      }
      return;
    }
    if (!spaceName(space)) return;
    const selected = selectedFields(data.configSync.fields);
    if (!selected) {
      this.invalidate();
      this.selectedTarget = session.target;
      this.selectedSpace = space;
      return;
    }
    if ((!Array.isArray(data.capabilities) || !data.capabilities.includes('config-sync')) && !this.active) return;
    if (this.active?.id !== session.id || this.active?.target !== session.target || this.spaceId !== space || !same(this.selectedFields, selected)) {
      this.invalidate();
      this.active = session;
      this.spaceId = space;
      this.selectedFields = selected;
    }
    this.selectedTarget = session.target;
    this.selectedSpace = space;
    this.ready = data.configSync.ready === true && selected.length > 0;
    if (data.syncedBrowserPreferences !== undefined) {
      const remote = preferences(data.syncedBrowserPreferences);
      if (!remote) { this.ready = false; return; }
      this.remote = selectPreferences(remote, selected);
    }
    this.schedule();
  }

  private rememberSelection(target: ClientTarget, space: string): void {
    void this.enqueue(async () => {
      if (this.state!.selected[target] === space) return;
      const next = cloneState(this.state!);
      next.selected[target] = space;
      this.state = next;
      await this.io.write({ [configurationSyncStorageKey]: next });
    }).catch(() => {});
  }

  storageChanged(changes: Record<string, { oldValue?: unknown; newValue?: unknown }>, area: string): void {
    if (area !== 'local') return;
    const patch: Patch = {};
    for (const key of [appearanceKey, languageKey]) {
      const change = changes[key];
      if (!change) continue;
      const expected = this.expected.get(key);
      const match = expected?.findIndex(value => same(value, change.newValue)) ?? -1;
      if (expected && match >= 0) { expected.splice(match, 1); if (!expected.length) this.expected.delete(key); continue; }
      // One unsupported disabled value must not hide another field's edit.
      // Keep supported disabled edits too, so enabling them can replay intent.
      for (const field of fields) {
        const old = fromStorage({ [key]: change.oldValue }, [field]), next = fromStorage({ [key]: change.newValue }, [field]);
        if (next?.[field] !== undefined && old?.[field] !== next[field]) Object.assign(patch, { [field]: next[field] });
      }
    }
    if (!Object.keys(patch).length) return;
    // Capture ownership before any await: later switching to another native
    // client must not relabel this edit as that new client's pending patch.
    const knownTarget = this.active?.target ?? this.selectedTarget;
    const knownSpace = this.spaceId ?? this.selectedSpace;
    const target = knownTarget ? Promise.resolve(knownTarget) : this.io.target?.() ?? Promise.resolve('release' as const);
    void this.enqueue(async () => {
      const owner = await target;
      if (!targetName(owner)) return;
      const next = cloneState(this.state!);
      const scope = scopeKey(owner, knownSpace ?? next.selected[owner]);
      const entry = next.scopes[scope] ?? { pending: {} };
      next.scopes[scope] = { ...entry, pending: { ...entry.pending, ...patch } };
      for (const field of fields) if (patch[field] !== undefined) next.owners[field] = scope;
      // Keep intent in memory even if this storage write fails. A later read
      // also compares local values to the last durable baseline after restart.
      this.state = next;
      try { await this.io.write({ [configurationSyncStorageKey]: next }); }
      finally { this.schedule(); }
    }).catch(() => {});
  }

  /** Includes offline ledger writes as well as native requests. */
  async settled(): Promise<void> {
    while (true) {
      const work = this.work, running = this.running;
      await work;
      if (running) await running;
      if (this.work === work && !this.running) return;
    }
  }

  private async load(): Promise<void> {
    if (!this.loading) this.loading = (async () => {
      const stored = await this.io.read();
      const state = syncState(stored[configurationSyncStorageKey]);
      if (!state) throw new Error('invalid_config_sync_state');
      this.state = state;
    })();
    await this.loading;
  }
  private enqueue<T>(operation: () => Promise<T>): Promise<T> {
    const result = this.work.then(async () => { await this.load(); return operation(); });
    this.work = result.then(() => {}, () => {});
    return result;
  }
  private current(session: ConfigurationSyncSession, space: string, generation: number): void {
    session.assertCurrent();
    if (this.active !== session || this.spaceId !== space || this.generation !== generation || !this.ready) throw new Error('stale_config_sync_session');
  }
  private accepts(session: ConfigurationSyncSession, space: string, generation: number, response: SyncData): boolean {
    this.current(session, space, generation);
    const selected = selectedFields(response.configSync?.fields);
    if (response.configSync?.enabled !== true || response.configSync.ready !== true || response.configSync.spaceId !== space || !selected || !same(this.selectedFields, selected)) {
      this.observe(session, response);
      return false;
    }
    return true;
  }
  private schedule(): void {
    if (!this.active || !this.ready || !this.spaceId || this.running) return;
    const session = this.active, space = this.spaceId, generation = this.generation;
    const operation = this.drain(session, space, generation).catch(() => {});
    this.running = operation;
    void operation.then(() => {
      if (this.running === operation) this.running = undefined;
      if (this.active && (this.active !== session || this.spaceId !== space || this.generation !== generation) && this.ready) this.schedule();
    });
  }

  private async drain(session: ConfigurationSyncSession, space: string, generation: number): Promise<void> {
    const scope = scopeKey(session.target, space);
    if (!this.initialized) {
      this.current(session, space, generation);
      // Complete the native/cloud read before deciding whether to seed local
      // preferences. A fresh device must first import the remote configuration.
      const response = await session.request<SyncData>({ protocol: PROTOCOL, action: 'config-sync-browser' });
      if (!this.accepts(session, space, generation, response)) return;
      const remote = response.syncedBrowserPreferences === undefined ? undefined : preferences(response.syncedBrowserPreferences);
      if (response.syncedBrowserPreferences !== undefined && !remote) return;
      await this.enqueue(async () => {
        this.current(session, space, generation);
        const stored = await this.io.read();
        this.current(session, space, generation);
        const local = fromStorage(stored, this.selectedFields);
        if (!local) throw new Error('invalid_local_preferences');
        const next = cloneState(this.state!);
        const entry = next.scopes[scope] ?? { pending: {} };
        const unbound = scopeKey(session.target);
        const pending = { ...next.scopes[unbound]?.pending, ...entry.pending };
        delete next.scopes[unbound];
        for (const field of this.selectedFields) {
          if (local[field] === undefined) continue;
          if (entry.baseline?.[field] !== undefined && next.owners[field] === scope && local[field] !== entry.baseline[field]) Object.assign(pending, { [field]: local[field] });
          else if (remote?.[field] === undefined && (next.owners[field] === undefined || next.owners[field] === scope) && pending[field] === undefined) Object.assign(pending, { [field]: local[field] });
        }
        next.scopes[scope] = { ...entry, pending };
        this.state = next;
        await this.io.write({ [configurationSyncStorageKey]: next });
        this.current(session, space, generation);
        if (remote) this.remote = selectPreferences(remote, this.selectedFields);
        this.initialized = true;
      });
    }
    while (true) {
      this.current(session, space, generation);
      if (this.remote) {
        const remote = this.remote;
        this.remote = undefined;
        await this.apply(session, space, generation, remote);
      }
      this.current(session, space, generation);
      const patch = await this.enqueue(async () => {
        this.current(session, space, generation);
        return selectPreferences(this.state!.scopes[scope]?.pending ?? {}, this.selectedFields);
      });
      this.current(session, space, generation);
      if (!Object.keys(patch).length) { if (this.remote) continue; return; }
      const response = await session.request<SyncData>({ protocol: PROTOCOL, action: 'config-sync-browser', syncSpaceId: space, syncPreferences: patch });
      if (!this.accepts(session, space, generation, response)) return;
      const remote = response.syncedBrowserPreferences === undefined ? undefined : preferences(response.syncedBrowserPreferences);
      if (response.syncedBrowserPreferences !== undefined && !remote) return;
      await this.enqueue(async () => {
        this.current(session, space, generation);
        const next = cloneState(this.state!);
        const entry = next.scopes[scope] ?? { pending: {} };
        for (const field of this.selectedFields) if (patch[field] !== undefined && entry.pending[field] === patch[field]) delete entry.pending[field];
        next.scopes[scope] = entry;
        this.state = next;
        await this.io.write({ [configurationSyncStorageKey]: next });
      });
      this.current(session, space, generation);
      if (remote) this.remote = selectPreferences(remote, this.selectedFields);
    }
  }

  private async apply(session: ConfigurationSyncSession, space: string, generation: number, remote: Patch): Promise<void> {
    const scope = scopeKey(session.target, space);
    try {
      await this.enqueue(async () => {
        const stored = await this.io.read();
        this.current(session, space, generation);
        if (!fromStorage(stored, this.selectedFields)) return;
        const next = cloneState(this.state!);
        const entry = next.scopes[scope] ?? { pending: {} };
        const merged = selectPreferences({ ...remote, ...entry.pending }, this.selectedFields);
        const writes: Stored = {};
        if (merged.themeStyle !== undefined || merged.theme !== undefined || merged.fontScale !== undefined) {
          const existing = stored[appearanceKey] as Record<string, unknown> | undefined;
          const appearance = { ...existing };
          if (merged.themeStyle !== undefined) appearance.style = merged.themeStyle;
          if (merged.theme !== undefined) appearance.appearance = merged.theme;
          if (merged.fontScale !== undefined) appearance.fontScale = merged.fontScale;
          if (!same(existing, appearance)) writes[appearanceKey] = appearance;
        }
        if (merged.language !== undefined && stored[languageKey] !== merged.language) writes[languageKey] = merged.language;
        const baseline = { ...entry.baseline };
        for (const field of this.selectedFields) {
          if (remote[field] === undefined) delete baseline[field];
          else Object.assign(baseline, { [field]: remote[field] });
          if (merged[field] !== undefined) next.owners[field] = scope;
        }
        next.scopes[scope] = { baseline, pending: entry.pending };
        writes[configurationSyncStorageKey] = next;
        this.current(session, space, generation);
        for (const [key, value] of Object.entries(writes)) if (key !== configurationSyncStorageKey) this.expected.set(key, [...(this.expected.get(key) ?? []), value]);
        try { await this.io.write(writes); }
        catch (error) {
          for (const [key, value] of Object.entries(writes)) {
            const expected = this.expected.get(key), index = expected?.findIndex(item => same(item, value)) ?? -1;
            if (expected && index >= 0) expected.splice(index, 1);
          }
          throw error;
        }
        this.state = next;
        this.current(session, space, generation);
      });
    } catch (error) {
      if (this.active === session && this.spaceId === space && this.generation === generation) this.remote = remote;
      throw error;
    }
  }
}

export function initializeConfigurationSync(): ConfigurationSync {
  const sync = new ConfigurationSync({
    read: () => browser.storage.local.get([appearanceKey, languageKey, configurationSyncStorageKey]),
    write: value => browser.storage.local.set(value),
    target: () => loadClientTarget(),
  });
  browser.storage.onChanged.addListener((changes, area) => sync.storageChanged(changes, area));
  return sync;
}
