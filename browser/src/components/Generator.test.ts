// @vitest-environment jsdom
import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { Theme } from '@radix-ui/themes';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Client, GeneratorOptions, UIRequest } from '../lib/types';
import { Generator } from './Generator';

let container: HTMLDivElement, root: Root;
beforeEach(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  container = document.createElement('div'); document.body.append(container); root = createRoot(container);
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (reason: unknown) => void;
  const promise = new Promise<T>((accept, decline) => { resolve = accept; reject = decline; });
  return { promise, resolve, reject };
}
function fixture() {
  const jobs: Array<{ options: GeneratorOptions; response: ReturnType<typeof deferred<{ password: string }>> }> = [];
  const copy = vi.fn(async (_password: string) => {}), usePassword = vi.fn(), close = vi.fn();
  const client: Client = {
    demo: true, copy, openOptions: async () => {},
    async request<T>(request: UIRequest): Promise<T> {
      if (request.action !== 'generate') throw new Error('Unexpected generator action');
      const response = deferred<{ password: string }>();
      jobs.push({ options: request.options, response });
      return response.promise as Promise<T>;
    },
  };
  const render = async (open = true) => { await act(async () => root.render(createElement(Theme, {}, createElement(Generator, { client, open, close, usePassword, canSave: true })))); };
  return { jobs, copy, usePassword, close, client, render };
}
const preview = () => container.querySelector<HTMLInputElement>('input[aria-label="生成的密码"]')!;
const lengthInput = () => container.querySelector<HTMLInputElement>('input[aria-label="密码长度数值"]')!;
const useButton = () => container.querySelector<HTMLButtonElement>('.generator-actions button:last-child')!;
const copyButton = () => container.querySelector<HTMLButtonElement>('.generator-actions button:first-child')!;
const refreshButton = () => container.querySelector<HTMLButtonElement>('button[aria-label="重新生成"]')!;
async function changeLength(value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(lengthInput(), value);
    lengthInput().dispatchEvent(new Event('input', { bubbles: true }));
  });
}
async function toggle(label: string) { await act(async () => container.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!.click()); }

