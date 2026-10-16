package desktop

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func themeContrast(a, b ui.Color) float64 {
	luminance := func(c ui.Color) float64 {
		linear := func(channel uint8) float64 {
			v := float64(channel) / 255
			if v <= .04045 {
				return v / 12.92
			}
			return math.Pow((v+.055)/1.055, 2.4)
		}
		return .2126*linear(c.R) + .7152*linear(c.G) + .0722*linear(c.B)
	}
	x, y := luminance(a), luminance(b)
	return (max(x, y) + .05) / (min(x, y) + .05)
}

func TestMonochromeSemanticRolesRemainReadableInBothAppearances(t *testing.T) {
	for _, dark := range []bool{false, true} {
		palette := clientPalette(app.ThemeStyleMonochrome, dark)
		for name, background := range map[string]ui.Color{
			"background": palette.Background, "surface": palette.Surface,
			"hover": palette.SurfaceHover, "pressed": palette.SurfacePressed,
			"selection": palette.Selection.Over(palette.Surface),
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
		if palette.Dark != dark || palette.Accent != palette.Text || palette.Selection.R != palette.Accent.R || palette.Focus.R != palette.Accent.R {
			t.Fatal("monochrome semantic roles lost appearance or neutral primary tint")
		}
	}
}

func TestBothThemeStylesFollowNativeAppearanceAndAccessibility(t *testing.T) {
	for _, style := range []string{app.ThemeStyleOnevra, app.ThemeStyleMonochrome} {
		a := fixture()
		a.settings, a.settingsLoaded = a.state.Settings, true
		a.settings.ThemeStyle = style
		var got *ui.Theme
		view := ui.NewTester(func(c *ui.Context) { got = a.clientTheme(c) }, 200, 100)
		view.SetDark(true)
		base := clientPalette(style, true)
		if !got.Dark || got.Background != base.Background {
			t.Fatalf("%s did not follow dark appearance", style)
		}
		view.SetPreferences(ui.Preferences{HighContrast: true, TextScale: 1.5, Accent: ui.Hex("#2563eb")})
		if got.FontSize != base.FontSize*1.5 || got.Border == base.Border || got.TextMuted == base.TextMuted || got.Focus.A != 255 || got.Accent != base.Accent {
			t.Fatalf("%s lost accessibility preferences or its primary color: %+v", style, got)
		}
		view.SetDark(false)
		if got.Dark || got.Background != clientPalette(style, false).Background {
			t.Fatalf("%s did not return to light appearance", style)
		}
	}
}

func TestSettingsSwitchThemeImmediatelyInManagerAndQuickSearch(t *testing.T) {
	a := fixture()
	a.page = "settings"
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("陶土橙"); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("黑白"); err != nil {
		t.Fatal(err)
	}
	if a.settings.ThemeStyle != app.ThemeStyleMonochrome || a.settings.Theme != "system" || !a.settingsDirty {
		t.Fatal("theme selection did not change locally without changing appearance")
	}
	view.Frame()
	monochrome := clientPalette(app.ThemeStyleMonochrome, false)
	if pixel := view.Image().RGBAAt(1000, 700); pixel.R != monochrome.Background.R || pixel.G != monochrome.Background.G || pixel.B != monochrome.Background.B {
		t.Fatalf("manager retained the previous background: %v", pixel)
	}
	quick := ui.NewTester(a.quickSearchView, 660, 460)
	if pixel := quick.Image().RGBAAt(640, 400); pixel.R != monochrome.Background.R || pixel.G != monochrome.Background.G || pixel.B != monochrome.Background.B {
		t.Fatalf("quick search did not share selected theme: %v", pixel)
	}
	if err := view.Click("黑白"); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("陶土橙"); err != nil {
		t.Fatal(err)
	}
	if a.settings.ThemeStyle != app.ThemeStyleOnevra {
		t.Fatal("default style could not be restored")
	}
}
