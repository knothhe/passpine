import type { AuditNode, Comparison, Insets, Measurement, Rect } from './types';

export const round = (value: number): number => Math.round(value * 100) / 100;
export const right = (rect: Rect): number => rect.x + rect.width;
export const bottom = (rect: Rect): number => rect.y + rect.height;
export function rectOf(value: Pick<DOMRect, 'x' | 'y' | 'width' | 'height'>): Rect {
  return { x: round(value.x), y: round(value.y), width: round(value.width), height: round(value.height) };
}
export function intersect(a: Rect, b: Rect): Rect | null {
  const x = Math.max(a.x, b.x), y = Math.max(a.y, b.y);
  const width = Math.min(right(a), right(b)) - x, height = Math.min(bottom(a), bottom(b)) - y;
  return width > 0 && height > 0 ? { x: round(x), y: round(y), width: round(width), height: round(height) } : null;
}
export function compareRects(a: Rect, b: Rect): Comparison {
  const horizontalGap = Math.max(0, b.x - right(a), a.x - right(b));
  const verticalGap = Math.max(0, b.y - bottom(a), a.y - bottom(b));
  return {
    horizontalGap: round(horizontalGap), verticalGap: round(verticalGap),
    overlapX: round(Math.max(0, Math.min(right(a), right(b)) - Math.max(a.x, b.x))),
    overlapY: round(Math.max(0, Math.min(bottom(a), bottom(b)) - Math.max(a.y, b.y))),
    deltaLeft: round(b.x - a.x), deltaTop: round(b.y - a.y),
    deltaRight: round(right(b) - right(a)), deltaBottom: round(bottom(b) - bottom(a)),
    deltaCenterX: round(b.x + b.width / 2 - a.x - a.width / 2),
    deltaCenterY: round(b.y + b.height / 2 - a.y - a.height / 2),
    distance: round(Math.hypot(horizontalGap, verticalGap)),
  };
}

function view(element: Element): Window & typeof globalThis { return element.ownerDocument.defaultView ?? window; }
function numeric(value: string): number { const parsed = parseFloat(value); return Number.isFinite(parsed) ? parsed : 0; }
function insets(css: CSSStyleDeclaration, prefix: 'padding' | 'margin' | 'border'): Insets {
  const suffix = prefix === 'border' ? '-width' : '';
  return Object.fromEntries(['top', 'right', 'bottom', 'left'].map(side => [side, numeric(css.getPropertyValue(`${prefix}-${side}${suffix}`))])) as unknown as Insets;
}
function scaleOf(element: Element, rect: Rect): { x: number; y: number } {
  if (!(element instanceof view(element).HTMLElement)) return { x: 1, y: 1 };
  const html = element as HTMLElement;
  return { x: html.offsetWidth ? rect.width / html.offsetWidth : 1, y: html.offsetHeight ? rect.height / html.offsetHeight : 1 };
}

/** Names deliberately contain no input value, title, accessible label, or text. */
export function describeElement(element: Element): string {
  const tag = element.tagName.toLowerCase();
  const id = element.id ? `#${element.id}` : '';
  const classes = Array.from(element.classList).slice(0, 4).map(name => `.${name}`).join('');
  return `${tag}${id}${classes}`;
}

