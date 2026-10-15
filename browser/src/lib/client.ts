import { browser } from 'wxt/browser';
import { unwrap } from './protocol';
import type { Client } from './types';

export const client: Client = {
  demo: false,
  request: async request => unwrap(await browser.runtime.sendMessage({ type: 'passpine', request })),
  openOptions: () => browser.runtime.openOptionsPage(),
  copy: text => navigator.clipboard.writeText(text),
};
