package desktop

import (
	"fmt"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/internal/linuxdesktop"
)

func nativeMenuCSS(t *ui.Theme) string {
	hex := func(c ui.Color) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }
	return fmt.Sprintf(`
menubar {
 background-image: none; background-color: %s; color: %s;
 box-shadow: none; border: none; border-bottom: 1px solid %s;
 padding: 2px 8px;
}
menubar > menuitem { padding: 5px 10px; border-radius: 5px; }
menu {
 background-image: none; background-color: %s; color: %s;
 border: 1px solid %s; padding: 4px; border-radius: 8px;
}
menu menuitem { padding: 6px 12px; border-radius: 4px; }
menubar > menuitem:hover, menu menuitem:hover {
 background-image: none; background-color: %s; color: %s;
}
menubar > menuitem:disabled, menu menuitem:disabled { color: %s; }
menu separator { background-color: %s; min-height: 1px; margin: 4px 8px; }
`, hex(t.Background), hex(t.Text), hex(t.Border), hex(t.Surface), hex(t.Text), hex(t.Border), hex(t.SurfaceHover), hex(t.Text), hex(t.TextMuted), hex(t.Border))
}

func (a *application) updateNativeMenuTheme() {
	if a.closing {
		return
	}
	palette := clientPalette(a.themeSettings().ThemeStyle, mygo.Theme.IsDark())
	if err := linuxdesktop.SetMenuTheme(nativeMenuCSS(palette)); err != nil {
		log.Printf("Passpine native menu theme: %v", err)
	}
}
