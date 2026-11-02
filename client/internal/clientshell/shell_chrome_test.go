package clientshell

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/theme"
)

func TestGlassChromeReplacesOpaqueSidebarAndHeader(t *testing.T) {
	backdrop := ui.RGB(90, 140, 220)
	left, header := ui.RGB(21, 34, 47), ui.RGB(51, 64, 77)
	glassy := true
	_, build := Layout(ShellOptions{
		LeftWidth: 224, Theme: theme.Onevra,
		LeftBackground:   func(*ui.Context) ui.Color { return left },
		HeaderBackground: func(*ui.Context) ui.Color { return header },
		Chrome: func(*ui.Context) Chrome {
			return Chrome{Glass: glassy, Backdrop: func(_ *ui.Context, root ui.Element) { root.Background(backdrop) }}
		},
	})
	view := ui.NewTester(build, 1120, 760)
	view.SetTitleBar(ui.TitleBar{Height: 48, Left: 80})
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.SetVibrancy(true)
	// The backdrop shows outside the floating pane, under the header and
	// through the transparent content region.
	assertShellPixel(t, view, 3, 400, backdrop)
	assertShellPixel(t, view, 500, 8, backdrop)
	assertShellPixel(t, view, 500, 400, backdrop)
	// Inside the pane the glass frosts the backdrop instead of painting
	// the opaque sidebar color.
	if got := view.Image().RGBAAt(120, 400); got.R == left.R && got.G == left.G && got.B == left.B {
		t.Fatal("glass sidebar painted the opaque sidebar background")
	}
	glassy = false
	view.Frame()
	assertShellPixel(t, view, 3, 400, left)
	assertShellPixel(t, view, 500, 8, header)
}

func TestCompactThemeShortensHeader(t *testing.T) {
	var comfortable, compact float32
	view := ui.NewTester(func(c *ui.Context) {
		comfortable = HeaderHeight(c)
		dense := *c.Theme()
		dense.Spacing = 3
		c.SetTheme(&dense)
		compact = HeaderHeight(c)
	}, 200, 100)
	view.Frame()
	if comfortable != 48 || compact >= comfortable || compact < 40 {
		t.Fatalf("header heights comfortable=%v compact=%v", comfortable, compact)
	}
}

func TestFloatingChromeInsetsOpaqueSidebarPane(t *testing.T) {
	left, header := ui.RGB(250, 250, 252), ui.RGB(51, 64, 77)
	var background ui.Color
	_, build := Layout(ShellOptions{
		LeftWidth: 224, Theme: theme.Onevra,
		LeftBackground:   func(*ui.Context) ui.Color { return left },
		HeaderBackground: func(*ui.Context) ui.Color { return header },
		Chrome:           func(*ui.Context) Chrome { return Chrome{Floating: true} },
		Content: func(c *ui.Context) {
			background = c.Theme().Background.Alpha(1)
		},
	})
	view := ui.NewTester(build, 1120, 760)
	view.SetTitleBar(ui.TitleBar{Height: 48, Left: 80})
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()
	// The window background surrounds the pane; the pane carries the
	// sidebar fill; the header has no fill or separator of its own.
	// The pane's soft shadow reaches the window edge; only its fill is exact.
	if got := view.Image().RGBAAt(2, 400); got.R == left.R && got.G == left.G && got.B == left.B {
		t.Fatal("the floating pane reaches the window edge")
	}
	assertShellPixel(t, view, 120, 400, left)
	assertShellPixel(t, view, 500, 8, background)
	assertShellPixel(t, view, 500, 47, background)
	assertShellPixel(t, view, 224, 400, background)
}
