import { browser } from 'wxt/browser';
import type { ClientTarget } from './client-target';
import type { ShortcutOutcome, TabContext } from './types';

export interface ShortcutResult { outcome: ShortcutOutcome; tab: TabContext; vaultId: string; target: ClientTarget }

// Firefox's background page can identify the actual action popup window.
// Chromium service workers identify that same document through runtime.getContexts.
export function popupInstance(doc: Document): string {
  return doc.documentElement.dataset.passpinePopup ??= crypto.randomUUID();
}

// Include TAB and other document contexts in this query. If popup.html is
// also open as a normal tab, the URL alone cannot identify the caller.
export async function uniquePopupAtURL(url: string): Promise<string | undefined> {
  try {
    const contexts = await browser.runtime.getContexts({ documentUrls: [url] });
    const pages = contexts.filter(context => context.documentUrl === url);
    const page = pages.length === 1 ? pages[0] : undefined;
    return page?.contextType === 'POPUP' ? page.documentId : undefined;
  } catch { return undefined; }
}

async function openedPopupDocument(windowId?: number): Promise<string | undefined> {
  if (browser.runtime.getURL('/').startsWith('moz-extension:')) {
    // Firefox resolves openPopup after dispatching the toolbar command; the
    // new document may only become visible to getViews on a subsequent turn.
    const deadline = Date.now() + 2000;
    do {
      const views = browser.extension.getViews({ type: 'popup', windowId });
      if (views.length > 1) return;
      const doc = (views[0] as unknown as Window | undefined)?.document;
      if (doc?.documentElement) return popupInstance(doc);
      await new Promise(resolve => setTimeout(resolve, 25));
    } while (Date.now() < deadline);
    return;
  }
  // Chrome reports windowId -1 for a toolbar popup, including one opened
  // with a specific windowId. Use its documentId even when MessageSender omits that field.
  const contexts = await browser.runtime.getContexts({ contextTypes: ['POPUP'] });
  return contexts.length === 1 ? contexts[0]!.documentId : undefined;
}

// Only the document opened by this invocation can claim its result. A toolbar
// popup opened later, or a worker restarted in the meantime, starts normally.
export function shortcutPopup() {
  type Pending = { page: Promise<string | undefined>; result: Promise<ShortcutResult | undefined>; expires: number };
  let pending: Pending | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const clear = () => { clearTimeout(timer); pending = undefined; };
  const open = async (result: () => Promise<ShortcutResult | undefined>, windowId?: number) => {
    clear();
    // Invoke synchronously so Firefox versions requiring a gesture still open.
    const opened = browser.action.openPopup(windowId === undefined ? {} : { windowId });
    const ticket = {
      page: opened.then(() => openedPopupDocument(windowId)).catch(() => undefined),
      result: result().catch(() => undefined),
      expires: Date.now() + 12_000,
    };
    pending = ticket;
    timer = setTimeout(clear, 12_000);
    try { await opened; }
    catch (error) { if (pending === ticket) clear(); throw error; }
  };
  const take = async (documentId: string): Promise<ShortcutResult | undefined> => {
    const ticket = pending;
    if (!ticket || await ticket.page !== documentId || pending !== ticket || Date.now() >= ticket.expires) return;
    clear();
    return ticket.result;
  };
  return { open, take, clear };
}
