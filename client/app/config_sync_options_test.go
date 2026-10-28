package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func saveSyncOptionsFixture(t *testing.T, s *Service, options ConfigSyncOptions) Data {
	t.Helper()
	return must(t, s, Request{Action: "config-sync-options", SyncOptions: &options})
}

func TestConfigSyncOptionsLocalPersistenceWithoutConnectionOrCredentialAccess(t *testing.T) {
	s, memory, engine, _, _, bucket := newSyncAppSpace(t)
	initial := must(t, s, Request{Action: "config-sync-status"})
	if initial.ConfigSync.Options != DefaultConfigSyncOptions() {
		t.Fatalf("legacy defaults changed: %+v", initial.ConfigSync.Options)
	}
	options := DefaultConfigSyncOptions()
	options.IntervalMinutes, options.SyncOnChange, options.Desktop = 30, false, false
	next := time.Now().Add(time.Minute)
	s.configSync.status.ErrorCode = "conflict"
	s.configSync.status.ConflictKeys = []string{"desktop.theme"}
	s.configSync.next = next
	d := saveSyncOptionsFixture(t, s, options)
	if d.Configured || d.ConfigSync.Options != options || d.ConfigSync.ErrorCode != "conflict" || len(d.ConfigSync.ConflictKeys) != 1 || !s.configSync.next.Equal(next) {
		t.Fatalf("local options changed connection/conflict state: %+v", d.ConfigSync)
	}
	if memory.loads != 0 || memory.retries != 0 || len(memory.values) != 0 {
		t.Fatal("saving local options accessed credentials")
	}
	if _, _, requests := bucket.snapshot(); requests != 0 {
		t.Fatal("saving local options contacted object storage")
	}
	reopened := newSyncServiceFixture(t, s.directory, memory, engine)
	d = must(t, reopened, Request{Action: "config-sync-status"})
	if d.ConfigSync.Options != options {
		t.Fatalf("options lost on restart: %+v", d.ConfigSync.Options)
	}
}

func TestConfigSyncOptionsRejectInvalidAndFailedWrites(t *testing.T) {
	s, _, _, _, _, _ := newSyncAppSpace(t)
	before := DefaultConfigSyncOptions()
	for _, interval := range []int{-1, 0, 2, 61, 9999} {
		invalid := before
		invalid.IntervalMinutes = interval
		out := s.Execute(Request{Action: "config-sync-options", SyncOptions: &invalid})
		if out.Status != "error" || out.Code != "invalid_sync_options" || out.Data.ConfigSync.Options != before {
			t.Fatalf("invalid interval accepted: %d status=%s code=%s", interval, out.Status, out.Code)
		}
	}
	if out := s.Execute(Request{Action: "config-sync-options"}); out.Code != "invalid_sync_options" {
		t.Fatal("missing options accepted")
	}
	metadataPath := filepath.Join(s.directory, "config-sync.json")
	if err := os.Mkdir(metadataPath, 0700); err != nil {
		t.Fatal(err)
	}
	options := before
	options.IntervalMinutes = 15
	out := s.Execute(Request{Action: "config-sync-options", SyncOptions: &options})
	if out.Status != "error" || out.Code != "local_storage" || out.Data.ConfigSync.Options != before {
		t.Fatal("failed options write changed active options")
	}
}

func TestConfigSyncOptionsIntervalsAndChangeScheduling(t *testing.T) {
	s, _, _, c, secrets, _ := newSyncAppSpace(t)
	d := saveSyncFixture(t, s, c, secrets)
	options := DefaultConfigSyncOptions()
	for _, interval := range []int{1, 5, 15, 30, 60} {
		options.IntervalMinutes = interval
		options.SyncOnChange = false
		saveSyncOptionsFixture(t, s, options)
		before := time.Now()
		must(t, s, Request{Action: "config-sync-now"})
		want := time.Duration(interval) * time.Minute
		if got := s.configSync.next.Sub(before); got < want || got > want+5*time.Second {
			t.Fatalf("interval %d not scheduled: %v", interval, got)
		}
	}
	next := s.configSync.next
	settings := d.Settings
	settings.Theme = "dark"
	must(t, s, Request{Action: "settings", Settings: &settings})
	if !s.configSync.dirty || !s.configSync.next.Equal(next) {
		t.Fatal("change-only scheduling ran while disabled")
	}
	options.SyncOnChange = true
	saveSyncOptionsFixture(t, s, options)
	if delay := time.Until(s.configSync.next); delay < 0 || delay > 3*time.Second {
		t.Fatalf("enabling sync on change did not schedule pending changes: %v", delay)
	}
	options.SyncOnChange = false
	saveSyncOptionsFixture(t, s, options)
	if delay := time.Until(s.configSync.next); delay < 59*time.Minute || delay > time.Hour {
		t.Fatalf("disabling sync on change retained expedited deadline: %v", delay)
	}
	// A user edit cannot bypass a failed operation's retry delay.
	next = time.Now().Add(time.Minute)
	s.configSync.next = next
	s.configSync.status.ErrorCode = "connection_failed"
	options.SyncOnChange = true
	saveSyncOptionsFixture(t, s, options)
	settings.Theme = "light"
	must(t, s, Request{Action: "settings", Settings: &settings})
	if !s.configSync.next.Equal(next) || s.configSync.status.ErrorCode != "connection_failed" {
		t.Fatal("options or local edits shortened failure retry")
	}
}

