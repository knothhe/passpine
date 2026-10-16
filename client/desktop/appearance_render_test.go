package desktop

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
)

func TestMonochromeInputsUseSolidReferenceSurfaces(t *testing.T) {
	for _, dark := range []bool{false, true} {
		value := ""
		view := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(clientPalette(app.ThemeStyleMonochrome, dark))
			ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
				textBox(c, &value, components.TextBoxOptions{}).Size(180, 44)
			})
		}, 200, 60)
		pixel, want := view.Image().RGBAAt(10, 10), monochromeInputBackground(dark)
		if pixel.R != want.R || pixel.G != want.G || pixel.B != want.B || pixel.A != 255 {
			t.Fatalf("dark=%v input inherited a translucent pane: got %v, want %v", dark, pixel, want)
		}
	}
}

func TestReturnKeyIconRetainsPrimaryContrastInBothAppearances(t *testing.T) {
	for _, dark := range []bool{false, true} {
		palette := clientPalette(app.ThemeStyleMonochrome, dark)
		view := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(palette)
			ui.Row(c).Fill().Background(palette.Accent).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				quickSearchKeycap(c, "↵", true)
			})
		}, 80, 40)
		bounds, found := view.Find("↵")
		if !found {
			t.Fatal("return keycap is missing")
		}
		background := palette.AccentText.Alpha(.08).Over(palette.Accent)
		var strongest float64
		for y := int(math.Ceil(float64(bounds.Y + 3))); y < int(bounds.Y+bounds.H-3); y++ {
			for x := int(math.Ceil(float64(bounds.X + 3))); x < int(bounds.X+bounds.W-3); x++ {
				pixel := view.Image().RGBAAt(x, y)
				strongest = max(strongest, themeContrast(ui.RGB(pixel.R, pixel.G, pixel.B), background))
			}
		}
		// Allow raster antialiasing, while rejecting a muted foreground that
		// loses its contrast on the inverted primary button.
		if strongest < themeContrast(palette.AccentText, background)*.72 {
			t.Fatalf("dark=%v return icon lost its primary foreground: contrast %.2f", dark, strongest)
		}
	}
}
