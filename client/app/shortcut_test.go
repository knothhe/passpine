package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeGlobalShortcut(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"", DefaultGlobalShortcut},
		{" control + option + p ", "Ctrl+Alt+P"},
		{"shift+command+space", "Cmd+Shift+Space"},
		{"CmdOrCtrl+Shift+f", "CmdOrCtrl+Shift+F"},
		{"Ctrl++", "Ctrl+Plus"},
		{"F12", "F12"},
		{"Alt+return", "Alt+Enter"},
	} {
		got, err := NormalizeGlobalShortcut(test.input)
		if err != nil || got != test.want {
			t.Errorf("%q: got %q, %v, want %q", test.input, got, err, test.want)
		}
	}
	for _, value := range []string{"A", "Shift+A", "Ctrl", "Ctrl+", "Ctrl+Alt+", "Ctrl+Ctrl+K", "Control+Ctrl+K", "CmdOrCtrl+Ctrl+K", "Ctrl+unavailable", "Ctrl+F25", "Ctrl+K+L", "Ctrl+汉", "Ctrl+\nK"} {
		if _, err := NormalizeGlobalShortcut(value); err == nil {
			t.Errorf("invalid shortcut accepted: %q", value)
		}
	}
}

func TestGlobalShortcutPersistsAndInvalidInputKeepsSavedSettings(t *testing.T) {
	directory := t.TempDir()
	s, err := New(Options{DataDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.Theme = "dark"
	settings.GlobalShortcut = " control + option + p "
	saved := must(t, s, Request{Action: "settings", Settings: &settings}).Settings
	if saved.GlobalShortcut != "Ctrl+Alt+P" {
		t.Fatalf("shortcut was not normalized: %q", saved.GlobalShortcut)
	}
	path := filepath.Join(directory, "preferences.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	settings.GlobalShortcut = "Ctrl+NoSuchKey"
	if response := s.Execute(Request{Action: "settings", Settings: &settings}); response.Status != "error" || response.Data.Settings != saved {
		t.Fatalf("invalid shortcut replaced saved settings: %#v", response)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("invalid shortcut changed preferences on disk", err)
	}
	s.Close()
	reopened, err := New(Options{DataDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := must(t, reopened, Request{Action: "status"}).Settings; got != saved {
		t.Fatalf("custom shortcut did not survive restart: %#v", got)
	}
}

func TestLegacyInvalidShortcutKeepsOtherPreferences(t *testing.T) {
	directory := t.TempDir()
	settings := DefaultSettings()
	settings.GlobalShortcut = "broken shortcut"
	settings.Theme = "dark"
	settings.CloseToTray = false
	data, err := json.Marshal(preferences{Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "preferences.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{DataDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	want := settings
	want.GlobalShortcut = DefaultGlobalShortcut
	if got := must(t, s, Request{Action: "status"}).Settings; got != want {
		t.Fatalf("recovering old shortcut discarded unrelated settings: %#v", got)
	}
}
