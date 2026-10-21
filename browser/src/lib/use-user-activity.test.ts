// @vitest-environment jsdom
import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { useUserActivity } from './use-user-activity';
import type { Client, UIRequest } from './types';

let container: HTMLDivElement, root: Root;
let request: Mock<(value: UIRequest) => Promise<unknown>>, client: Client;
const spyAdded = () => vi.spyOn(document, 'addEventListener');
const spyRemoved = () => vi.spyOn(document, 'removeEventListener');
let added: ReturnType<typeof spyAdded>, removed: ReturnType<typeof spyRemoved>;
const events = ['pointerdown', 'keydown', 'input'] as const;
function Surface({ enabled = true, locale = 'zh-CN' }: { enabled?: boolean; locale?: string }) {
  useUserActivity(client, enabled);
  return createElement('input', { 'aria-label': locale, defaultValue: 'private local edit' });
}
async function render(props: Parameters<typeof Surface>[0] = {}) {
  await act(async () => root.render(createElement(Surface, props)));
}
function listener(type: string) {
  const call = added.mock.calls.find(([event]) => event === type);
  expect(call).toBeDefined();
  return call![1] as EventListener;
}
async function trusted(type: string) {
  // jsdom cannot manufacture trusted DOM events. Invoke the installed browser
  // listener with only the trusted metadata, without entry text or input data.
  await act(async () => listener(type)({ isTrusted: true } as Event));
}
function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>(done => { resolve = done; });
  return { promise, resolve };
}

beforeEach(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  vi.useFakeTimers();
  request = vi.fn(async (_value: UIRequest) => ({}));
  client = { demo: false, request: async <T>(value: UIRequest) => await request(value) as T, copy: async () => {}, openOptions: async () => {} };
  added = spyAdded(); removed = spyRemoved();
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false);
  container = document.createElement('div'); document.body.append(container); root = createRoot(container);
});
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks(); vi.useRealTimers();
});

describe('extension user activity', () => {
  it('ignores synthetic input, rendering, locale changes and metadata lifecycle events', async () => {
    await render();
    for (const type of [...events, 'visibilitychange', 'focus', 'change']) document.dispatchEvent(new Event(type, { bubbles: true }));
    await render({ locale: 'en' });
    await act(async () => vi.advanceTimersByTime(60_000));
    expect(request).not.toHaveBeenCalled();
    for (const type of events) expect(added.mock.calls.filter(([event]) => event === type)).toHaveLength(1);
    expect(container.querySelector('input')?.value).toBe('private local edit');
  });

  it('reports real pointer, keyboard and local input activity at most once per second without secret payloads', async () => {
    await render();
    await trusted('pointerdown');
    await trusted('keydown'); await trusted('input');
    expect(request).toHaveBeenCalledExactlyOnceWith({ action: 'touch' });
    await act(async () => vi.advanceTimersByTime(999));
    await trusted('keydown');
    expect(request).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTime(1));
    await trusted('input');
    expect(request).toHaveBeenCalledTimes(2);
    // Continued local typing must keep reaching the service well inside its
    // five-second Git idle window even when no credential action is needed.
    for (let index = 0; index < 6; index++) {
      await act(async () => vi.advanceTimersByTime(1000));
      await trusted('keydown');
    }
    expect(request).toHaveBeenCalledTimes(8);
    expect(request.mock.calls.every(([value]) => JSON.stringify(value) === '{"action":"touch"}')).toBe(true);
    await act(async () => vi.advanceTimersByTime(60_000));
    expect(request).toHaveBeenCalledTimes(8);
  });

  it('allows only one notification in flight and recovers silently after a failed report', async () => {
    const first = deferred(); request.mockReturnValueOnce(first.promise);
    await render(); await trusted('input');
    await act(async () => vi.advanceTimersByTime(5000));
    await trusted('keydown');
    expect(request).toHaveBeenCalledTimes(1);
    await act(async () => first.resolve());
    request.mockRejectedValueOnce(new Error('client disconnected'));
    await trusted('pointerdown');
    expect(request).toHaveBeenCalledTimes(2);
    await act(async () => vi.advanceTimersByTime(1000));
    await trusted('input');
    expect(request).toHaveBeenCalledTimes(3);
    expect(container.querySelector('input')?.disabled).toBe(false);
    expect(container.querySelector('input')?.value).toBe('private local edit');
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it('skips demos and hidden surfaces and cleans up all listeners without trailing reports', async () => {
    client = { ...client, demo: true };
    await render();
    expect(added.mock.calls.filter(([event]) => events.includes(event as typeof events[number]))).toHaveLength(0);
    client = { ...client, demo: false };
    await render({ enabled: false });
    await render();
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true);
    await trusted('input');
    expect(request).not.toHaveBeenCalled();
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false);
    await trusted('input');
    expect(request).toHaveBeenCalledTimes(1);
    const previous = listener('input');
    await render({ enabled: false });
    for (const type of events) expect(removed).toHaveBeenCalledWith(type, listener(type), true);
    await act(async () => vi.advanceTimersByTime(60_000));
    await act(async () => previous({ isTrusted: true } as Event));
    expect(request).toHaveBeenCalledTimes(1);
  });
});
