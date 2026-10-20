import { beforeEach, describe, expect, it, vi } from 'vitest';
import { associationKey } from './model';
import type { FillResult, NativeRequest, NativeResponse, Snapshot, TabContext, UIRequest, VaultState } from './types';

const environment = vi.hoisted(() => ({
  native: vi.fn(),
  browser: {
    runtime: { id: 'test-extension', getURL: vi.fn(), onMessage: { addListener: vi.fn() } },
    tabs: { query: vi.fn() },
    storage: { local: { get: vi.fn(), set: vi.fn() } },
    scripting: { executeScript: vi.fn() },
    commands: { onCommand: { addListener: vi.fn() } },
    action: { openPopup: vi.fn() },
  },
}));
vi.mock('wxt/browser', () => ({ browser: environment.browser }));
vi.mock('./appearance', () => ({ initializeToolbarAppearance: vi.fn() }));
vi.mock('./native', () => ({ NativeConnection: class { request = environment.native; } }));

type MessageListener = (message: unknown, sender: { id: string; url: string }, respond: (response: NativeResponse) => void) => boolean;
const tab: TabContext = { id: 7, url: 'https://login.example.test/sign-in', hostname: 'login.example.test', secure: true };
const account = 'Personal/account';
let current: { id: number; url: string }, stored: Record<string, unknown>, listener: MessageListener;
let state: VaultState;

function request<T = Record<string, never>>(value: UIRequest): Promise<NativeResponse<T>> {
  return new Promise(resolve => {
    expect(listener({ type: 'passpine', request: value }, { id: 'test-extension', url: 'chrome-extension://test-extension/popup.html' }, response => resolve(response as NativeResponse<T>))).toBe(true);
  });
}
const fill = (entry = account) => request<FillResult>({ action: 'fill', vaultId: 'vault', entry, tab });
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
  environment.browser.tabs.query.mockImplementation(async () => [current]);
  environment.browser.storage.local.get.mockImplementation(async () => structuredClone(stored));
  environment.browser.storage.local.set.mockImplementation(async value => { stored = structuredClone({ ...stored, ...value }); });
  environment.browser.scripting.executeScript.mockResolvedValue([{ frameId: 0, result: { filled: 2 } }]);
  vi.stubGlobal('defineBackground', (callback: () => void) => callback());
  await import('../../entrypoints/background');
  listener = environment.browser.runtime.onMessage.addListener.mock.calls[0]![0] as MessageListener;
});

describe('background fill and website associations', () => {
  it('copies generated passwords directly through native messaging without reading vault or tab state', async () => {
    environment.native.mockResolvedValue({});
    expect(await request({ action: 'copy-generated', contents: 'fresh-password' })).toEqual({ protocol: 1, status: 'ok', data: {} });
    expect(environment.native).toHaveBeenCalledExactlyOnceWith({ protocol: 1, action: 'copy-generated', contents: 'fresh-password' });
    expect(environment.browser.tabs.query).not.toHaveBeenCalled();
    expect(environment.browser.storage.local.set).not.toHaveBeenCalled();
    expect(environment.browser.storage.local.get).not.toHaveBeenCalled();
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
    environment.browser.storage.local.get.mockImplementation(async () => {
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
