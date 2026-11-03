// @vitest-environment jsdom
import { act, createElement, type ReactNode } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuditOverlay } from './AuditOverlay';
import { useLayoutAudit } from './useLayoutAudit';
import { measureElement, compareRects } from './geometry';
import type { AuditOptions } from './types';

let reactRoot: Root, host: HTMLElement, product: HTMLElement, first: HTMLButtonElement, second: HTMLInputElement;
let current: ReturnType<typeof useLayoutAudit>;
let callbacks: Map<number, FrameRequestCallback>, sequence: number;
const observers: { disconnect: ReturnType<typeof vi.fn>; observe: ReturnType<typeof vi.fn> }[] = [];
function bounds(element: Element, x: number, y: number, width: number, height: number) {
  vi.spyOn(element, 'getBoundingClientRect').mockReturnValue({ x, y, width, height, left: x, top: y, right: x + width, bottom: y + height, toJSON: () => ({}) });
}
function Harness(options: AuditOptions & { overlay?: boolean; children?: ReactNode }) {
  current = useLayoutAudit(options);
  return options.overlay ? createElement(AuditOverlay, { measurement: current.selected, rootRect: current.rootRect, ownerDocument: options.root?.ownerDocument, grid: 8, formatValue: value => `${value} px` }) : null;
}
async function render(options: Partial<AuditOptions> = {}, overlay = false) {
  await act(async () => reactRoot.render(createElement(Harness, { root: product, ...options, overlay })));
  await flush();
}
async function flush() {
  await act(async () => {
    const pending = [...callbacks]; callbacks.clear();
    for (const [, callback] of pending) callback(0);
  });
}
beforeEach(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  sequence = 0; callbacks = new Map(); observers.length = 0;
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => { callbacks.set(++sequence, callback); return sequence; });
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(id => { callbacks.delete(id); });
  vi.stubGlobal('ResizeObserver', class {
    observe = vi.fn(); unobserve = vi.fn(); disconnect = vi.fn();
    constructor() { observers.push(this); }
  });
  product = document.createElement('main'); first = document.createElement('button'); second = document.createElement('input');
  first.textContent = 'Product action'; second.value = 'unchanged draft'; product.append(first, second);
  host = document.createElement('aside'); host.dataset.layoutAuditUi = '';
  document.body.append(product, host); reactRoot = createRoot(host);
  bounds(product, 0, 0, 380, 460); bounds(first, 14, 20, 100, 30); bounds(second, 14, 56, 100, 30);
});
afterEach(async () => {
  await act(async () => reactRoot.unmount());
  document.body.replaceChildren(); vi.restoreAllMocks(); vi.unstubAllGlobals();
});

