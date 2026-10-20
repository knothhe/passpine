import { browser } from 'wxt/browser';
import { PassPineError } from './errors';
import { unwrap } from './protocol';
import type { Client } from './types';

export const client: Client = {
  demo: false,
  request: async request => {
    let response: unknown;
    try { response = await browser.runtime.sendMessage({ type: 'passpine', request }); }
    catch { throw new PassPineError('background_unavailable'); }
    if (response === undefined || response === null) throw new PassPineError('background_unavailable');
    return unwrap(response);
  },
  openOptions: () => browser.runtime.openOptionsPage(),
  copy: async text => { await client.request({ action: 'copy-generated', contents: text }); },
};
