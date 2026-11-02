package desktop

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	sidebar "github.com/knothhe/passpine/client/internal/clientshell"
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
	if app.NormalizeThemeStyle(style) == app.ThemeStyleGlass {
		applyGlassPalette(t, dark)
		return t
	}
	if app.NormalizeThemeStyle(style) == app.ThemeStyleOnevra {
		// Keep subtly shaded light navigation beside warm white content panes.
		if dark {
			t.Surface = ui.Hex("#292723")
		} else {
			t.Background, t.Surface = ui.Hex("#f5f3ef"), t.Background
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

// applyGlassPalette keeps control faces opaque so inputs stay legible;
// translucency belongs to the glass panes, and hover and press states only
// tint what is beneath them.
// A cool neutral base and a system-blue accent follow macOS's Liquid Glass.
func applyGlassPalette(t *ui.Theme, dark bool) {
	if dark {
		t.Background = ui.Hex("#14161e")
		t.Surface = ui.Hex("#22252f")
		// Hover and press tint whatever is under a control, so quiet buttons
		// and rows read as light on glass rather than as opaque blocks.
		t.SurfaceHover = ui.RGBA(255, 255, 255, .08)
		t.SurfacePressed = ui.RGBA(255, 255, 255, .14)
		t.Border = ui.RGBA(255, 255, 255, .11)
		t.Text = ui.Hex("#eceef4")
		t.TextMuted = ui.Hex("#b0b5c3")
		t.Accent = ui.Hex("#4c9bff")
		t.AccentHover = ui.Hex("#66aaff")
		t.AccentPressed = ui.Hex("#3a86ea")
		t.AccentText = ui.Hex("#06101f")
	} else {
		t.Background = ui.Hex("#eef1f8")
		t.Surface = ui.Hex("#ffffff")
		t.SurfaceHover = ui.RGBA(24, 34, 70, .06)
		t.SurfacePressed = ui.RGBA(24, 34, 70, .11)
		t.Border = ui.RGBA(28, 36, 64, .13)
		t.Text = ui.Hex("#14161d")
		t.TextMuted = ui.Hex("#596074")
		t.Accent = ui.Hex("#0a6cff")
		t.AccentHover = ui.Hex("#005fe6")
		t.AccentPressed = ui.Hex("#004fc4")
		t.AccentText = ui.Hex("#ffffff")
	}
	t.Radius = 10
	t.Selection = t.Accent.Alpha(.16)
	t.Focus = t.Accent.Alpha(.55)
	t.Scrollbar = t.Text.Alpha(.3)
	t.Inverse, t.InverseText = t.Text, t.Background
}

func isGlassStyle(style string) bool {
	return app.NormalizeThemeStyle(style) == app.ThemeStyleGlass
}

// glassBackdrop is the soft, colored field Liquid Glass refracts: enough
// color for the panes to bend at their rims without competing with the text
// placed on them.
func glassBackdrop(c *ui.Context, root ui.Element) {
	stops := []ui.GradientStop{
		{Color: ui.Hex("#cfdcff"), At: 0}, {Color: ui.Hex("#e4e6ff"), At: .35},
		{Color: ui.Hex("#f1e6fb"), At: .7}, {Color: ui.Hex("#ffe4d6"), At: 1},
	}
	if c.Theme().Dark {
		stops = []ui.GradientStop{
			{Color: ui.Hex("#0d1a3a"), At: 0}, {Color: ui.Hex("#161735"), At: .35},
			{Color: ui.Hex("#1d1530"), At: .7}, {Color: ui.Hex("#2a1520"), At: 1},
		}
	}
	root.LinearGradient(ui.LinearGradient{Angle: 135, Oklab: true, Stops: stops})
}

// densitySpacing maps the layout density onto MyGo's spacing unit, so
// MyGo and Onevra widgets tighten with the client's own layout.
func densitySpacing(density string) float32 {
	if app.NormalizeDensity(density) == app.DensityCompact {
		return 3
	}
	return 4
}

func clientThemeForStyle(c *ui.Context, style string) *ui.Theme {
	return clientThemeForPreferences(c, style, app.DefaultFontScale)
}

func clientThemeForPreferences(c *ui.Context, style string, fontScale int) *ui.Theme {
	return clientThemeForSettings(c, app.Settings{ThemeStyle: style, FontScale: fontScale})
}

func clientThemeForSettings(c *ui.Context, settings app.Settings) *ui.Theme {
	style, fontScale := settings.ThemeStyle, settings.FontScale
	t := clientPalette(style, c.Theme().Dark)
	t.Spacing = densitySpacing(settings.Density)
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
	return clientThemeForSettings(c, a.themeSettings())
}

// shellChrome follows the saved style and density on every frame.
func (a *application) shellChrome(c *ui.Context) sidebar.Chrome {
	settings := a.themeSettings()
	compact := app.NormalizeDensity(settings.Density) == app.DensityCompact
	chrome := sidebar.Chrome{Compact: compact, Floating: !compact}
	if isGlassStyle(settings.ThemeStyle) {
		chrome.Glass, chrome.Backdrop = true, glassBackdrop
	}
	return chrome
}

func nativeThemeBackground(style string) string {
	light, dark := clientPalette(style, false).Background, clientPalette(style, true).Background
	return fmt.Sprintf("light-dark(#%02x%02x%02x, #%02x%02x%02x)", light.R, light.G, light.B, dark.R, dark.G, dark.B)
}
