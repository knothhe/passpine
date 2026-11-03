package desktop

import (
	"time"

	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	sidebar "github.com/knothhe/passpine/client/internal/clientshell"
)

func TestGlassPaletteKeepsOpaqueReadableControls(t *testing.T) {
	for _, dark := range []bool{false, true} {
		palette := clientPalette(app.ThemeStyleGlass, dark)
		for name, background := range map[string]ui.Color{
			"background": palette.Background, "surface": palette.Surface,
			"hover": palette.SurfaceHover.Over(palette.Surface), "pressed": palette.SurfacePressed.Over(palette.Surface),
			"selection":    palette.Selection.Over(palette.Surface),
			"content tint": glassContentTint(palette).Over(palette.Background),
		} {
			for role, text := range map[string]ui.Color{"text": palette.Text, "muted": palette.TextMuted} {
				if ratio := themeContrast(text, background); ratio < 4.5 {
					t.Errorf("dark=%v %s on %s contrast=%.2f, want >=4.5", dark, role, name, ratio)
				}
			}
		}
		for name, background := range map[string]ui.Color{"accent": palette.Accent, "hover": palette.AccentHover, "pressed": palette.AccentPressed} {
			if ratio := themeContrast(palette.AccentText, background); ratio < 4.5 {
				t.Errorf("dark=%v primary text on %s contrast=%.2f, want >=4.5", dark, name, ratio)
			}
		}
		// Inputs draw over content and keep opaque faces; hover and press only
		// tint, so quiet buttons on glass never turn into opaque blocks.
		if palette.Surface.A != 255 || palette.Background.A != 255 || palette.Dark != dark {
			t.Fatalf("dark=%v glass control faces are translucent", dark)
		}
		if palette.SurfaceHover.A == 255 || palette.SurfacePressed.A == 255 {
			t.Fatalf("dark=%v glass hover states are opaque", dark)
		}
	}
}

func TestDensityFollowsSettingsInThemeAndChrome(t *testing.T) {
	a := fixture()
	a.settings, a.settingsLoaded = a.state.Settings, true
	var theme *ui.Theme
	var chrome sidebar.Chrome
	var header float32
	view := ui.NewTester(func(c *ui.Context) {
		theme = a.clientTheme(c)
		c.SetTheme(theme)
		chrome, header = a.shellChrome(c), sidebar.HeaderHeight(c)
	}, 400, 300)
	if theme.Space(1) != 4 || chrome.Compact || chrome.Glass || header != 48 {
		t.Fatalf("comfortable: spacing=%v chrome=%+v header=%v", theme.Space(1), chrome, header)
	}
	comfortable := header
	a.settings.Density, a.settings.ThemeStyle = app.DensityCompact, app.ThemeStyleGlass
	view.Frame()
	if theme.Space(1) != 3 || !chrome.Compact || !chrome.Glass || chrome.Backdrop == nil || header >= comfortable {
		t.Fatalf("compact glass: spacing=%v chrome=%+v header=%v", theme.Space(1), chrome, header)
	}
}

func TestGlassStyleRendersBackdropAroundFloatingPanes(t *testing.T) {
	a := fixture()
	a.state.Settings.ThemeStyle = app.ThemeStyleGlass
	view := ui.NewTester(testView(a), 1120, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()
	background := clientPalette(app.ThemeStyleGlass, false).Background
	// The window edge beside the floating sidebar shows the gradient
	// backdrop, not the opaque background other styles paint.
	for _, point := range [][2]int{{3, 400}, {204, 400}, {1116, 400}} {
		got := view.Image().RGBAAt(point[0], point[1])
		if got.R == background.R && got.G == background.G && got.B == background.B {
			t.Fatalf("pixel %v shows the opaque background, want the glass backdrop", point)
		}
	}
	if !view.HasText(a.t("全部条目")) {
		t.Fatal("navigation is missing under glass")
	}
}

func TestCompactDensityShortensNavigationAndEntryRows(t *testing.T) {
	heights := map[string][2]float32{}
	for _, density := range []string{app.DensityComfortable, app.DensityCompact} {
		a := fixture()
		a.state.Settings.Density = density
		view := ui.NewTester(testView(a), 1120, 760)
		view.SetPreferences(ui.Preferences{ReduceMotion: true})
		view.Frame()
		nav, ok := view.Find(a.t("加密草稿"))
		if !ok {
			t.Fatalf("%s: navigation row missing", density)
		}
		entry, ok := view.Find(a.t("账号 %s", a.state.Entries[0].Path))
		if !ok {
			t.Fatalf("%s: entry row missing", density)
		}
		heights[density] = [2]float32{nav.H, entry.H}
	}
	comfortable, compact := heights[app.DensityComfortable], heights[app.DensityCompact]
	if compact[0] >= comfortable[0] || compact[1] >= comfortable[1] {
		t.Fatalf("compact rows %v are not shorter than comfortable %v", compact, comfortable)
	}
}

func TestGlassDialogsAndQuickSearchAreTranslucent(t *testing.T) {
	for _, style := range []string{app.ThemeStyleMonochrome, app.ThemeStyleGlass} {
		a := fixture()
		a.state.Settings.ThemeStyle = style
		view := ui.NewTester(testView(a), 1120, 760)
		view.SetPreferences(ui.Preferences{ReduceMotion: true})
		a.toggleCommandPalette()
		view.Advance(time.Second)
		view.Frame()
		surface := clientPalette(style, false).Surface
		// A blank spot in the palette's footer, inside the panel.
		got := view.Image().RGBAAt(560, 596)
		opaque := got.R == surface.R && got.G == surface.G && got.B == surface.B
		if (style == app.ThemeStyleGlass) == opaque {
			t.Fatalf("%s palette panel opaque=%v at %v", style, opaque, got)
		}
	}
	a := fixture()
	a.state.Settings.ThemeStyle = app.ThemeStyleGlass
	view := ui.NewTester(a.quickSearchView, 660, 460)
	view.Frame()
	background := clientPalette(app.ThemeStyleGlass, false).Background
	if got := view.Image().RGBAAt(4, 230); got.R == background.R && got.G == background.G && got.B == background.B {
		t.Fatal("glass quick search painted an opaque panel")
	}
	if !view.HasText(a.t("复制密码")) {
		t.Fatal("glass quick search lost its footer")
	}
	// The search field floats in its own capsule above the results card,
	// keeps its accessible name and still filters the results.
	field, ok := view.Find(a.t("快速搜索条目"))
	if !ok {
		t.Fatal("glass quick search field is missing")
	}
	if field.H < 24 || field.Y > 80 {
		t.Fatalf("glass search field is not in the top capsule: %v", field)
	}
	if err := view.Click(a.t("快速搜索条目")); err != nil {
		t.Fatal(err)
	}
	view.Type("Figma")
	if len(a.quick.results) != 1 || a.quick.results[0].Name != "Figma" {
		t.Fatalf("glass search field did not filter: %+v", a.quick.results)
	}
}
