package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestThemeStyleLegacyPreferencesPreserveExistingSettings(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, style := range []string{"", `,"themeStyle":""`, `,"themeStyle":"removed-style"`} {
		dir := t.TempDir()
		contents := `{"settings":{"theme":"dark","language":"en","clipboardSeconds":45` + style + `}}`
		if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		s, err := New(Options{DataDir: dir, Preview: true})
		if err != nil {
			t.Fatal(err)
		}
		settings := must(t, s, Request{Action: "settings"}).Settings
		_ = s.Close()
		if settings.ThemeStyle != ThemeStyleMonochrome || settings.Theme != "dark" || settings.Language != "en" || settings.ClipboardSeconds != 45 {
			t.Fatalf("legacy style reset other preferences: %+v", settings)
		}
	}
}

func TestThemeStylePersistsSeparatelyFromAppearanceAndLegacyUpdates(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Options{DataDir: dir, Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.Theme, settings.ThemeStyle = "dark", ThemeStyleMonochrome
	if got := must(t, s, Request{Action: "settings", Settings: &settings}).Settings; got.ThemeStyle != ThemeStyleMonochrome || got.Theme != "dark" {
		t.Fatalf("theme style did not save: %+v", got)
	}
	_ = s.Close()
	s, err = New(Options{DataDir: dir, Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	settings = must(t, s, Request{Action: "settings"}).Settings
	if settings.ThemeStyle != ThemeStyleMonochrome || settings.Theme != "dark" {
		t.Fatalf("theme style did not survive restart: %+v", settings)
	}
	settings.ThemeStyle, settings.ClipboardSeconds = "", 45
	if got := must(t, s, Request{Action: "settings", Settings: &settings}).Settings; got.ThemeStyle != ThemeStyleMonochrome || got.ClipboardSeconds != 45 {
		t.Fatalf("legacy settings update reset selected style: %+v", got)
	}
	for _, invalid := range []string{"magpie", "light", "MONOCHROME"} {
		settings.ThemeStyle = invalid
		if response := s.Execute(Request{Action: "settings", Settings: &settings}); response.Status != "error" {
			t.Fatalf("accepted unknown theme style %q", invalid)
		}
		if got := must(t, s, Request{Action: "settings"}).Settings; got.ThemeStyle != ThemeStyleMonochrome {
			t.Fatal("invalid update changed the saved theme")
		}
	}
}

func TestOmarchyThemeStylePersists(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	dir := t.TempDir()
	s, err := New(Options{DataDir: dir, Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	settings := must(t, s, Request{Action: "settings"}).Settings
	settings.ThemeStyle = ThemeStyleOmarchy
	if got := must(t, s, Request{Action: "settings", Settings: &settings}).Settings; got.ThemeStyle != ThemeStyleOmarchy {
		t.Fatal("Omarchy style not retained")
	}
	_ = s.Close()
	s, err = New(Options{DataDir: dir, Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := must(t, s, Request{Action: "settings"}).Settings; got.ThemeStyle != ThemeStyleOmarchy {
		t.Fatal("Omarchy style did not survive restart")
	}
}

func TestOmarchyDefaultPreservesExplicitChoices(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if defaultThemeStyle() != ThemeStyleMonochrome {
		t.Fatal("non-Omarchy default changed")
	}
	theme := filepath.Join(os.Getenv("XDG_STATE_HOME"), "omarchy", "current", "theme")
	if err := os.MkdirAll(theme, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(theme, "colors.toml"), []byte("accent = '#abcdef'"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, choice := range []string{"", ThemeStyleMonochrome, ThemeStyleOnevra} {
		dir := t.TempDir()
		if choice != "" {
			if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(`{"settings":{"themeStyle":"`+choice+`"}}`), 0600); err != nil {
				t.Fatal(err)
			}
		}
		s, err := New(Options{DataDir: dir, Preview: true})
		if err != nil {
			t.Fatal(err)
		}
		got := must(t, s, Request{Action: "settings"}).Settings.ThemeStyle
		_ = s.Close()
		want := choice
		if want == "" {
			want = ThemeStyleOmarchy
		}
		if got != want {
			t.Fatalf("saved=%q: got %q, want %q", choice, got, want)
		}
	}
}
