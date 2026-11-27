import { browser } from 'wxt/browser';
import { PassPineError, errorMessage } from '../src/lib/errors';
import { associationKey, parseDocument, pathMatchesHost, sameTab, tabContext } from '../src/lib/model';
import { fillPage } from '../src/lib/fill';
import { nativeRequest, trustedSender, validUIRequest } from '../src/lib/protocol';
import { NativeConnection } from '../src/lib/native';
import { hostForTarget, loadClientTarget, subscribeClientTarget, type ClientTarget } from '../src/lib/client-target';
import { canonicalShortcut, FILL_COMMAND, OPEN_COMMAND } from '../src/lib/shortcuts';
import { hasPageShortcutAccess, readPageShortcuts, type PageShortcutCommand } from '../src/lib/page-shortcuts';
import { initializePageShortcuts } from '../src/lib/page-shortcut-registration';
import { initializeToolbarAppearance } from '../src/lib/appearance';
import { initializeConfigurationSync } from '../src/lib/config-sync';
import { associationCapability, initializeAssociationSync } from '../src/lib/associations';
import { shortcutPopup, uniquePopupAtURL, type ShortcutResult } from '../src/lib/shortcut-popup';
import { PROTOCOL, type EntryDocument, type FillResult, type NativeRequest, type SessionStatus, type Snapshot, type TabContext, type UIRequest, type VaultState } from '../src/lib/types';

interface NativeSession {
  readonly target: ClientTarget;
  assertCurrent(): void;
  request<T>(request: NativeRequest): Promise<T>;
}

