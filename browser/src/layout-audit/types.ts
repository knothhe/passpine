export interface Rect { x: number; y: number; width: number; height: number }
export interface Insets { top: number; right: number; bottom: number; left: number }
export interface FontMetrics { size: number; lineHeight: string; weight: string; family: string; letterSpacing: string }
export interface Measurement {
  element: Element;
  selector: string;
  /** Axis-aligned bounds in viewport CSS pixels, including CSS transforms. */
  rect: Rect;
  /** Bounds relative to the root, in the root's unscaled CSS coordinates. */
  localRect: Rect;
  visibleRect: Rect | null;
  /** Viewport and ancestor clipping region, also used for margin overlays. */
  clipRect: Rect | null;
  rootRect: Rect;
  padding: Insets;
  margin: Insets;
  border: Insets;
  gap: { row: string; column: string };
  font: FontMetrics;
  display: string;
  position: string;
  scale: { x: number; y: number };
  clipped: boolean;
}
export interface AuditNode { element: Element; selector: string; depth: number }
export interface Comparison {
  horizontalGap: number;
  verticalGap: number;
  overlapX: number;
  overlapY: number;
  /** Signed offsets from the selected element to the reference element. */
  deltaLeft: number;
  deltaTop: number;
  deltaRight: number;
  deltaBottom: number;
  deltaCenterX: number;
  deltaCenterY: number;
  distance: number;
}
export type SelectionSlot = 'selected' | 'reference';
export interface AuditOptions {
  root: HTMLElement | null;
  enabled?: boolean;
  inspect?: boolean;
  pickSlot?: SelectionSlot;
  /** Opt in to specific portal roots owned by this UI, never a whole document. */
  portalSelector?: string;
  /** Inspector UI always uses data-layout-audit-ui and is excluded automatically. */
  excludeSelector?: string;
}
