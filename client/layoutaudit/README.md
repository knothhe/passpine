# MyGo layout audit

`layoutaudit` is a reusable Go package. Its only non-standard-library import is
`github.com/egoist/mygo/ui`. It has no dependency on Passpine state, vaults,
translations, Onevra components, private MyGo internals, or reflection.

The adapter deliberately combines two tools:

| Tool | Responsibility |
| --- | --- |
| `layoutaudit.State.Wrap` | Actual subtree bounds through MyGo's public `Root().Debug()`, a configurable logical grid, and manual two-point horizontal/vertical rulers. |
| MyGo native inspector | Rendered element tree, element picking, actual content/padding/border/margin highlight, computed dimensions and typography, source location, accessibility properties. |

The wrapper does not infer component gaps or computed font styles from manual
points. Those values belong to the native inspector, which can access the real
layout tree. MyGo 0.5.5 does not expose that tree as a public traversal API.

## Reuse

Keep one `State` per native window and mutate it on the UI thread. Pass your own
localized labels; `Options.Labels` is called on every frame, so changing language
does not recreate the view or discard draft inputs.

```go
import (
    "github.com/egoist/mygo"
    "github.com/egoist/mygo/ui"
    "github.com/knothhe/passpine/client/layoutaudit"
)

var audit = layoutaudit.State{Enabled: true, Outlines: true}

func windowOptions(view func(*ui.Context), labels func() layoutaudit.Labels) mygo.WindowOptions {
    return mygo.WindowOptions{
        Width: 1120, Height: 760,
        Page: mygo.PageOptions{DevTools: mygo.DevToolsEnabled},
        Content: ui.View(audit.Wrap(view, layoutaudit.Options{
            Labels: labels,
            GridStep: 8,
            TopInset: 56,
        })),
    }
}
```

Supply all strings and the `Viewport`/`Distance` formatting callbacks in
`Labels` from your app's catalog. Passpine's integration is in
[`desktop/layout_audit.go`](../desktop/layout_audit.go), with copy in
[`i18n/layout_audit.go`](../i18n/layout_audit.go).

The zero state is disabled and adds no elements. The wrapper calls the original
view with its original context, without adding a layout parent. Grid and bounds
pass pointer events through; enabling the ruler captures clicks, and Escape
exits ruler mode before the application's own Escape handler. The ruler measures
window logical units (DIPs), with one state per viewport. Resize clears points to
prevent stale measurements. `Measurement()` exposes a completed measurement for
host tooling.

## Native inspector

Set `Page.DevTools` to `mygo.DevToolsEnabled`. F12 toggles the native inspector;
macOS also supports Command+Option+I and other platforms Control+Shift+I. Pick an
element with Command+Shift+C on macOS or Control+Shift+C elsewhere. A host menu
can use `mygo.RoleToggleDevTools`.

The inspector highlights hovered elements and the selected tree element when
the tree has focus. Its box-model colors are blue content, green padding, yellow
border and orange margin. The wrapper's whole-tree debug guides use blue outer
bounds, green inner content edges and orange margin boundaries. They are two
distinct views of the same native layout.

The upstream inspector currently uses English. A build with the
`mygo_noinspector` tag excludes it; `InspectorAvailable()` reports this and the
Passpine toolbar explains the limitation. The wrapper's bounds/grid/ruler still
work. Root debug outlines include overlays, including the audit toolbar itself.
Ruler points are manual positions, do not attach to elements, and are not an
automatic spacing or typography quality score.

## Passpine preview and renderer screenshots

From `client/`:

```sh
go run ./cmd/passpine --preview --layout-audit
```

The opt-in flag requires `--preview` and rejects `--data-dir` and
`--seed-test-vault`, so this entry point always creates an isolated temporary
vault with fictional entries. Normal client
launches keep their existing inspector policy and have no audit toolbar. Both
the manager shell and the separate quick-search window are wrapped; their ruler
state is independent. The quick-search toolbar starts collapsed.

For deterministic native-renderer screenshots without launching a native window:

```sh
PASSPINE_LAYOUT_AUDIT_SCREENSHOTS=/tmp/passpine-audit-images \
  go test ./desktop -run TestLayoutAuditScreensAndScreenshotExport -count=1
```

This renders real client views with fictional test data in Chinese/light and
English/dark themes: entries, settings, recovery, setup, locked, editor,
generator, confirmation dialog and quick search. `ui.NewTester` renders the
wrapper but does not enable MyGo's inspector; validate the inspector in the
native preview. These captures do not replace target-platform native checks.

```sh
go test ./layoutaudit ./desktop ./internal/clientshell ./cmd/passpine ./i18n
go test -tags mygo_noinspector ./layoutaudit ./desktop -run 'TestLayoutAudit|TestMeasurement|TestWrapper|TestResizeClears'
```
