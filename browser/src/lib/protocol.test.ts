import { describe, expect, it } from 'vitest';
import { nativeRequest, trustedSender, unwrap, validUIRequest } from './protocol';
import type { UIRequest } from './types';

describe('extension boundary', () => {
  const id = 'our-extension', root = 'chrome-extension://our-extension/';
  it('accepts only exact trusted extension UI pages', () => {
    expect(trustedSender({ id, url: `${root}popup.html` }, id, root)).toBe(true);
    expect(trustedSender({ id, url: `${root}options.html` }, id, root)).toBe(true);
    for (const sender of [{ id: 'other', url: `${root}popup.html` }, { id, url: 'https://evil.test' }, { id, url: `${root}popup.html`, tab: {} }, { id, url: `${root}popup.html?untrusted` }, { id, url: `${root}background.js` }]) expect(trustedSender(sender, id, root)).toBe(false);
  });
  it('whitelists protocol arguments and retains the original edit revision', () => {
    const encoded = nativeRequest({ action: 'save', vaultId: 'vault', entry: 'old', destination: 'new', contents: 'secret', mode: 'edit', revision: 'original', directory: '/tmp/attacker' } as UIRequest & { action: 'save' });
    expect(encoded).toEqual({ protocol: 1, action: 'save', vaultId: 'vault', entry: 'old', destination: 'new', contents: 'secret', mode: 'edit', revision: 'original' });
    const generated = nativeRequest({ action: 'generate', options: { length: 24, lowercase: true, uppercase: true, numbers: true, symbols: true, excludeAmbiguous: true, action: 'unlock', passphrase: 'injected' } } as unknown as Extract<UIRequest, { action: 'generate' }>);
    expect(generated.action).toBe('generate');
    expect(generated).not.toHaveProperty('passphrase');
  });
  it('requires vault identity and revision for destructive operations', () => {
    expect(validUIRequest({ action: 'delete', entry: 'entry', revision: 'old' })).toBe(false);
    expect(validUIRequest({ action: 'save', vaultId: 'vault', entry: 'entry', mode: 'edit', contents: 'new' })).toBe(false);
    expect(validUIRequest({ action: 'save', vaultId: 'vault', entry: 'entry', mode: 'edit', contents: 'new', revision: 'old' })).toBe(true);
    expect(validUIRequest({ action: 'fetch', vaultId: 'vault', entry: '../secret' })).toBe(false);
  });
  it('rejects protocol mismatch and malformed replies', () => {
    expect(() => unwrap({ protocol: 2, status: 'ok', data: {} })).toThrow();
    expect(() => unwrap({ protocol: 1, status: 'ok' })).toThrow();
    expect(() => unwrap({ protocol: 1, status: 'error', code: 'locked' })).toThrow(/锁定/);
    expect(unwrap({ protocol: 1, status: 'ok', data: {} })).toEqual({});
  });
});
