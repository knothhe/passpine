package linuxdesktop

import (
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
)

var menuTheme struct {
	once     sync.Once
	err      error
	screen   func() uintptr
	create   func() uintptr
	load     func(uintptr, string, int, uintptr) bool
	add      func(uintptr, uintptr, uint32)
	provider uintptr
	css      string
}

// SetMenuTheme runs on GTK's main thread. The provider belongs to this process;
// it changes neither the desktop GTK theme nor any user's configuration files.
func SetMenuTheme(css string) error {
	menuTheme.once.Do(func() {
		lib, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			menuTheme.err = err
			return
		}
		for _, symbol := range []struct {
			target any
			name   string
		}{
			{&menuTheme.screen, "gdk_screen_get_default"},
			{&menuTheme.create, "gtk_css_provider_new"},
			{&menuTheme.load, "gtk_css_provider_load_from_data"},
			{&menuTheme.add, "gtk_style_context_add_provider_for_screen"},
		} {
			address, err := purego.Dlsym(lib, symbol.name)
			if err != nil {
				menuTheme.err = err
				return
			}
			purego.RegisterFunc(symbol.target, address)
		}
	})
	if menuTheme.err != nil {
		return menuTheme.err
	}
	if menuTheme.css == css {
		return nil
	}
	if menuTheme.provider == 0 {
		screen := menuTheme.screen()
		if screen == 0 {
			return fmt.Errorf("GTK menu theme has no screen")
		}
		menuTheme.provider = menuTheme.create()
		menuTheme.add(screen, menuTheme.provider, 600)
	}
	if !menuTheme.load(menuTheme.provider, css, len(css), 0) {
		return fmt.Errorf("GTK rejected menu theme CSS")
	}
	menuTheme.css = css
	return nil
}
