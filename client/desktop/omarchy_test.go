package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestOmarchyThemeRefreshAndFallback(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "omarchy", "current", "theme")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	themePath := filepath.Join(path, "colors.toml")
	write := func(raw string) {
		t.Helper()
		if err := os.WriteFile(themePath, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		refreshOmarchyPalette()
	}
	defer func() {
		omarchyColors.Lock()
		omarchyColors.loaded = false
		omarchyColors.colors = nil
		omarchyColors.raw = ""
		omarchyColors.Unlock()
	}()
	write("background = \"#112233\"\nforeground = '#eeeeee'\naccent = \"#88aaff\" # comment\n")
	if omarchyAppearance() != "dark" {
		t.Fatal("Omarchy system appearance not detected")
	}
	palette := clientPalette(app.ThemeStyleOmarchy, true)
	if palette.Background != ui.Hex("#112233") || palette.Accent != ui.Hex("#88aaff") {
		t.Fatal("active system colors not applied")
	}
	if themeContrastRatio(palette.AccentText, palette.Accent) < 4.5 {
		t.Fatal("unreadable primary label")
	}
	light := clientPalette(app.ThemeStyleOmarchy, false)
	if light.Dark || light.Background == palette.Background || light.Accent != palette.Accent {
		t.Fatal("explicit light appearance not preserved")
	}
	if refreshOmarchyPalette() {
		t.Fatal("unchanged theme caused update")
	}
	write("background = \"#112233\"\nforeground = \"#eeeeee\"\naccent = \"#ff8800\"\n")
	if clientPalette(app.ThemeStyleOmarchy, true).Accent != ui.Hex("#ff8800") {
		t.Fatal("theme switch not applied")
	}
	write("accent = \"not a color\"")
	if clientPalette(app.ThemeStyleOmarchy, true).Accent != monochromeAccent(true) {
		t.Fatal("invalid theme did not fall back")
	}
	stateTheme := filepath.Join(os.Getenv("XDG_STATE_HOME"), "omarchy", "current", "theme")
	if err := os.MkdirAll(stateTheme, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateTheme, "colors.toml"), []byte("background = \"#112233\"\nforeground = \"#eeeeee\"\naccent = \"#abcdef\""), 0600); err != nil {
		t.Fatal(err)
	}
	refreshOmarchyPalette()
	if clientPalette(app.ThemeStyleOmarchy, true).Accent != ui.Hex("#abcdef") {
		t.Fatal("state theme not preferred over legacy config")
	}
	if err := os.Remove(filepath.Join(stateTheme, "colors.toml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(themePath); err != nil {
		t.Fatal(err)
	}
	refreshOmarchyPalette()
	if clientPalette(app.ThemeStyleOmarchy, true).Accent != monochromeAccent(true) {
		t.Fatal("missing theme did not fall back")
	}
}

func TestParseOmarchyColorsRejectsIncompleteAndInvalidData(t *testing.T) {
	for _, raw := range []string{"", "background = \"#123456\"", "background = \"#zzzzzz\"\nforeground = \"#ffffff\"\naccent = \"#123456\""} {
		if parseOmarchyColors(raw) != nil {
			t.Fatalf("accepted invalid colors: %q", raw)
		}
	}
}

func TestOmarchyExplicitAppearanceOverridesLuminance(t *testing.T) {
	for _, test := range []struct {
		raw        string
		background ui.Color
		dark       bool
	}{
		{`mode = "light"`, ui.Hex("#112233"), false},
		{`mode = 'dark'`, ui.Hex("#eeeeee"), true},
		{"mode = \"light\"\ntheme_type = \"dark\"", ui.Hex("#112233"), false},
		{`theme_type = "light"`, ui.Hex("#112233"), false},
		{"", ui.Hex("#112233"), true},
		{"", ui.Hex("#eeeeee"), false},
	} {
		if got := parseOmarchyDark(test.raw, test.background); got != test.dark {
			t.Errorf("%q: dark=%v, want %v", test.raw, got, test.dark)
		}
	}
}
