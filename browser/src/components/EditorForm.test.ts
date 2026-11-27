// @vitest-environment jsdom
import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { Theme } from '@radix-ui/themes';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Client, UIRequest } from '../lib/types';
import { EditorForm } from './EditorForm';

let container: HTMLDivElement, root: Root;
beforeEach(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  container = document.createElement('div'); document.body.append(container); root = createRoot(container);
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.unstubAllGlobals(); });

async function change(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(Object.getPrototypeOf(input), 'value')!.set!.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}
async function click(text: string) {
  const button = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(button => button.textContent?.trim() === text)!;
  expect(button).toBeDefined(); await act(async () => button.click());
}
async function dialogClick(text: string) {
  const button = Array.from(document.querySelectorAll<HTMLButtonElement>('[role="alertdialog"] button')).find(button => button.textContent?.trim() === text)!;
  expect(button).toBeDefined(); await act(async () => button.click());
}

describe('linking a newly created account', () => {
  const tab = { id: 7, hostname: 'login.example.test', url: 'https://login.example.test/first', secure: true };
  async function renderCreate(request: Client['request']) {
    const saved = vi.fn(async () => {});
    const client: Client = { request, demo: true, openOptions: async () => {}, copy: async () => {} };
    const value = { vaultId: 'vault', creationTab: tab, password: 'secret' };
    await act(async () => root.render(createElement(Theme, {}, createElement(EditorForm, { client, value, defaultURL: new URL(tab.url).origin, close: () => {}, saved }))));
    await change(container.querySelector<HTMLInputElement>('input[placeholder^="path/to/entry"]')!, 'work/hosting');
    return saved;
  }

  it.each([false, true])('saves an entry and links the captured site only when selected (link=%s)', async link => {
    const requests: UIRequest[] = [];
    const saved = await renderCreate(async <T>(request: UIRequest) => { requests.push(request); return {} as T; });
    const checkbox = container.querySelector<HTMLElement>('[role="checkbox"]')!;
    expect(checkbox.closest('label')?.textContent).toContain('保存时关联到 login.example.test');
    expect(checkbox.getAttribute('aria-checked')).toBe('false');
    await change(container.querySelector<HTMLInputElement>('input[autocomplete="url"]')!, 'https://different.test');
    if (link) await act(async () => checkbox.click());
    await click(link ? '保存并关联' : '保存');
    expect(requests[0]).toMatchObject({ action: 'save', entry: 'work/hosting', vaultId: 'vault', mode: 'create', contents: expect.stringContaining('url: https://different.test') });
    expect(requests.slice(1)).toEqual(link ? [{ action: 'associate', vaultId: 'vault', entry: 'work/hosting', tab }] : []);
    expect(saved.mock.calls).toEqual(link ? [[tab.hostname]] : [[]]);
  });

  it.each(['retry', 'leave'])('keeps the created entry after an association failure and allows %s without another save', async action => {
    const requests: UIRequest[] = [];
    let failLink = true;
    const saved = await renderCreate(async <T>(request: UIRequest) => {
      requests.push(request);
      if (request.action === 'associate' && failLink) throw new Error('offline');
      return {} as T;
    });
    await act(async () => container.querySelector<HTMLElement>('[role="checkbox"]')!.click());
    await click('保存并关联');
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('「work/hosting」已加密保存。未能关联到 login.example.test。');
    expect(container.querySelector('form')).toBeNull();
    expect(saved).not.toHaveBeenCalled();
    failLink = false;
    if (action === 'retry') {
      const retry = Array.from(container.querySelectorAll('button')).find(button => button.textContent === '重试关联')!;
      await act(async () => { retry.click(); retry.click(); });
      expect(requests.map(request => request.action)).toEqual(['save', 'associate', 'associate']);
      expect(requests[2]).toEqual(requests[1]);
      expect(saved).toHaveBeenCalledWith(tab.hostname);
    } else {
      await click('返回密码库');
      expect(requests.map(request => request.action)).toEqual(['save', 'associate']);
      expect(saved).toHaveBeenCalledWith();
    }
  });

  it('keeps the form available to correct a failed create without associating an existing entry', async () => {
    const requests: UIRequest[] = [];
    await renderCreate(async <T>(request: UIRequest) => {
      requests.push(request);
      throw new Error('already exists');
    });
    await act(async () => container.querySelector<HTMLElement>('[role="checkbox"]')!.click());
    await click('保存并关联');
    expect(container.querySelector('form [role="alert"]')?.textContent).toContain('already exists');
    expect(requests.map(request => request.action)).toEqual(['save']);
  });
});

