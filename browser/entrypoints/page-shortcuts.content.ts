import { browser } from 'wxt/browser';
import { defaultPageShortcuts, pageShortcutAccessKey, pageShortcutForEvent, pageShortcutKey, readPageShortcuts } from '../src/lib/page-shortcuts';

export default defineContentScript({
  // Access is optional. Register only after the user grants website access;
  // empty matches keep WXT from adding required host permissions.
  matches: [], registration: 'runtime',
  main(ctx) {
    let shortcuts = defaultPageShortcuts(), ready = false, revision = 0;
    const read = async () => {
      const current = ++revision;
      try {
        const [value, access] = await Promise.all([readPageShortcuts(), browser.storage.local.get(pageShortcutAccessKey)]);
        if (!ctx.isInvalid && current === revision) {
          shortcuts = value; ready = access[pageShortcutAccessKey] === true;
          document.removeEventListener('keydown', keydown, true);
          if (ready) document.addEventListener('keydown', keydown, true);
        }
      }
      catch { if (current === revision) { ready = false; document.removeEventListener('keydown', keydown, true); } }
    };
    const changed = (changes: Record<string, unknown>, area: string) => { if (area === 'local' && (pageShortcutKey in changes || pageShortcutAccessKey in changes)) { ready = false; document.removeEventListener('keydown', keydown, true); void read(); } };
    const keydown = (event: KeyboardEvent) => {
      if (!ready || ctx.isInvalid || !event.isTrusted) return;
      const command = pageShortcutForEvent(event, shortcuts);
      if (!command) return;
      event.preventDefault(); event.stopPropagation();
      // This separate message exposes only these two actions to content scripts;
      // credential reads and arbitrary UI requests remain extension-page-only.
      void browser.runtime.sendMessage({ type: 'passpine-page-shortcut', command, shortcut: shortcuts[command] }).catch(() => {});
    };
    ctx.onInvalidated(() => { revision++; ready = false; document.removeEventListener('keydown', keydown, true); browser.storage.onChanged.removeListener(changed); });
    browser.storage.onChanged.addListener(changed);
    void read();
  },
});
