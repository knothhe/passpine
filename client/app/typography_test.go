package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFontScaleDefaultsLegacyMigrationAndPersistence(t *testing.T) {
	for _, preference := range []string{`{"theme":"dark"}`, `{"theme":"dark","fontScale":0}`} {
		t.Run(preference, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(`{"settings":`+preference+`}`), 0600); err != nil {
				t.Fatal(err)
			}
			s, err := New(Options{DataDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			settings := must(t, s, Request{Action: "settings"}).Settings
			if settings.FontScale != 100 || settings.Theme != "dark" {
				t.Fatalf("legacy typography lost defaults or existing values: %+v", settings)
			}
			settings.FontScale = 120
			must(t, s, Request{Action: "settings", Settings: &settings})
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := New(Options{DataDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if got := must(t, reopened, Request{Action: "settings"}).Settings; got.FontScale != 120 {
				t.Fatalf("restart changed the saved font scale: %d", got.FontScale)
			}
			settings.FontScale = 0 // A client predating the option omits it.
			if got := must(t, reopened, Request{Action: "settings", Settings: &settings}).Settings; got.FontScale != 120 {
				t.Fatal("an older client reset the saved text size")
			}
		})
	}
}

func TestFontScaleAcceptsOnlySegmentedChoices(t *testing.T) {
	for _, scale := range []int{80, 90, 100, 110, 120} {
		settings := DefaultSettings()
		settings.FontScale = scale
		if err := validateSettings(settings); err != nil {
			t.Fatalf("valid font scale %d rejected: %v", scale, err)
		}
	}
	for _, scale := range []int{-1, 0, 79, 85, 105, 121, 200} {
		settings := DefaultSettings()
		settings.FontScale = scale
		if validateSettings(settings) == nil {
			t.Fatalf("invalid font scale %d accepted", scale)
		}
	}
}