describe('unsaved entry changes', () => {
  const original = { contents: 'secret\nusername: alice\ntoken: existing', revision: 'revision-1', entry: { path: 'work/account', title: 'account', folder: 'work', matched: false }, vaultId: 'vault', receivedAt: Date.now() };
  async function render() {
    const close = vi.fn(), saved = vi.fn(async () => {}), request = vi.fn(async () => ({}));
    const client: Client = { demo: true, openOptions: async () => {}, copy: async () => {}, request: request as Client['request'] };
    await act(async () => root.render(createElement(Theme, {}, createElement(EditorForm, { client, value: { vaultId: 'vault', original }, defaultURL: '', close, saved }))));
    return { close, saved, request };
  }

  it.each(['back', 'cancel'])('leaves unchanged or restored input without confirmation through %s', async action => {
    const { close } = await render();
    const username = container.querySelector<HTMLInputElement>('input[placeholder="用户名或邮箱"]')!;
    await change(username, 'changed'); await change(username, 'alice');
    // An empty, newly added custom row does not change the saved document.
    await click('添加字段');
    if (action === 'cancel') await click('取消');
    else await act(async () => container.querySelector<HTMLButtonElement>('button[aria-label="返回账号详情"]')!.click());
    expect(document.querySelector('[role="alertdialog"]')).toBeNull();
    expect(close).toHaveBeenCalledOnce();
  });

  it.each(['back', 'cancel'])('keeps changed fields and selection when continuing after %s', async action => {
    const { close, request } = await render();
    const username = container.querySelector<HTMLInputElement>('input[placeholder="用户名或邮箱"]')!;
    await change(username, '中文 user@example.test');
    username.focus(); username.setSelectionRange(3, 7, 'backward');
    if (action === 'cancel') await click('取消');
    else await act(async () => container.querySelector<HTMLButtonElement>('button[aria-label="返回账号详情"]')!.click());
    expect(document.querySelector('[role="alertdialog"]')?.textContent).toContain('放弃未保存的修改？');
    const restoreFrames: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { restoreFrames.push(callback); return 1; });
    await dialogClick('继续编辑');
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });
    await act(async () => { for (const frame of restoreFrames) frame(0); });
    expect(document.querySelector('[role="alertdialog"]')).toBeNull();
    expect(username.value).toBe('中文 user@example.test');
    expect(document.activeElement).toBe(username);
    expect([username.selectionStart, username.selectionEnd, username.selectionDirection]).toEqual([3, 7, 'backward']);
    expect(close).not.toHaveBeenCalled(); expect(request).not.toHaveBeenCalled();
  });

  it('confirms removed custom fields and only leaves after discard', async () => {
    const { close, request } = await render();
    await act(async () => container.querySelector<HTMLButtonElement>('button[aria-label="删除字段 1"]')!.click());
    await click('取消');
    expect(close).not.toHaveBeenCalled();
    await dialogClick('放弃修改');
    expect(close).toHaveBeenCalledOnce(); expect(request).not.toHaveBeenCalled();
  });

  it('returns focus to the form if the last edited custom field was removed', async () => {
    await render();
    container.querySelector<HTMLInputElement>('input[aria-label="字段 1 名称"]')!.focus();
    await act(async () => container.querySelector<HTMLButtonElement>('button[aria-label="删除字段 1"]')!.click());
    await click('取消');
    const restoreFrames: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { restoreFrames.push(callback); return 1; });
    await dialogClick('继续编辑');
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });
    await act(async () => { for (const frame of restoreFrames) frame(0); });
    expect(document.activeElement).toBe(container.querySelector('form input'));
  });

  it('saves changes without a leave confirmation', async () => {
    const { close, saved, request } = await render();
    await change(container.querySelector<HTMLInputElement>('input[placeholder="用户名或邮箱"]')!, 'new-user');
    await act(async () => container.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
    expect(request).toHaveBeenCalledOnce(); expect(saved).toHaveBeenCalledOnce(); expect(close).not.toHaveBeenCalled();
    expect(document.querySelector('[role="alertdialog"]')).toBeNull();
  });
});

