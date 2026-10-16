package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLifecyclePreferencesDefaultsMigrationAndPersistence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(`{"settings":{"theme":"dark","closeToTray":false,"idleMinutes":15}}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	settings := must(t, s, Request{Action: "settings"}).Settings
	if settings.DockIcon != DockIconShow || settings.TrayClickAction != TrayClickQuickPanel || settings.LightweightMode || settings.LaunchAtLogin {
		t.Fatalf("legacy preferences did not receive lifecycle defaults: %+v", settings)
	}
	if settings.Theme != "dark" || settings.CloseToTray || settings.IdleMinutes != 15 {
		t.Fatal("lifecycle migration overwrote existing preferences")
	}
	settings.DockIcon, settings.TrayClickAction = DockIconFollowWindow, TrayClickMainWindow
	settings.LightweightMode, settings.LaunchAtLogin = true, true
	must(t, s, Request{Action: "settings", Settings: &settings})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := must(t, reopened, Request{Action: "settings"}).Settings; got != settings {
		t.Fatalf("restart changed lifecycle preferences: %+v", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, "preferences.json"))
	var persisted preferences
	if err != nil || json.Unmarshal(data, &persisted) != nil || persisted.Settings != settings {
		t.Fatal("lifecycle preferences were not written to disk")
	}
}

func TestLifecycleSettingsRejectInvalidValuesAndPreserveOlderClientsEnums(t *testing.T) {
	for _, value := range []string{"", "regular", "unknown"} {
		settings := DefaultSettings()
		settings.DockIcon = value
		if validateSettings(settings) == nil {
			t.Fatalf("accepted invalid dock policy %q", value)
		}
		settings = DefaultSettings()
		settings.TrayClickAction = value
		if validateSettings(settings) == nil {
			t.Fatalf("accepted invalid tray action %q", value)
		}
	}
	previous := DefaultSettings()
	previous.DockIcon, previous.TrayClickAction = DockIconHide, TrayClickMainWindow
	older := previous
	older.DockIcon, older.TrayClickAction = "", ""
	normalizeLifecycleSettings(&older, previous)
	if older != previous {
		t.Fatal("an older client's missing enums reset lifecycle preferences")
	}
}
