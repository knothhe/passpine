import { browser } from 'wxt/browser';
import { PassPineError, errorMessage } from '../src/lib/errors';
import { associationKey, parseDocument, pathMatchesHost, sameTab, tabContext } from '../src/lib/model';
import { fillPage } from '../src/lib/fill';
import { nativeRequest, trustedSender, validUIRequest } from '../src/lib/protocol';
import { NativeConnection } from '../src/lib/native';
import { hostForTarget, loadClientTarget, subscribeClientTarget, type ClientTarget } from '../src/lib/client-target';
import { FILL_COMMAND } from '../src/lib/shortcuts';
import { initializeToolbarAppearance } from '../src/lib/appearance';
import { PROTOCOL, type EntryDocument, type FillResult, type NativeRequest, type SessionStatus, type Snapshot, type TabContext, type UIRequest, type VaultState } from '../src/lib/types';

interface NativeSession {
  assertCurrent(): void;
  request<T>(request: NativeRequest): Promise<T>;
}

export default defineBackground(() => {
  initializeToolbarAppearance();
  let generation = 0;
  let selected: { target: Awaited<ReturnType<typeof loadClientTarget>>; connection: NativeConnection } | undefined;
  const invalidateTarget = () => {
    generation++;
    selected?.connection.disconnect();
    selected = undefined;
  };
  subscribeClientTarget(() => {}, false, invalidateTarget);
  const session = async (expectedTarget?: ClientTarget): Promise<NativeSession> => {
    const loadingGeneration = generation;
    // Read through the install policy for every operation, including direct
    // mutations. A stored Dev value cannot reroute a store installation.
    const target = await loadClientTarget();
    if (loadingGeneration !== generation || (expectedTarget !== undefined && expectedTarget !== target)) throw new PassPineError('native_connection_failed');
    if (selected && selected.target !== target) invalidateTarget();
    if (!selected) selected = { target, connection: new NativeConnection(() => browser.runtime.connectNative(hostForTarget(target))) };
    const captured = selected, capturedGeneration = generation;
    const assertCurrent = () => {
      if (capturedGeneration !== generation || selected !== captured) throw new PassPineError('native_connection_failed');
    };
    return {
      assertCurrent,
      async request<T>(request: NativeRequest): Promise<T> {
        assertCurrent();
        const response = await captured.connection.request<T>(request);
        assertCurrent();
        return response;
      },
    };
  };
  const activeTab = async () => (await browser.tabs.query({ active: true, currentWindow: true }))[0];
  const currentTab = async (expected: TabContext) => {
    const tab = await activeTab();
    if (!tab || !sameTab(expected, tab)) throw new PassPineError('tab_changed');
    return tabContext(tab)!;
  };
  const associations = async (): Promise<Record<string, boolean>> => {
    const data = await browser.storage.local.get('associations');
    return data.associations && typeof data.associations === 'object' && !Array.isArray(data.associations) ? Object.fromEntries(Object.entries(data.associations).filter(([key, value]) => /^[a-f0-9]{64}$/.test(key) && typeof value === 'boolean')) : {};
  };
  const snapshot = async (native: NativeSession): Promise<Snapshot> => {
    native.assertCurrent();
    const [state, tab, links] = await Promise.all([native.request<VaultState>({ protocol: PROTOCOL, action: 'configure', includeEntries: true }), activeTab(), associations()]);
    native.assertCurrent();
    const context = tab && tabContext(tab);
    const entries = Array.isArray(state.entries) ? state.entries : [];
    const matches = context ? (await Promise.all(entries.map(async entry => {
      const linked = links[await associationKey(state.vaultId, entry.path, context.hostname)];
      return (linked ?? pathMatchesHost(entry.path, context.hostname)) ? entry.path : undefined;
    }))).filter((value): value is string => !!value) : [];
    native.assertCurrent();
    return { ...state, entries, capabilities: state.capabilities ?? [], tab: context, matches };
  };
  let pendingAssociation: Promise<unknown> = Promise.resolve();
  const setAssociation = (request: Extract<UIRequest, { action: 'fill' | 'associate' | 'dissociate' }>, associated: boolean, native: NativeSession): Promise<void> => {
    const write = pendingAssociation.then(async () => {
      native.assertCurrent();
      const context = await currentTab(request.tab);
      const state = await native.request<VaultState>({ protocol: PROTOCOL, action: 'list', vaultId: request.vaultId });
      if (state.vaultId !== request.vaultId || !state.entries.some(entry => entry.path === request.entry)) throw new PassPineError('vault_changed');
      const key = await associationKey(request.vaultId, request.entry, context.hostname);
      native.assertCurrent();
      const links = await associations();
      await currentTab(request.tab);
      native.assertCurrent();
      links[key] = associated;
      await browser.storage.local.set({ associations: links });
      native.assertCurrent();
    });
    pendingAssociation = write.catch(() => {});
    return write;
  };
  const execute = async (request: UIRequest, native: NativeSession): Promise<unknown> => {
    native.assertCurrent();
    if (request.action === 'snapshot') return snapshot(native);
    if (request.action === 'status' || request.action === 'lock') return native.request({ protocol: PROTOCOL, action: request.action });
    if (request.action === 'unlock') return native.request({ protocol: PROTOCOL, action: 'unlock', interactive: true });
    if (request.action === 'fetch') {
      const detail = await native.request<EntryDocument & SessionStatus>(nativeRequest(request));
      const { configured, locked, vaultId, vaultName, entriesVersion } = detail;
      return { ...detail, session: { configured, locked, vaultId, vaultName, entriesVersion } };
    }
    if (request.action === 'associate' || request.action === 'dissociate') {
      await setAssociation(request, request.action === 'associate', native);
      return {};
    }
    if (request.action === 'fill') {
      const context = await currentTab(request.tab);
      const detail = await native.request<EntryDocument>({ protocol: PROTOCOL, action: 'fetch', vaultId: request.vaultId, entry: request.entry, purpose: 'fill', interactive: true });
      const credentials = parseDocument(detail.contents);
      await currentTab(request.tab);
      native.assertCurrent();
      const results = await browser.scripting.executeScript({ target: { tabId: context.id, frameIds: [0] }, func: fillPage,
        args: [context.url, { username: credentials.username, password: credentials.password, otp: detail.otp }] });
      native.assertCurrent();
      const result = results.find(result => result.frameId === 0)?.result;
      if (result?.code || !result?.filled) throw new PassPineError(result?.code ?? 'no_fields');
      // Filling has already succeeded. A later navigation or association failure
      // must not turn that completed action into a reported fill failure.
      let associated = false;
      try { await setAssociation(request, true, native); associated = true; } catch {}
      native.assertCurrent();
      return { filled: result.filled, associated } satisfies FillResult;
    }
    return native.request(nativeRequest(request));
  };
  const dispatch = async (request: UIRequest, expectedTarget: ClientTarget): Promise<unknown> => execute(request, await session(expectedTarget));
  browser.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (!trustedSender(sender, browser.runtime.id, browser.runtime.getURL('/')) || message?.type !== 'passpine' || !validUIRequest(message.request)) return false;
    const fail = (error: unknown) => sendResponse({ protocol: PROTOCOL, status: 'error', code: error instanceof PassPineError ? error.code : 'operation_failed', message: errorMessage(error) });
    if (message.clientTarget !== 'release' && message.clientTarget !== 'dev') {
      fail(new PassPineError('native_connection_failed'));
      return true;
    }
    dispatch(message.request, message.clientTarget).then(data => sendResponse({ protocol: PROTOCOL, status: 'ok', data }), fail);
    return true;
  });
  browser.commands.onCommand.addListener(async command => {
    if (command !== FILL_COMMAND) return;
    try {
      const native = await session();
      const state = await snapshot(native);
      // Ambiguous matches always go through the user's account selection.
      if (state.matches.length === 1 && state.tab) await execute({ action: 'fill', vaultId: state.vaultId, entry: state.matches[0]!, tab: state.tab }, native);
      else await browser.action.openPopup();
    } catch { await browser.action.openPopup().catch(() => {}); }
  });
});
