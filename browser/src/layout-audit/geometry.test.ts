// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest';
import { collectElements, compareRects, describeElement, inScope, measureElement, visibleRect } from './geometry';

function bounds(element: Element, x: number, y: number, width: number, height: number, layoutWidth = width, layoutHeight = height) {
  vi.spyOn(element, 'getBoundingClientRect').mockReturnValue({ x, y, width, height, left: x, top: y, right: x + width, bottom: y + height, toJSON: () => ({}) });
  Object.defineProperties(element, { offsetWidth: { configurable: true, value: layoutWidth }, offsetHeight: { configurable: true, value: layoutHeight } });
}
function fixture() {
  const root = document.createElement('main'), scroll = document.createElement('section'), child = document.createElement('button');
  root.append(scroll); scroll.append(child); document.body.append(root);
  bounds(root, 10, 20, 380, 460); bounds(scroll, 20, 100, 350, 260); bounds(child, 30, 110, 120, 40);
  return { root, scroll, child };
}
afterEach(() => { document.body.replaceChildren(); vi.restoreAllMocks(); });

describe('DOM audit geometry', () => {
  it('reports actual box model and font rules without using text or input values', () => {
    const { root, child } = fixture();
    child.className = 'sample'; child.textContent = 'private account name';
    child.style.cssText = 'padding: 4px 8px; margin: 2px 6px; border: 1px solid; font-size: 13px; line-height: 18px; font-weight: 600;';
    const value = measureElement(child, root);
    expect(value.rect).toEqual({ x: 30, y: 110, width: 120, height: 40 });
    expect(value.localRect).toEqual({ x: 20, y: 90, width: 120, height: 40 });
    expect(value.padding).toEqual({ top: 4, right: 8, bottom: 4, left: 8 });
    expect(value.margin).toEqual({ top: 2, right: 6, bottom: 2, left: 6 });
    expect(value.font).toMatchObject({ size: 13, lineHeight: '18px', weight: '600' });
    expect(describeElement(child)).toBe('button.sample');
    expect(JSON.stringify(collectElements(root).map(({ selector }) => selector))).not.toContain('private account');
  });

  it('keeps full bounds while reporting the part clipped by a scrolled container', () => {
    const { root, scroll, child } = fixture();
    scroll.style.overflowX = 'auto'; scroll.style.overflowY = 'auto';
    bounds(child, 30, 80, 120, 40);
    const value = measureElement(child, root);
    expect(value.rect).toEqual({ x: 30, y: 80, width: 120, height: 40 });
    expect(value.visibleRect).toEqual({ x: 30, y: 100, width: 120, height: 20 });
    expect(value.clipped).toBe(true);
    bounds(child, 30, 50, 120, 40);
    expect(visibleRect(child, root)).toBeNull();
    expect(collectElements(root).some(node => node.element === child)).toBe(false);
  });

  it('excludes scrollport borders and scrollbars when clipping', () => {
    const { root, scroll, child } = fixture();
    scroll.style.overflowX = 'auto'; scroll.style.overflowY = 'auto';
    Object.defineProperties(scroll, { clientLeft: { configurable: true, value: 2 }, clientTop: { configurable: true, value: 2 }, clientWidth: { configurable: true, value: 330 }, clientHeight: { configurable: true, value: 250 } });
    bounds(child, 340, 340, 40, 40);
    expect(visibleRect(child, root)).toEqual({ x: 340, y: 340, width: 12, height: 12 });
  });

  it('converts root-relative coordinates when an ancestor uses axis-aligned scaling', () => {
    const { root, child } = fixture();
    bounds(root, 10, 20, 760, 460, 380, 230);
    bounds(child, 50, 100, 240, 80, 120, 40);
    const value = measureElement(child, root);
    expect(value.scale).toEqual({ x: 2, y: 2 });
    expect(value.localRect).toEqual({ x: 20, y: 40, width: 120, height: 40 });
    expect(value.rect.width).toBe(240);
  });

  it('allows only opted-in portals and excludes tool UI even inside a portal', () => {
    const { root } = fixture();
    const portal = document.createElement('section'), item = document.createElement('button');
    portal.dataset.auditPortal = ''; portal.append(item); document.body.append(portal);
    bounds(portal, 500, 100, 200, 300); bounds(item, 510, 110, 150, 30);
    expect(inScope(item, root)).toBe(false);
    expect(inScope(item, root, '[data-audit-portal]')).toBe(true);
    expect(measureElement(item, root).visibleRect).toEqual({ x: 510, y: 110, width: 150, height: 30 });
    item.dataset.layoutAuditUi = '';
    expect(inScope(item, root, '[data-audit-portal]')).toBe(false);
    expect(collectElements(root, '[data-audit-portal]').some(node => node.element === item)).toBe(false);
  });

  it('uses the target iframe window for styles and element constructors', () => {
    const iframe = document.createElement('iframe'); document.body.append(iframe);
    const doc = iframe.contentDocument!, win = iframe.contentWindow!;
    const root = doc.createElement('main'), item = doc.createElement('input'); root.append(item); doc.body.append(root);
    bounds(root, 0, 0, 380, 460); bounds(item, 14, 20, 352, 34);
    item.style.fontSize = '17px'; item.value = 'do not inspect this value';
    const spy = vi.spyOn(win, 'getComputedStyle');
    expect(measureElement(item, root).font.size).toBe(17);
    expect(spy).toHaveBeenCalledWith(item);
    expect(collectElements(root).some(node => node.element === item)).toBe(true);
    item.dataset.auditPortal = '';
    const foreignRoot = document.createElement('main'); document.body.append(foreignRoot);
    expect(inScope(item, foreignRoot, '[data-audit-portal]')).toBe(false);
  });

  it('ignores inert and invisible elements without excluding aria-hidden decorative icons', () => {
    const { root, child } = fixture();
    child.setAttribute('inert', ''); expect(visibleRect(child, root)).toBeNull();
    child.removeAttribute('inert'); child.style.opacity = '0'; expect(visibleRect(child, root)).toBeNull();
    child.style.opacity = '1'; child.setAttribute('aria-hidden', 'true'); expect(visibleRect(child, root)).not.toBeNull();
  });
});

describe('comparison geometry', () => {
  it('measures vertical spacing, overlap and signed alignment independently', () => {
    const a = { x: 14, y: 20, width: 100, height: 30 }, b = { x: 16, y: 56, width: 100, height: 40 };
    expect(compareRects(a, b)).toEqual({ horizontalGap: 0, verticalGap: 6, overlapX: 98, overlapY: 0, deltaLeft: 2, deltaTop: 36, deltaRight: 2, deltaBottom: 46, deltaCenterX: 2, deltaCenterY: 41, distance: 6 });
    expect(compareRects(b, a).deltaLeft).toBe(-2);
    expect(compareRects(b, a).verticalGap).toBe(6);
  });
  it('does not call overlap a negative gap and measures diagonal distance', () => {
    expect(compareRects({ x: 0, y: 0, width: 50, height: 50 }, { x: 10, y: 10, width: 10, height: 10 })).toMatchObject({ horizontalGap: 0, verticalGap: 0, overlapX: 10, overlapY: 10 });
    expect(compareRects({ x: 0, y: 0, width: 10, height: 10 }, { x: 13, y: 14, width: 10, height: 10 })).toMatchObject({ horizontalGap: 3, verticalGap: 4, distance: 5 });
  });
});
