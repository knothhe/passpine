package desktop

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	sidebar "github.com/knothhe/passpine/client/internal/clientshell"
)

// Layout metrics shared by the window chrome and the framed columns. The
// comfortable values are the client's original layout; compact tightens them
// together with the theme's spacing unit (see densitySpacing).

func compactLayout(c *ui.Context) bool { return sidebar.Compact(c) }

// glassChrome reports whether this frame draws Liquid Glass panes.
func (a *application) glassChrome() bool {
	return isGlassStyle(a.themeSettings().ThemeStyle)
}

func managerFrameInsetFor(c *ui.Context) float32 {
	if compactLayout(c) {
		return 4
	}
	return managerFrameInset
}

func managerFrameRadius(c *ui.Context, glassy bool) float32 {
	switch {
	case compactLayout(c):
		return 8
	case glassy:
		return 16
	default:
		return 12
	}
}

// glassContentTint lies over the content pane's glass so dense lists and
// details keep the contrast of an opaque surface while the rim still bends
// the backdrop.
func glassContentTint(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(22, 24, 32, .52)
	}
	return ui.RGBA(255, 255, 255, .58)
}

// paneSurface is the fill for panes inside the framed columns: transparent
// over glass, so the frame's tint shows through instead of stacking.
func (a *application) paneSurface(c *ui.Context) ui.Color {
	if a.glassChrome() {
		return ui.Transparent
	}
	return c.Theme().Surface
}

// managerFrame builds the inset, rounded card holding the list and detail
// columns of the entry manager and the recovery center.
func (a *application) managerFrame(c *ui.Context, insetKey, key string, build func()) {
	t := c.Theme()
	glassy := a.glassChrome()
	outer := ui.Column(c).Key(insetKey).Fill().MinWidth(0).MinHeight(0).Padding(managerFrameInsetFor(c))
	if !glassy {
		outer.Background(t.Background)
	}
	outer.Children(func() {
		card := ui.Row(c).Key(key).Fill().MinWidth(0).MinHeight(0).Radius(managerFrameRadius(c, glassy)).Clip()
		if !glassy {
			card.Border(1, t.Border).Background(t.Surface).Children(build)
			return
		}
		radius, tint := managerFrameRadius(c, glassy), glassContentTint(t)
		card.Material(glass.Glass{}).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, tint, radius)
		}).Children(build)
	})
}

// headerControlSize is the height of title bar buttons and selectors.
func headerControlSize(c *ui.Context) float32 {
	if compactLayout(c) {
		return 28
	}
	return 32
}

// headerControlRadius rounds title bar controls into capsules in the
// comfortable layout, concentric with the action group around them.
func headerControlRadius(c *ui.Context) float32 {
	if compactLayout(c) {
		return c.Theme().Radius
	}
	return headerControlSize(c) / 2
}

func headerGap(c *ui.Context) float32 {
	if compactLayout(c) {
		return 4
	}
	return 8
}

// headerActionGroup lays out the title bar's vault actions. Comfortable
// headers gather them in one capsule, of Liquid Glass under the glass style,
// as macOS toolbars group related items; compact headers keep them flat.
func (a *application) headerActionGroup(c *ui.Context, build func()) {
	group := ui.Row(c).Key("header-actions").Shrink(0).AlignItems(ui.Center)
	if compactLayout(c) {
		group.Gap(2).Children(build)
		return
	}
	t := c.Theme()
	group.Gap(2).Padding(2).Radius(headerControlSize(c)/2 + 2)
	if a.glassChrome() {
		group.Material(glass.Glass{})
	} else {
		group.Background(t.Surface).Border(1, t.Border)
	}
	group.Children(build)
}

// pageFrame places a full page on the same frosted card as the entry
// manager under the glass style, so its text never sits on the bare backdrop.
// Other styles build the page directly on the window background.
func (a *application) pageFrame(c *ui.Context, key string, build func()) {
	if !a.glassChrome() {
		build()
		return
	}
	t := c.Theme()
	ui.Column(c).Key(key + "-frame-inset").Fill().MinWidth(0).MinHeight(0).Padding(managerFrameInsetFor(c)).Children(func() {
		radius, tint := managerFrameRadius(c, true), glassContentTint(t)
		ui.Column(c).Key(key + "-frame").Fill().MinWidth(0).MinHeight(0).Radius(radius).Clip().
			Material(glass.Glass{}).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, tint, radius)
		}).Children(build)
	})
}

// Navigation rows: comfortable rows keep generous touch targets; compact
// rows fit roughly a third more items in the same height.
type navigationMetrics struct {
	rowHeight, padY, padX, gap, indent, fontSize float32
	// Sidebar section padding: top, sides and bottom of section labels.
	sectionTop, sectionX, sectionBottom float32
	listX                               float32
}

func navigationMetricsFor(c *ui.Context) navigationMetrics {
	if compactLayout(c) {
		return navigationMetrics{rowHeight: 28, padY: 4, padX: 8, gap: 1, indent: 12, fontSize: 12,
			sectionTop: 12, sectionX: 14, sectionBottom: 6, listX: 6}
	}
	return navigationMetrics{rowHeight: 36, padY: 8, padX: 8, gap: 4, indent: 14, fontSize: 13,
		sectionTop: 24, sectionX: 20, sectionBottom: 12, listX: 12}
}

// sidebarInset is how far sidebar content stays inside a floating glass pane.
func (a *application) sidebarInset(c *ui.Context) float32 {
	return sidebar.SidebarPaneInset(a.shellChrome(c))
}

// Entry list rows show a name and a subtitle in both densities; compact rows
// drop the extra breathing room around them.
type entryListMetrics struct {
	top, side, bottom, gap      float32
	rowHeight, rowPadY, lineGap float32
}

func entryListMetricsFor(c *ui.Context) entryListMetrics {
	if compactLayout(c) {
		return entryListMetrics{top: 16, side: 12, bottom: 8, gap: 12, rowHeight: 44, rowPadY: 4, lineGap: 1}
	}
	return entryListMetrics{top: 24, side: 16, bottom: 12, gap: 16, rowHeight: 60, rowPadY: 8, lineGap: 3}
}

// sidebarBackground fills the sidebar: a raised surface for the floating
// comfortable pane, the window background when it docks in compact layouts.
func (a *application) sidebarBackground(c *ui.Context) ui.Color {
	if compactLayout(c) {
		return c.Theme().Background
	}
	return c.Theme().Surface
}