export default defineBackground(() => {
  initializeToolbarAppearance();
  initializePageShortcuts();
  const configurationSync = initializeConfigurationSync();
  const associationSync = initializeAssociationSync();
  const popup = shortcutPopup();
  let associationState: VaultState | undefined;
  let generation = 0;
  let selected: { target: Awaited<ReturnType<typeof loadClientTarget>>; connection: NativeConnection } | undefined;
  const invalidateTarget = () => {
    generation++;
    popup.clear();
    configurationSync.invalidate();
    associationSync.invalidate();
    associationState = undefined;
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
    const nativeSession: NativeSession = {
      target,
      assertCurrent,
      async request<T>(request: NativeRequest): Promise<T> {
        assertCurrent();
        const response = await captured.connection.request<T>(request);
        assertCurrent();
        if (request.action === 'configure' || request.action === 'status') configurationSync.observe({ id: capturedGeneration, target: captured.target, assertCurrent, request: next => nativeSession.request(next) }, response);
        if (request.action === 'configure' || request.action === 'list') associationState = response as VaultState;
        return response;
      },
    };
    return nativeSession;
  };
  const activeTab = async () => (await browser.tabs.query({ active: true, currentWindow: true }))[0];
  const currentTab = async (expected: TabContext) => {
    const tab = await activeTab();
    if (!tab || !sameTab(expected, tab)) throw new PassPineError('tab_changed');
    return tabContext(tab)!;
  };
  const snapshot = async (native: NativeSession): Promise<Snapshot> => {
    native.assertCurrent();
    const [state, tab] = await Promise.all([native.request<VaultState>({ protocol: PROTOCOL, action: 'configure', includeEntries: true }), activeTab()]);
    const { links, vaultIds, revision } = await associationSync.read(native, state);
    native.assertCurrent();
    const context = tab && tabContext(tab);
    const entries = Array.isArray(state.entries) ? state.entries : [];
    const matches = context ? (await Promise.all(entries.map(async entry => {
      const values = await Promise.all(vaultIds.map(async id => links[await associationKey(id, entry.path, context.hostname)]));
      // The canonical scope takes priority. For conflicting legacy aliases,
      // retain an explicit unlink until the user links this account again.
      const linked = values[0] ?? (values.includes(false) ? false : values.find(value => value !== undefined));
      return (linked ?? pathMatchesHost(entry.path, context.hostname)) ? entry.path : undefined;
    }))).filter((value): value is string => !!value) : [];
    native.assertCurrent();
    return { ...state, entries, capabilities: state.capabilities ?? [], tab: context, matches, associationVersion: revision };
  };
  let pendingAssociation: Promise<unknown> = Promise.resolve();
  const setAssociation = (request: Extract<UIRequest, { action: 'fill' | 'associate' | 'dissociate' }>, associated: boolean, native: NativeSession): Promise<void> => {
    const write = pendingAssociation.then(async () => {
      native.assertCurrent();
      const context = await currentTab(request.tab);
      const state = await native.request<VaultState>({ protocol: PROTOCOL, action: 'list', vaultId: request.vaultId });
      if (state.vaultId !== request.vaultId || !state.entries.some(entry => entry.path === request.entry)) throw new PassPineError('vault_changed');
      await associationSync.set(native, state, request.entry, context.hostname, associated, async () => {
        await currentTab(request.tab);
        native.assertCurrent();
      });
      native.assertCurrent();
    });
    pendingAssociation = write.catch(() => {});
    return write;
  };
  const execute = async (request: UIRequest, native: NativeSession): Promise<unknown> => {
    native.assertCurrent();
    if (request.action === 'snapshot') return snapshot(native);
    if (request.action === 'status') {
      const state = await native.request<SessionStatus>({ protocol: PROTOCOL, action: 'status' });
      const known = associationState;
      if (state.configured && known?.vaultId === state.vaultId && known.capabilities?.includes(associationCapability)) {
        const view = await associationSync.read(native, known);
        native.assertCurrent();
        return { ...state, associationVersion: view.revision };
      }
      return state;
    }
    if (request.action === 'lock') return native.request({ protocol: PROTOCOL, action: 'lock' });
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
      if (!request.remember) return { filled: result.filled } satisfies FillResult;
      // Filling has already succeeded. A later navigation or association failure
      // must not turn that completed action into a reported fill failure.
      let associated = false;
      try { await setAssociation(request, true, native); associated = true; } catch {}
      native.assertCurrent();
      return { filled: result.filled, associated } satisfies FillResult;
    }
    return native.request(nativeRequest(request));
  };
  const dispatch = async (request: UIRequest, expectedTarget: ClientTarget, documentId?: string): Promise<unknown> => {
    const native = await session(expectedTarget);
    if (request.action !== 'snapshot' || !documentId) return execute(request, native);
    const result = await popup.take(documentId);
    const state = await snapshot(native);
    if (result?.target === native.target && result.vaultId === state.vaultId && state.tab && sameTab(result.tab, state.tab)) {
      state.shortcutOutcome = result.outcome;
    }
    return state;
  };
  async function shortcutResult(expected: TabContext): Promise<ShortcutResult | undefined> {
    await currentTab(expected);
    const native = await session();
    const state = await snapshot(native);
    await currentTab(expected);
    if (!state.tab || !sameTab(expected, state.tab)) throw new PassPineError('tab_changed');
    const scope = { tab: expected, vaultId: state.vaultId, target: native.target };
    if (state.matches.length > 1) return { ...scope, outcome: 'choose-match' };
    if (state.matches.length === 0) return;
    try {
      await execute({ action: 'fill', vaultId: state.vaultId, entry: state.matches[0]!, tab: expected, remember: false }, native);
      return { ...scope, outcome: 'filled' };
    } catch (error) {
      if (error instanceof PassPineError && error.code === 'ambiguous_form') return { ...scope, outcome: 'ambiguous_form' };
      throw error;
    }
  }
  const runShortcut = async (command: PageShortcutCommand, expected?: TabContext, windowId?: number) => {
    popup.clear();
    if (command === OPEN_COMMAND) {
      if (expected) await currentTab(expected);
      await browser.action.openPopup();
      return;
    }
    const target = expected ?? tabContext(await activeTab() ?? {});
    if (!target) { await browser.action.openPopup(); return; }
    let result: ShortcutResult | undefined;
    try { result = await shortcutResult(target); }
    catch { await currentTab(target); }
    if (result?.outcome === 'filled') return;
    await currentTab(target);
    if (result) await popup.open(async () => result, windowId);
    else await browser.action.openPopup();
  };
  browser.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (message?.type === 'passpine-page-shortcut') {
      const expected = sender.id === browser.runtime.id && sender.tab ? tabContext(sender.tab) : undefined;
      if (!expected || !sender.url || !/^https?:\/\//.test(sender.url) || ![OPEN_COMMAND, FILL_COMMAND].includes(message.command) || typeof message.shortcut !== 'string' || !message.shortcut) return false;
      void (async () => {
        if (!await hasPageShortcutAccess()) return false;
        const shortcuts = await readPageShortcuts();
        const command = message.command as PageShortcutCommand;
        // Ignore late presses from a content script that still has old bindings.
        if (canonicalShortcut(shortcuts[command]) !== canonicalShortcut(message.shortcut)) return false;
        await runShortcut(command, expected);
        return true;
      })().then(handled => sendResponse({ handled }), () => sendResponse({ handled: false }));
      return true;
    }
    if (!trustedSender(sender, browser.runtime.id, browser.runtime.getURL('/')) || message?.type !== 'passpine' || !validUIRequest(message.request)) return false;
    const fail = (error: unknown) => sendResponse({ protocol: PROTOCOL, status: 'error', code: error instanceof PassPineError ? error.code : 'operation_failed', message: errorMessage(error) });
    if (message.clientTarget !== 'release' && message.clientTarget !== 'dev') {
      fail(new PassPineError('native_connection_failed'));
      return true;
    }
    const root = browser.runtime.getURL('/');
    const identifyPopup = async (): Promise<string | undefined> => {
      const url = new URL('popup.html', root).href;
      if (message.request.action !== 'snapshot' || sender.url !== url) return;
      const claim = typeof message.popupInstance === 'string' ? message.popupInstance : undefined;
      if (root.startsWith('moz-extension:')) return claim;
      if (sender.documentId) return sender.documentId;
      // A tab displaying the extension page cannot borrow a toolbar popup's ID.
      if (claim && await uniquePopupAtURL(url) === claim) return claim;
    };
    identifyPopup().then(id => dispatch(message.request, message.clientTarget, id)).then(data => sendResponse({ protocol: PROTOCOL, status: 'ok', data }), fail);
    return true;
  });
  browser.commands.onCommand.addListener(async (command, tab) => {
    if (command !== FILL_COMMAND) return;
    const expected = tab && tabContext(tab);
    const firefoxVersion = Number(/Firefox\/(\d+)/.exec(navigator.userAgent)?.[1]);
    if (firefoxVersion > 0 && firefoxVersion < 149) {
      // Older Firefox requires openPopup in the command's user-gesture stack.
      // The popup waits for this same operation, including a sole-match fill.
      await popup.open(async () => {
        const target = expected ?? tabContext(await activeTab() ?? {});
        return target && shortcutResult(target);
      }, tab?.windowId).catch(() => {});
    } else await runShortcut(FILL_COMMAND, expected, tab?.windowId).catch(() => {});
  });
  const associationAlarm = 'passpine-associations-refresh';
  // Alarms survive MV3 worker suspension. No page or credential access is
  // needed to refresh the client-owned cache while the popup is closed.
  browser.alarms.onAlarm.addListener(alarm => {
    if (alarm.name !== associationAlarm) return;
    void (async () => {
      const native = await session();
      const state = await native.request<VaultState>({ protocol: PROTOCOL, action: 'configure', includeEntries: false });
      if (state.configured && state.capabilities?.includes(associationCapability)) await associationSync.read(native, state, true);
    })().catch(() => {});
  });
  void browser.alarms.create(associationAlarm, { periodInMinutes: 1 }).catch(() => {});
});
