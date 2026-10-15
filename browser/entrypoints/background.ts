import { browser } from 'wxt/browser';
import { PassPineError, errorMessage } from '../src/lib/errors';
import { associationKey, credentialMatchesHost, parseDocument, pathMatchesHost, sameTab, tabContext } from '../src/lib/model';
import { fillPage } from '../src/lib/fill';
import { nativeRequest, trustedSender, validUIRequest } from '../src/lib/protocol';
import { NativeConnection } from '../src/lib/native';
import { FILL_COMMAND } from '../src/lib/shortcuts';
import { HOST, PROTOCOL, type EntryDocument, type NativeRequest, type SessionStatus, type Snapshot, type TabContext, type UIRequest, type VaultState } from '../src/lib/types';

export default defineBackground(() => {
  const connection = new NativeConnection(() => browser.runtime.connectNative(HOST));
  const native = <T>(request: NativeRequest): Promise<T> => connection.request<T>(request);
  const activeTab = async () => (await browser.tabs.query({ active: true, currentWindow: true }))[0];
  const currentTab = async (expected: TabContext) => {
    const tab = await activeTab();
    if (!tab || !sameTab(expected, tab)) throw new PassPineError('tab_changed');
    return tabContext(tab)!;
  };
  const associations = async (): Promise<Record<string, boolean>> => {
    const data = await browser.storage.local.get('associations');
    return data.associations && typeof data.associations === 'object' && !Array.isArray(data.associations) ? Object.fromEntries(Object.entries(data.associations).filter(([key, value]) => /^[a-f0-9]{64}$/.test(key) && value === true)) : {};
  };
  const snapshot = async (): Promise<Snapshot> => {
    const [state, tab, links] = await Promise.all([native<VaultState>({ protocol: PROTOCOL, action: 'configure', includeEntries: true }), activeTab(), associations()]);
    const context = tab && tabContext(tab);
    const entries = Array.isArray(state.entries) ? state.entries : [];
    const matches = context ? (await Promise.all(entries.map(async entry => pathMatchesHost(entry.path, context.hostname) || links[await associationKey(state.vaultId, entry.path, context.hostname)] ? entry.path : undefined))).filter((value): value is string => !!value) : [];
    return { ...state, entries, capabilities: state.capabilities ?? [], tab: context, matches };
  };
  let pendingAssociation: Promise<unknown> = Promise.resolve();
  const dispatch = async (request: UIRequest): Promise<unknown> => {
    if (request.action === 'snapshot') return snapshot();
    if (request.action === 'status' || request.action === 'lock') return native({ protocol: PROTOCOL, action: request.action });
    if (request.action === 'unlock') return native({ protocol: PROTOCOL, action: 'unlock', interactive: true });
    if (request.action === 'fetch') {
      const detail = await native<EntryDocument & SessionStatus>(nativeRequest(request));
      const { configured, locked, vaultId, vaultName, entriesVersion } = detail;
      return { ...detail, session: { configured, locked, vaultId, vaultName, entriesVersion } };
    }
    if (request.action === 'associate') {
      const context = await currentTab(request.tab);
      const state = await native<VaultState>({ protocol: PROTOCOL, action: 'list', vaultId: request.vaultId });
      if (state.vaultId !== request.vaultId || !state.entries.some(entry => entry.path === request.entry)) throw new PassPineError('vault_changed');
      const key = await associationKey(request.vaultId, request.entry, context.hostname);
      const write = pendingAssociation.then(async () => {
        await currentTab(request.tab);
        const links = await associations();
        links[key] = true;
        await browser.storage.local.set({ associations: links });
      });
      pendingAssociation = write.catch(() => {});
      await write;
      return {};
    }
    if (request.action === 'fill') {
      const context = await currentTab(request.tab);
      const links = await associations();
      const detail = await native<EntryDocument>({ protocol: PROTOCOL, action: 'fetch', vaultId: request.vaultId, entry: request.entry, purpose: 'fill', interactive: true });
      const credentials = parseDocument(detail.contents);
      const linked = links[await associationKey(request.vaultId, request.entry, context.hostname)];
      if (!linked && !pathMatchesHost(request.entry, context.hostname) && !credentialMatchesHost(credentials, context.hostname)) throw new PassPineError('domain_mismatch');
      await currentTab(request.tab);
      const results = await browser.scripting.executeScript({ target: { tabId: context.id, frameIds: [0] }, func: fillPage,
        args: [context.url, { username: credentials.username, password: credentials.password, otp: detail.otp }] });
      const result = results.find(result => result.frameId === 0)?.result;
      if (result?.code || !result?.filled) throw new PassPineError(result?.code ?? 'no_fields');
      return result;
    }
    return native(nativeRequest(request));
  };
  browser.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (!trustedSender(sender, browser.runtime.id, browser.runtime.getURL('/')) || message?.type !== 'passpine' || !validUIRequest(message.request)) return false;
    dispatch(message.request).then(data => sendResponse({ protocol: PROTOCOL, status: 'ok', data }), error => sendResponse({ protocol: PROTOCOL, status: 'error', code: error instanceof PassPineError ? error.code : 'operation_failed', message: errorMessage(error) }));
    return true;
  });
  browser.commands.onCommand.addListener(async command => {
    if (command !== FILL_COMMAND) return;
    try {
      const state = await snapshot();
      // Ambiguous matches always go through the user's account selection.
      if (state.matches.length === 1 && state.tab) await dispatch({ action: 'fill', vaultId: state.vaultId, entry: state.matches[0]!, tab: state.tab });
      else await browser.action.openPopup();
    } catch { await browser.action.openPopup().catch(() => {}); }
  });
});
