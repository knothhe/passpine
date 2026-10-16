package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIdleLockDefaultsAndSavedPreferences(t *testing.T) {
	for _, test := range []struct {
		name     string
		contents string
		minutes  int
	}{
		{"fresh", "", 1},
		{"legacy without idle duration", `{"settings":{"theme":"dark"}}`, 1},
		{"previous preset", `{"settings":{"idleMinutes":5}}`, 5},
		{"previous default", `{"settings":{"idleMinutes":10}}`, 10},
		{"custom duration", `{"settings":{"idleMinutes":15}}`, 15},
		{"longer preset", `{"settings":{"idleMinutes":30}}`, 30},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if test.contents != "" {
				if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(test.contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			s, err := New(Options{DataDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			settings := must(t, s, Request{Action: "settings"}).Settings
			if settings.IdleMinutes != test.minutes {
				t.Fatalf("loaded idle duration = %d, want %d", settings.IdleMinutes, test.minutes)
			}
			v, err := s.newVault(filepath.Join(dir, "vault"))
			if err != nil {
				t.Fatal(err)
			}
			defer v.Close()
			if v.Config().IdleTimeout != time.Duration(test.minutes)*time.Minute {
				t.Fatal("vault session did not use the configured idle duration")
			}
			must(t, s, Request{Action: "settings", Settings: &settings})
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := New(Options{DataDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if got := must(t, reopened, Request{Action: "settings"}).Settings.IdleMinutes; got != test.minutes {
				t.Fatalf("restart changed saved idle duration to %d, want %d", got, test.minutes)
			}
		})
	}
}

func TestActivityDefaultsAndLegacyPreferences(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(`{"settings":{"theme":"dark","language":"en","clipboardSeconds":45}}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	settings := must(t, s, Request{Action: "settings"}).Settings
	if settings.ActivityMaxEntries != 1000 || settings.ActivityRetentionDays != 7 || settings.Theme != "dark" || settings.ClipboardSeconds != 45 || settings.Language != "en" {
		t.Fatalf("legacy preferences lost values or new defaults: %+v", settings)
	}
	if DefaultSettings().Language != "system" {
		t.Fatal("language does not default to system")
	}
}

func TestActivityPrunesOnStartupReadAndSettingsWhileRecordingIsOff(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	recent := func(action string, age time.Duration) Activity {
		return Activity{Time: now.Add(-age).Format(time.RFC3339), Action: action, Source: "desktop", Outcome: "ok"}
	}
	records := []Activity{recent("newest", time.Hour), recent("second", 2*time.Hour), recent("excess", 3*time.Hour), recent("expired", 8*24*time.Hour), {Time: "invalid", Action: "invalid"}}
	settings := DefaultSettings()
	settings.RecordActivity = false
	settings.ActivityMaxEntries = 2
	prefs, _ := json.Marshal(preferences{Settings: settings})
	data, _ := json.Marshal(records)
	for name, contents := range map[string][]byte{"preferences.json": prefs, "activity.json": data} {
		if err := os.WriteFile(filepath.Join(dir, name), contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := New(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := must(t, s, Request{Action: "activity"}).Activity; len(got) != 2 || got[0].Action != "newest" {
		t.Fatalf("startup did not retain newest records: %+v", got)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "activity.json"))
	if err != nil || strings.Contains(string(onDisk), "expired") || strings.Contains(string(onDisk), "excess") {
		t.Fatal("startup did not persist retention cleanup")
	}
	s.mu.Lock()
	s.activity = append(s.activity, recent("expired-on-read", 8*24*time.Hour))
	s.mu.Unlock()
	if got := must(t, s, Request{Action: "activity"}).Activity; len(got) != 2 {
		t.Fatal("read did not apply retention")
	}
	settings.ActivityMaxEntries = 1
	must(t, s, Request{Action: "settings", Settings: &settings})
	if got := must(t, s, Request{Action: "activity"}).Activity; len(got) != 1 || got[0].Action != "newest" {
		t.Fatalf("settings change did not immediately prune: %+v", got)
	}
}

func TestActivityDefaultRetainsOneThousandAndSevenDays(t *testing.T) {
	s := &Service{directory: t.TempDir(), prefs: preferences{Settings: DefaultSettings()}}
	now := time.Now().UTC()
	for i := range 1100 {
		s.activity = append(s.activity, Activity{Time: now.Add(-time.Duration(i) * time.Second).Format(time.RFC3339), Action: "existing"})
	}
	s.activity = append(s.activity, Activity{Time: now.Add(-8 * 24 * time.Hour).Format(time.RFC3339), Action: "expired"})
	s.log("new", "desktop", "ok")
	if len(s.activity) != 1000 || s.activity[0].Action != "new" {
		t.Fatalf("log write retained %d entries", len(s.activity))
	}
	if err := s.pruneActivity(now.Add(8 * 24 * time.Hour)); err != nil || len(s.activity) != 0 {
		t.Fatal("records did not expire after seven days", err)
	}
}

func TestRetentionSettingsRejectInvalidLimits(t *testing.T) {
	for _, limits := range [][2]int{{0, 7}, {-1, 7}, {100001, 7}, {1000, 0}, {1000, 3651}} {
		settings := DefaultSettings()
		settings.ActivityMaxEntries, settings.ActivityRetentionDays = limits[0], limits[1]
		if validateSettings(settings) == nil {
			t.Fatalf("accepted invalid retention: %v", limits)
		}
	}
}

func TestVaultDirectoryNameAndRenamePersistWithoutOverwritingCustomName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	s, err := New(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	dir := filepath.Join(home, ".password-store")
	s.mu.Lock()
	err = s.remember(dir, "")
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	data := must(t, s, Request{Action: "status"})
	if data.VaultName != "~/.password-store" {
		t.Fatalf("default name does not identify directory: %q", data.VaultName)
	}
	data = must(t, s, Request{Action: "vault-rename", Name: "  Work passwords  "})
	if data.VaultName != "Work passwords" || data.Vaults[0].Directory != dir {
		t.Fatal("rename did not update label independently of directory")
	}
	s.mu.Lock()
	err = s.remember(dir, "")
	s.mu.Unlock()
	if err != nil || must(t, s, Request{Action: "status"}).VaultName != "Work passwords" {
		t.Fatal("reopening a vault overwrote its custom name")
	}
	for _, name := range []string{"bad\nname", strings.Repeat("x", 201)} {
		if result := s.Execute(Request{Action: "vault-rename", Name: name}); result.Status != "error" {
			t.Fatalf("accepted invalid vault name %q", name)
		}
	}
	persisted, err := os.ReadFile(filepath.Join(s.directory, "preferences.json"))
	if err != nil || !strings.Contains(string(persisted), "Work passwords") {
		t.Fatal("renamed vault was not persisted")
	}
	data = must(t, s, Request{Action: "vault-rename", Name: " "})
	if data.VaultName != "~/.password-store" || data.Vaults[0].NameCustomized {
		t.Fatal("blank rename did not restore directory default")
	}
	var response Response
	_ = json.Unmarshal(s.handleBrowser([]byte(`{"protocol":1,"action":"vault-rename","name":"attacker"}`)), &response)
	if response.Code != "unsupported_action" {
		t.Fatal("browser was permitted to rename desktop vault")
	}
}

func TestLegacyAutomaticVaultNameMigratesAndExplicitNameSurvives(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	prefs := preferences{Settings: DefaultSettings(), Vaults: []VaultInfo{
		{ID: "legacy", Name: "我的密码库", Directory: filepath.Join(home, ".password-store")},
		{ID: "custom", Name: "Team vault", Directory: filepath.Join(home, "team")},
		{ID: "explicit", Name: "我的密码库", Directory: filepath.Join(home, "explicit"), NameCustomized: true},
	}}
	b, _ := json.Marshal(prefs)
	if err := os.WriteFile(filepath.Join(dir, "preferences.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got := must(t, s, Request{Action: "status"}).Vaults
	if got[0].Name != "~/.password-store" || got[1].Name != "Team vault" || got[2].Name != "我的密码库" {
		t.Fatalf("migration changed explicit names or missed the default: %+v", got)
	}
	must(t, s, Request{Action: "vault-rename", ID: "legacy", Name: "我的密码库"})
	s.mu.Lock()
	changed := s.migrateVaultNames()
	s.mu.Unlock()
	if changed || must(t, s, Request{Action: "status"}).Vaults[0].Name != "我的密码库" {
		t.Fatal("migration reset an explicitly renamed vault")
	}
	if b, err := os.ReadFile(filepath.Join(dir, "preferences.json")); err != nil || !strings.Contains(string(b), `"nameCustomized": true`) {
		t.Fatal("custom name provenance was not saved")
	}
}
