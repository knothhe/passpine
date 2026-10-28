import { beforeEach, describe, expect, it, vi } from 'vitest';
import { associationKey } from './model';
import type { FillResult, NativeRequest, NativeResponse, Snapshot, TabContext, UIRequest, VaultState } from './types';

const environment = vi.hoisted(() => ({
  native: vi.fn(),
  disconnect: vi.fn(),
  browser: {
    runtime: { id: 'test-extension', getURL: vi.fn(), connectNative: vi.fn(), onMessage: { addListener: vi.fn() } },
    management: { getSelf: vi.fn() },
    tabs: { query: vi.fn() },
    storage: { local: { get: vi.fn(), set: vi.fn() }, onChanged: { addListener: vi.fn(), removeListener: vi.fn() } },
    scripting: { executeScript: vi.fn() },
    commands: { onCommand: { addListener: vi.fn() } },
    alarms: { create: vi.fn(), onAlarm: { addListener: vi.fn() } },
    action: { openPopup: vi.fn() },
    permissions: { contains: vi.fn() },
  },
}));
vi.mock('wxt/browser', () => ({ browser: environment.browser }));
vi.mock('./page-shortcut-registration', () => ({ initializePageShortcuts: vi.fn() }));
vi.mock('./appearance', () => ({ initializeToolbarAppearance: vi.fn() }));
vi.mock('./config-sync', () => ({ initializeConfigurationSync: vi.fn(() => ({ observe: vi.fn(), invalidate: vi.fn() })) }));
vi.mock('./native', () => ({ NativeConnection: class {
  private connected = false;
  constructor(private connect: () => unknown) {}
  request = (value: NativeRequest) => {
    if (!this.connected) { this.connect(); this.connected = true; }
    return environment.native(value);
  };
  disconnect = environment.disconnect;
} }));

type MessageListener = (message: unknown, sender: { id: string; url: string; tab?: { id: number; url: string } }, respond: (response: NativeResponse) => void) => boolean;
const tab: TabContext = { id: 7, url: 'https://login.example.test/sign-in', hostname: 'login.example.test', secure: true };
const account = 'Personal/account';
let current: { id: number; url: string }, stored: Record<string, unknown>, listener: MessageListener;
let state: VaultState;

function request<T = Record<string, never>>(value: UIRequest, clientTarget: 'release' | 'dev' = 'release'): Promise<NativeResponse<T>> {
  return new Promise(resolve => {
    expect(listener({ type: 'passpine', clientTarget, request: value }, { id: 'test-extension', url: 'chrome-extension://test-extension/popup.html' }, response => resolve(response as NativeResponse<T>))).toBe(true);
  });
}
const fill = (entry = account) => request<FillResult>({ action: 'fill', vaultId: 'vault', entry, tab });
function switchTarget(target: 'release' | 'dev') {
  stored.clientTarget = target;
  const onChanged = environment.browser.storage.onChanged.addListener.mock.calls[0]![0];
  onChanged({ clientTarget: { newValue: target } }, 'local');
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
}

beforeEach(async () => {
  vi.resetAllMocks(); vi.resetModules();
  current = { id: tab.id, url: tab.url }; stored = {};
  state = { configured: true, vaultId: 'vault', vaultName: 'Personal', locked: false, entries: [{ path: account, name: 'account', folder: 'Personal' }], capabilities: [] };
  environment.native.mockImplementation(async (value: NativeRequest) => value.action === 'fetch' ? { contents: 'secret\nusername: alice\nurl: https://unrelated.test', revision: 'revision' } : state);
  environment.browser.runtime.getURL.mockReturnValue('chrome-extension://test-extension/');
  environment.browser.permissions.contains.mockResolvedValue(true);
  environment.browser.alarms.create.mockResolvedValue(undefined);
  environment.browser.action.openPopup.mockResolvedValue(undefined);
  environment.browser.management.getSelf.mockResolvedValue({ installType: 'development' });
  environment.browser.tabs.query.mockImplementation(async () => [current]);
  environment.browser.storage.local.get.mockImplementation(async () => structuredClone(stored));
  environment.browser.storage.local.set.mockImplementation(async value => { stored = structuredClone({ ...stored, ...value }); });
  environment.browser.scripting.executeScript.mockResolvedValue([{ frameId: 0, result: { filled: 2 } }]);
  vi.stubGlobal('defineBackground', (callback: () => void) => callback());
  await import('../../entrypoints/background');
  listener = environment.browser.runtime.onMessage.addListener.mock.calls[0]![0] as MessageListener;
});

