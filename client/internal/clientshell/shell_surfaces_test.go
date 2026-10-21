package clientshell

import (
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/theme"
)

func assertShellPixel(t *testing.T, view *ui.Tester, x, y int, want ui.Color) {
	t.Helper()
	got := view.Image().RGBAAt(x, y)
	if expected := (color.RGBA{R: want.R, G: want.G, B: want.B, A: want.A}); got != expected {
		t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, expected)
	}
}

func TestExplicitShellSurfacesCoverMaterialHeaderAndSidebar(t *testing.T) {
	left, header := ui.RGB(21, 34, 47), ui.RGB(51, 64, 77)
	var border, surface ui.Color
	shell, build := Layout(ShellOptions{
		LeftWidth: 224, LeftOverlayBelow: 960, Theme: theme.Onevra,
		LeftBackground:   func(*ui.Context) ui.Color { return left },
		HeaderBackground: func(*ui.Context) ui.Color { return header },
		LeftToolbar:      func(c *ui.Context) { ui.Box(c).Width(16).Height(24) },
		Content: func(c *ui.Context) {
			border, surface = c.Theme().Border, c.Theme().Surface
			ui.Column(c).Fill().Background(surface)
		},
	})
	view := ui.NewTester(build, 1120, 760)
	view.SetTitleBar(ui.TitleBar{Height: 48, Left: 80})
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.SetVibrancy(true)
	checkHeader := func() {
		t.Helper()
		// Native-control space and the content title share the same header.
		assertShellPixel(t, view, 40, 8, header)
		assertShellPixel(t, view, 500, 8, header)
		assertShellPixel(t, view, 40, 47, border)
		assertShellPixel(t, view, 500, 47, border)
	}
	checkSidebar := func() {
		t.Helper()
		assertShellPixel(t, view, 120, 100, left)
		assertShellPixel(t, view, 120, 750, left)
	}
	checkHeader()
	checkSidebar()
	assertShellPixel(t, view, 500, 100, surface)

	// Collapsing and switching to an overlay retain the same surface treatment.
	shell.SetLeftExpanded(false)
	view.Frame()
	checkHeader()
	assertShellPixel(t, view, 120, 100, surface)
	view.SetSize(860, 760)
	shell.SetLeftExpanded(true)
	view.Frame()
	checkHeader()
	checkSidebar()

	// Callbacks run each frame so palette changes cover toolbar and header too.
	left, header = ui.RGB(81, 94, 107), ui.RGB(111, 124, 137)
	view.SetDark(true)
	checkHeader()
	checkSidebar()
}

func TestUnsetShellSurfacesKeepTransparentHeaderAndMaterial(t *testing.T) {
	var background, surface ui.Color
	_, build := Layout(ShellOptions{
		LeftWidth: 224, Theme: theme.Onevra,
		Content: func(c *ui.Context) {
			background, surface = c.Theme().Background.Alpha(1), c.Theme().Surface.Alpha(1)
		},
	})
	view := ui.NewTester(build, 1120, 760)
	view.SetTitleBar(ui.TitleBar{Height: 48, Left: 80})
	// Without callbacks the header preserves each underlying region and has
	// no bottom border, matching the shell's previous appearance.
	assertShellPixel(t, view, 40, 47, surface)
	assertShellPixel(t, view, 500, 47, background)
	view.SetVibrancy(true)
	assertShellPixel(t, view, 40, 47, ui.Transparent)
	assertShellPixel(t, view, 40, 100, ui.Transparent)
	assertShellPixel(t, view, 500, 47, background)
}
