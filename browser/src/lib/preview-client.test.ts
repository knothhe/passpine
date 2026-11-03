// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { EntryDocument, Snapshot } from './types';

beforeEach(() => {
  vi.resetModules();
  history.replaceState(null, '', '/?fixture=long');
});
afterEach(() => {
  history.replaceState(null, '', '/');
  vi.restoreAllMocks();
});

describe('long preview fixture', () => {
  it.each(['zh-CN', 'en'])('provides complete fictional documents in %s', async locale => {
    vi.spyOn(navigator, 'language', 'get').mockReturnValue(locale);
    const { demoClient } = await import('../../preview/client');
    const snapshot = await demoClient.request<Snapshot>({ action: 'snapshot' });
    const prefix = locale === 'en' ? 'Work/Developer tools/' : '工作/开发工具/';
    const longEntries = snapshot.entries.filter(entry => entry.path.startsWith(prefix));
    expect(longEntries).toHaveLength(24);
    expect(snapshot.entries).toHaveLength(30);
    for (const entry of longEntries) {
      const document = await demoClient.request<EntryDocument>({ action: 'fetch', vaultId: snapshot.vaultId, entry: entry.path });
      expect(document.contents).toMatch(/^preview-only-long-\d+\nusername: layout-demo-\d+@example\.test\nurl: https:\/\/example\.test\/organizations\/layout\/sample-\d+\n\n.+$/);
    }
  });

  it('persists edits without duplicating paths or restoring deleted examples', async () => {
    const { demoClient } = await import('../../preview/client');
    const initial = await demoClient.request<Snapshot>({ action: 'snapshot' });
    const entry = initial.entries.find(value => value.path.startsWith('工作/开发工具/'))!.path;
    const original = await demoClient.request<EntryDocument>({ action: 'fetch', vaultId: initial.vaultId, entry });
    const contents = `${original.contents}\nEdited fictional note.`;
    await demoClient.request({ action: 'save', vaultId: initial.vaultId, entry, contents, mode: 'edit', revision: original.revision });
    for (let index = 0; index < 2; index++) {
      const updated = await demoClient.request<Snapshot>({ action: 'snapshot' });
      expect(updated.entries).toHaveLength(initial.entries.length);
      expect(new Set(updated.entries.map(value => value.path)).size).toBe(updated.entries.length);
    }
    const saved = await demoClient.request<EntryDocument>({ action: 'fetch', vaultId: initial.vaultId, entry });
    expect(saved.contents).toBe(contents);
    await demoClient.request({ action: 'delete', vaultId: initial.vaultId, entry, revision: saved.revision });
    const deleted = await demoClient.request<Snapshot>({ action: 'snapshot' });
    expect(deleted.entries).toHaveLength(initial.entries.length - 1);
    expect(deleted.entries.some(value => value.path === entry)).toBe(false);
  });

  it('keeps the regular fixture small', async () => {
    history.replaceState(null, '', '/');
    const { demoClient } = await import('../../preview/client');
    const snapshot = await demoClient.request<Snapshot>({ action: 'snapshot' });
    expect(snapshot.entries).toHaveLength(6);
  });
});
