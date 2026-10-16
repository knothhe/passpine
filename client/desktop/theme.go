package desktop

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/theme"
)

// Palettes provide semantic roles to MyGo and Onevra controls. The monochrome
// surfaces follow Racomi/Magpie; primary controls use neutral black and white.
// Appearance (system/light/dark) remains owned by mygo.Theme.SetSource.
func clientPalette(style string, dark bool) *ui.Theme {
	t := theme.Palette(dark)
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
		t.Background = ui.Hex("#1a1a1e")
		t.Surface = ui.Hex("#232327")
		t.SurfaceHover = ui.Hex("#2d2d33")
		t.SurfacePressed = ui.Hex("#36363d")
		t.Border = ui.Hex("#333339")
		t.Text = ui.Hex("#ededf1")
		t.TextMuted = ui.Hex("#a1a1aa")
		t.AccentText = t.Background
		t.AccentHover = ui.Hex("#ffffff")
		t.AccentPressed = ui.Hex("#cbcbd2")
	} else {
		t.Background = ui.Hex("#f4f4f6")
		t.Surface = ui.Hex("#ffffff")
		t.SurfaceHover = ui.Hex("#f1f1f4")
		t.SurfacePressed = ui.Hex("#e9e9ee")
		t.Border = ui.Hex("#e3e3e8")
		t.Text = ui.Hex("#1c1c21")
		t.TextMuted = ui.Hex("#686872")
		t.AccentText = t.Surface
		t.AccentHover = ui.Hex("#35353d")
		t.AccentPressed = ui.Hex("#0f0f13")
	}
	t.Accent = t.Text
	t.Selection = t.Accent.Alpha(.08)
	t.Focus = t.Accent.Alpha(.65)
	t.Scrollbar = t.Text.Alpha(.32)
	t.Inverse, t.InverseText = t.Text, t.Background
	return t
}

func clientThemeForStyle(c *ui.Context, style string) *ui.Theme {
	t := clientPalette(style, c.Theme().Dark)
	// Match Onevra's native accessibility preferences for both palettes.
	p := c.Preferences()
	if p.HighContrast {
		t.Border = t.Border.Mix(t.Text, .45)
		t.TextMuted = t.TextMuted.Mix(t.Text, .4)
		t.Focus.A = 255
		t.Scrollbar = t.Scrollbar.Alpha(1.6)
	}
	if scale := p.TextScale; scale > 0 && scale != 1 {
		t.FontSize *= scale
	}
	return t
}

func clientTheme(c *ui.Context) *ui.Theme {
	return clientThemeForStyle(c, app.ThemeStyleOnevra)
}

func (a *application) themeSettings() app.Settings {
	if a.settingsLoaded {
		return a.settings
	}
	return a.state.Settings
}

func (a *application) clientTheme(c *ui.Context) *ui.Theme {
	return clientThemeForStyle(c, a.themeSettings().ThemeStyle)
}

func nativeThemeBackground(style string) string {
	light, dark := clientPalette(style, false).Background, clientPalette(style, true).Background
	return fmt.Sprintf("light-dark(#%02x%02x%02x, #%02x%02x%02x)", light.R, light.G, light.B, dark.R, dark.G, dark.B)
}
