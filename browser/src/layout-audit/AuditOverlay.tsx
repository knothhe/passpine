import { useEffect, useId, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { bottom, intersect, right, round } from './geometry';
import type { Comparison, Measurement, Rect } from './types';

export interface AuditOverlayProps {
  measurement?: Measurement | null;
  reference?: Measurement | null;
  comparison?: Comparison | null;
  rootRect?: Rect | null;
  ownerDocument?: Document;
  grid?: number | false;
  dimensions?: boolean;
  alignment?: boolean;
  padding?: boolean;
  margin?: boolean;
  /** Localize numeric units in the host. No copy or app state lives here. */
  formatValue(value: number): string;
  colors?: Partial<{ selected: string; reference: string; padding: string; margin: string; labelBackground: string }>;
}
const defaults = { selected: '#bd3d73', reference: '#167f72', padding: '#167f7229', margin: '#d59e3426', labelBackground: '#fff' };

/** Non-interactive overlay in the inspected document, including same-origin frames. */
export function AuditOverlay({ measurement, reference, comparison, rootRect, ownerDocument, grid = false, dimensions = true, alignment = true, padding = true, margin = false, formatValue, colors }: AuditOverlayProps) {
  const doc = ownerDocument ?? measurement?.element.ownerDocument ?? reference?.element.ownerDocument ?? (typeof document === 'undefined' ? undefined : document);
  const win = doc?.defaultView;
  const [viewport, setViewport] = useState({ width: win?.innerWidth ?? 0, height: win?.innerHeight ?? 0 });
  useEffect(() => {
    if (!win) return;
    const update = () => setViewport({ width: win.innerWidth, height: win.innerHeight });
    update(); win.addEventListener('resize', update); win.visualViewport?.addEventListener('resize', update);
    return () => { win.removeEventListener('resize', update); win.visualViewport?.removeEventListener('resize', update); };
  }, [win]);
  const id = useId().replaceAll(':', ''), color = { ...defaults, ...colors };
  if (!doc?.body || !viewport.width || !viewport.height) return null;
  const screen: Rect = { x: 0, y: 0, width: viewport.width, height: viewport.height };
  const label = (x: number, y: number, value: number, stroke: string) => {
    const text = formatValue(round(value)), width = Math.max(28, text.length * 6.6 + 12);
    const left = Math.max(2, Math.min(viewport.width - width - 2, x - width / 2));
    const top = Math.max(2, Math.min(viewport.height - 22, y - 10));
    return <g transform={`translate(${left},${top})`}><rect width={width} height={20} rx={4} fill={color.labelBackground} stroke={stroke} strokeWidth={.7}/><text x={width / 2} y={10} dominantBaseline="central" textAnchor="middle" fill={stroke} style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace', fontSize: 10, fontWeight: 500 }}>{text}</text></g>;
  };
  const draw = (value: Measurement, stroke: string, suffix: string): ReactNode => {
    if (!value.visibleRect) return null;
    const r = value.rect, visible = intersect(value.visibleRect, screen);
    if (!visible) return null;
    const p = value.padding, b = value.border, m = value.margin, scale = value.scale;
    const inner: Rect = { x: r.x + b.left * scale.x, y: r.y + b.top * scale.y, width: Math.max(0, r.width - (b.left + b.right) * scale.x), height: Math.max(0, r.height - (b.top + b.bottom) * scale.y) };
    const top = Math.min(inner.height, p.top * scale.y), bottomPad = Math.min(inner.height, p.bottom * scale.y);
    const left = Math.min(inner.width, p.left * scale.x), rightPad = Math.min(inner.width, p.right * scale.x);
    const widthY = visible.y > 25 ? visible.y - 14 : bottom(visible) + 14;
    const heightX = right(visible) + 26 < viewport.width ? right(visible) + 14 : visible.x - 14;
    return <g key={suffix}>
      <defs><clipPath id={`${id}-${suffix}`}><rect x={visible.x} y={visible.y} width={visible.width} height={visible.height}/></clipPath><clipPath id={`${id}-${suffix}-margin`}>{value.clipRect && <rect x={value.clipRect.x} y={value.clipRect.y} width={value.clipRect.width} height={value.clipRect.height}/>}</clipPath></defs>
      {margin && <path clipPath={`url(#${id}-${suffix}-margin)`} d={`M${r.x - Math.max(0, m.left) * scale.x},${r.y - Math.max(0, m.top) * scale.y} h${r.width + (Math.max(0, m.left) + Math.max(0, m.right)) * scale.x} v${r.height + (Math.max(0, m.top) + Math.max(0, m.bottom)) * scale.y} h${-r.width - (Math.max(0, m.left) + Math.max(0, m.right)) * scale.x} Z M${r.x},${r.y} v${r.height} h${r.width} v${-r.height} Z`} fill={color.margin} fillRule="evenodd"/>}
      <g clipPath={`url(#${id}-${suffix})`}>
        {padding && <g fill={color.padding}><rect x={inner.x} y={inner.y} width={inner.width} height={top}/><rect x={inner.x} y={bottom(inner) - bottomPad} width={inner.width} height={bottomPad}/><rect x={inner.x} y={inner.y + top} width={left} height={Math.max(0, inner.height - top - bottomPad)}/><rect x={right(inner) - rightPad} y={inner.y + top} width={rightPad} height={Math.max(0, inner.height - top - bottomPad)}/></g>}
        <rect x={r.x} y={r.y} width={r.width} height={r.height} fill="none" stroke={stroke} strokeWidth={1.25}/>
      </g>
      {value.clipped && <rect x={visible.x} y={visible.y} width={visible.width} height={visible.height} fill="none" stroke={stroke} strokeDasharray="3 3"/>}
      {alignment && <g stroke={stroke} strokeWidth={.7} strokeDasharray="3 4" opacity={.55}><path d={`M${r.x},0 V${viewport.height} M${right(r)},0 V${viewport.height} M0,${r.y} H${viewport.width} M0,${bottom(r)} H${viewport.width}`}/></g>}
      {dimensions && <><g fill="none" stroke={stroke} strokeWidth={1}><path d={`M${visible.x},${widthY} H${right(visible)} M${visible.x},${widthY - 4} v8 M${right(visible)},${widthY - 4} v8`}/><path d={`M${heightX},${visible.y} V${bottom(visible)} M${heightX - 4},${visible.y} h8 M${heightX - 4},${bottom(visible)} h8`}/></g>{label(visible.x + visible.width / 2, widthY, r.width, stroke)}{label(heightX, visible.y + visible.height / 2, r.height, stroke)}</>}
    </g>;
  };
  const pair = (): ReactNode => {
    if (!measurement?.visibleRect || !reference?.visibleRect || !comparison) return null;
    const a = measurement.rect, b = reference.rect;
    const x1 = right(a) <= b.x ? right(a) : right(b) <= a.x ? right(b) : Math.max(a.x, b.x);
    const x2 = right(a) <= b.x ? b.x : right(b) <= a.x ? a.x : x1;
    const y1 = bottom(a) <= b.y ? bottom(a) : bottom(b) <= a.y ? bottom(b) : Math.max(a.y, b.y);
    const y2 = bottom(a) <= b.y ? b.y : bottom(b) <= a.y ? a.y : y1;
    const horizontalY = Math.max(12, Math.min(viewport.height - 12, Math.max(a.y, b.y) + comparison.overlapY / 2));
    const verticalX = Math.max(12, Math.min(viewport.width - 12, Math.max(a.x, b.x) + comparison.overlapX / 2));
    return <g stroke={color.selected} fill="none" strokeWidth={1}>
      {comparison.horizontalGap > 0 && <><path d={`M${x1},${horizontalY} H${x2} M${x1},${horizontalY - 4} v8 M${x2},${horizontalY - 4} v8`}/>{label((x1 + x2) / 2, horizontalY, comparison.horizontalGap, color.selected)}</>}
      {comparison.verticalGap > 0 && <><path d={`M${verticalX},${y1} V${y2} M${verticalX - 4},${y1} h8 M${verticalX - 4},${y2} h8`}/>{label(verticalX, (y1 + y2) / 2, comparison.verticalGap, color.selected)}</>}
    </g>;
  };
  const gridRect = rootRect && intersect(rootRect, screen);
  return createPortal(<svg data-layout-audit-ui="overlay" aria-hidden="true" focusable="false" width="100%" height="100%" style={{ position: 'fixed', inset: 0, zIndex: 2147483647, pointerEvents: 'none', overflow: 'hidden' }}>
    {grid && grid > 0 && gridRect && <><defs><pattern id={`${id}-grid`} x={rootRect!.x} y={rootRect!.y} width={grid} height={grid} patternUnits="userSpaceOnUse"><path d={`M${grid},0 H0 V${grid}`} fill="none" stroke={color.reference} strokeWidth={.5} opacity={.25}/></pattern></defs><rect x={gridRect.x} y={gridRect.y} width={gridRect.width} height={gridRect.height} fill={`url(#${id}-grid)`}/></>}
    {reference && draw(reference, color.reference, 'reference')}{measurement && draw(measurement, color.selected, 'selected')}{dimensions && pair()}
  </svg>, doc.body);
}
