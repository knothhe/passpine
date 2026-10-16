package desktop

import (
	"runtime"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

// Explicit sizes were authored against MyGo's original platform font size.
// Follow the effective theme for both saved text size and OS accessibility.
func scaledFontSize(c *ui.Context, size float32) float32 {
	base := float32(14)
	if runtime.GOOS == "darwin" {
		base = 13
	}
	if c.Theme().FontSize <= 0 {
		return size
	}
	return size * c.Theme().FontSize / base
}

func applyTypography(c *ui.Context, theme *ui.Theme, scale int) {
	scale = app.NormalizeFontScale(scale)
	if !app.ValidFontScale(scale) {
		scale = app.DefaultFontScale
	}
	theme.FontSize *= float32(scale) / 100
	if systemScale := c.Preferences().TextScale; systemScale > 0 {
		theme.FontSize *= systemScale
	}
}
