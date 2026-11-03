// @vitest-environment jsdom
import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { LayoutAudit } from '../../preview/audit/LayoutAudit';
import { chineseCopy } from '../constants/copy';

vi.mock('wxt/browser', () => ({ browser: {} }));

let host: HTMLElement, root: Root, reportBlob: Blob | undefined;
let frames: Map<number, FrameRequestCallback>, frameID: number;
beforeEach(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  history.replaceState(null, '', '/?view=audit&state=connected');
  frames = new Map(); frameID = 0; reportBlob = undefined;
  vi.stubGlobal('URL', class extends URL {
    static override createObjectURL(blob: Blob) { reportBlob = blob; return 'blob:layout-audit'; }
    static override revokeObjectURL() {}
  });
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
  host = document.createElement('div'); document.body.append(host); root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove(); history.replaceState(null, '', '/');
  vi.restoreAllMocks(); vi.unstubAllGlobals();
});

async function flushFrames() {
  await act(async () => {
    const pending = [...frames.values()]; frames.clear();
    for (const callback of pending) callback(0);
  });
}

describe('layout audit report', () => {
  it('exports initial fixture state separately from live iframe metadata and excludes page contents', async () => {
    await act(async () => root.render(createElement(LayoutAudit)));
    const iframe = host.querySelector('iframe')!;
    const document = iframe.contentDocument!, win = iframe.contentWindow!;
    document.open(); document.write('<!doctype html><html><head></head><body></body></html>'); document.close();
    Object.assign(win, {
      requestAnimationFrame: (callback: FrameRequestCallback) => { frames.set(++frameID, callback); return frameID; },
      cancelAnimationFrame: (id: number) => { frames.delete(id); },
      ResizeObserver: class { observe() {} unobserve() {} disconnect() {} },
    });
    document.body.innerHTML = '<main class="popup-shell"><button id="lock-action">Lock</button><input value="private-draft-value"></main>';
    const product = document.querySelector<HTMLElement>('.popup-shell')!;
    vi.spyOn(product, 'getBoundingClientRect').mockReturnValue({ x: 0, y: 0, width: 380, height: 460, left: 0, top: 0, right: 380, bottom: 460, toJSON: () => ({}) });
    await act(async () => iframe.dispatchEvent(new Event('load')));
    await flushFrames();

    // The iframe can change after its initial fixture was chosen. Exports must
    // not label that original configuration as the current connection state.
    product.querySelector('button')!.addEventListener('click', () => {
      product.append('private-locked-page-text');
      document.documentElement.lang = 'en';
      document.documentElement.dataset.fontScale = '120';
      product.style.fontSize = '18px';
      win.history.replaceState(null, '', '#appearance');
    });
    await act(async () => product.querySelector<HTMLButtonElement>('button')!.click());
    await flushFrames();
    const exportButton = [...host.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === chineseCopy.audit.exportReport)!;
    await act(async () => exportButton.click());
    expect(reportBlob).toBeDefined();
    const source = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader(); reader.onload = () => resolve(String(reader.result)); reader.onerror = reject; reader.readAsText(reportBlob!);
    });
    const report = JSON.parse(source);
    expect(report).toMatchObject({ schema: 'layout-audit/v1', surface: 'popup', initialConnection: 'connected', section: '#appearance', locale: 'en', fontScale: 120, viewport: { width: 380, height: 460 }, selected: { selector: 'main.popup-shell', rect: { width: 380, height: 460 }, font: { size: 18 } } });
    expect(report).not.toHaveProperty('connection');
    expect(source).not.toContain('private-draft-value');
    expect(source).not.toContain('private-locked-page-text');
    expect(report.selected).not.toHaveProperty('element');
  });
});
