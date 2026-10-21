import { useEffect } from 'react';
import type { Client } from './types';

const activityEvents = ['pointerdown', 'keydown', 'input'] as const;
const activityIntervalMs = 1000;

// Report only real extension UI interactions. Metadata reads, React renders,
// language changes and synthetic events must not keep background sync idle.
export function useUserActivity(client: Client, enabled = true) {
  useEffect(() => {
    if (!enabled || client.demo) return;
    let active = true, pending = false, lastReportAt = -Infinity;
    const activity = (event: Event) => {
      if (!active || !event.isTrusted || document.hidden || pending) return;
      const now = performance.now();
      if (now - lastReportAt < activityIntervalMs) return;
      lastReportAt = now;
      pending = true;
      // No event details or decrypted entry data cross this boundary. This
      // best-effort notification never changes UI state or opens authentication.
      void (async () => {
        try { await client.request({ action: 'touch' }); }
        catch {}
        finally { pending = false; }
      })();
    };
    for (const type of activityEvents) document.addEventListener(type, activity, { capture: true, passive: true });
    return () => {
      active = false;
      for (const type of activityEvents) document.removeEventListener(type, activity, true);
    };
  }, [client, enabled]);
}
