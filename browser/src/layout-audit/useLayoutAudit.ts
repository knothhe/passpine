import { useCallback, useEffect, useRef, useState } from 'react';
import { collectElements, compareRects, inScope, isExcluded, measureElement, rectOf } from './geometry';
import type { AuditNode, AuditOptions, Comparison, Measurement, Rect, SelectionSlot } from './types';

interface Snapshot {
  selected: Measurement | null;
  reference: Measurement | null;
  comparison: Comparison | null;
  elements: AuditNode[];
  rootRect: Rect | null;
}
const empty: Snapshot = { selected: null, reference: null, comparison: null, elements: [], rootRect: null };
function sameMeasurement(a: Measurement | null, b: Measurement | null): boolean {
  if (a === b) return true;
  if (!a || !b || a.element !== b.element) return false;
  const { element: _a, ...left } = a, { element: _b, ...right } = b;
  return JSON.stringify(left) === JSON.stringify(right);
}
function sameSnapshot(a: Snapshot, b: Snapshot): boolean {
  return sameMeasurement(a.selected, b.selected) && sameMeasurement(a.reference, b.reference)
    && JSON.stringify(a.rootRect) === JSON.stringify(b.rootRect)
    && a.elements.length === b.elements.length
    && a.elements.every((node, index) => {
      const other = b.elements[index];
      return node.element === other?.element && node.selector === other.selector && node.depth === other.depth;
    });
}

