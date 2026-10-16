package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestThemeStyleLegacyPreferencesPreserveExistingSettings(t *testing.T) {
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