describe('entry editor', () => {
  it('saves multiline notes with literal field-like lines alongside custom fields', async () => {
    const requests: UIRequest[] = [];
    const client: Client = { demo: true, openOptions: async () => {}, copy: async () => {}, async request<T>(request: UIRequest) { requests.push(request); return {} as T; } };
    const original = { contents: 'secret\ntenant: acme\nnote: call support', revision: 'revision-1', entry: { path: 'work/account', title: 'account', folder: 'work', matched: false }, vaultId: 'vault', receivedAt: Date.now() };
    await act(async () => root.render(createElement(Theme, {}, createElement(EditorForm, { client, value: { vaultId: 'vault', original }, defaultURL: 'https://unrelated.example', close: () => {}, saved: async () => {} }))));
    const notes = container.querySelector('textarea')!;
    expect(notes.labels?.[0]?.textContent).toContain('备注');
    expect(notes.value).toBe('call support');
    await change(notes, 'Updated\n\ntenant: a literal note');
    await act(async () => container.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
    expect(requests).toEqual([expect.objectContaining({ contents: 'secret\ntenant: acme\nnotes: Updated\nnotes: \nnotes: tenant: a literal note' })]);
  });
  it('preserves surviving custom source lines when an earlier row is removed', async () => {
    const requests: UIRequest[] = [];
    const client: Client = { demo: true, openOptions: async () => {}, copy: async () => {}, async request<T>(request: UIRequest) { requests.push(request); return {} as T; } };
    const original = { contents: 'secret\r\na:\tone\nb: two\r\nnotes: exact', revision: 'revision-1', entry: { path: 'work/account', title: 'account', folder: 'work', matched: false }, vaultId: 'vault', receivedAt: Date.now() };
    await act(async () => root.render(createElement(Theme, {}, createElement(EditorForm, { client, value: { vaultId: 'vault', original }, defaultURL: '', close: () => {}, saved: async () => {} }))));
    await act(async () => container.querySelector<HTMLButtonElement>('button[aria-label="删除字段 1"]')!.click());
    await act(async () => container.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
    expect(requests).toEqual([expect.objectContaining({ action: 'save', contents: original.contents.replace('a:\tone\n', '') })]);
  });
  it.each(['note', 'NoTe', 'notes', 'NOTES'])('rejects the reserved custom key %s and retains existing notes after correction', async key => {
    const requests: UIRequest[] = [];
    const client: Client = { demo: true, openOptions: async () => {}, copy: async () => {}, async request<T>(request: UIRequest) { requests.push(request); return {} as T; } };
    const saved = vi.fn(async () => {});
    const original = { contents: 'old-password\r\nnotes:  keep exact  \nbody note', revision: 'revision-1', entry: { path: 'work/account', title: 'account', folder: 'work', matched: false }, vaultId: 'vault', receivedAt: Date.now() };
    await act(async () => root.render(createElement(Theme, {}, createElement(EditorForm, { client, value: { vaultId: 'vault', original }, defaultURL: '', close: () => {}, saved }))));
    await click('添加字段');
    const name = container.querySelector<HTMLInputElement>('input[aria-label="字段 1 名称"]')!;
    await change(name, key);
    await change(container.querySelector<HTMLInputElement>('input[aria-label="字段 1 值"]')!, 'private-token');
    await act(async () => container.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('note 和 notes 是备注保留字段名');
    expect(requests).toEqual([]);
    expect(saved).not.toHaveBeenCalled();
    await change(name, 'api_token');
    await act(async () => container.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
    expect(requests).toEqual([expect.objectContaining({ action: 'save', contents: original.contents + '\r\napi_token: private-token' })]);
    expect(saved).toHaveBeenCalledOnce();
  });

  it.each(['username', 'LOGIN', 'user', 'email', 'url', 'URI', 'website', 'site', 'link'])('rejects new primary alias %s without saving', async key => {
    const request = vi.fn(async () => ({}));
    const client: Client = { demo: true, openOptions: async () => {}, copy: async () => {}, request: request as Client['request'] };
    await act(async () => root.render(createElement(Theme, {}, createElement(EditorForm, { client, value: { vaultId: 'vault', password: 'secret' }, defaultURL: '', close: () => {}, saved: async () => {} }))));
    await change(container.querySelector<HTMLInputElement>('input[placeholder^="path/to/entry"]')!, 'work/account');
    await click('添加字段');
    await change(container.querySelector<HTMLInputElement>('input[aria-label="字段 1 名称"]')!, key);
    await change(container.querySelector<HTMLInputElement>('input[aria-label="字段 1 值"]')!, 'shadow');
    await act(async () => container.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('是用户名或网站保留字段名');
    expect(request).not.toHaveBeenCalled();
  });

  it('rejects a custom rename that would shadow the primary username', async () => {
    const request = vi.fn(async () => ({}));
    const client: Client = { demo: true, openOptions: async () => {}, copy: async () => {}, request: request as Client['request'] };
    const original = { contents: 'secret\ntoken: untouched\nusername: alice', revision: 'revision-1', entry: { path: 'work/account', title: 'account', folder: 'work', matched: false }, vaultId: 'vault', receivedAt: Date.now() };
    await act(async () => root.render(createElement(Theme, {}, createElement(EditorForm, { client, value: { vaultId: 'vault', original }, defaultURL: '', close: () => {}, saved: async () => {} }))));
    await change(container.querySelector<HTMLInputElement>('input[aria-label="字段 1 名称"]')!, 'username');
    await act(async () => container.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('username 是用户名或网站保留字段名');
    expect(request).not.toHaveBeenCalled();
  });

  it.each(['²', 'Ⅳ'])('rejects numeric non-digit key %s instead of saving it as native plaintext notes', async key => {
    const request = vi.fn(async () => ({}));
    const client: Client = { demo: true, openOptions: async () => {}, copy: async () => {}, request: request as Client['request'] };
    await act(async () => root.render(createElement(Theme, {}, createElement(EditorForm, { client, value: { vaultId: 'vault', password: 'secret' }, defaultURL: '', close: () => {}, saved: async () => {} }))));
    await change(container.querySelector<HTMLInputElement>('input[placeholder^="path/to/entry"]')!, 'work/account');
    await click('添加字段');
    await change(container.querySelector<HTMLInputElement>('input[aria-label="字段 1 名称"]')!, key);
    await change(container.querySelector<HTMLInputElement>('input[aria-label="字段 1 值"]')!, 'private-token');
    await act(async () => container.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('十进制数字');
    expect(request).not.toHaveBeenCalled();
  });

  it('allows existing secondary aliases to retain their keys and change values', async () => {
    const requests: UIRequest[] = [];
    const client: Client = { demo: true, openOptions: async () => {}, copy: async () => {}, async request<T>(request: UIRequest) { requests.push(request); return {} as T; } };
    const original = { contents: 'secret\nlogin: alice\nusername: alternate\nurl: https://primary.test\nwebsite: https://secondary.test', revision: 'revision-1', entry: { path: 'work/account', title: 'account', folder: 'work', matched: false }, vaultId: 'vault', receivedAt: Date.now() };
    await act(async () => root.render(createElement(Theme, {}, createElement(EditorForm, { client, value: { vaultId: 'vault', original }, defaultURL: '', close: () => {}, saved: async () => {} }))));
    await change(container.querySelector<HTMLInputElement>('input[aria-label="字段 1 值"]')!, 'updated');
    await change(container.querySelector<HTMLInputElement>('input[aria-label="字段 2 值"]')!, 'https://updated.test');
    await act(async () => container.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
    expect(requests).toEqual([expect.objectContaining({ action: 'save', contents: original.contents.replace('username: alternate', 'username: updated').replace('website: https://secondary.test', 'website: https://updated.test') })]);
  });

  it('keeps the draft through the generator and saves custom fields without rewriting old notes', async () => {
    const requests: UIRequest[] = [];
    const client: Client = { demo: true, openOptions: async () => {}, copy: async () => {}, async request<T>(request: UIRequest) { requests.push(request); return { password: 'generated-password' } as T; } };
    const saved = vi.fn(async () => {});
    const original = { contents: 'old-password\r\nlogin: alice\r\ncustom: first\r\nnotes:  keep exact  \nbody note', revision: 'revision-1', entry: { path: 'work/account', title: 'account', folder: 'work', matched: false }, vaultId: 'vault', receivedAt: Date.now() };
    await act(async () => root.render(createElement(Theme, {}, createElement(EditorForm, { client, value: { vaultId: 'vault', original }, defaultURL: '', close: () => {}, saved }))));
    expect(container.querySelector('textarea')?.value).toBe('keep exact  \nbody note');
    const path = container.querySelector<HTMLInputElement>('input[placeholder^="path/to/entry"]')!;
    await change(path, 'work/renamed');
    await change(container.querySelector<HTMLInputElement>('input[aria-label="字段 1 名称"]')!, 'environment');
    await change(container.querySelector<HTMLInputElement>('input[aria-label="字段 1 值"]')!, 'production');
    await click('重新生成密码');
    expect(container.querySelector('.generator-screen')?.textContent).toContain('密码生成器');
    expect(container.querySelector('form')?.getAttribute('aria-hidden')).toBe('true');
    expect(container.querySelector('form')?.hasAttribute('inert')).toBe(true);
    await click('使用此密码');
    expect(container.querySelector('form')?.getAttribute('aria-hidden')).toBe('false');
    expect(container.querySelector('form')?.hasAttribute('inert')).toBe(false);
    expect(container.querySelector('.generator-screen')).toBeNull();
    expect(path.value).toBe('work/renamed');
    const password = container.querySelector<HTMLInputElement>('#entry-password')!;
    expect(password.value).toBe('generated-password');
    expect(password.type).toBe('password');
    expect(document.activeElement).toBe(password);
    await act(async () => container.querySelector<HTMLButtonElement>('button[aria-label="显示密码"]')!.click());
    expect(password.type).toBe('text');
    await act(async () => container.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
    expect(requests.find(request => request.action === 'save')).toMatchObject({ action: 'save', destination: 'work/renamed', revision: 'revision-1', contents: 'generated-password\r\nlogin: alice\r\nenvironment: production\r\nnotes:  keep exact  \nbody note' });
    expect(saved).toHaveBeenCalledOnce();
  });
});
