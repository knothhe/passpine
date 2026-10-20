import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { client } from './client';

const runtime = vi.hoisted(() => ({ sendMessage: vi.fn(), openOptionsPage: vi.fn() }));
vi.mock('wxt/browser', () => ({ browser: { runtime } }));
const writeText = vi.fn();
beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal('navigator', { language: 'zh-CN', clipboard: { writeText } });
});
afterEach(() => vi.unstubAllGlobals());

describe('generated password clipboard client', () => {
  it('uses the native background request for copying passwords', async () => {
    runtime.sendMessage.mockResolvedValue({ protocol: 1, status: 'ok', data: {} });
    await client.copy('fresh-password');
    expect(runtime.sendMessage).toHaveBeenCalledExactlyOnceWith({ type: 'passpine', request: { action: 'copy-generated', contents: 'fresh-password' } });
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
