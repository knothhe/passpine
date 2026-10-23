import { browser } from 'wxt/browser';
import { PassPineError } from './errors';
import { unwrap } from './protocol';
import { loadClientTarget, subscribeClientTarget } from './client-target';
import type { Client } from './types';

let targetGeneration = 0, watchingTarget = false;
function scope(): Client {
  if (!watchingTarget) {
    subscribeClientTarget(() => {}, false, () => { targetGeneration++; });
    watchingTarget = true;
  }
  const generation = targetGeneration;
  // Capture the source when a UI mounts, before any later event handler or
  // asynchronous callback can run against a newly selected desktop client.
  const target = loadClientTarget().then(value => ({ value }), error => ({ error }));
  const assertCurrent = () => {
    if (generation !== targetGeneration) throw new PassPineError('native_connection_failed');
  };
  const scoped: Client = {
    demo: false,
    scope: () => scoped,
    request: async request => {
      assertCurrent();
      const captured = await target;
      assertCurrent();
      if ('error' in captured) throw captured.error;
      let response: unknown;
      try { response = await browser.runtime.sendMessage({ type: 'passpine', clientTarget: captured.value, request }); }
      catch { throw new PassPineError('background_unavailable'); }
      assertCurrent();
      if (response === undefined || response === null) throw new PassPineError('background_unavailable');
      return unwrap(response);
    },
    openOptions: () => browser.runtime.openOptionsPage(),
    copy: async text => { await scoped.request({ action: 'copy-generated', contents: text }); },
  };
  return scoped;
}

export const client: Client = {
  demo: false,
  scope,
  request: request => scope().request(request),
  openOptions: () => browser.runtime.openOptionsPage(),
  copy: text => scope().copy(text),
};
