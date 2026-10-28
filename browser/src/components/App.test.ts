// @vitest-environment jsdom
import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Theme } from '@radix-ui/themes';
import { App } from './App';
import type { Client, EntryDocument, FillResult, Snapshot, UIRequest } from '../lib/types';
import { saveClientTarget } from '../lib/client-target';

const extension = vi.hoisted(() => ({ storage: { local: { get: vi.fn() }, onChanged: { addListener: vi.fn(), removeListener: vi.fn() } } }));
vi.mock('wxt/browser', () => ({ browser: extension }));

let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
  localStorage.clear();
  extension.storage.local.get.mockResolvedValue({});
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  container = document.createElement('div');
  document.body.append(container);
  root = createRoot(container);
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

function fixture(unlock: (state: Snapshot) => void | Promise<void>) {
  const state: Snapshot = { configured: true, vaultId: 'first', vaultName: '测试密码库', locked: true, entriesVersion: 1, capabilities: [], entries: [], matches: [], tab: { id: 1, url: 'https://example.test', hostname: 'example.test', secure: true } };
  const actions: string[] = [];
  const client: Client = {
    demo: true, openOptions: async () => {}, copy: async () => {},
    async request<T>(request: UIRequest): Promise<T> {
      actions.push(request.action);
      if (request.action === 'unlock') { await unlock(state); return {} as T; }
      return { ...state } as T;
    },
  };
  return { client, actions };
}
async function newAccount() {
  const button = container.querySelector<HTMLButtonElement>('button[aria-label="新建账号"]')!;
  expect(button.disabled).toBe(false);
  await act(async () => button.click());
}
async function render(client: Client) {
  await act(async () => root.render(createElement(Theme, {}, createElement(App, { client }))));
}

function navigationFixture() {
  const state: Snapshot = { configured: true, vaultId: 'first', vaultName: '测试密码库', locked: false, entriesVersion: 1, capabilities: [], entries: ['Alex', 'Blair', 'Casey'].map(name => ({ path: `example.test/${name}`, name, folder: 'example.test' })), matches: [] };
  const client: Client = { demo: true, openOptions: async () => {}, copy: async () => {}, async request<T>() { return { ...state } as T; } };
  return { client, state };
}
async function pressNavigation(target: Element, key: string, modifiers: KeyboardEventInit = {}) {
  const event = new KeyboardEvent('keydown', { key, code: key.startsWith('Arrow') ? key : `Key${key.toUpperCase()}`, bubbles: true, cancelable: true, ...modifiers });
  await act(async () => target.dispatchEvent(event));
  return event;
}

