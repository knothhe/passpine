import { afterEach, describe, expect, it, vi } from 'vitest';
import { NativeConnection, type NativePort } from './native';
import type { NativeRequest } from './types';

function mockPort() {
  const messages: NativeRequest[] = [];
  let onMessage: (message: unknown) => void = () => {}, onDisconnect: () => void = () => {};
  const port: NativePort = { postMessage: message => messages.push(message as NativeRequest), disconnect: () => onDisconnect(), onMessage: { addListener: fn => { onMessage = fn; } }, onDisconnect: { addListener: fn => { onDisconnect = fn; } } };
  return { port, messages, disconnect: () => onDisconnect(), reply: (index: number, data: unknown) => onMessage({ protocol: 1, requestId: messages[index]!.requestId, status: 'ok', data }) };
}
afterEach(() => vi.useRealTimers());
describe('persistent native connection', () => {
  it('reuses one process, correlates replies, and coalesces only configure reads', async () => {
    const channel = mockPort(), connect = vi.fn(() => channel.port), connection = new NativeConnection(connect);
    const first = connection.request({ protocol: 1, action: 'configure' });
    const shared = connection.request({ protocol: 1, action: 'configure' });
    const fetch = connection.request({ protocol: 1, action: 'fetch', vaultId: 'vault', entry: 'entry' });
    expect(connect).toHaveBeenCalledTimes(1); expect(channel.messages).toHaveLength(2);
    channel.reply(1, { contents: 'secret' }); channel.reply(0, { locked: false });
    expect(await first).toEqual(await shared); expect(await fetch).toEqual({ contents: 'secret' });
    const later = connection.request({ protocol: 1, action: 'configure' });
    expect(channel.messages).toHaveLength(3); channel.reply(2, {}); await later;
  });
  it('rejects in-flight mutations on disconnect and never replays them', async () => {
    const first = mockPort(), second = mockPort(); let count = 0;
    const connection = new NativeConnection(() => count++ === 0 ? first.port : second.port);
    const save = connection.request({ protocol: 1, action: 'save', mode: 'edit', entry: 'entry', revision: 'old' });
    const result = expect(save).rejects.toThrow();
    first.disconnect(); await result;
    expect(count).toBe(1); expect(second.messages).toHaveLength(0);
    const retry = connection.request({ protocol: 1, action: 'configure' });
    expect(count).toBe(2); expect(second.messages.map(message => message.action)).toEqual(['configure']);
    second.reply(0, {}); await retry;
  });
  it('retires a selected client, rejects queued metadata and never reconnects it', async () => {
    const channel = mockPort(), connect = vi.fn(() => channel.port), connection = new NativeConnection(connect);
    const fetch = connection.request({ protocol: 1, action: 'fetch', vaultId: 'vault', entry: 'entry', interactive: true });
    const configure = connection.request({ protocol: 1, action: 'configure' });
    const shared = connection.request({ protocol: 1, action: 'configure' });
    const status = connection.request({ protocol: 1, action: 'status' });
    const results = Promise.allSettled([fetch, configure, shared, status]);
    expect(channel.messages.map(message => message.action)).toEqual(['fetch']);
    connection.disconnect();
    for (const result of await results) {
      expect(result.status).toBe('rejected');
      if (result.status === 'rejected') expect(result.reason).toMatchObject({ code: 'native_connection_failed' });
    }
    channel.reply(0, { contents: 'late-secret' });
    await expect(connection.request({ protocol: 1, action: 'configure' })).rejects.toMatchObject({ code: 'native_connection_failed' });
    await expect(connection.request({ protocol: 1, action: 'save', entry: 'entry', contents: 'changed', mode: 'edit', revision: 'old' })).rejects.toMatchObject({ code: 'native_connection_failed' });
    expect(connect).toHaveBeenCalledTimes(1);
    expect(channel.messages.map(message => message.action)).toEqual(['fetch']);
  });
  it.each(['copy', 'copy-generated'] as const)('does not deduplicate %s requests', async action => {
    const channel = mockPort(), connection = new NativeConnection(() => channel.port);
    const contents = action === 'copy-generated' ? { contents: 'fresh-password' } : { entry: 'same' };
    const one = connection.request({ protocol: 1, action, ...contents });
    const two = connection.request({ protocol: 1, action, ...contents });
    expect(channel.messages).toHaveLength(2);
    expect(channel.messages[0]!.requestId).not.toEqual(channel.messages[1]!.requestId);
    channel.reply(0, {}); channel.reply(1, {}); await Promise.all([one, two]);
  });
  it('expires stalled requests without retrying and ignores their late replies', async () => {
    vi.useFakeTimers(); const channel = mockPort(), connection = new NativeConnection(() => channel.port, 100);
    const request = connection.request({ protocol: 1, action: 'delete', entry: 'entry', revision: 'old' });
    const assertion = expect(request).rejects.toThrow();
    await vi.advanceTimersByTimeAsync(101); await assertion;
    channel.reply(0, {}); expect(channel.messages).toHaveLength(1);
  });
});