/** Geometric clipping only. This does not attempt to infer stacking/occlusion. */
export function clippingRect(element: Element, root?: HTMLElement): Rect | null {
  const win = view(element), document = element.ownerDocument;
  if (!element.isConnected || element.closest('[hidden], [inert]')) return null;
  let visible: Rect | null = {
    x: 0, y: 0, width: document.documentElement.clientWidth || win.innerWidth,
    height: document.documentElement.clientHeight || win.innerHeight,
  };
  // A portal has its own ancestors and must not be clipped against an unrelated root.
  if (root && root !== element && root.contains(element) && visible) visible = intersect(visible, rectOf(root.getBoundingClientRect()));
  for (let node: Element | null = element; node && visible; node = node.parentElement) {
    const css = win.getComputedStyle(node);
    if (css.display === 'none' || css.visibility === 'hidden' || css.visibility === 'collapse' || css.opacity === '0') return null;
    if (node === element) continue;
    const rect = rectOf(node.getBoundingClientRect());
    const clipX = /(hidden|clip|auto|scroll)/.test(css.overflowX || css.overflow);
    const clipY = /(hidden|clip|auto|scroll)/.test(css.overflowY || css.overflow);
    // The border does not belong to the scrollport. Client sizes exclude it and scrollbars.
    const html = node as HTMLElement, scale = scaleOf(node, rect);
    const x = rect.x + (html.clientLeft || 0) * scale.x, y = rect.y + (html.clientTop || 0) * scale.y;
    const width = html.clientWidth ? html.clientWidth * scale.x : rect.width;
    const height = html.clientHeight ? html.clientHeight * scale.y : rect.height;
    if (clipX || clipY) visible = intersect(visible, {
      x: clipX ? x : visible.x, y: clipY ? y : visible.y,
      width: clipX ? width : visible.width, height: clipY ? height : visible.height,
    });
  }
  return visible;
}

export function visibleRect(element: Element, root?: HTMLElement): Rect | null {
  const clip = clippingRect(element, root);
  return clip ? intersect(rectOf(element.getBoundingClientRect()), clip) : null;
}

export function measureElement(element: Element, root: HTMLElement): Measurement {
  const css = view(element).getComputedStyle(element), rect = rectOf(element.getBoundingClientRect());
  const rootRect = rectOf(root.getBoundingClientRect()), rootScale = scaleOf(root, rootRect), visible = visibleRect(element, root);
  return {
    element, selector: describeElement(element), rect, rootRect,
    localRect: { x: round((rect.x - rootRect.x) / rootScale.x), y: round((rect.y - rootRect.y) / rootScale.y), width: round(rect.width / rootScale.x), height: round(rect.height / rootScale.y) },
    visibleRect: visible, clipRect: clippingRect(element, root), scale: scaleOf(element, rect),
    padding: insets(css, 'padding'), margin: insets(css, 'margin'), border: insets(css, 'border'),
    gap: { row: css.rowGap, column: css.columnGap },
    font: { size: numeric(css.fontSize), lineHeight: css.lineHeight, weight: css.fontWeight, family: css.fontFamily, letterSpacing: css.letterSpacing },
    display: css.display, position: css.position,
    clipped: !visible || Math.abs(visible.x - rect.x) > .1 || Math.abs(visible.y - rect.y) > .1 || Math.abs(visible.width - rect.width) > .1 || Math.abs(visible.height - rect.height) > .1,
  };
}

export const internalSelector = '[data-layout-audit-ui]';
export function isExcluded(element: Element, excludeSelector?: string): boolean {
  return !!element.closest(internalSelector) || !!(excludeSelector && element.closest(excludeSelector));
}
export function inScope(element: Element, root: HTMLElement, portalSelector?: string, excludeSelector?: string): boolean {
  if (element.ownerDocument !== root.ownerDocument || isExcluded(element, excludeSelector)) return false;
  return root === element || root.contains(element) || !!(portalSelector && element.closest(portalSelector));
}
export function collectElements(root: HTMLElement, portalSelector?: string, excludeSelector?: string): AuditNode[] {
  const surfaces: Element[] = [root, ...(portalSelector ? root.ownerDocument.querySelectorAll(portalSelector) : [])];
  const seen = new Set<Element>(), result: AuditNode[] = [];
  for (const surface of surfaces) {
    for (const element of [surface, ...surface.querySelectorAll('*')]) {
      if (seen.has(element) || isExcluded(element, excludeSelector) || ['script', 'style', 'link', 'meta', 'title'].includes(element.tagName.toLowerCase())) continue;
      seen.add(element);
      if (!visibleRect(element, root)) continue;
      let depth = 0;
      for (let parent = element.parentElement; parent && parent !== surface; parent = parent.parentElement) depth++;
      result.push({ element, selector: describeElement(element), depth: element === surface ? 0 : depth + 1 });
    }
  }
  return result;
}
