import { PassPineError } from './errors';
import { validateEntry } from './model';
import { PROTOCOL, type NativeRequest, type NativeResponse, type UIRequest } from './types';

export function nativeRequest(request: Exclude<UIRequest, { action: 'snapshot' | 'status' | 'lock' | 'unlock' } | { action: 'fill' | 'associate' | 'dissociate' }>): NativeRequest {
  if (request.action === 'touch') return { protocol: PROTOCOL, action: 'touch' };
  if (request.action === 'generate') return { protocol: PROTOCOL, action: 'generate', length: request.options.length, lowercase: request.options.lowercase, uppercase: request.options.uppercase, numbers: request.options.numbers, symbols: request.options.symbols, excludeAmbiguous: request.options.excludeAmbiguous };
  if (request.action === 'copy-generated') return { protocol: PROTOCOL, action: 'copy-generated', contents: request.contents };
  if (request.action === 'fetch') return { protocol: PROTOCOL, action: 'fetch', vaultId: request.vaultId, entry: request.entry, purpose: 'view', ...(request.interactive === true ? { interactive: true } : {}) };
  if (request.action === 'copy') return { protocol: PROTOCOL, action: 'copy', vaultId: request.vaultId, entry: request.entry, field: request.field, interactive: true };
  if (request.action === 'delete') return { protocol: PROTOCOL, action: 'delete', vaultId: request.vaultId, entry: request.entry, revision: request.revision };
  return { protocol: PROTOCOL, action: 'save', vaultId: request.vaultId, entry: request.entry, contents: request.contents, mode: request.mode, revision: request.revision, destination: request.destination };
}

export function unwrap<T>(value: unknown): T {
  const response = value as NativeResponse<T> | undefined;
  if (!response || typeof response !== 'object' || Array.isArray(response) || typeof response.protocol !== 'number') throw new PassPineError('invalid_response');
  if (response.protocol !== PROTOCOL) throw new PassPineError('protocol_mismatch');
  if (!['ok', 'error'].includes(response.status)) throw new PassPineError('invalid_response');
  if (response.status !== 'ok') throw new PassPineError(response.code ?? 'invalid_request', response.message);
  if (response.data === undefined) throw new PassPineError('invalid_response');
  return response.data;
}

export function trustedSender(sender: { id?: string; url?: string; tab?: unknown }, runtimeId: string, rootURL: string): boolean {
  if (sender.id !== runtimeId || !sender.url) return false;
  try {
    const page = new URL(sender.url).href;
    const options = new URL(page); options.hash = '';
    // Options are opened in a browser tab, so their trusted extension sender
    // may include tab metadata. The exact extension URL still excludes scripts
    // injected into arbitrary websites.
    if (options.href === new URL('options.html', rootURL).href) return true;
    return sender.tab === undefined && page === new URL('popup.html', rootURL).href;
  } catch { return false; }
}

export function validUIRequest(value: unknown): value is UIRequest {
  if (!value || typeof value !== 'object') return false;
  const request = value as Record<string, unknown>;
  if (['snapshot', 'status', 'touch', 'lock', 'unlock'].includes(String(request.action))) return true;
  if (request.action === 'copy-generated') return typeof request.contents === 'string' && request.contents.length > 0 && request.contents.length <= 128;
  if (request.action === 'generate') {
    const options = request.options as Record<string, unknown> | undefined;
    return !!options && Number.isInteger(options.length) && Number(options.length) >= 4 && Number(options.length) <= 128 &&
      ['lowercase', 'uppercase', 'numbers', 'symbols', 'excludeAmbiguous'].every(key => typeof options[key] === 'boolean');
  }
  if (typeof request.vaultId !== 'string' || !request.vaultId || request.vaultId.length > 256 || typeof request.entry !== 'string' || !validateEntry(request.entry)) return false;
  if (request.action === 'fetch') return request.interactive === undefined || typeof request.interactive === 'boolean';
  if (request.action === 'copy') return ['password', 'username', 'otp'].includes(String(request.field));
  if (request.action === 'delete') return typeof request.revision === 'string';
  if (request.action === 'save') return typeof request.contents === 'string' && request.contents.length <= 512_000 &&
    ['create', 'edit'].includes(String(request.mode)) && (request.mode === 'create' || typeof request.revision === 'string') &&
    (request.destination === undefined || (typeof request.destination === 'string' && validateEntry(request.destination)));
  if (['fill', 'associate', 'dissociate'].includes(String(request.action))) {
    const tab = request.tab as Record<string, unknown> | undefined;
    return !!tab && Number.isInteger(tab.id) && typeof tab.url === 'string' && typeof tab.hostname === 'string';
  }
  return false;
}
