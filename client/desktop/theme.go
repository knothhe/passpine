package desktop

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/theme"
)

func monochromeAccent(dark bool) ui.Color {
	if dark {
		return ui.Hex("#c4c4cc")
	}
	return ui.Hex("#1c1c21")
}

// The primary fill is independent of body text, especially in dark appearance.
func isMonochromePalette(t *ui.Theme) bool {
	return t.Accent == monochromeAccent(t.Dark)
}

// Palettes provide semantic roles to MyGo and Onevra controls. The monochrome
// surfaces follow Racomi/Magpie; primary controls use neutral black and gray.
// Appearance (system/light/dark) remains owned by mygo.Theme.SetSource.
func clientPalette(style string, dark bool) *ui.Theme {
	t := theme.Palette(dark)
	// A compact default; the user's percentage and system accessibility scale
	// are applied together when the frame's palette is installed.
	t.FontSize = 12
	if style == app.ThemeStyleOmarchy {
		if applyOmarchyPalette(t) {
			return t
		}
	}
	if app.NormalizeThemeStyle(style) == app.ThemeStyleOnevra {
		// Preserve Passpine's existing opaque navigation surfaces.
		if dark {
			t.Surface = ui.Hex("#292723")
		} else {
			t.Surface = ui.Hex("#f0eee9")
		}
		return t
	}
	if dark {
		// Magpie's frosted panel renders brighter than its raw CSS tint. Use
		// the supplied reference's composited color for our opaque surface.
		t.Background = ui.Hex("#2b2c30")
		t.Surface = ui.Hex("#25262c")
		t.SurfaceHover = ui.Hex("#323338")
		t.SurfacePressed = ui.Hex("#36363d")
		t.Border = ui.Hex("#404145")
		t.Text = ui.Hex("#ededf1")
		t.TextMuted = ui.Hex("#a1a1aa")
		t.AccentText = ui.Hex("#1c1c21")
		t.AccentHover = ui.Hex("#d2d2da")
		t.AccentPressed = ui.Hex("#b4b4bd")
	} else {
		t.Background = ui.Hex("#f2f2f5")
		t.Surface = ui.Hex("#ffffff")
		t.SurfaceHover = ui.Hex("#f4f4f7")
		t.SurfacePressed = ui.Hex("#e9e9ee")
		t.Border = ui.Hex("#e3e3e8")
		t.Text = ui.Hex("#1c1c21")
		t.TextMuted = ui.Hex("#63636b")
		t.AccentText = t.Surface
		t.AccentHover = ui.Hex("#35353d")
		t.AccentPressed = ui.Hex("#0f0f13")
	}
	t.Accent = monochromeAccent(dark)
	t.Selection = t.SurfacePressed
	if dark {
		t.Selection = t.SurfaceHover
	}
	t.Focus = t.Accent.Alpha(.65)
	t.Scrollbar = t.Text.Alpha(.32)
	t.Inverse, t.InverseText = t.Text, t.Background
	return t
}

func clientThemeForStyle(c *ui.Context, style string) *ui.Theme {
	return clientThemeForPreferences(c, style, app.DefaultFontScale)
}

func clientThemeForPreferences(c *ui.Context, style string, fontScale int) *ui.Theme {
	t := clientPalette(style, c.Theme().Dark)
	// Match Onevra's native accessibility preferences for both palettes.
	p := c.Preferences()
	if p.HighContrast {
		t.Border = t.Border.Mix(t.Text, .45)
		t.TextMuted = t.TextMuted.Mix(t.Text, .4)
		t.Focus.A = 255
		t.Scrollbar = t.Scrollbar.Alpha(1.6)
	}
	applyTypography(c, t, fontScale)
	return t
}

func clientTheme(c *ui.Context) *ui.Theme {
	return clientThemeForStyle(c, app.ThemeStyleMonochrome)
}

func (a *application) themeSettings() app.Settings {
	if a.settingsLoaded {
		return a.settings
	}
	return a.state.Settings
}

func (a *application) clientTheme(c *ui.Context) *ui.Theme {
	settings := a.themeSettings()
	return clientThemeForPreferences(c, settings.ThemeStyle, settings.FontScale)
}

func nativeThemeBackground(style string) string {
	light, dark := clientPalette(style, false).Background, clientPalette(style, true).Background
	return fmt.Sprintf("light-dark(#%02x%02x%02x, #%02x%02x%02x)", light.R, light.G, light.B, dark.R, dark.G, dark.B)
}