func TestConfigSyncExcludedDesktopRetainsAppliedBaselineAcrossRestart(t *testing.T) {
	for _, localEdit := range []bool{false, true} {
		name := "unchanged local imports cloud"
		if localEdit {
			name = "local and cloud edits conflict"
		}
		t.Run(name, func(t *testing.T) {
			s, memory, engine, c, secrets, _ := newSyncAppSpace(t)
			d := saveSyncFixture(t, s, c, secrets)
			options := DefaultConfigSyncOptions()
			options.Desktop = false
			saveSyncOptionsFixture(t, s, options)
			remote, err := engine.Read(context.Background(), c, secrets)
			if err != nil {
				t.Fatal(err)
			}
			edit := copySyncValues(remote.Values)
			edit["desktop.theme"] = jsonValue("dark")
			edit["future.feature"] = jsonValue(true)
			if _, err = engine.Sync(context.Background(), c, secrets, edit, remote.Values); err != nil {
				t.Fatal(err)
			}
			settings := d.Settings
			if localEdit {
				settings.Theme = "light"
				must(t, s, Request{Action: "settings", Settings: &settings})
			}
			d = must(t, s, Request{Action: "config-sync-now"})
			if d.Settings.Theme != settings.Theme || s.configSync.dirty {
				t.Fatal("excluded namespace applied remote or kept fast retry dirty")
			}
			profile := s.configSync.meta.Profiles["s3"]
			stored, err := s.loadSyncSecret(profile.SecretRef)
			if err != nil || string(stored.Baseline["desktop.theme"]) != string(remote.Values["desktop.theme"]) {
				t.Fatal("excluded namespace advanced last applied baseline", err)
			}
			reopened := newSyncServiceFixture(t, s.directory, memory, engine)
			options.Desktop = true
			saveSyncOptionsFixture(t, reopened, options)
			out := reopened.Execute(Request{Action: "config-sync-now"})
			if localEdit {
				if out.Status != "error" || out.Code != "conflict" || out.Data.Settings.Theme != "light" {
					t.Fatalf("re-enabled conflicting edits lost: %s %s", out.Status, out.Code)
				}
			} else if out.Status != "ok" || out.Data.Settings.Theme != "dark" {
				t.Fatalf("re-enabled namespace overwrote cloud with unchanged local: %s %s", out.Status, out.Code)
			}
			latest, err := engine.Read(context.Background(), c, secrets)
			if err != nil || string(latest.Values["desktop.theme"]) != `"dark"` || string(latest.Values["future.feature"]) != "true" {
				t.Fatal("excluded remote or unknown fields changed", err)
			}
			for key := range latest.Values {
				if key == "intervalMinutes" || key == "syncOnChange" || key == "desktop" || key == "browser" {
					t.Fatal("local sync options entered encrypted document")
				}
			}
		})
	}
}

func TestConfigSyncInitiallyExcludedNamespacesImportOnFirstEnable(t *testing.T) {
	s, memory, engine, c, secrets, bucket := newSyncAppSpace(t)
	remoteValues := syncTestValues("desktop.theme", `"dark"`, "desktop.themeStyle", `"onevra"`, "desktop.language", `"zh-CN"`, "desktop.fontScale", "110", "browser.theme", `"dark"`, "browser.themeStyle", `"onevra"`, "browser.language", `"zh-CN"`, "browser.fontScale", "120")
	if _, err := engine.Sync(context.Background(), c, secrets, remoteValues, nil); err != nil {
		t.Fatal(err)
	}
	options := DefaultConfigSyncOptions()
	options.Desktop, options.Browser = false, false
	saveSyncOptionsFixture(t, s, options)
	before, puts, _ := bucket.snapshot()
	d := saveSyncFixture(t, s, c, secrets)
	if d.Settings.Theme != DefaultSettings().Theme || d.SyncedBrowserPreferences != nil {
		t.Fatal("excluded initial namespaces applied")
	}
	after, newPuts, _ := bucket.snapshot()
	if puts != newPuts || !bytes.Equal(before, after) {
		t.Fatal("excluded initial namespaces changed remote object")
	}
	reopened := newSyncServiceFixture(t, s.directory, memory, engine)
	options.Desktop, options.Browser = true, true
	saveSyncOptionsFixture(t, reopened, options)
	d = must(t, reopened, Request{Action: "config-sync-now"})
	if d.Settings.Theme != "dark" || d.Settings.Language != "zh-CN" || d.Settings.FontScale != 110 || d.SyncedBrowserPreferences == nil || *d.SyncedBrowserPreferences.Theme != "dark" || *d.SyncedBrowserPreferences.FontScale != 120 {
		t.Fatal("first enabling namespace failed to import cloud preferences")
	}
	var metadata map[string]json.RawMessage
	data, err := os.ReadFile(filepath.Join(s.directory, "config-sync.json"))
	if err != nil || json.Unmarshal(data, &metadata) != nil || metadata["options"] == nil {
		t.Fatal("sync options were not persisted locally")
	}
}

func TestConfigSyncExcludedNamespaceIsNotRecreatedAfterRemoteDeletion(t *testing.T) {
	s, _, engine, c, secrets, bucket := newSyncAppSpace(t)
	saveSyncFixture(t, s, c, secrets)
	options := DefaultConfigSyncOptions()
	options.Desktop = false
	saveSyncOptionsFixture(t, s, options)
	bucket.mu.Lock()
	bucket.body = nil
	bucket.mu.Unlock()
	must(t, s, Request{Action: "config-sync-now"})
	remote, err := engine.Read(context.Background(), c, secrets)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range desktopConfigSyncKeys {
		if _, ok := remote.Values[key]; ok {
			t.Fatal("excluded namespace recreated after cloud object deletion")
		}
	}
}
