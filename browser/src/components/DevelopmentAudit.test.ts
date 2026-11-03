// @vitest-environment jsdom
import { act, createElement, type ReactNode } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { Theme } from '@radix-ui/themes';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import DevelopmentAudit from './DevelopmentAudit';
import { LanguageProvider } from './LanguageProvider';
import { App } from './App';
import { chineseCopy } from '../constants/copy';
import { englishCopy } from '../i18n/en';
import { writeLanguage } from '../lib/language';
import type { Client, Snapshot, UIRequest } from '../lib/types';

vi.mock('wxt/browser', () => ({ browser: { storage: { local: { get: vi.fn(async () => ({})) }, onChanged: { addListener: vi.fn(), removeListener: vi.fn() } } } }));
let container: HTMLDivElement, root: Root;
let frames: Map<number, FrameRequestCallback>, frameID: number;
beforeEach(() => {
  localStorage.clear();
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  frames = new Map(); frameID = 0;
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => { frames.set(++frameID, callback); return frameID; });
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(id => { frames.delete(id); });
  container = document.createElement('div'); container.id = 'root'; document.body.append(container); root = createRoot(container);
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

function fixture() {
  const snapshot: Snapshot = { configured: true, vaultId: 'audit-fixture', vaultName: '示例库', locked: false, entriesVersion: 1, capabilities: [], entries: [], matches: [], tab: { id: 1, url: 'https://example.test', hostname: 'example.test', secure: true } };
  const request = vi.fn(async (_request: UIRequest) => snapshot);
  const client: Client = { demo: true, request: async <T>(value: UIRequest) => await request(value) as T, openOptions: vi.fn(async () => {}), copy: async () => {} };
  return { client, request };
}
async function press(target: EventTarget, overrides: KeyboardEventInit = {}) {
  const event = new KeyboardEvent('keydown', { key: 'L', code: 'KeyL', ctrlKey: true, altKey: true, shiftKey: true, bubbles: true, cancelable: true, ...overrides });
  await act(async () => { target.dispatchEvent(event); });
  return event;
}
async function render(client?: Client, surface: 'popup' | 'options' = 'popup', children?: ReactNode) {
  await act(async () => root.render(createElement(LanguageProvider, { demo: true, children: createElement(Theme, {}, client && createElement(App, { client }), children, createElement(DevelopmentAudit, { surface })) })));
}
async function flushFrames() {
  await act(async () => {
    const pending = [...frames.values()]; frames.clear();
    for (const callback of pending) callback(0);
  });
}

describe('development inspector sidecar', () => {
  it('preserves the real editor node, draft and client requests across inspector and language toggles', async () => {
    const { client, request } = fixture();
    await render(client);
    await act(async () => container.querySelector<HTMLButtonElement>('button[aria-label="新建账号"]')!.click());
    const input = container.querySelector<HTMLInputElement>('input[placeholder="用户名或邮箱"]')!;
    expect(input).not.toBeNull();
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'draft-user@example.test');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    const requests = request.mock.calls.length;
    await press(input);
    expect(container.querySelector('.development-audit')).not.toBeNull();
    await act(async () => writeLanguage('en', true));
    expect(container.querySelector('input[placeholder="Username or email"]')).toBe(input);
    expect(input.value).toBe('draft-user@example.test');
    expect(request.mock.calls.length).toBe(requests);
    await act(async () => container.querySelector<HTMLButtonElement>(`.development-audit button[aria-label="${englishCopy.audit.inspect}"]`)!.click());
    const escape = await press(input, { key: 'Escape', code: 'Escape', ctrlKey: false, altKey: false, shiftKey: false });
    expect(escape.defaultPrevented).toBe(true);
    expect(container.querySelector('.development-audit')).toBeNull();
    expect(container.querySelector('input[placeholder="Username or email"]')).toBe(input);
    expect(input.value).toBe('draft-user@example.test');
    expect(request.mock.calls.length).toBe(requests);
  });

  it('requires the exact chord and ignores repeated and composing key events', async () => {
    await render();
    expect(container.querySelector('[data-layout-audit-ui]')).toBeNull();
    for (const modifiers of [{ altKey: false }, { shiftKey: false }, { ctrlKey: false }, { metaKey: true }, { repeat: true }, { isComposing: true }]) {
      expect((await press(document, modifiers)).defaultPrevented).toBe(false);
      expect(container.querySelector('.development-audit')).toBeNull();
    }
    expect((await press(document, { ctrlKey: false, metaKey: true })).defaultPrevented).toBe(true);
    expect(container.querySelector('.development-audit')).not.toBeNull();
    await press(document, { ctrlKey: false, metaKey: true });
    expect(container.querySelector('[data-layout-audit-ui]')).toBeNull();
  });

  it('leaves the popup footer unobstructed while closed and retains the settings launcher', async () => {
    const { client } = fixture();
    await render(client);
    const settings = () => container.querySelector<HTMLButtonElement>(`button[aria-label="${chineseCopy.vault.settings}"]`)!;
    expect(container.querySelector('[data-layout-audit-ui]')).toBeNull();
    await act(async () => settings().click());
    expect(client.openOptions).toHaveBeenCalledOnce();
    await press(document); await press(document);
    expect(container.querySelector('[data-layout-audit-ui]')).toBeNull();
    await act(async () => settings().click());
    expect(client.openOptions).toHaveBeenCalledTimes(2);
    await render(undefined, 'options');
    const launcher = container.querySelector<HTMLButtonElement>(`button[aria-label="${chineseCopy.audit.devOpen}"]`)!;
    expect(launcher).not.toBeNull();
    await act(async () => launcher.click());
    expect(container.querySelector('.development-audit')).not.toBeNull();
  });

  it('uses the active A or B slot when selecting from the component tree', async () => {
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({ x: 0, y: 0, width: 100, height: 30, left: 0, top: 0, right: 100, bottom: 30, toJSON: () => ({}) });
    await render(undefined, 'popup', createElement('main', {}, createElement('button', { id: 'target-a' }, 'A'), createElement('button', { id: 'target-b' }, 'B')));
    await press(document); await flushFrames();
    const changeTree = async (id: string) => {
      const select = container.querySelector<HTMLSelectElement>('.development-audit-tree select')!;
      const option = [...select.options].find(option => option.textContent?.includes(`#${id}`))!;
      await act(async () => { select.value = option.value; select.dispatchEvent(new Event('change', { bubbles: true })); });
      await flushFrames();
      expect(select.value).toBe(option.value);
      expect(container.querySelector('.development-audit-selector')?.textContent).toContain(`#${id}`);
    };
    const choose = async (label: string) => {
      await act(async () => [...container.querySelectorAll<HTMLButtonElement>('.development-audit-tools button')].find(button => button.textContent === label)!.click());
      await act(async () => container.querySelector<HTMLButtonElement>(`button[aria-label="${chineseCopy.audit.devExpand}"]`)!.click());
    };
    await changeTree('target-a');
    await choose(chineseCopy.audit.pickReference);
    await changeTree('target-b');
    expect(container.querySelector('.development-audit-comparison code')?.textContent).toContain('#target-b');
    await choose(chineseCopy.audit.pickPrimary);
    expect(container.querySelector('.development-audit-selector')?.textContent).toContain('#target-a');
    expect(container.querySelector<HTMLSelectElement>('.development-audit-tree select')?.selectedOptions[0]?.textContent).toContain('#target-a');
  });
});
