package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigSyncFailedActivationRestoresCurrentPreferences(t *testing.T) {
	for _, newerEdit := range []bool{false, true} {
		name := "unchanged-local"
		if newerEdit {
			name = "edit-during-secret-save"
		}
		t.Run(name, func(t *testing.T) {
			s, memory, engine, activeConfig, secrets, _ := newSyncAppSpace(t)
			associationFixtureVault(t, s, "")
			association := associationRead(t, s)
			hash := strings.Repeat("a", 64)
			associationPatchFixture(t, s, association, map[string]bool{hash: false})
			data := saveSyncFixture(t, s, activeConfig, secrets)
			settings := data.Settings
			settings.Theme = "light"
			must(t, s, Request{Action: "settings", Settings: &settings})
			browserTheme := "light"
			must(t, s, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{Theme: &browserTheme}})
			must(t, s, Request{Action: "config-sync-now"})
			previous := s.configSync.meta.Profiles["s3"]
			candidateBucket := &syncTestBucket{}
			server := httptest.NewServer(http.HandlerFunc(candidateBucket.serve))
			t.Cleanup(server.Close)
			candidate := activeConfig
			candidate.Endpoint = server.URL
			candidate.Prefix = "candidate-space"
			remote := syncTestValues("desktop.theme", `"dark"`, "browser.theme", `"dark"`, "browser.themeStyle", `"onevra"`, "browser.language", `"en"`, "browser.fontScale", `120`)
			remote[associationValuePrefix+association.AssociationScope+"."+hash] = jsonValue(true)
			if _, err := engine.Sync(t.Context(), candidate, secrets, remote, nil); err != nil {
				t.Fatal(err)
			}
			metadataPath := filepath.Join(s.directory, "config-sync.json")
			backupPath := filepath.Join(s.directory, "saved-config-sync.json")
			t.Cleanup(func() {
				if info, err := os.Stat(metadataPath); err == nil && info.IsDir() {
					_ = os.Remove(metadataPath)
					_ = os.Rename(backupPath, metadataPath)
				}
			})
			wantTheme := "light"
			var expectedPreferences []byte
			var expectedAssociations []byte
			memory.mu.Lock()
			memory.saveHook = func() {
				if newerEdit {
					wantTheme = "system"
					current := must(t, s, Request{Action: "config-sync-status"}).Settings
					current.Theme = wantTheme
					must(t, s, Request{Action: "settings", Settings: &current})
					must(t, s, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{Theme: &wantTheme}})
				}
				var err error
				expectedPreferences, err = os.ReadFile(filepath.Join(s.directory, "preferences.json"))
				if err != nil {
					t.Fatal(err)
				}
				expectedAssociations, err = os.ReadFile(filepath.Join(s.directory, browserAssociationFile))
				if err != nil {
					t.Fatal(err)
				}
				if err = os.Rename(metadataPath, backupPath); err != nil {
					t.Fatal(err)
				}
				if err = os.Mkdir(metadataPath, 0700); err != nil {
					t.Fatal(err)
				}
			}
			memory.mu.Unlock()
			result := s.Execute(Request{Action: "config-sync-save", SyncConnection: &SyncConnectionRequest{Provider: "s3", Config: candidate, Secrets: secrets}})
			if result.Status != "error" {
				t.Fatal("failed activation metadata write reported success")
			}
			if active := s.configSync.meta.Profiles["s3"]; active.SecretRef != previous.SecretRef || active.Config != activeConfig {
				t.Fatal("failed activation replaced the active profile")
			}
			if result.Data.Settings.Theme != wantTheme || result.Data.SyncedBrowserPreferences == nil || *result.Data.SyncedBrowserPreferences.Theme != wantTheme {
				t.Fatalf("failed activation imported candidate preferences: desktop=%s browser=%+v", result.Data.Settings.Theme, result.Data.SyncedBrowserPreferences)
			}
			stored, err := os.ReadFile(filepath.Join(s.directory, "preferences.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(stored, expectedPreferences) {
				t.Fatal("failed activation did not restore the current on-disk preference snapshot")
			}
			storedAssociations, err := os.ReadFile(filepath.Join(s.directory, browserAssociationFile))
			if err != nil || !bytes.Equal(storedAssociations, expectedAssociations) {
				t.Fatal("failed activation did not restore website associations", err)
			}
			if err = os.Remove(metadataPath); err != nil {
				t.Fatal(err)
			}
			if err = os.Rename(backupPath, metadataPath); err != nil {
				t.Fatal(err)
			}
			must(t, s, Request{Action: "config-sync-now"})
			activeRemote, err := engine.Read(t.Context(), activeConfig, secrets)
			if err != nil {
				t.Fatal(err)
			}
			if string(activeRemote.Values["desktop.theme"]) != string(jsonValue(wantTheme)) || string(activeRemote.Values["browser.theme"]) != string(jsonValue(wantTheme)) {
				t.Fatal("old active space received preferences from failed candidate")
			}
		})
	}
}

func TestConfigSyncOmarchyFallbackDoesNotHideLaterRemoteStyleEdit(t *testing.T) {
	s, _, engine, config, secrets, bucket := newSyncAppSpace(t)
	if _, err := engine.Sync(t.Context(), config, secrets, syncTestValues("desktop.themeStyle", `"omarchy"`), nil); err != nil {
		t.Fatal(err)
	}
	data := saveSyncFixture(t, s, config, secrets)
	if data.Settings.ThemeStyle != NormalizeThemeStyle(ThemeStyleOmarchy) {
		t.Fatal("platform-specific Omarchy behavior changed")
	}
	baseline, err := engine.Read(t.Context(), config, secrets)
	if err != nil {
		t.Fatal(err)
	}
	remote := copySyncValues(baseline.Values)
	remote["desktop.themeStyle"] = json.RawMessage(`"onevra"`)
	if _, err = engine.Sync(t.Context(), config, secrets, remote, baseline.Values); err != nil {
		t.Fatal(err)
	}
	_, putsBefore, _ := bucket.snapshot()
	data = must(t, s, Request{Action: "config-sync-now"})
	if data.Settings.ThemeStyle != ThemeStyleOnevra {
		t.Fatalf("platform fallback hid remote style change: %s", data.Settings.ThemeStyle)
	}
	must(t, s, Request{Action: "config-sync-now"})
	current, err := engine.Read(t.Context(), config, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if string(current.Values["desktop.themeStyle"]) != `"onevra"` {
		t.Fatal("fallback was uploaded over the newer remote theme style")
	}
	_, putsAfter, _ := bucket.snapshot()
	if putsAfter != putsBefore {
		t.Fatal("an unedited platform fallback was treated as a local theme edit")
	}
}