describe('account list keyboard navigation', () => {
  it('clears an empty search from the results body and restores the search focus without refetching', async () => {
    const fixture = navigationFixture(), request = vi.spyOn(fixture.client, 'request');
    await render(fixture.client);
    const search = container.querySelector<HTMLInputElement>('input[aria-label="搜索密码库"]')!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(search, 'no such account');
      search.dispatchEvent(new Event('input', { bubbles: true }));
    });
    expect(container.querySelector('.no-results')?.textContent).toContain('换个关键词，或清除搜索查看全部账号。');
    await act(async () => container.querySelector<HTMLButtonElement>('.no-results button')!.click());
    expect(search.value).toBe(''); expect(document.activeElement).toBe(search);
    expect(container.querySelectorAll('button[data-entry-path]')).toHaveLength(3);
    expect(request).toHaveBeenCalledOnce();
  });

  it.each([
    ['n', { ctrlKey: true }, [0, 1, 2]],
    ['p', { ctrlKey: true }, [2, 1, 0]],
    ['ArrowDown', {}, [0, 1, 2]],
    ['ArrowUp', {}, [2, 1, 0]],
  ] as const)('moves with %s from search through entries and back to search', async (key, modifiers, order) => {
    await render(navigationFixture().client);
    const search = container.querySelector<HTMLInputElement>('input[aria-label="搜索密码库"]')!;
    const entries = Array.from(container.querySelectorAll<HTMLButtonElement>('button[data-entry-path]'));
    search.focus();
    for (const index of order) {
      expect((await pressNavigation(document.activeElement!, key, modifiers)).defaultPrevented).toBe(true);
      expect(document.activeElement).toBe(entries[index]);
    }
    await pressNavigation(document.activeElement!, key, modifiers);
    expect(document.activeElement).toBe(search);
  });

  it('moves from entry actions, supports held keys, and takes precedence over custom popup bindings', async () => {
    localStorage.setItem('popupShortcuts', JSON.stringify([{ id: 'new', action: 'new', shortcut: 'Ctrl+N' }]));
    await render(navigationFixture().client);
    const copy = container.querySelector<HTMLButtonElement>('button[aria-label="复制用户名 Alex"]')!;
    copy.focus();
    await pressNavigation(copy, 'n', { ctrlKey: true });
    expect(document.activeElement?.getAttribute('data-entry-path')).toBe('example.test/Blair');
    expect(container.querySelector('form')).toBeNull();
    await pressNavigation(document.activeElement!, 'n', { ctrlKey: true, repeat: true });
    expect(document.activeElement?.getAttribute('data-entry-path')).toBe('example.test/Casey');
  });

  it('navigates only the filtered search results without changing the query', async () => {
    await render(navigationFixture().client);
    const search = container.querySelector<HTMLInputElement>('input[aria-label="搜索密码库"]')!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(search, 'blair');
      search.dispatchEvent(new Event('input', { bubbles: true }));
    });
    expect(container.querySelectorAll('button[data-entry-path]')).toHaveLength(1);
    await pressNavigation(search, 'p', { ctrlKey: true });
    expect(document.activeElement?.getAttribute('data-entry-path')).toBe('example.test/Blair');
    await pressNavigation(document.activeElement!, 'n', { ctrlKey: true });
    expect(document.activeElement).toBe(search);
    expect(search.value).toBe('blair');
  });

  it.each([
    { ctrlKey: true, altKey: true },
    { ctrlKey: true, metaKey: true },
    { ctrlKey: true, shiftKey: true },
    { metaKey: true },
    { ctrlKey: true, isComposing: true },
    {},
  ])('does not intercept typing or other modifier combinations (%j)', async modifiers => {
    await render(navigationFixture().client);
    const search = container.querySelector<HTMLInputElement>('input[aria-label="搜索密码库"]')!;
    search.focus();
    expect((await pressNavigation(search, 'n', modifiers)).defaultPrevented).toBe(false);
    expect(document.activeElement).toBe(search);
  });

  it('keeps empty searches focused', async () => {
    const fixture = navigationFixture(); fixture.state.entries = [];
    await render(fixture.client);
    const search = container.querySelector<HTMLInputElement>('input[aria-label="搜索密码库"]')!;
    search.focus();
    expect((await pressNavigation(search, 'p', { ctrlKey: true })).defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(search);
  });

  it('leaves the account menu trigger to its own navigation', async () => {
    await render(navigationFixture().client);
    const more = container.querySelector<HTMLButtonElement>('button[aria-label="更多操作 Alex"]')!;
    more.focus();
    expect((await pressNavigation(more, 'n', { ctrlKey: true })).defaultPrevented).toBe(false);
    expect(document.activeElement).toBe(more);
  });

  it('does not intercept Ctrl+N/P inside account editor fields', async () => {
    await render(navigationFixture().client);
    await newAccount();
    const input = container.querySelector<HTMLInputElement>('input[placeholder="path/to/entry（支持多级路径）"]')!;
    input.focus();
    for (const key of ['n', 'p']) expect((await pressNavigation(input, key, { ctrlKey: true })).defaultPrevented).toBe(false);
    expect(document.activeElement).toBe(input);
  });
});