describe('background fill and website associations', () => {
  function clientAssociations(initial: Record<string, boolean> = {}, aliases = ['vault']) {
    state.capabilities = ['browser-associations-v1'];
    const links = { ...initial }, scope = 'b'.repeat(64), storeId = 'a'.repeat(64);
    environment.native.mockImplementation(async (value: NativeRequest) => {
      if (value.action === 'fetch') return { contents: 'secret\nusername: alice', revision: 'revision' };
      if (value.action !== 'browser-associations') return state;
      for (const [key, enabled] of Object.entries(value.associationImport ?? {})) if (!(key in links)) links[key] = enabled;
      Object.assign(links, value.associationPatch);
      return { associationStoreId: storeId, associationScope: scope, associationVaultIds: aliases, associations: { ...links } };
    });
    return { links, scope };
  }

  it('migrates legacy links to a capable client and stores explicit unlinks under its portable scope', async () => {
    const legacy = await associationKey('vault', account, tab.hostname);
    stored.associations = { [legacy]: true };
    const server = clientAssociations();
    expect((await request<Snapshot>({ action: 'snapshot' })).data?.matches).toEqual([account]);
    expect(server.links[legacy]).toBe(true);
    expect((await request({ action: 'dissociate', vaultId: 'vault', entry: account, tab })).status).toBe('ok');
    expect(server.links[await associationKey(server.scope, account, tab.hostname)]).toBe(false);
    expect(stored.associations).toEqual({ [legacy]: true });
    expect((await request<Snapshot>({ action: 'snapshot' })).data?.matches).toEqual([]);
  });

  it('uses migrated aliases across machines, preferring legacy unlinks until a canonical choice exists', async () => {
    const local = await associationKey('vault', account, tab.hostname);
    const remote = await associationKey('other-machine', account, tab.hostname);
    const server = clientAssociations({ [local]: true, [remote]: false }, ['vault', 'other-machine']);
    expect((await request<Snapshot>({ action: 'snapshot' })).data?.matches).toEqual([]);
    expect((await fill()).data?.associated).toBe(true);
    expect(server.links[await associationKey(server.scope, account, tab.hostname)]).toBe(true);
    expect((await request<Snapshot>({ action: 'snapshot' })).data?.matches).toEqual([account]);
  });

  it('refreshes client associations on an alarm without reading a page or credentials, and exposes a separate version', async () => {
    const server = clientAssociations();
    const key = await associationKey(server.scope, account, tab.hostname);
    const initial = (await request<Snapshot>({ action: 'snapshot' })).data!;
    const alarm = environment.browser.alarms.onAlarm.addListener.mock.calls[0]![0];
    expect(environment.browser.alarms.create).toHaveBeenCalledWith('passpine-associations-refresh', { periodInMinutes: 1 });
    server.links[key] = true;
    environment.native.mockClear(); environment.browser.tabs.query.mockClear(); environment.browser.scripting.executeScript.mockClear();
    alarm({ name: 'passpine-associations-refresh' });
    await vi.waitFor(() => expect(environment.native).toHaveBeenCalledWith({ protocol: 1, action: 'browser-associations', vaultId: 'vault' }));
    const next = (await request<Snapshot>({ action: 'status' })).data!;
    expect(next.associationVersion).not.toBe(initial.associationVersion);
    expect(next.entriesVersion).toBe(initial.entriesVersion);
    expect(environment.browser.tabs.query).not.toHaveBeenCalled();
    expect(environment.browser.scripting.executeScript).not.toHaveBeenCalled();
    expect(environment.native.mock.calls.some(([value]) => value.action === 'fetch')).toBe(false);
    expect((await request<Snapshot>({ action: 'snapshot' })).data?.matches).toEqual([account]);
  });

  it('does not send association requests to an older client during scheduled refresh', async () => {
    const alarm = environment.browser.alarms.onAlarm.addListener.mock.calls[0]![0];
    alarm({ name: 'passpine-associations-refresh' });
    await vi.waitFor(() => expect(environment.native).toHaveBeenCalledWith({ protocol: 1, action: 'configure', includeEntries: false }));
    expect(environment.native.mock.calls.some(([value]) => value.action === 'browser-associations')).toBe(false);
  });

  it('forwards activity alone without reading credentials, tabs or associations', async () => {
    environment.native.mockResolvedValue({});
    expect(await request({ action: 'touch' })).toEqual({ protocol: 1, status: 'ok', data: {} });
    expect(environment.native).toHaveBeenCalledExactlyOnceWith({ protocol: 1, action: 'touch' });
    expect(environment.browser.tabs.query).not.toHaveBeenCalled();
    expect(environment.browser.storage.local.get).toHaveBeenCalledExactlyOnceWith('clientTarget');
    expect(environment.browser.storage.local.set).not.toHaveBeenCalled();
    expect(environment.browser.scripting.executeScript).not.toHaveBeenCalled();
  });

  it('copies generated passwords directly through native messaging without reading vault or tab state', async () => {
    environment.native.mockResolvedValue({});
    expect(await request({ action: 'copy-generated', contents: 'fresh-password' })).toEqual({ protocol: 1, status: 'ok', data: {} });
    expect(environment.native).toHaveBeenCalledExactlyOnceWith({ protocol: 1, action: 'copy-generated', contents: 'fresh-password' });
    expect(environment.browser.tabs.query).not.toHaveBeenCalled();
    expect(environment.browser.storage.local.set).not.toHaveBeenCalled();
    expect(environment.browser.storage.local.get).toHaveBeenCalledExactlyOnceWith('clientTarget');
  });

  it('retains native clipboard failures for generated passwords', async () => {
    const { PassPineError } = await import('./errors');
    environment.native.mockRejectedValue(new PassPineError('native_connection_failed'));
    expect(await request({ action: 'copy-generated', contents: 'fresh-password' })).toMatchObject({ protocol: 1, status: 'error', code: 'native_connection_failed' });
  });

  it('fills a manually selected unrelated account and then associates its exact host', async () => {
    expect(await fill()).toEqual({ protocol: 1, status: 'ok', data: { filled: 2, associated: true } });
    expect(environment.native).toHaveBeenCalledWith({ protocol: 1, action: 'fetch', vaultId: 'vault', entry: account, purpose: 'fill', interactive: true });
    expect(environment.browser.scripting.executeScript).toHaveBeenCalledWith(expect.objectContaining({ target: { tabId: tab.id, frameIds: [0] }, args: [tab.url, { username: 'alice', password: 'secret', otp: undefined }] }));
    const key = await associationKey('vault', account, tab.hostname);
    expect(stored.associations).toEqual({ [key]: true });
    expect((await request<Snapshot>({ action: 'snapshot' })).data?.matches).toEqual([account]);
    current = { id: tab.id, url: 'https://example.test/sign-in' };
    expect((await request<Snapshot>({ action: 'snapshot' })).data?.matches).toEqual([]);
  });

  it.each([{ filled: 0, code: 'no_fields' }, { filled: 1, code: 'tab_changed' }])('does not associate a failed DOM fill: %j', async result => {
    environment.browser.scripting.executeScript.mockResolvedValue([{ frameId: 0, result }]);
    expect((await fill()).code).toBe(result.code);
    expect(environment.browser.storage.local.set).not.toHaveBeenCalled();
    expect(environment.native.mock.calls.map(([value]) => value.action)).toEqual(['fetch']);
  });

  it.each(['vault_changed', 'entry_not_found'])('keeps native %s errors from filling or associating', async code => {
    const { PassPineError } = await import('./errors');
    environment.native.mockRejectedValue(new PassPineError(code));
    expect((await fill()).code).toBe(code);
    expect(environment.browser.scripting.executeScript).not.toHaveBeenCalled();
    expect(environment.browser.storage.local.set).not.toHaveBeenCalled();
  });

  it('rejects a tab change during credential retrieval before injecting', async () => {
    environment.native.mockImplementation(async () => {
      current = { ...current, url: 'https://different.test/' };
      return { contents: 'secret' };
    });
    expect((await fill()).code).toBe('tab_changed');
    expect(environment.browser.scripting.executeScript).not.toHaveBeenCalled();
    expect(environment.browser.storage.local.set).not.toHaveBeenCalled();
  });

  it('reports a completed fill without association when the page changes after injection', async () => {
    environment.browser.scripting.executeScript.mockImplementation(async () => {
      current = { ...current, url: 'https://different.test/' };
      return [{ frameId: 0, result: { filled: 2 } }];
    });
    expect((await fill()).data).toEqual({ filled: 2, associated: false });
    expect(environment.browser.storage.local.set).not.toHaveBeenCalled();
  });

  it('rechecks the tab immediately before persisting an association', async () => {
    environment.browser.storage.local.get.mockImplementation(async key => {
      if (key !== 'associations') return {};
      current = { ...current, url: 'https://different.test/' };
      return {};
    });
    expect((await fill()).data).toEqual({ filled: 2, associated: false });
    expect(environment.browser.storage.local.set).not.toHaveBeenCalled();
  });

  it.each(['vault changed', 'entry deleted'])('does not persist an association when the %s after filling', async reason => {
    environment.native.mockImplementation(async (value: NativeRequest) => {
      if (value.action === 'fetch') return { contents: 'secret' };
      return { ...state, ...(reason === 'vault changed' ? { vaultId: 'different' } : { entries: [] }) };
    });
    expect((await fill()).data).toEqual({ filled: 2, associated: false });
    expect(environment.browser.storage.local.set).not.toHaveBeenCalled();
  });

  it('reports successful filling separately from storage failure and recovers on the next fill', async () => {
    environment.browser.storage.local.set.mockRejectedValueOnce(new Error('storage unavailable'));
    expect((await fill()).data).toEqual({ filled: 2, associated: false });
    expect((await fill()).data).toEqual({ filled: 2, associated: true });
  });

  it('retains an explicit removal over path matching, isolates it by host and reassociates after filling', async () => {
    const entry = `${tab.hostname}/work`;
    state.entries = [{ path: entry, name: 'work', folder: tab.hostname }];
    expect((await request<Snapshot>({ action: 'snapshot' })).data?.matches).toEqual([entry]);
    expect((await request({ action: 'dissociate', vaultId: 'vault', entry, tab })).status).toBe('ok');
    const key = await associationKey('vault', entry, tab.hostname);
    expect(stored.associations).toEqual({ [key]: false });
    expect((await request<Snapshot>({ action: 'snapshot' })).data?.matches).toEqual([]);
    expect((await fill(entry)).data).toEqual({ filled: 2, associated: true });
    expect(stored.associations).toEqual({ [key]: true });
    expect((await request<Snapshot>({ action: 'snapshot' })).data?.matches).toEqual([entry]);
  });

  it('rejects manual association changes for a stale tab, vault or entry', async () => {
    current = { ...current, id: 8 };
    expect((await request({ action: 'dissociate', vaultId: 'vault', entry: account, tab })).code).toBe('tab_changed');
    current = { id: tab.id, url: tab.url };
    expect((await request({ action: 'associate', vaultId: 'other', entry: account, tab })).code).toBe('vault_changed');
    expect((await request({ action: 'dissociate', vaultId: 'vault', entry: 'missing', tab })).code).toBe('vault_changed');
    expect(environment.browser.storage.local.set).not.toHaveBeenCalled();
  });

  it('serializes concurrent association writes without losing other accounts or explicit removals', async () => {
    const other = 'Personal/other', removed = await associationKey('vault', 'removed', tab.hostname);
    state.entries.push({ path: other, name: 'other', folder: 'Personal' });
    stored = { associations: { [removed]: false, invalid: true, ['a'.repeat(64)]: 'invalid' } };
    const firstWrite = deferred<void>();
    environment.browser.storage.local.set.mockImplementationOnce(async value => { await firstWrite.promise; stored = structuredClone(value); });
    const one = fill(), two = fill(other);
    await vi.waitFor(() => expect(environment.browser.storage.local.set).toHaveBeenCalledTimes(1));
    firstWrite.resolve();
    expect((await one).data?.associated).toBe(true); expect((await two).data?.associated).toBe(true);
    expect(stored.associations).toEqual({ [removed]: false, [await associationKey('vault', account, tab.hostname)]: true, [await associationKey('vault', other, tab.hostname)]: true });
  });

  it('does not associate a queued fill after its page changes', async () => {
    const other = 'Personal/other';
    state.entries.push({ path: other, name: 'other', folder: 'Personal' });
    const firstWrite = deferred<void>();
    environment.browser.storage.local.set.mockImplementationOnce(async value => { await firstWrite.promise; stored = structuredClone(value); });
    const one = fill(), two = fill(other);
    await vi.waitFor(() => expect(environment.browser.storage.local.set).toHaveBeenCalledTimes(1));
    current = { ...current, url: 'https://different.test/' };
    firstWrite.resolve();
    await one;
    expect((await two).data).toEqual({ filled: 2, associated: false });
    expect(environment.browser.storage.local.set).toHaveBeenCalledTimes(1);
  });
});

