# DOM layout audit component

This folder contains a reusable TypeScript and React inspector. It depends on React and React DOM, and has no Passpine, WXT, localization, credential, or preview-client dependency. Copy this folder into another React application and provide its control UI and localized numeric formatter.

## Integration

```tsx
import { useState } from 'react';
import { AuditOverlay, useLayoutAudit } from './layout-audit';

function Example({ copy, children }) {
  const [root, setRoot] = useState<HTMLDivElement | null>(null);
  const [inspect, setInspect] = useState(false);
  const audit = useLayoutAudit({
    root,
    enabled: true,
    inspect,
    pickSlot: 'selected',
    portalSelector: '[data-radix-popper-content-wrapper], [role="dialog"]',
  });
  return <>
    <div ref={setRoot}>{children}</div>
    <aside data-layout-audit-ui>
      <button onClick={() => setInspect(!inspect)}>{copy.inspect}</button>
      <button onClick={() => audit.selectParent()}>{copy.parent}</button>
    </aside>
    <AuditOverlay
      measurement={audit.selected}
      reference={audit.reference}
      comparison={audit.comparison}
      rootRect={audit.rootRect}
      grid={8}
      formatValue={copy.pixels}
    />
  </>;
}
```

The host owns the enabled and inspect switches. Set `enabled: false` to detach observers and input capture and release selections. With `inspect: false`, normal application interactions pass through. In inspect mode, pointerdown and click are captured before component handlers, keyboard activation and editing are intercepted, and Tab remains available. Escape and the modifier combination Control/Command + Alt pass to the host so it can dismiss or toggle the inspector. The host should handle Escape before its application shortcuts when needed.

Use `pickSlot: 'reference'` for the second element. `select(element, slot)`, `clearReference()`, `clear()`, and `selectParent()` also support a component tree or keyboard-driven toolbar. `elements` contains visible element references, structural selectors and depth, never labels, input values or text content. The host is responsible for protecting any exported screenshot or separately gathered application data.

Place `data-layout-audit-ui` on all tool controls. Those subtrees are excluded from picking, inventory, and mutation feedback. `excludeSelector` provides additional host exclusions. Portal support is opt-in through `portalSelector`; the selector should identify only portals owned by the inspected application.

## Frames and coordinates

For a same-origin iframe, pass an element from `iframe.contentDocument` as `root` and pass that document as `AuditOverlay.ownerDocument`. Capture events, styles, animations, observers and viewport geometry use the target document's window. A host can use iframe width presets to test the real media-query viewport. Cross-origin documents are not accessible.

- `rect` is the complete axis-aligned rendered border box in viewport CSS pixels, including transforms.
- `visibleRect` is the intersection with the viewport, chosen root and overflow-clipping ancestors. A partly scrolled element retains both its complete size and its visible bounds.
- `clipRect` is the ancestor clipping region used for margin paint.
- `localRect` is relative to the root in its unscaled coordinates. Axis-aligned scaling is handled separately from screen coordinates.
- Padding, margin, border and font metrics are computed CSS values. `scale` describes how the element's layout box maps to its rendered bounds.
- Comparison gaps are nonnegative distances between border boxes. Signed deltas describe reference minus selected edge or center alignment. Overlapping boxes have zero gap and an independent overlap value.

The overlay renders in a portal with fixed positioning and `pointer-events: none`. It does not wrap or resize the application, and all controls remain host-owned. A grid, box-model paint, alignment guides and dimensions can be enabled independently. Numeric labels are formatted by the host.

## Measurement boundaries

Measurements describe DOM geometry, not aesthetic quality or WCAG conformance. A container's inherited font does not prove the font of a nested text node. Rotated or skewed boxes are represented by axis-aligned bounding rectangles; box-model shading is an approximation in those cases. Clipping covers rectangular overflow scrollports, not arbitrary clip paths, rounded corners, stacking occlusion or shadow DOM traversal. Canvas drawing and native widgets require their own adapters.

Mutation, size, scroll, font-ready and animation events refresh measurements. Selected elements affected by geometry animations are sampled while those animations run. Animation of unrelated decorative elements does not force continuous full-tree measurement. Removed elements are dropped and disabling the hook releases stored element references.

## Validation

`geometry.test.ts` verifies box model, scale conversion, clipped scrollports, portal boundaries, same-origin frames and independent gap/alignment measures. `useLayoutAudit.test.ts` verifies event interception/pass-through, A/B selection, live style updates, retained draft inputs, observer teardown, disabled cleanup, iframe picking and overlay portal placement.