describe('useLayoutAudit lifecycle and input isolation', () => {
  it('attaches no observers and leaves input events untouched when disabled', async () => {
    await render({ enabled: false, inspect: true });
    const callback = vi.fn(); first.addEventListener('pointerdown', callback);
    const event = new MouseEvent('pointerdown', { bubbles: true, cancelable: true }); first.dispatchEvent(event);
    expect(observers).toHaveLength(0); expect(current.elements).toHaveLength(0);
    expect(callback).toHaveBeenCalledOnce(); expect(event.defaultPrevented).toBe(false);
  });

  it('tracks selection, live style changes and A/B distance without replacing product nodes', async () => {
    await render();
    await act(async () => { current.select(first); current.select(second, 'reference'); }); await flush();
    expect(current.selected?.element).toBe(first); expect(current.reference?.element).toBe(second);
    expect(current.comparison?.verticalGap).toBe(6);
    await act(async () => { first.style.fontSize = '18px'; }); await flush();
    expect(current.selected?.font.size).toBe(18);
    expect(product.querySelector('input')).toBe(second); expect(second.value).toBe('unchanged draft');
    await render({ inspect: true });
    expect(current.selected?.element).toBe(first);
    await act(async () => current.clearReference()); await flush(); expect(current.comparison).toBeNull();
  });

  it('refreshes fixed-size elements when body or an intermediate ancestor changes styles', async () => {
    const ancestor = document.createElement('section'); product.replaceWith(ancestor); ancestor.append(product);
    const stylesheet = document.createElement('style');
    stylesheet.textContent = 'button { font-size: 12px; line-height: 20px; } body.audit-large button { font-size: 24px; } section.audit-spaced button { letter-spacing: 3px; }';
    document.head.append(stylesheet);
    try {
      await render(); await act(async () => current.select(first)); await flush();
      expect(current.selected?.font.size).toBe(12);
      const originalRect = current.selected?.rect;
      await act(async () => document.body.classList.add('audit-large')); await flush();
      expect(current.selected?.font.size).toBe(24); expect(current.selected?.rect).toEqual(originalRect);
      await act(async () => ancestor.classList.add('audit-spaced')); await flush();
      expect(current.selected?.font.letterSpacing).toBe('3px'); expect(current.selected?.rect).toEqual(originalRect);
    } finally { stylesheet.remove(); document.body.classList.remove('audit-large'); }
  });

  it('suppresses pointerdown, click and keyboard activation before product handlers', async () => {
    await render({ inspect: true });
    const action = vi.fn(), pointerAction = vi.fn();
    first.addEventListener('click', action); first.addEventListener('pointerdown', pointerAction);
    const pointer = new MouseEvent('pointerdown', { bubbles: true, cancelable: true });
    await act(async () => { first.dispatchEvent(pointer); first.click(); }); await flush();
    expect(pointer.defaultPrevented).toBe(true); expect(action).not.toHaveBeenCalled(); expect(pointerAction).not.toHaveBeenCalled();
    expect(current.selected?.element).toBe(first);
    const keyboard = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }); second.dispatchEvent(keyboard);
    expect(keyboard.defaultPrevented).toBe(true);
    const keyUpAction = vi.fn(), pointerUpAction = vi.fn();
    second.addEventListener('keyup', keyUpAction); second.addEventListener('pointerup', pointerUpAction);
    second.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', bubbles: true, cancelable: true }));
    second.dispatchEvent(new MouseEvent('pointerup', { bubbles: true, cancelable: true }));
    expect(keyUpAction).not.toHaveBeenCalled(); expect(pointerUpAction).not.toHaveBeenCalled();
    const edit = new InputEvent('beforeinput', { data: 'x', inputType: 'insertText', bubbles: true, cancelable: true }); second.dispatchEvent(edit);
    expect(edit.defaultPrevented).toBe(true); expect(second.value).toBe('unchanged draft');
    const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }); second.dispatchEvent(escape);
    expect(escape.defaultPrevented).toBe(false);
    await flush(); expect(current.selected?.element).toBe(second);
  });

  it('blocks middle-button activation only while inspecting and removes its capture handler on unmount', async () => {
    const action = vi.fn(); first.addEventListener('auxclick', action);
    const middleClick = () => {
      const event = new MouseEvent('auxclick', { bubbles: true, cancelable: true, button: 1 }); first.dispatchEvent(event); return event;
    };
    await render({ inspect: true });
    expect(middleClick().defaultPrevented).toBe(true); expect(action).not.toHaveBeenCalled();
    await render({ inspect: false });
    expect(middleClick().defaultPrevented).toBe(false); expect(action).toHaveBeenCalledTimes(1);
    await render({ inspect: true });
    await act(async () => reactRoot.unmount());
    expect(middleClick().defaultPrevented).toBe(false); expect(action).toHaveBeenCalledTimes(2);
    reactRoot = createRoot(host);
  });

  it('allows inspector controls and restores app events when inspect is switched off', async () => {
    await render({ inspect: true });
    const control = document.createElement('button'); control.dataset.layoutAuditUi = ''; product.append(control);
    const controlAction = vi.fn(); control.addEventListener('click', controlAction); control.click();
    expect(controlAction).toHaveBeenCalledOnce();
    await render({ inspect: false });
    const action = vi.fn(); first.addEventListener('click', action); first.click();
    expect(action).toHaveBeenCalledOnce();
    await flush(); expect(current.selected).toBeNull();
  });

  it('inspects opted-in portals and drops removed elements', async () => {
    await render({ inspect: true, portalSelector: '[data-sample-portal]' });
    const portal = document.createElement('section'), button = document.createElement('button');
    portal.dataset.samplePortal = ''; portal.append(button);
    bounds(portal, 400, 10, 200, 100); bounds(button, 410, 20, 100, 30);
    await act(async () => document.body.append(portal)); await flush();
    expect(current.elements.some(node => node.element === button)).toBe(true);
    await act(async () => button.click()); await flush(); expect(current.selected?.element).toBe(button);
    await act(async () => portal.remove()); await flush();
    expect(current.selected).toBeNull(); expect(current.elements.some(node => node.element === button)).toBe(false);
  });

  it('reconciles portal selector attributes being removed and restored without layout changes', async () => {
    const portal = document.createElement('section'), button = document.createElement('button');
    portal.dataset.samplePortal = ''; portal.append(button); document.body.append(portal);
    bounds(portal, 400, 10, 200, 100); bounds(button, 410, 20, 100, 30);
    await render({ portalSelector: '[data-sample-portal]' });
    await act(async () => current.select(button)); await flush();
    expect(current.selected?.element).toBe(button);
    await act(async () => portal.removeAttribute('data-sample-portal')); await flush();
    expect(current.selected).toBeNull(); expect(current.elements.some(node => node.element === button)).toBe(false);
    await act(async () => { portal.dataset.samplePortal = ''; }); await flush();
    expect(current.elements.some(node => node.element === button)).toBe(true);
  });

  it('detects an ancestor selector creating and removing a portal scope', async () => {
    const container = document.createElement('section'), portal = document.createElement('div'), button = document.createElement('button');
    portal.dataset.samplePortal = ''; portal.append(button); container.append(portal); document.body.append(container);
    bounds(portal, 400, 10, 200, 100); bounds(button, 410, 20, 100, 30);
    await render({ portalSelector: '.audit-active [data-sample-portal]' });
    expect(current.elements.some(node => node.element === button)).toBe(false);
    await act(async () => { container.className = 'audit-active'; }); await flush();
    expect(current.elements.some(node => node.element === button)).toBe(true);
    await act(async () => current.select(button, 'reference')); await flush();
    await act(async () => { container.className = ''; }); await flush();
    expect(current.reference).toBeNull(); expect(current.elements.some(node => node.element === button)).toBe(false);
  });

  it.each(['data-audit-ignore', 'data-layout-audit-ui'])('reconciles the %s exclusion being added and removed', async attribute => {
    await render({ excludeSelector: '[data-audit-ignore]' });
    await act(async () => current.select(first)); await flush();
    await act(async () => first.setAttribute(attribute, '')); await flush();
    expect(current.selected).toBeNull(); expect(current.elements.some(node => node.element === first)).toBe(false);
    await act(async () => first.removeAttribute(attribute)); await flush();
    expect(current.elements.some(node => node.element === first)).toBe(true);
  });

  it('ignores toolbar and overlay mutations without scheduling measurement feedback', async () => {
    const control = document.createElement('div'); control.dataset.layoutAuditUi = ''; control.textContent = 'Tool'; product.append(control);
    await render({}, true); await flush();
    await act(async () => current.select(first)); await flush();
    expect(callbacks.size).toBe(0);
    const selected = current.selected;
    await act(async () => {
      control.textContent = 'Updated tool'; control.style.width = '100px';
      const extra = document.createElement('aside'); extra.dataset.layoutAuditUi = ''; document.body.append(extra); extra.remove();
      host.setAttribute('data-status', 'updated');
    });
    expect(callbacks.size).toBe(0); expect(current.selected).toBe(selected);
  });

  it('drops measurements when the root DOM node is removed without a prop update', async () => {
    await render(); await act(async () => current.select(first)); await flush();
    expect(current.selected?.element).toBe(first);
    await act(async () => product.remove()); await flush();
    expect(current.selected).toBeNull(); expect(current.elements).toHaveLength(0); expect(current.rootRect).toBeNull();
  });

  it('releases selected references when disabled or when its root is removed', async () => {
    await render(); await act(async () => current.select(first)); await flush();
    expect(current.selected?.element).toBe(first);
    await render({ enabled: false }); expect(current.selected).toBeNull();
    await render(); expect(current.selected).toBeNull();
    await act(async () => current.select(second)); await flush();
    await render({ root: null }); expect(current.selected).toBeNull(); expect(current.elements).toHaveLength(0);
    await render(); expect(current.selected).toBeNull();
  });

  it('hides all dimension labels when dimensions are disabled and clips margin paint', async () => {
    first.style.margin = '20px';
    const a = measureElement(first, product), b = measureElement(second, product);
    await act(async () => reactRoot.render(createElement(AuditOverlay, {
      measurement: a, reference: b, comparison: compareRects(a.rect, b.rect), ownerDocument: document,
      dimensions: false, margin: true, formatValue: value => `${value} px`,
    })));
    const overlay = document.querySelector('[data-layout-audit-ui="overlay"]')!;
    expect(overlay.querySelectorAll('text')).toHaveLength(0);
    const margin = overlay.querySelector('path[fill-rule="evenodd"][clip-path*="-selected-margin"]')!;
    expect(margin.getAttribute('clip-path')).toMatch(/^url\(#.+-selected-margin\)$/);
    const clips = [...overlay.querySelectorAll('clipPath[id$="-selected-margin"] rect')];
    expect(clips).toHaveLength(1); expect(clips[0]?.getAttribute('width')).toBe('380');
  });

  it('disconnects observers, pending frames and capture handlers on unmount', async () => {
    await render({ inspect: true });
    current.refresh(); expect(callbacks.size).toBe(1);
    await act(async () => reactRoot.unmount());
    expect(observers.every(observer => observer.disconnect.mock.calls.length === 1)).toBe(true);
    expect(callbacks.size).toBe(0);
    const action = vi.fn(); first.addEventListener('click', action); first.click(); expect(action).toHaveBeenCalledOnce();
    // Re-create an empty React root so shared cleanup can unmount safely.
    reactRoot = createRoot(host);
  });

  it('reads and picks elements in a same-origin iframe and portals the overlay there', async () => {
    const iframe = document.createElement('iframe'); document.body.append(iframe);
    const doc = iframe.contentDocument!, win = iframe.contentWindow!;
    const frameRoot = doc.createElement('main'), button = doc.createElement('button'); frameRoot.append(button); doc.body.append(frameRoot);
    bounds(frameRoot, 0, 0, 380, 460); bounds(button, 14, 20, 100, 30);
    vi.spyOn(win, 'requestAnimationFrame').mockImplementation(callback => { callbacks.set(++sequence, callback); return sequence; });
    vi.spyOn(win, 'cancelAnimationFrame').mockImplementation(id => { callbacks.delete(id); });
    await render({ root: frameRoot, inspect: true }, true);
    await act(async () => button.click()); await flush();
    expect(current.selected?.element).toBe(button);
    expect(doc.querySelector('[data-layout-audit-ui="overlay"]')).not.toBeNull();
    expect(document.querySelector('[data-layout-audit-ui="overlay"]')).toBeNull();
  });
});
