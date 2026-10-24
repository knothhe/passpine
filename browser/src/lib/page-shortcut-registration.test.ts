import { beforeEach, expect, it, vi } from 'vitest';
import { initializePageShortcuts } from './page-shortcut-registration';

const api = vi.hoisted(() => ({
  permissions: { contains: vi.fn(), onAdded: { addListener: vi.fn() }, onRemoved: { addListener: vi.fn() } },
  storage: { local: { set: vi.fn() } }, tabs: { query: vi.fn() },
  scripting: { getRegisteredContentScripts: vi.fn(), registerContentScripts: vi.fn(), unregisterContentScripts: vi.fn(), executeScript: vi.fn() },
}));
vi.mock('wxt/browser', () => ({ browser: api }));
beforeEach(() => { vi.clearAllMocks(); api.storage.local.set.mockResolvedValue(undefined); api.scripting.getRegisteredContentScripts.mockResolvedValue([]); api.tabs.query.mockResolvedValue([{ id: 7 }]); api.scripting.executeScript.mockResolvedValue([]); });

it('keeps page listeners disabled without optional website access', async () => {
  api.permissions.contains.mockResolvedValue(false);
  initializePageShortcuts();
  await vi.waitFor(() => expect(api.storage.local.set).toHaveBeenCalledWith({ pageShortcutAccess: false }));
  expect(api.scripting.registerContentScripts).not.toHaveBeenCalled();
  expect(api.scripting.executeScript).not.toHaveBeenCalled();
});

it('registers after permission is granted, injects existing tabs, and unregisters after revocation', async () => {
  api.permissions.contains.mockResolvedValue(true);
  initializePageShortcuts();
  await vi.waitFor(() => expect(api.scripting.executeScript).toHaveBeenCalledWith({ target: { tabId: 7, allFrames: true }, files: ['/content-scripts/page-shortcuts.js'] }));
  expect(api.scripting.registerContentScripts).toHaveBeenCalledWith([expect.objectContaining({ matches: ['http://*/*', 'https://*/*'], allFrames: true })]);
  api.permissions.contains.mockResolvedValue(false);
  api.scripting.getRegisteredContentScripts.mockResolvedValue([{ id: 'passpine-page-shortcuts' }]);
  api.permissions.onRemoved.addListener.mock.calls[0]![0]();
  await vi.waitFor(() => expect(api.scripting.unregisterContentScripts).toHaveBeenCalledWith({ ids: ['passpine-page-shortcuts'] }));
  expect(api.storage.local.set).toHaveBeenLastCalledWith({ pageShortcutAccess: false });
});
