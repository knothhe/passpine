// @vitest-environment jsdom
import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Theme } from '@radix-ui/themes';
import { App } from './App';
import type { Client, EntryDocument, Snapshot, UIRequest } from '../lib/types';

let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
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

describe('create an account from a locked popup', () => {
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
    await act(async () => currentPage().querySelector<HTMLButtonElement>('button[aria-label="更多操作"]')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })));
    const menuItem = Array.from(document.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(item => item.textContent === '移入回收站')!;
    expect(menuItem).not.toBeUndefined();
    await act(async () => menuItem.click());
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
    const edit = Array.from(currentPage().querySelectorAll<HTMLButtonElement>('button')).find(button => button.textContent === '编辑')!;
    await act(async () => edit.click());
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