/** A DOM-only inspector. No values, labels, text, credentials, or app state are read. */
export function useLayoutAudit({ root, enabled = true, inspect = false, pickSlot = 'selected', portalSelector, excludeSelector }: AuditOptions) {
  const [snapshot, setSnapshot] = useState<Snapshot>(empty);
  const selected = useRef<Element | null>(null), reference = useRef<Element | null>(null);
  const schedule = useRef<() => void>(() => {});
  const input = useRef({ inspect, pickSlot });
  input.current = { inspect, pickSlot };
  const refresh = useCallback(() => schedule.current(), []);
  const select = useCallback((element: Element | null, slot: SelectionSlot = 'selected') => {
    if (!enabled || !root || (element && !inScope(element, root, portalSelector, excludeSelector))) return;
    (slot === 'selected' ? selected : reference).current = element;
    refresh();
  }, [root, enabled, portalSelector, excludeSelector, refresh]);
  const clear = useCallback(() => { selected.current = null; reference.current = null; refresh(); }, [refresh]);
  const clearReference = useCallback(() => { reference.current = null; refresh(); }, [refresh]);
  const selectParent = useCallback(() => {
    if (!root || !selected.current || selected.current === root) return;
    const parent = selected.current.parentElement;
    if (parent && inScope(parent, root, portalSelector, excludeSelector)) select(parent);
  }, [root, portalSelector, excludeSelector, select]);

  useEffect(() => {
    if (!enabled || !root || !root.ownerDocument.defaultView) {
      selected.current = null; reference.current = null;
      setSnapshot(previous => sameSnapshot(previous, empty) ? previous : empty);
      return;
    }
    const document = root.ownerDocument, win = document.defaultView!;
    let active = true, frame = 0;
    const observed = new Set<Element>();
    let measuredAncestors = new Set<Element>();
    const update = () => {
      frame = 0;
      if (!active) return;
      const elements = collectElements(root, portalSelector, excludeSelector);
      const nodes = new Set<Element>([root, ...elements.map(node => node.element)]);
      for (const node of observed) if (!nodes.has(node)) { resize?.unobserve(node); observed.delete(node); }
      for (const node of nodes) if (!observed.has(node)) { resize?.observe(node); observed.add(node); }
      const measurement = (value: Element | null): Measurement | null => value?.isConnected && inScope(value, root, portalSelector, excludeSelector) ? measureElement(value, root) : null;
      const a = measurement(selected.current), b = measurement(reference.current);
      // Removed DOM references are dropped rather than retaining deleted nodes.
      if (!a) selected.current = null;
      if (!b) reference.current = null;
      // Keep prior membership until the next measurement. A portal or exclusion
      // attribute can change before MutationObserver delivers its records.
      const ancestors = new Set<Element>();
      for (const node of [...nodes, selected.current, reference.current]) {
        for (let ancestor = node; ancestor && !ancestors.has(ancestor); ancestor = ancestor.parentElement) ancestors.add(ancestor);
      }
      measuredAncestors = ancestors;
      const next: Snapshot = { selected: a, reference: b, comparison: a && b ? compareRects(a.rect, b.rect) : null, elements, rootRect: root.isConnected ? rectOf(root.getBoundingClientRect()) : null };
      setSnapshot(previous => sameSnapshot(previous, next) ? previous : next);
      // CSS transitions may move an element without resizing it or mutating DOM.
      // Track geometry animations while they run; a decorative spinner alone
      // must not keep the whole inspector measuring at frame rate.
      const moving = document.getAnimations?.().some(animation => {
        if (animation.playState !== 'running' || typeof win.KeyframeEffect !== 'function' || !(animation.effect instanceof win.KeyframeEffect)) return false;
        const target = animation.effect.target;
        return target instanceof win.Element && inScope(target, root, portalSelector, excludeSelector)
          && (target === root || (!!selected.current && target.contains(selected.current)) || (!!reference.current && target.contains(reference.current)))
          && animation.effect.getKeyframes().some(keyframe => Object.keys(keyframe).some(key => /^(transform|translate|rotate|scale|left|top|right|bottom|width|height|padding|margin|fontSize|lineHeight|gap)/.test(key)));
      });
      if (moving) request();
    };
    const request = () => { if (active && !frame) frame = win.requestAnimationFrame(update); };
    schedule.current = request;
    const resize = typeof win.ResizeObserver === 'function' ? new win.ResizeObserver(request) : null;
    const containsPortal = (element: Element) => !!(portalSelector && (element.matches(portalSelector) || element.querySelector(portalSelector)));
    const mutations = new win.MutationObserver(records => {
      if (records.some(record => {
        const target = record.target.nodeType === 1 ? record.target as Element : record.target.parentElement;
        const wasMeasured = !!target && measuredAncestors.has(target);
        if (target && isExcluded(target, excludeSelector) && !wasMeasured) return false;
        const changedNodes = [...record.addedNodes, ...record.removedNodes];
        // Overlay and toolbar updates must not cause measurement feedback. A
        // formerly measured subtree becoming excluded still needs one refresh.
        if (record.type === 'childList' && changedNodes.every(node => node.nodeType === 1 && isExcluded(node as Element, excludeSelector) && !measuredAncestors.has(node as Element))) return false;
        return !target || wasMeasured || target.contains(root) || inScope(target, root, portalSelector, excludeSelector) || containsPortal(target)
          || changedNodes.some(node => node.nodeType === 1 && (measuredAncestors.has(node as Element) || containsPortal(node as Element)));
      })) request();
    });
    // Ancestor styles can change geometry without a resize. Observe the document
    // so reparenting, scope changes and root removal are covered as well; the
    // record filter restricts refreshes to current or previously measured scope.
    mutations.observe(document.documentElement, { subtree: true, childList: true, attributes: true, characterData: true });
    const eventElement = (event: Event): Element | null => {
      for (const item of event.composedPath()) if (item instanceof win.Element) return item;
      return event.target instanceof win.Element ? event.target : null;
    };
    const ownedTarget = (event: Event): Element | null => {
      const element = eventElement(event);
      return input.current.inspect && element && inScope(element, root, portalSelector, excludeSelector) ? element : null;
    };
    const cancel = (event: Event) => { event.preventDefault(); event.stopImmediatePropagation(); };
    const pointer = (event: Event) => {
      const element = ownedTarget(event);
      if (!element) return;
      cancel(event);
      (input.current.pickSlot === 'reference' ? reference : selected).current = element;
      request();
    };
    const activation = (event: Event) => { if (ownedTarget(event)) cancel(event); };
    const keyboard = (event: KeyboardEvent) => {
      if (!ownedTarget(event) || event.key === 'Tab' || event.key === 'Escape' || (event.altKey && (event.ctrlKey || event.metaKey))) return;
      cancel(event);
      if (event.type === 'keydown' && (event.key === 'Enter' || event.key === ' ')) {
        const element = eventElement(event);
        (input.current.pickSlot === 'reference' ? reference : selected).current = element;
        request();
      }
    };
    document.addEventListener('pointerdown', pointer, true);
    document.addEventListener('click', pointer, true);
    document.addEventListener('pointerup', activation, true);
    document.addEventListener('mousedown', activation, true);
    document.addEventListener('mouseup', activation, true);
    document.addEventListener('dblclick', activation, true);
    document.addEventListener('auxclick', activation, true);
    document.addEventListener('contextmenu', activation, true);
    document.addEventListener('beforeinput', activation, true);
    win.addEventListener('keydown', keyboard, true);
    win.addEventListener('keyup', keyboard, true);
    document.addEventListener('scroll', request, true);
    document.addEventListener('transitionrun', request, true);
    document.addEventListener('animationstart', request, true);
    document.addEventListener('transitionend', request, true);
    document.addEventListener('animationend', request, true);
    win.addEventListener('resize', request);
    win.visualViewport?.addEventListener('resize', request);
    win.visualViewport?.addEventListener('scroll', request);
    void document.fonts?.ready.then(request);
    request();
    return () => {
      active = false; schedule.current = () => {};
      win.cancelAnimationFrame(frame); resize?.disconnect(); mutations.disconnect();
      document.removeEventListener('pointerdown', pointer, true);
      document.removeEventListener('click', pointer, true);
      document.removeEventListener('pointerup', activation, true);
      document.removeEventListener('mousedown', activation, true);
      document.removeEventListener('mouseup', activation, true);
      document.removeEventListener('dblclick', activation, true);
      document.removeEventListener('auxclick', activation, true);
      document.removeEventListener('contextmenu', activation, true);
      document.removeEventListener('beforeinput', activation, true);
      win.removeEventListener('keydown', keyboard, true);
      win.removeEventListener('keyup', keyboard, true);
      document.removeEventListener('scroll', request, true);
      document.removeEventListener('transitionrun', request, true);
      document.removeEventListener('animationstart', request, true);
      document.removeEventListener('transitionend', request, true);
      document.removeEventListener('animationend', request, true);
      win.removeEventListener('resize', request);
      win.visualViewport?.removeEventListener('resize', request);
      win.visualViewport?.removeEventListener('scroll', request);
    };
  }, [root, enabled, portalSelector, excludeSelector]);

  return { ...snapshot, select, clear, clearReference, selectParent, refresh };
}