describe('create an account from a locked popup', () => {
  it('places icon-only creation beside the domain and executes a stored custom shortcut through unlock', async () => {
    localStorage.setItem('popupShortcuts', JSON.stringify([{ id: 'new', action: 'new', shortcut: 'Alt+N' }]));
    const { client, actions } = fixture(state => { state.locked = false; });
    await render(client);
    expect(container.querySelector('.app-header')).toBeNull();
    expect(container.querySelector('.site-context')?.textContent).not.toContain('当前网站');
    const create = container.querySelector<HTMLButtonElement>('button[aria-label="新建账号"]')!;
    expect(create.closest('.site-context-label')?.textContent).toBe('example.test');
    expect(create.textContent).toBe('');
    await act(async () => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'n', code: 'KeyN', altKey: true, bubbles: true, cancelable: true })));
    expect(actions).toEqual(['snapshot', 'unlock', 'snapshot']);
    expect(container.querySelector('form')).not.toBeNull();
  });
  it('authenticates and refreshes before opening the editor', async () => {
    const { client, actions } = fixture(state => { state.locked = false; });
    await render(client);
    await newAccount();
    expect(actions).toEqual(['snapshot', 'unlock', 'snapshot']);
    expect(container.querySelector('form')).not.toBeNull();
    expect(container.querySelector<HTMLInputElement>('input[autocomplete="url"]')?.value).toBe('https://example.test');
  });

  it('keeps the list and allows retry when authentication is cancelled', async () => {
    const { client } = fixture(() => { throw new Error('已取消解锁'); });
    await render(client);
    await newAccount();
    expect(container.querySelector('form')).toBeNull();
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('已取消解锁');
    expect(container.querySelector<HTMLButtonElement>('button[aria-label="新建账号"]')?.disabled).toBe(false);
  });

  it('does not create in a different vault selected during authentication', async () => {
    const { client } = fixture(state => { state.locked = false; state.vaultId = 'second'; });
    await render(client);
    await newAccount();
    expect(container.querySelector('form')).toBeNull();
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('当前密码库已切换');
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (reason: unknown) => void;
  const promise = new Promise<T>((accept, decline) => { resolve = accept; reject = decline; });
  return { promise, resolve, reject };
}

describe('account list primary action', () => {
  function fillFixture({ locked = false, matched = false, error, associated = true, associationError, demo = true, fillResult }: { locked?: boolean; matched?: boolean; error?: Error; associated?: boolean; associationError?: Error; demo?: boolean; fillResult?: Promise<FillResult> } = {}) {
    const state: Snapshot = { configured: true, vaultId: 'first', vaultName: '测试密码库', locked, entriesVersion: 1, capabilities: [], entries: [{ path: 'accounts/Alex', name: 'Alex', folder: 'accounts' }], matches: matched ? ['accounts/Alex'] : [], tab: { id: 1, url: 'https://example.test/login', hostname: 'example.test', secure: true } };
    const requests: UIRequest[] = [];
    const client: Client = {
      demo, openOptions: async () => {}, copy: async () => {},
      async request<T>(request: UIRequest): Promise<T> {
        requests.push(request);
        if (request.action === 'unlock') state.locked = false;
        if (request.action === 'fill') {
          if (error) throw error;
          const result = fillResult ? await fillResult : { filled: 2, associated };
          if (result.associated) state.matches = [request.entry];
          return result as T;
        }
        if (request.action === 'fetch') return credentials as T;
        if (request.action === 'associate' || request.action === 'dissociate') {
          if (associationError) throw associationError;
          state.matches = request.action === 'associate' ? [request.entry] : [];
        }
        return { ...state } as T;
      },
    };
    return { client, state, requests };
  }

  it.each([[false, false], [false, true], [true, false], [true, true]])('fills from the entry body and stays in the list (locked=%s, matched=%s)', async (locked, matched) => {
    const fixture = fillFixture({ locked, matched });
    await render(fixture.client);
    expect(container.querySelector('button[data-entry-path]')?.getAttribute('aria-label')).toBe('填充「Alex」的账号与密码');
    await act(async () => container.querySelector<HTMLButtonElement>('button[data-entry-path]')!.click());
    expect(fixture.requests.map(request => request.action)).toEqual(locked ? ['snapshot', 'unlock', 'snapshot', 'fill', 'snapshot'] : ['snapshot', 'fill', 'snapshot']);
    expect(fixture.requests.find(request => request.action === 'fill')).toEqual({ action: 'fill', vaultId: 'first', entry: 'accounts/Alex', tab: fixture.state.tab });
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(container.querySelector('[role="status"]')?.textContent).toContain('已填写 2 个登录字段，已关联 example.test');
    expect(container.querySelector('.site-account-badge')?.textContent).toBe('当前网站');
    expect(container.querySelector('.site-match-count')?.textContent).toBe('1 个关联账号');
  });

  it('shows a filling failure without opening details or associating the entry', async () => {
    const fixture = fillFixture({ error: new Error('当前页面未找到可填写的登录字段。') });
    await render(fixture.client);
    await act(async () => container.querySelector<HTMLButtonElement>('button[data-entry-path]')!.click());
    expect(fixture.requests.map(request => request.action)).toEqual(['snapshot', 'fill']);
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('当前页面未找到可填写的登录字段。');
    expect(container.querySelector<HTMLButtonElement>('button[data-entry-path]')?.disabled).toBe(false);
    expect(container.querySelector('.site-account-badge')).toBeNull();
  });

  it('distinguishes successful filling from a failed association and allows retrying the association', async () => {
    const fixture = fillFixture({ associated: false });
    await render(fixture.client);
    await act(async () => container.querySelector<HTMLButtonElement>('button[data-entry-path]')!.click());
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('已填写 2 个登录字段，但未能关联 example.test，请重试。');
    expect(container.querySelector('.site-account-badge')).toBeNull();
    await clickAction('查看 Alex');
    const associate = currentPage().querySelector<HTMLButtonElement>('.detail-associate')!;
    expect(associate.textContent).toContain('关联到 example.test');
    await act(async () => associate.click());
    expect(currentPage().querySelector('.detail-associate')?.textContent).toContain('取消关联当前网站');
    expect(fixture.requests.filter(request => request.action === 'fill')).toHaveLength(1);
    expect(fixture.requests.filter(request => request.action === 'associate')).toHaveLength(1);
  });

  it.each(['button', 'shortcut'])('fills an unassociated account from details using the %s and keeps association state in sync', async trigger => {
    localStorage.setItem('popupShortcuts', JSON.stringify([{ id: 'fill', action: 'fill', shortcut: 'Alt+F' }]));
    const fixture = fillFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    expect(currentPage().querySelector<HTMLButtonElement>('button[aria-label="填充账号与密码"]')?.disabled).toBe(false);
    expect(currentPage().querySelector('button[aria-label="填充账号与密码"]')?.textContent).toBe('填充账号与密码');
    expect(currentPage().querySelector('button[aria-label="编辑账号"]')?.textContent).toBe('');
    if (trigger === 'button') await clickAction('填充账号与密码');
    else await pressNavigation(window.document.activeElement!, 'f', { altKey: true });
    expect(fixture.requests.some(request => request.action === 'fill')).toBe(true);
    expect(currentPage().dataset.popupPage).toBe('detail');
    expect(currentPage().querySelector('[aria-label="已关联当前网站"]')).not.toBeNull();
    const cancel = currentPage().querySelector<HTMLButtonElement>('.detail-associate')!;
    expect(cancel.textContent).toContain('取消关联当前网站');
    await act(async () => cancel.click());
    expect(fixture.requests).toContainEqual({ action: 'dissociate', vaultId: 'first', entry: 'accounts/Alex', tab: fixture.state.tab });
    expect(currentPage().querySelector('[aria-label="已关联当前网站"]')).toBeNull();
    expect(container.querySelector('.notice[role="status"]')?.textContent).toContain('已取消关联 example.test');
    expect(currentPage().querySelector<HTMLButtonElement>('button[aria-label="填充账号与密码"]')?.disabled).toBe(false);
    await clickAction('返回密码库');
    expect(container.querySelector('.site-account-badge')).toBeNull();
    expect(container.querySelector('.site-match-count')?.textContent).toBe('尚无关联账号');
  });

  it('keeps a failed cancellation associated and retryable', async () => {
    const fixture = fillFixture({ matched: true, associationError: new Error('关联保存失败，请重试。') });
    await render(fixture.client); await clickAction('查看 Alex');
    await act(async () => currentPage().querySelector<HTMLButtonElement>('.detail-associate')!.click());
    expect(currentPage().querySelector('[aria-label="已关联当前网站"]')).not.toBeNull();
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('关联保存失败，请重试。');
    expect(currentPage().querySelector<HTMLButtonElement>('.detail-associate')?.disabled).toBe(false);
  });

  it('cancels an association through the account menu and fills again without requiring manual association', async () => {
    const fixture = fillFixture({ matched: true });
    await render(fixture.client);
    const more = container.querySelector<HTMLButtonElement>('button[aria-label="更多操作 Alex"]')!;
    await pressNavigation(more, 'Enter');
    const cancel = Array.from(document.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(item => item.textContent === '取消关联当前网站')!;
    expect(cancel).toBeDefined();
    await act(async () => cancel.click());
    expect(container.querySelector('.site-account-badge')).toBeNull();
    await act(async () => container.querySelector<HTMLButtonElement>('button[data-entry-path]')!.click());
    expect(container.querySelector('.site-account-badge')).not.toBeNull();
    expect(fixture.requests.map(request => request.action)).toContain('dissociate');
    expect(fixture.requests.map(request => request.action)).not.toContain('associate');
  });

  it('keeps the copy icons independent from filling', async () => {
    const fixture = fillFixture();
    await render(fixture.client);
    await clickAction('复制用户名 Alex');
    await clickAction('复制密码 Alex');
    expect(fixture.requests).toEqual([
      { action: 'snapshot' },
      { action: 'copy', vaultId: 'first', entry: 'accounts/Alex', field: 'username' },
      { action: 'copy', vaultId: 'first', entry: 'accounts/Alex', field: 'password' },
    ]);
    expect(currentPage().dataset.popupPage).toBe('list');
  });

  it.each(['list', 'detail button', 'detail shortcut'])('closes the extension popup only after successful filling from the %s, without refreshing again', async trigger => {
    const close = vi.spyOn(window, 'close').mockImplementation(() => {});
    const pending = deferred<FillResult>();
    const fixture = fillFixture({ demo: false, fillResult: pending.promise });
    extension.storage.local.get.mockResolvedValue({ popupShortcuts: [{ id: 'fill', action: 'fill', shortcut: 'Alt+F' }] });
    await render(fixture.client);
    if (trigger === 'list') await act(async () => container.querySelector<HTMLButtonElement>('button[data-entry-path]')!.click());
    else {
      await clickAction('查看 Alex');
      if (trigger === 'detail button') await clickAction('填充账号与密码');
      else await pressNavigation(document.activeElement!, 'f', { altKey: true });
    }
    const actionsBeforeResult = fixture.requests.map(request => request.action);
    expect(actionsBeforeResult).toEqual(trigger === 'list' ? ['snapshot', 'fill'] : ['snapshot', 'fetch', 'status', 'fill']);
    expect(close).not.toHaveBeenCalled();
    await act(async () => pending.resolve({ filled: 2, associated: true }));
    expect(close).toHaveBeenCalledTimes(1);
    expect(fixture.requests.map(request => request.action)).toEqual(actionsBeforeResult);
  });

  it('closes after fields are filled even if automatic association fails', async () => {
    const close = vi.spyOn(window, 'close').mockImplementation(() => {});
    const fixture = fillFixture({ demo: false, associated: false });
    await render(fixture.client);
    await act(async () => container.querySelector<HTMLButtonElement>('button[data-entry-path]')!.click());
    expect(close).toHaveBeenCalledTimes(1);
    expect(fixture.requests.map(request => request.action)).toEqual(['snapshot', 'fill']);
  });

  it('keeps the extension popup open with a retryable error when filling fails', async () => {
    const close = vi.spyOn(window, 'close').mockImplementation(() => {});
    const fixture = fillFixture({ demo: false, error: new Error('当前页面未找到可填写的登录字段。') });
    await render(fixture.client);
    await act(async () => container.querySelector<HTMLButtonElement>('button[data-entry-path]')!.click());
    expect(close).not.toHaveBeenCalled();
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('当前页面未找到可填写的登录字段。');
    expect(container.querySelector<HTMLButtonElement>('button[data-entry-path]')?.disabled).toBe(false);
    expect(fixture.requests.map(request => request.action)).toEqual(['snapshot', 'fill']);
  });

  it('keeps the extension popup open when no fields were filled', async () => {
    const close = vi.spyOn(window, 'close').mockImplementation(() => {});
    const fixture = fillFixture({ demo: false, fillResult: Promise.resolve({ filled: 0, associated: false }) });
    await render(fixture.client);
    await act(async () => container.querySelector<HTMLButtonElement>('button[data-entry-path]')!.click());
    expect(close).not.toHaveBeenCalled();
  });

  it('keeps the demo preview open and displays successful filling feedback', async () => {
    const close = vi.spyOn(window, 'close').mockImplementation(() => {});
    const fixture = fillFixture();
    await render(fixture.client);
    await act(async () => container.querySelector<HTMLButtonElement>('button[data-entry-path]')!.click());
    expect(close).not.toHaveBeenCalled();
    expect(container.querySelector('[role="status"]')?.textContent).toContain('演示：已填写 2 个登录字段');
    expect(fixture.requests.map(request => request.action)).toEqual(['snapshot', 'fill', 'snapshot']);
  });
});

describe('manual refresh feedback', () => {
  it.each([true, false])('shows immediate progress and completion feedback (success=%s)', async success => {
    const pending = deferred<Snapshot>();
    const state: Snapshot = { configured: true, vaultId: 'first', vaultName: '测试密码库', locked: false, entriesVersion: 1, capabilities: [], entries: [], matches: [] };
    let count = 0;
    const client: Client = { demo: true, openOptions: async () => {}, copy: async () => {}, async request<T>(request: UIRequest) { return (request.action === 'snapshot' && count++ > 0 ? pending.promise : { ...state }) as Promise<T>; } };
    await render(client);
    const refresh = container.querySelector<HTMLButtonElement>('button[aria-label="刷新密码库"]')!;
    await act(async () => refresh.click());
    expect(refresh.disabled).toBe(true);
    expect(refresh.getAttribute('aria-busy')).toBe('true');
    expect(refresh.querySelector('.connection-checking')).not.toBeNull();
    expect(container.textContent).not.toContain('正在连接…');
    await act(async () => { if (success) pending.resolve(state); else pending.reject(new Error('刷新连接失败')); });
    expect(refresh.disabled).toBe(false);
    expect(refresh.getAttribute('aria-busy')).toBeNull();
    expect(container.querySelector(success ? '[role="status"]' : '[role="alert"]')?.textContent).toContain(success ? '密码库已刷新' : '刷新连接失败');
  });

  it('keeps metadata refreshes quiet', async () => {
    const state: Snapshot = { configured: true, vaultId: 'first', vaultName: '测试密码库', locked: false, entriesVersion: 1, capabilities: [], entries: [], matches: [] };
    const actions: string[] = [];
    const client: Client = { demo: true, openOptions: async () => {}, copy: async () => {}, async request<T>(request: UIRequest) { actions.push(request.action); return { ...state } as T; } };
    await render(client);
    state.entriesVersion = 2;
    await act(async () => document.dispatchEvent(new Event('visibilitychange')));
    expect(actions).toEqual(['snapshot', 'status', 'snapshot']);
    expect(container.querySelector('.notice')).toBeNull();
    expect(container.querySelector('button[aria-label="刷新密码库"]')?.getAttribute('aria-busy')).toBeNull();
  });
});
function detailFixture(locked = false) {
  const state: Snapshot = { configured: true, vaultId: 'first', vaultName: '测试密码库', locked, entriesVersion: 1, capabilities: [], entries: [{ path: 'example.test/Alex', name: 'Alex', folder: 'example.test' }], matches: [], tab: { id: 1, url: 'https://example.test', hostname: 'example.test', secure: true } };
  const fetch = deferred<EntryDocument>(), status = deferred<Snapshot>(), copy = deferred<unknown>();
  const actions: string[] = [];
  let deferStatus = false, deferCopy = false, statusError: Error | undefined;
  const client: Client = {
    demo: true, openOptions: async () => {}, copy: async () => {},
    async request<T>(request: UIRequest): Promise<T> {
      actions.push(request.action);
      if (request.action === 'fetch') return fetch.promise as Promise<T>;
      if (request.action === 'status' && statusError) throw statusError;
      if (request.action === 'status' && deferStatus) return status.promise as Promise<T>;
      if (request.action === 'copy' && deferCopy) return copy.promise as Promise<T>;
      if (request.action === 'unlock') state.locked = false;
      if (request.action === 'lock') state.locked = true;
      return { ...state } as T;
    },
  };
  return { client, state, fetch, status, copy, actions, deferStatus: () => { deferStatus = true; }, deferCopy: () => { deferCopy = true; }, failStatus: () => { statusError = new Error('客户端连接已中断'); } };
}
const credentials: EntryDocument = { contents: 'sensitive-password\nusername: private-user\nurl: https://example.test\nnotes: sensitive-note', revision: '1' };
function currentPage() { return container.querySelector<HTMLElement>('[data-popup-page]:not([aria-hidden="true"])')!; }
async function clickAction(label: string) {
  const button = currentPage().querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!;
  expect(button).not.toBeNull();
  await act(async () => button.click());
}

describe('responsive detail navigation and session safety', () => {
  it.each([false, true])('refreshes association matches while preserving the open detail or editor (editing=%s)', async editing => {
    const fixture = detailFixture();
    fixture.state.associationVersion = 1;
    await render(fixture.client); await clickAction('查看 Alex');
    await act(async () => fixture.fetch.resolve(credentials));
    if (editing) await clickAction('编辑账号');
    fixture.state.matches = ['example.test/Alex'];
    fixture.state.associationVersion = 2;
    await act(async () => document.dispatchEvent(new Event('visibilitychange')));
    expect(currentPage().dataset.popupPage).toBe(editing ? 'editor' : 'detail');
    expect(fixture.actions.filter(action => action === 'fetch')).toHaveLength(1);
    if (editing) expect(Array.from(currentPage().querySelectorAll('input')).some(input => input.value === 'private-user')).toBe(true);
    else expect(currentPage().querySelector('[aria-label="已关联当前网站"]')).not.toBeNull();
  });

  it('clears details and unsaved edits when switching clients even with identical vault metadata', async () => {
    const fixture = detailFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    await act(async () => fixture.fetch.resolve(credentials));
    await clickAction('编辑账号');
    expect(Array.from(container.querySelectorAll('input')).some(input => input.value === 'sensitive-password')).toBe(true);
    await act(async () => saveClientTarget('dev', true));
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(Array.from(container.querySelectorAll('input')).some(input => input.value === 'sensitive-password' || input.value === 'private-user')).toBe(false);
    expect(container.textContent).not.toContain('sensitive-note');
    expect(fixture.actions.filter(action => action === 'snapshot')).toHaveLength(2);
  });

  it('ignores a late credential response from the previous client', async () => {
    const fixture = detailFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    await act(async () => saveClientTarget('dev', true));
    await act(async () => fixture.fetch.resolve(credentials));
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(container.textContent).not.toContain('private-user');
    expect(container.textContent).not.toContain('sensitive-note');
  });

  it.each(['lock', 'session expiry', 'visibility loss'])('scrubs dirty editor and its leave dialog immediately on %s', async trigger => {
    const fixture = detailFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    await act(async () => fixture.fetch.resolve(credentials));
    await clickAction('编辑账号');
    const username = currentPage().querySelector<HTMLInputElement>('input[placeholder="用户名或邮箱"]')!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(username, 'private-draft');
      username.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await clickAction('返回账号详情');
    expect(document.querySelector('[role="alertdialog"]')).not.toBeNull();
    if (trigger === 'lock') await act(async () => container.querySelector<HTMLButtonElement>('button[aria-label="锁定密码库"]')!.click());
    else {
      if (trigger === 'session expiry') fixture.state.locked = true;
      else vi.spyOn(document, 'hidden', 'get').mockReturnValue(true);
      await act(async () => document.dispatchEvent(new Event('visibilitychange')));
    }
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(document.querySelector('[role="alertdialog"]')).toBeNull();
    expect(Array.from(container.querySelectorAll('input')).some(input => ['private-draft', 'sensitive-password', 'private-user'].includes(input.value))).toBe(false);
    expect(container.textContent).not.toContain('sensitive-note');
  });

  it('animates forward into details and editing, then reverses direction when going back', async () => {
    const fixture = detailFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    expect(currentPage().dataset.navigationDirection).toBe('forward');
    await act(async () => fixture.fetch.resolve(credentials));
    await clickAction('编辑账号');
    expect(currentPage().dataset.popupPage).toBe('editor');
    expect(currentPage().dataset.navigationDirection).toBe('forward');
    await clickAction('返回账号详情');
    expect(currentPage().dataset.popupPage).toBe('detail');
    expect(currentPage().dataset.navigationDirection).toBe('back');
    await clickAction('返回密码库');
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(currentPage().dataset.navigationDirection).toBe('back');
    expect(container.textContent).not.toContain('private-user');
  });

  it('shows a non-secret detail shell immediately and still waits for the status safety check', async () => {
    const fixture = detailFixture(); fixture.deferStatus();
    await render(fixture.client);
    expect(currentPage().style.opacity).not.toBe('0');
    await clickAction('查看 Alex');
    expect(currentPage().dataset.popupPage).toBe('detail');
    expect(currentPage().textContent).toContain('正在读取账号…');
    expect(currentPage().textContent).not.toContain('private-user');
    expect(document.activeElement?.tagName).toBe('H1');
    expect(fixture.actions).toEqual(['snapshot', 'fetch']);
    await act(async () => fixture.fetch.resolve(credentials));
    expect(fixture.actions).toEqual(['snapshot', 'fetch', 'status']);
    expect(currentPage().textContent).not.toContain('private-user');
    await act(async () => fixture.status.resolve({ ...fixture.state }));
    expect(currentPage().textContent).toContain('private-user');
    expect(currentPage().textContent).toContain('sensitive-note');
    expect(currentPage().textContent).not.toContain('sensitive-password');
  });

  it('cancels a loading detail on back and ignores a late response without blocking the list', async () => {
    const fixture = detailFixture();
    await render(fixture.client);
    const list = currentPage().querySelector<HTMLElement>('.entries-list')!; list.scrollTop = 95;
    await clickAction('查看 Alex');
    await clickAction('返回密码库');
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(currentPage().querySelector<HTMLElement>('.entries-list')?.scrollTop).toBe(95);
    expect(document.activeElement?.getAttribute('data-entry-path')).toBe('example.test/Alex');
    expect(currentPage().querySelector<HTMLButtonElement>('button[aria-label="查看 Alex"]')?.disabled).toBe(false);
    await act(async () => fixture.fetch.resolve(credentials));
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(container.textContent).not.toContain('sensitive-note');
    expect(fixture.actions).toEqual(['snapshot', 'fetch']);
  });

  it('returns to the list with a retryable notice when fetching fails', async () => {
    const fixture = detailFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    await act(async () => fixture.fetch.reject(new Error('读取未完成，请重试')));
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('读取未完成，请重试');
    expect(currentPage().querySelector<HTMLButtonElement>('button[aria-label="查看 Alex"]')?.disabled).toBe(false);
  });

  it('returns to the list after deleting instead of leaving an orphaned loading shell', async () => {
    const fixture = detailFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    await act(async () => fixture.fetch.resolve(credentials));
    await clickAction('删除账号');
    const confirm = Array.from(document.querySelectorAll<HTMLButtonElement>('[role="alertdialog"] button')).find(button => button.textContent === '移入回收站')!;
    expect(confirm).not.toBeUndefined();
    await act(async () => confirm.click());
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(container.textContent).not.toContain('private-user');
    expect(container.textContent).toContain('已移入加密回收站');
  });

  it('does not display a fetched secret when the verification observes a lock', async () => {
    const fixture = detailFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    fixture.state.locked = true;
    await act(async () => fixture.fetch.resolve(credentials));
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(container.textContent).not.toContain('private-user');
    expect(container.textContent).not.toContain('sensitive-note');
  });

  it('keeps the non-secret loading shell across an expected unlock and then shows verified details', async () => {
    const fixture = detailFixture(true);
    await render(fixture.client); await clickAction('查看 Alex');
    expect(currentPage().dataset.popupPage).toBe('detail');
    expect(fixture.actions).toEqual(['snapshot', 'unlock', 'snapshot', 'fetch']);
    await act(async () => fixture.fetch.resolve(credentials));
    expect(currentPage().textContent).toContain('private-user');
  });

  it('keeps details after the interactive fetch authenticates a vault locked between polls', async () => {
    const fixture = detailFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    fixture.state.entriesVersion = 2;
    await act(async () => fixture.fetch.resolve({ ...credentials, session: { ...fixture.state } }));
    expect(currentPage().dataset.popupPage).toBe('detail');
    expect(currentPage().textContent).toContain('private-user');
    expect(fixture.actions).toEqual(['snapshot', 'fetch', 'status', 'snapshot']);
  });

  it('rejects a fetched document when the session changes after that response', async () => {
    const fixture = detailFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    const fetchedSession = { ...fixture.state, entriesVersion: 2 };
    fixture.state.entriesVersion = 3;
    await act(async () => fixture.fetch.resolve({ ...credentials, session: fetchedSession }));
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(container.textContent).not.toContain('private-user');
  });

  it('scrubs detail secrets immediately while the empty exit shell remains inert', async () => {
    const fixture = detailFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    await act(async () => fixture.fetch.resolve(credentials));
    await clickAction('显示密码');
    expect(container.textContent).toContain('sensitive-password');
    await clickAction('返回密码库');
    expect(container.textContent).not.toContain('sensitive-password');
    expect(container.textContent).not.toContain('private-user');
    expect(container.textContent).not.toContain('sensitive-note');
    for (const outgoing of container.querySelectorAll('[aria-hidden="true"][data-popup-page]')) expect(outgoing.hasAttribute('inert')).toBe(true);
  });

  it('scrubs editor values immediately when the popup becomes hidden', async () => {
    const fixture = detailFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    await act(async () => fixture.fetch.resolve(credentials));
    await clickAction('编辑账号');
    expect(currentPage().dataset.popupPage).toBe('editor');
    expect(Array.from(container.querySelectorAll('input')).some(input => input.value === 'sensitive-password')).toBe(true);
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true);
    await act(async () => document.dispatchEvent(new Event('visibilitychange')));
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(Array.from(container.querySelectorAll('input')).some(input => input.value === 'sensitive-password' || input.value === 'private-user')).toBe(false);
    expect(container.textContent).not.toContain('sensitive-note');
  });

  it('scrubs loaded details as soon as a lock or disconnection is observed', async () => {
    const fixture = detailFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    await act(async () => fixture.fetch.resolve(credentials));
    await act(async () => container.querySelector<HTMLButtonElement>('button[aria-label="锁定密码库"]')!.click());
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(container.textContent).not.toContain('private-user');
    expect(container.textContent).not.toContain('sensitive-note');
    await clickAction('查看 Alex');
    expect(currentPage().textContent).toContain('private-user');
    fixture.failStatus();
    await act(async () => document.dispatchEvent(new Event('visibilitychange')));
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(container.textContent).not.toContain('private-user');
    expect(container.textContent).not.toContain('sensitive-note');
    expect(container.textContent).toContain('客户端连接已中断');
  });

  it('cancels pending details on visibility loss and never reopens them from a late response', async () => {
    const fixture = detailFixture();
    await render(fixture.client); await clickAction('查看 Alex');
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true);
    await act(async () => document.dispatchEvent(new Event('visibilitychange')));
    await act(async () => fixture.fetch.resolve(credentials));
    expect(currentPage().dataset.popupPage).toBe('list');
    expect(container.textContent).not.toContain('private-user');
    vi.restoreAllMocks();
  });

  it('does not let a cancelled read clear the busy state or notice belonging to a newer operation', async () => {
    const fixture = detailFixture(); fixture.deferCopy();
    await render(fixture.client); await clickAction('查看 Alex'); await clickAction('返回密码库');
    await clickAction('复制用户名 Alex');
    await act(async () => fixture.fetch.reject(new Error('old read failed')));
    expect(currentPage().querySelector<HTMLButtonElement>('button[aria-label="查看 Alex"]')?.disabled).toBe(true);
    expect(container.textContent).not.toContain('old read failed');
    await act(async () => fixture.copy.resolve({}));
    expect(currentPage().querySelector<HTMLButtonElement>('button[aria-label="查看 Alex"]')?.disabled).toBe(false);
    expect(container.textContent).toContain('示例数据：未复制凭证');
  });
});
