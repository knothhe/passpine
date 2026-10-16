// @vitest-environment jsdom
import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useExpiringValue } from './use-expiring-value';

let container: HTMLDivElement, root: Root;
let show: (message: string) => void;
function Feedback() {
  const [message, setMessage] = useExpiringValue('', 4500);
  show = setMessage;
  return createElement('span', { role: 'status' }, message);
}
beforeEach(async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-16T00:00:00Z'));
  container = document.createElement('div'); document.body.append(container); root = createRoot(container);
  await act(async () => root.render(createElement(Feedback)));
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.useRealTimers(); });

describe('feedback expiry', () => {
  it('continues counting down without window focus', async () => {
    await act(async () => show('已复制'));
    window.dispatchEvent(new Event('blur'));
    await act(async () => vi.advanceTimersByTime(4499));
    expect(container.textContent).toBe('已复制');
    await act(async () => vi.advanceTimersByTime(1));
    expect(container.textContent).toBe('');
  });

  it.each(['focus', 'visibilitychange'])('clears overdue feedback on %s when background timers were suspended', async event => {
    await act(async () => show('已复制'));
    // Advance the wall clock without running timers, as a suspended page would.
    vi.setSystemTime(Date.now() + 60000);
    await act(async () => (event === 'focus' ? window : document).dispatchEvent(new Event(event)));
    expect(container.textContent).toBe('');
  });

  it('preserves the original deadline across focus changes and gives a newer notice its own deadline', async () => {
    await act(async () => show('第一条提示'));
    await act(async () => vi.advanceTimersByTime(3000));
    await act(async () => window.dispatchEvent(new Event('focus')));
    await act(async () => vi.advanceTimersByTime(1000));
    await act(async () => show('第二条提示'));
    await act(async () => vi.advanceTimersByTime(500));
    expect(container.textContent).toBe('第二条提示');
    await act(async () => vi.advanceTimersByTime(3999));
    expect(container.textContent).toBe('第二条提示');
    await act(async () => vi.advanceTimersByTime(1));
    expect(container.textContent).toBe('');
  });
});
