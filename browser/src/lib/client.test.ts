import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Client } from './types';

const runtime = vi.hoisted(() => ({ sendMessage: vi.fn(), openOptionsPage: vi.fn(), getURL: vi.fn(), getContexts: vi.fn() }));
const environment = vi.hoisted(() => ({
  management: { getSelf: vi.fn() },
  storage: { local: { get: vi.fn() }, onChanged: { addListener: vi.fn(), removeListener: vi.fn() } },
}));
vi.mock('wxt/browser', () => ({ browser: { runtime, ...environment } }));
const writeText = vi.fn();
let client: Client, target: 'release' | 'dev';
function switchTarget(next: 'release' | 'dev') {
  target = next;
  for (const [listener] of environment.storage.onChanged.addListener.mock.calls) listener({ clientTarget: { newValue: next } }, 'local');
}
beforeEach(async () => {
  vi.resetAllMocks(); vi.resetModules(); target = 'release';
  environment.management.getSelf.mockResolvedValue({ installType: 'development' });
  environment.storage.local.get.mockImplementation(async () => ({ clientTarget: target }));
  runtime.sendMessage.mockResolvedValue({ protocol: 1, status: 'ok', data: {} });
  vi.stubGlobal('navigator', { language: 'zh-CN', clipboard: { writeText } });
  client = (await import('./client')).client;
});
afterEach(() => vi.unstubAllGlobals());

describe('generated password clipboard client', () => {
  it('uses the native background request for copying passwords', async () => {
    runtime.sendMessage.mockResolvedValue({ protocol: 1, status: 'ok', data: {} });
    await client.copy('fresh-password');
    expect(runtime.sendMessage).toHaveBeenCalledExactlyOnceWith({ type: 'passpine', clientTarget: 'release', request: { action: 'copy-generated', contents: 'fresh-password' } });
    expect(writeText).not.toHaveBeenCalled();
  });

  it.each([
    ['background is unavailable', undefined, 'background_unavailable'],
    ['native host is unavailable', { protocol: 1, status: 'error', code: 'native_connection_failed' }, 'native_connection_failed'],
    ['native host cannot copy', { protocol: 1, status: 'error', code: 'operation_failed' }, 'operation_failed'],
  ])('never falls back to browser copying when %s', async (_reason, response, code) => {
    runtime.sendMessage.mockResolvedValue(response);
    await expect(client.copy('fresh-password')).rejects.toMatchObject({ code });
    expect(writeText).not.toHaveBeenCalled();
  });

  it('reports a disconnected background without browser fallback', async () => {
    runtime.sendMessage.mockRejectedValue(new Error('disconnected'));
    await expect(client.copy('fresh-password')).rejects.toMatchObject({ code: 'background_unavailable' });
    expect(writeText).not.toHaveBeenCalled();
  });
});