describe('selected desktop client isolation', () => {
  it('defaults to the release client and opens Dev only after an explicit selection', async () => {
    await request({ action: 'touch' });
    expect(environment.browser.runtime.connectNative).toHaveBeenCalledExactlyOnceWith('com.knothhe.passpine');
    switchTarget('dev');
    expect(environment.disconnect).toHaveBeenCalledTimes(1);
    await request({ action: 'touch' }, 'dev');
    expect(environment.browser.runtime.connectNative.mock.calls).toEqual([['com.knothhe.passpine'], ['com.knothhe.passpine.dev']]);
  });

  it('enforces the store target policy even for a directly stored Dev value', async () => {
    environment.browser.management.getSelf.mockResolvedValue({ installType: 'normal' });
    stored.clientTarget = 'dev';
    await request({ action: 'copy-generated', contents: 'fresh-password' });
    expect(environment.browser.runtime.connectNative).toHaveBeenCalledExactlyOnceWith('com.knothhe.passpine');
  });

  it.each(['release', 'dev'] as const)('never falls back or replays a failed %s mutation on the other client', async target => {
    stored.clientTarget = target;
    const { PassPineError } = await import('./errors');
    environment.native.mockRejectedValue(new PassPineError('app_unavailable'));
    expect(await request({ action: 'save', vaultId: 'vault', entry: account, contents: 'edited', mode: 'edit', revision: 'old' }, target)).toMatchObject({ status: 'error', code: 'app_unavailable' });
    expect(environment.native).toHaveBeenCalledTimes(1);
    expect(environment.browser.runtime.connectNative).toHaveBeenCalledExactlyOnceWith(target === 'dev' ? 'com.knothhe.passpine.dev' : 'com.knothhe.passpine');
  });

  it('discards credentials from an old target before injecting or associating them', async () => {
    const credentials = deferred<{ contents: string }>();
    environment.native.mockImplementation((value: NativeRequest) => value.action === 'fetch' ? credentials.promise : Promise.resolve(state));
    const filling = fill();
    await vi.waitFor(() => expect(environment.native).toHaveBeenCalledTimes(1));
    switchTarget('dev');
    expect(environment.disconnect).toHaveBeenCalledTimes(1);
    credentials.resolve({ contents: 'old-target-secret' });
    expect(await filling).toMatchObject({ status: 'error', code: 'native_connection_failed' });
    expect(environment.browser.scripting.executeScript).not.toHaveBeenCalled();
    expect(environment.browser.storage.local.set).not.toHaveBeenCalled();
    expect(environment.browser.runtime.connectNative).toHaveBeenCalledExactlyOnceWith('com.knothhe.passpine');
    await request({ action: 'touch' }, 'dev');
    expect(environment.browser.runtime.connectNative).toHaveBeenLastCalledWith('com.knothhe.passpine.dev');
  });

  it('rejects a fill waiting for tab metadata without sending its old vault request to a new target', async () => {
    const tabs = deferred<Array<{ id: number; url: string }>>();
    environment.browser.tabs.query.mockReturnValueOnce(tabs.promise);
    const filling = fill();
    await vi.waitFor(() => expect(environment.browser.tabs.query).toHaveBeenCalledTimes(1));
    switchTarget('dev');
    tabs.resolve([current]);
    expect(await filling).toMatchObject({ status: 'error', code: 'native_connection_failed' });
    expect(environment.native).not.toHaveBeenCalled();
    expect(environment.browser.scripting.executeScript).not.toHaveBeenCalled();
    expect(environment.browser.storage.local.set).not.toHaveBeenCalled();
    await request({ action: 'touch' }, 'dev');
    expect(environment.browser.runtime.connectNative).toHaveBeenCalledExactlyOnceWith('com.knothhe.passpine.dev');
  });

  it('stops association writes when the target switches after DOM filling', async () => {
    environment.browser.scripting.executeScript.mockImplementation(async () => {
      switchTarget('dev');
      return [{ frameId: 0, result: { filled: 2 } }];
    });
    expect(await fill()).toMatchObject({ status: 'error', code: 'native_connection_failed' });
    expect(environment.native.mock.calls.map(([value]) => value.action)).toEqual(['fetch']);
    expect(environment.browser.storage.local.set).not.toHaveBeenCalled();
    expect(environment.browser.runtime.connectNative).toHaveBeenCalledExactlyOnceWith('com.knothhe.passpine');
  });

  it('invalidates a request synchronously while the installation policy is still resolving', async () => {
    const policy = deferred<{ installType: string }>();
    environment.browser.management.getSelf.mockReturnValue(policy.promise);
    const pending = request({ action: 'save', vaultId: 'vault', entry: account, contents: 'edited', mode: 'edit', revision: 'old' });
    switchTarget('dev');
    policy.resolve({ installType: 'development' });
    expect(await pending).toMatchObject({ status: 'error', code: 'native_connection_failed' });
    expect(environment.native).not.toHaveBeenCalled();
    expect(environment.browser.runtime.connectNative).not.toHaveBeenCalled();
  });

  it.each([
    { action: 'save', vaultId: 'vault', entry: account, contents: 'old-editor', mode: 'edit', revision: 'old' },
    { action: 'lock' },
    { action: 'copy-generated', contents: 'old-generated-password' },
  ] satisfies UIRequest[])('rejects a late $action message from the previous UI even with the same vault identity', async mutation => {
    expect((await request({ action: 'snapshot' })).status).toBe('ok');
    environment.native.mockClear();
    switchTarget('dev');
    expect(await request(mutation, 'release')).toMatchObject({ status: 'error', code: 'native_connection_failed' });
    expect(environment.native).not.toHaveBeenCalled();
    expect(environment.browser.runtime.connectNative).toHaveBeenCalledExactlyOnceWith('com.knothhe.passpine');
    // The Dev fixture deliberately uses the same vaultId. Only an explicit
    // request from a fresh Dev UI may reach it.
    expect((await request({ action: 'status' }, 'dev')).status).toBe('ok');
    expect(environment.browser.runtime.connectNative).toHaveBeenLastCalledWith('com.knothhe.passpine.dev');
  });

  it.each([undefined, 'other', null])('rejects a missing or invalid UI source target: %s', async clientTarget => {
    const reply = vi.fn();
    expect(listener({ type: 'passpine', clientTarget, request: { action: 'lock' } }, { id: 'test-extension', url: 'chrome-extension://test-extension/popup.html' }, reply)).toBe(true);
    expect(reply).toHaveBeenCalledWith(expect.objectContaining({ status: 'error', code: 'native_connection_failed' }));
    expect(environment.native).not.toHaveBeenCalled();
    expect(environment.browser.runtime.connectNative).not.toHaveBeenCalled();
  });

  it('accepts bound UI messages after a background worker restart without requiring a new UI handshake', async () => {
    expect((await request({ action: 'status' }, 'release')).status).toBe('ok');
    vi.resetModules();
    await import('../../entrypoints/background');
    listener = environment.browser.runtime.onMessage.addListener.mock.calls.at(-1)![0] as MessageListener;
    expect((await request({ action: 'status' }, 'release')).status).toBe('ok');
    expect(environment.browser.runtime.connectNative.mock.calls).toEqual([['com.knothhe.passpine'], ['com.knothhe.passpine']]);
  });

  it('does not trust a source target attached to a message from a web page', () => {
    const reply = vi.fn();
    expect(listener({ type: 'passpine', clientTarget: 'dev', request: { action: 'lock' } }, { id: 'test-extension', url: 'https://attacker.test/' }, reply)).toBe(false);
    expect(reply).not.toHaveBeenCalled();
    expect(environment.native).not.toHaveBeenCalled();
  });
});

