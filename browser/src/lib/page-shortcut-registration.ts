import { browser } from 'wxt/browser';
import { hasPageShortcutAccess, pageShortcutAccessKey, pageShortcutFile, pageShortcutOrigins, pageShortcutScriptId } from './page-shortcuts';

export function initializePageShortcuts() {
  let pending = Promise.resolve();
  const sync = (inject = false) => {
    pending = pending.catch(() => {}).then(async () => {
      const enabled = await hasPageShortcutAccess();
      await browser.storage.local.set({ [pageShortcutAccessKey]: enabled });
      const registered = await browser.scripting.getRegisteredContentScripts({ ids: [pageShortcutScriptId] });
      if (!enabled) {
        if (registered.length) await browser.scripting.unregisterContentScripts({ ids: [pageShortcutScriptId] });
        return;
      }
      if (!registered.length) await browser.scripting.registerContentScripts([{ id: pageShortcutScriptId, js: [pageShortcutFile], matches: pageShortcutOrigins, allFrames: true, runAt: 'document_start', persistAcrossSessions: true }]);
      // Newly granted access also works in existing pages. The WXT context
      // invalidates a previous copy when a document already has this script.
      if (inject || !registered.length) {
        const tabs = await browser.tabs.query({ url: pageShortcutOrigins });
        await Promise.allSettled(tabs.filter(tab => tab.id !== undefined).map(tab => browser.scripting.executeScript({ target: { tabId: tab.id!, allFrames: true }, files: [pageShortcutFile] })));
      }
    });
    // Permission/registration errors do not break the extension's existing commands.
    void pending.catch(() => {});
  };
  browser.permissions.onAdded.addListener(() => sync(true));
  browser.permissions.onRemoved.addListener(() => sync());
  sync(true);
}