describe('UI source scopes', () => {
  it('captures the target before a UI operation and keeps copy bound to the same scope', async () => {
    target = 'dev';
    const scoped = client.scope!();
    expect(environment.management.getSelf).toHaveBeenCalled();
    await vi.waitFor(() => expect(environment.storage.local.get).toHaveBeenCalledExactlyOnceWith('clientTarget'));
    expect(runtime.sendMessage).not.toHaveBeenCalled();
    await scoped.copy('dev-password');
    expect(runtime.sendMessage).toHaveBeenCalledExactlyOnceWith({ type: 'passpine', clientTarget: 'dev', request: { action: 'copy-generated', contents: 'dev-password' } });
    expect(scoped.scope!()).toBe(scoped);
  });

  it.each(['save', 'lock', 'copy-generated'] as const)('rejects a late %s callback from an old UI scope before sending it', async action => {
    const scoped = client.scope!();
    await scoped.request({ action: 'status' });
    runtime.sendMessage.mockClear();
    switchTarget('dev');
    const operation = action === 'save'
      ? scoped.request({ action, vaultId: 'same-vault', entry: 'entry', contents: 'changed', mode: 'edit', revision: 'old' })
      : action === 'lock' ? scoped.request({ action }) : scoped.copy('old-generated-password');
    await expect(operation).rejects.toMatchObject({ code: 'native_connection_failed' });
    expect(runtime.sendMessage).not.toHaveBeenCalled();
  });

  it('keeps an old scope invalid after switching away and back while new requests use the current target', async () => {
    const scoped = client.scope!();
    await scoped.request({ action: 'status' });
    runtime.sendMessage.mockClear();
    switchTarget('dev'); switchTarget('release');
    await expect(scoped.copy('old-password')).rejects.toMatchObject({ code: 'native_connection_failed' });
    expect(runtime.sendMessage).not.toHaveBeenCalled();
    switchTarget('dev');
    await client.copy('new-password');
    expect(runtime.sendMessage).toHaveBeenCalledExactlyOnceWith({ type: 'passpine', clientTarget: 'dev', request: { action: 'copy-generated', contents: 'new-password' } });
    expect(environment.storage.onChanged.addListener).toHaveBeenCalledTimes(1);
  });

  it('invalidates scopes created before target loading finishes', async () => {
    let resolve!: (value: { installType: string }) => void;
    environment.management.getSelf.mockReturnValue(new Promise(done => { resolve = done; }));
    const scoped = client.scope!();
    switchTarget('dev');
    resolve({ installType: 'development' });
    await expect(scoped.request({ action: 'lock' })).rejects.toMatchObject({ code: 'native_connection_failed' });
    expect(runtime.sendMessage).not.toHaveBeenCalled();
  });

  it('discards an old target response without accepting its data after a switch', async () => {
    let resolve!: (value: unknown) => void;
    runtime.sendMessage.mockReturnValue(new Promise(done => { resolve = done; }));
    const pending = client.scope!().request({ action: 'status' });
    await vi.waitFor(() => expect(runtime.sendMessage).toHaveBeenCalledTimes(1));
    switchTarget('dev');
    resolve({ protocol: 1, status: 'ok', data: { vaultId: 'old-vault' } });
    await expect(pending).rejects.toMatchObject({ code: 'native_connection_failed' });
  });
});

describe('popup snapshot identity', () => {
  const url = 'chrome-extension://test-extension/popup.html';
  beforeEach(() => {
    const self = {};
    vi.stubGlobal('window', Object.assign(self, { top: self }));
    vi.stubGlobal('location', new URL(url));
    runtime.getURL.mockReturnValue(url);
  });

  it('sends a verified document claim only on snapshots of the toolbar popup', async () => {
    runtime.getContexts.mockResolvedValue([{ contextType: 'POPUP', documentUrl: url, documentId: 'popup-doc' }]);
    await client.request({ action: 'snapshot' });
    expect(runtime.getContexts).toHaveBeenCalledExactlyOnceWith({ documentUrls: [url] });
    expect(runtime.sendMessage.mock.calls[0]![0].popupInstance).toBe('popup-doc');
    await client.request({ action: 'status' });
    expect(runtime.getContexts).toHaveBeenCalledTimes(1);
    expect(runtime.sendMessage.mock.calls[1]![0]).not.toHaveProperty('popupInstance');
  });

  it('omits the claim on ambiguous, tab, framed, or unidentifiable pages', async () => {
    const popup = { contextType: 'POPUP', documentId: 'popup-doc', documentUrl: url };
    const tab = { contextType: 'TAB', documentId: 'tab-doc', documentUrl: url };
    for (const contexts of [[tab, popup], [tab], [{ ...popup, documentUrl: url + '?test' }], []]) {
      runtime.getContexts.mockResolvedValue(contexts);
      await client.request({ action: 'snapshot' });
    }
    runtime.getContexts.mockRejectedValueOnce(new Error('unsupported'));
    await client.request({ action: 'snapshot' });
    runtime.getContexts.mockResolvedValue([popup]);
    vi.stubGlobal('window', { top: {} });
    await client.request({ action: 'snapshot' });
    expect(runtime.getContexts).toHaveBeenCalledTimes(5);
    expect(runtime.sendMessage).toHaveBeenCalledTimes(6);
    for (const [message] of runtime.sendMessage.mock.calls) expect(message).not.toHaveProperty('popupInstance');
  });
});