describe('page shortcut messages', () => {
  const press = (command: string, shortcut: string, sender = { id: 'test-extension', url: tab.url, tab: { id: tab.id, url: tab.url } }): Promise<unknown> => new Promise(resolve => {
    expect(listener({ type: 'passpine-page-shortcut', command, shortcut }, sender, resolve)).toBe(true);
  });
  it('opens the popup without exposing or reading credentials', async () => {
    stored.pageShortcuts = { '_execute_action': 'Alt+O', 'fill-best': '' };
    expect(await press('_execute_action', 'Alt+O')).toEqual({ handled: true });
    expect(environment.browser.action.openPopup).toHaveBeenCalledTimes(1);
    expect(environment.native).not.toHaveBeenCalled();
  });
  it('fills the sole matching account and returns no credentials to the content script', async () => {
    stored.pageShortcuts = { '_execute_action': '', 'fill-best': 'Alt+F' };
    state.entries = [{ path: 'login.example.test/account', name: 'account', folder: 'login.example.test' }];
    expect(await press('fill-best', 'Alt+F')).toEqual({ handled: true });
    expect(environment.native).toHaveBeenCalledWith(expect.objectContaining({ action: 'fetch', purpose: 'fill', entry: 'login.example.test/account' }));
    expect(environment.browser.scripting.executeScript).toHaveBeenCalledWith(expect.objectContaining({ target: { tabId: tab.id, frameIds: [0] } }));
  });
  it('rejects stale keys, revoked access and a tab that is no longer active', async () => {
    stored.pageShortcuts = { '_execute_action': 'Alt+O', 'fill-best': '' };
    expect(await press('_execute_action', 'Alt+L')).toEqual({ handled: false });
    environment.browser.permissions.contains.mockResolvedValueOnce(false);
    expect(await press('_execute_action', 'Alt+O')).toEqual({ handled: false });
    current = { id: 99, url: tab.url };
    expect(await press('_execute_action', 'Alt+O')).toEqual({ handled: false });
    expect(environment.browser.action.openPopup).not.toHaveBeenCalled();
    expect(environment.native).not.toHaveBeenCalled();
  });
  it('rejects webpage credential requests and unknown shortcut actions', () => {
    const sender = { id: 'test-extension', url: tab.url, tab: { id: tab.id, url: tab.url } };
    expect(listener({ type: 'passpine', clientTarget: 'release', request: { action: 'fetch', vaultId: 'vault', entry: account } }, sender, vi.fn())).toBe(false);
    expect(listener({ type: 'passpine-page-shortcut', command: 'fetch', shortcut: 'Alt+F' }, sender, vi.fn())).toBe(false);
    expect(environment.native).not.toHaveBeenCalled();
  });
});
