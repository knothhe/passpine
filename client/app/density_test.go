package app

import (
	"encoding/json"
	"strings"

	"github.com/knothhe/passpine/client/cloudsync"
	"os"
	"path/filepath"
	"testing"
)

func TestGlassStyleAndLayoutDensityPersist(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Options{DataDir: dir, Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	settings := must(t, s, Request{Action: "settings"}).Settings
	if settings.Density != DensityComfortable {
		t.Fatalf("default density = %q, want comfortable", settings.Density)
	}
	settings.ThemeStyle, settings.Density = ThemeStyleGlass, DensityCompact
	must(t, s, Request{Action: "settings", Settings: &settings})

	// Older clients omit the density; the saved choice is preserved.
	older := settings
	older.Density = ""
	must(t, s, Request{Action: "settings", Settings: &older})
	_ = s.Close()

	s, err = New(Options{DataDir: dir, Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got := must(t, s, Request{Action: "settings"}).Settings
	if got.ThemeStyle != ThemeStyleGlass || got.Density != DensityCompact {
		t.Fatalf("reloaded style=%q density=%q", got.ThemeStyle, got.Density)
	}
	invalid := got
	invalid.Density = "spacious"
	if s.Execute(Request{Action: "settings", Settings: &invalid}).Status == "ok" {
		t.Fatal("an unknown density was accepted")
	}
}

func TestMissingOrStaleDensityLoadsComfortable(t *testing.T) {
	for _, saved := range []string{`{"settings":{}}`, `{"settings":{"density":"roomy"}}`} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(saved), 0600); err != nil {
			t.Fatal(err)
		}
		s, err := New(Options{DataDir: dir, Preview: true})
		if err != nil {
			t.Fatal(err)
		}
		got := must(t, s, Request{Action: "settings"}).Settings.Density
		_ = s.Close()
		if got != DensityComfortable {
			t.Fatalf("saved %s: density = %q", saved, got)
		}
	}
}

func TestConfigSyncAcceptsWellFormedThemeStyles(t *testing.T) {
	// Known, platform-specific and future styles all pass validation; each
	// client falls back to a style it supports when applying them.
	for _, key := range []string{"desktop.themeStyle", "browser.themeStyle"} {
		for _, style := range []string{`"glass"`, `"omarchy"`, `"aurora-2"`} {
			if err := validateSyncedValues(cloudsync.Values{key: json.RawMessage(style)}); err != nil {
				t.Fatalf("%s=%s was rejected: %v", key, style, err)
			}
		}
		for _, style := range []string{`""`, `null`, `42`, `"Glass"`, `"../x"`, `"` + strings.Repeat("a", 33) + `"`} {
			if validateSyncedValues(cloudsync.Values{key: json.RawMessage(style)}) == nil {
				t.Fatalf("%s=%s was accepted", key, style)
			}
		}
	}
	if NormalizeBrowserThemeStyle(ThemeStyleGlass) != ThemeStyleMonochrome || NormalizeThemeStyle("aurora") != ThemeStyleMonochrome {
		t.Fatal("unknown styles do not fall back to the default")
	}
}