describe('compact password generator', () => {
  it('provides the client-sized controls and immediate progress without disabling options', async () => {
    const f = fixture(); await f.render();
    expect(lengthInput().min).toBe('8'); expect(lengthInput().max).toBe('128');
    expect(container.querySelector('[role="slider"]')?.getAttribute('aria-valuemax')).toBe('128');
    expect(refreshButton().getAttribute('aria-busy')).toBe('true');
    expect(refreshButton().disabled).toBe(false); expect(lengthInput().disabled).toBe(false);
    expect(useButton().disabled).toBe(true); expect(copyButton().disabled).toBe(true);
    expect(Array.from(container.querySelectorAll('.generator-character-toggle')).map(button => button.textContent)).toEqual(['a–z', 'A–Z', '0–9', '!@#']);
    expect(container.querySelector('.generator-exclusion-row')?.textContent).toContain('I l 1 O 0');
    await act(async () => f.jobs[0]!.response.resolve({ password: 'initial-password' }));
    expect(preview().value).toBe('initial-password'); expect(useButton().disabled).toBe(false);
    expect(refreshButton().getAttribute('aria-busy')).toBeNull();
  });

  it('keeps the preview while adjusting and applies only the latest matching options', async () => {
    const f = fixture(); await f.render();
    await act(async () => f.jobs[0]!.response.resolve({ password: 'initial-password' }));
    await changeLength('128');
    expect(f.jobs[1]!.options.length).toBe(128);
    expect(preview().value).toBe('initial-password'); expect(useButton().disabled).toBe(true); expect(copyButton().disabled).toBe(true);
    await toggle('大写字母');
    expect(f.jobs[2]!.options).toMatchObject({ length: 128, uppercase: false });
    await act(async () => f.jobs[1]!.response.resolve({ password: 'old-options' }));
    expect(preview().value).toBe('initial-password'); expect(refreshButton().getAttribute('aria-busy')).toBe('true');
    await act(async () => f.jobs[2]!.response.resolve({ password: 'latest-options' }));
    expect(preview().value).toBe('latest-options'); expect(useButton().disabled).toBe(false);
    await act(async () => useButton().click());
    expect(f.usePassword).toHaveBeenCalledExactlyOnceWith('latest-options');
  });

  it('does not offer a previous password after the latest generation fails and permits retry', async () => {
    const f = fixture(); await f.render();
    await act(async () => f.jobs[0]!.response.resolve({ password: 'previous-password' }));
    await changeLength('64');
    await act(async () => f.jobs[1]!.response.reject(new Error('生成失败')));
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('生成失败');
    expect(preview().value).toBe(''); expect(useButton().disabled).toBe(true); expect(copyButton().disabled).toBe(true);
    await act(async () => useButton().click()); expect(f.usePassword).not.toHaveBeenCalled();
    await act(async () => refreshButton().click());
    expect(refreshButton().getAttribute('aria-busy')).toBe('true');
    await act(async () => f.jobs[2]!.response.resolve({ password: 'retry-password' }));
    expect(useButton().disabled).toBe(false); expect(preview().value).toBe('retry-password');
  });

  it('allows partial numeric drafts, blocks stale actions, and clamps committed input', async () => {
    const f = fixture(); await f.render();
    await act(async () => f.jobs[0]!.response.resolve({ password: 'initial-password' }));
    await changeLength(''); expect(lengthInput().value).toBe(''); expect(useButton().disabled).toBe(true);
    await changeLength('129'); expect(f.jobs).toHaveLength(1); expect(useButton().disabled).toBe(true);
    await act(async () => lengthInput().dispatchEvent(new FocusEvent('focusout', { bubbles: true })));
    expect(lengthInput().value).toBe('128'); expect(f.jobs[1]!.options.length).toBe(128);
    await act(async () => f.jobs[1]!.response.resolve({ password: 'clamped-password' }));
    expect(useButton().disabled).toBe(false);
  });

  it('retains at least one character group while generation is pending', async () => {
    const f = fixture(); await f.render();
    await toggle('小写字母'); await toggle('大写字母'); await toggle('数字');
    const last = container.querySelector<HTMLButtonElement>('button[aria-label="符号"]')!;
    expect(last.disabled).toBe(true); expect(last.getAttribute('aria-pressed')).toBe('true');
    await act(async () => last.click());
    expect(f.jobs.at(-1)!.options).toMatchObject({ lowercase: false, uppercase: false, numbers: false, symbols: true });
    expect(f.jobs).toHaveLength(4);
    expect(container.querySelector<HTMLButtonElement>('button[aria-label="小写字母"]')!.disabled).toBe(false);
  });

  it('ignores a cancelled generation after reopening', async () => {
    const f = fixture(); await f.render();
    await act(async () => container.querySelector<HTMLButtonElement>('button[aria-label="返回密码库"]')!.click());
    expect(f.close).toHaveBeenCalledOnce();
    await f.render(false); await f.render(true);
    await act(async () => f.jobs[0]!.response.resolve({ password: 'cancelled-password' }));
    expect(preview().value).toBe(''); expect(useButton().disabled).toBe(true);
    await act(async () => f.jobs[1]!.response.resolve({ password: 'reopened-password' }));
    expect(preview().value).toBe('reopened-password'); expect(useButton().disabled).toBe(false);
  });

  it('does not attach late clipboard feedback to a different generated password', async () => {
    const f = fixture(), clipboard = deferred<void>();
    f.copy.mockImplementation(() => clipboard.promise);
    await f.render(); await act(async () => f.jobs[0]!.response.resolve({ password: 'copied-old-password' }));
    await act(async () => copyButton().click());
    expect(f.copy).toHaveBeenCalledExactlyOnceWith('copied-old-password');
    await changeLength('32'); await act(async () => f.jobs[1]!.response.resolve({ password: 'new-password' }));
    await act(async () => clipboard.reject(new Error('迟到的复制失败')));
    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(copyButton().textContent?.trim()).toBe('复制'); expect(useButton().disabled).toBe(false);
  });
});
