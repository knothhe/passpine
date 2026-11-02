package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// A style added by a newer client must neither fail configuration sync on
// older ones nor be overwritten by their fallback.
func TestConfigSyncUnknownThemeStylesFallBackWithoutOverwritingRemote(t *testing.T) {
	s, _, engine, config, secrets, bucket := newSyncAppSpace(t)
	initial := syncTestValues("desktop.themeStyle", `"aurora"`, "browser.themeStyle", `"aurora"`, "browser.theme", `"dark"`)
	if _, err := engine.Sync(t.Context(), config, secrets, initial, nil); err != nil {
		t.Fatal(err)
	}
	data := saveSyncFixture(t, s, config, secrets)
	if data.ConfigSync == nil || data.ConfigSync.LastSyncedAt == "" {
		t.Fatalf("sync failed on an unknown style: %+v", data.ConfigSync)
	}
	if data.Settings.ThemeStyle != ThemeStyleMonochrome {
		t.Fatalf("desktop style = %q, want the monochrome fallback", data.Settings.ThemeStyle)
	}
	if p := data.SyncedBrowserPreferences; p == nil || p.ThemeStyle == nil || *p.ThemeStyle != ThemeStyleMonochrome || p.Theme == nil || *p.Theme != "dark" {
		t.Fatalf("browser preferences = %+v, want dark with the monochrome fallback", p)
	}

	_, putsBefore, _ := bucket.snapshot()
	must(t, s, Request{Action: "config-sync-now"})
	remote, err := engine.Read(t.Context(), config, secrets)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"desktop.themeStyle", "browser.themeStyle"} {
		if string(remote.Values[key]) != `"aurora"` {
			t.Fatalf("fallback was uploaded over remote %s: %s", key, remote.Values[key])
		}
	}
	if _, putsAfter, _ := bucket.snapshot(); putsAfter != putsBefore {
		t.Fatal("an unedited fallback was treated as a local theme edit")
	}

	// Choosing a style on this device still synchronizes it.
	settings := must(t, s, Request{Action: "settings"}).Settings
	settings.ThemeStyle = ThemeStyleGlass
	must(t, s, Request{Action: "settings", Settings: &settings})
	must(t, s, Request{Action: "config-sync-now"})
	remote, err = engine.Read(t.Context(), config, secrets)
	if err != nil {
		t.Fatal(err)
	}
	var style string
	if json.Unmarshal(remote.Values["desktop.themeStyle"], &style) != nil || style != ThemeStyleGlass {
		t.Fatalf("local style choice was not uploaded: %s", remote.Values["desktop.themeStyle"])
	}
}

// Choosing the fallback style on purpose is a real edit: it must synchronize
// even though its value equals the fallback the device showed.
func TestConfigSyncExplicitFallbackStyleChoiceSynchronizes(t *testing.T) {
	s, memory, engine, config, secrets, _ := newSyncAppSpace(t)
	initial := syncTestValues("desktop.themeStyle", `"aurora"`, "browser.themeStyle", `"aurora"`, "browser.theme", `"dark"`)
	if _, err := engine.Sync(t.Context(), config, secrets, initial, nil); err != nil {
		t.Fatal(err)
	}
	saveSyncFixture(t, s, config, secrets)

	// The untouched fallback survives a restart without being uploaded.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = newSyncServiceFixture(t, s.directory, memory, engine)
	must(t, s, Request{Action: "config-sync-now"})
	remote, err := engine.Read(t.Context(), config, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if string(remote.Values["desktop.themeStyle"]) != `"aurora"` || string(remote.Values["browser.themeStyle"]) != `"aurora"` {
		t.Fatalf("restart uploaded the fallback: %s %s", remote.Values["desktop.themeStyle"], remote.Values["browser.themeStyle"])
	}

	// Onevra and then Monochrome before syncing: the final choice wins.
	settings := must(t, s, Request{Action: "settings"}).Settings
	for _, style := range []string{ThemeStyleOnevra, ThemeStyleMonochrome} {
		settings.ThemeStyle = style
		must(t, s, Request{Action: "settings", Settings: &settings})
	}
	for _, style := range []string{ThemeStyleOnevra, ThemeStyleMonochrome} {
		style := style
		must(t, s, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{ThemeStyle: &style}})
	}
	must(t, s, Request{Action: "config-sync-now"})
	remote, err = engine.Read(t.Context(), config, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if string(remote.Values["desktop.themeStyle"]) != `"monochrome"` || string(remote.Values["browser.themeStyle"]) != `"monochrome"` {
		t.Fatalf("explicit fallback choice was dropped: %s %s", remote.Values["desktop.themeStyle"], remote.Values["browser.themeStyle"])
	}
}

// Preferences written before the fallback record existed infer each key from
// the baseline; settling one key must not stop inference for another.
func TestConfigSyncLegacyFallbackInferenceIsPerKey(t *testing.T) {
	s, _, engine, config, secrets, _ := newSyncAppSpace(t)
	initial := syncTestValues("desktop.themeStyle", `"aurora"`, "browser.themeStyle", `"onevra"`, "browser.theme", `"dark"`)
	if _, err := engine.Sync(t.Context(), config, secrets, initial, nil); err != nil {
		t.Fatal(err)
	}
	saveSyncFixture(t, s, config, secrets)
	// As after an update from a version without the record.
	s.mu.Lock()
	s.prefs.ThemeStyleFallbacks = nil
	if err := s.savePreferences(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()

	style := ThemeStyleMonochrome
	must(t, s, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{ThemeStyle: &style}})
	must(t, s, Request{Action: "config-sync-now"})
	remote, err := engine.Read(t.Context(), config, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if string(remote.Values["desktop.themeStyle"]) != `"aurora"` {
		t.Fatalf("a browser edit stopped desktop fallback inference: %s", remote.Values["desktop.themeStyle"])
	}
	if string(remote.Values["browser.themeStyle"]) != `"monochrome"` {
		t.Fatalf("browser edit was not uploaded: %s", remote.Values["browser.themeStyle"])
	}
}

// A failed switch to another sync space restores the fallback record with
// the rest of the preferences, so the active space keeps its style.
func TestConfigSyncFailedActivationRestoresThemeStyleFallbacks(t *testing.T) {
	s, memory, engine, activeConfig, secrets, _ := newSyncAppSpace(t)
	if _, err := engine.Sync(t.Context(), activeConfig, secrets, syncTestValues("desktop.themeStyle", `"aurora"`), nil); err != nil {
		t.Fatal(err)
	}
	saveSyncFixture(t, s, activeConfig, secrets)
	if s.prefs.ThemeStyleFallbacks == nil || s.prefs.ThemeStyleFallbacks.Keys["desktop.themeStyle"] != "aurora" {
		t.Fatalf("fallback was not recorded: %+v", s.prefs.ThemeStyleFallbacks)
	}

	candidateBucket := &syncTestBucket{}
	server := httptest.NewServer(http.HandlerFunc(candidateBucket.serve))
	t.Cleanup(server.Close)
	candidate := activeConfig
	candidate.Endpoint, candidate.Prefix = server.URL, "candidate-space"
	if _, err := engine.Sync(t.Context(), candidate, secrets, syncTestValues("desktop.themeStyle", `"onevra"`), nil); err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(s.directory, "config-sync.json")
	backupPath := filepath.Join(s.directory, "saved-config-sync.json")
	restore := func() {
		if info, err := os.Stat(metadataPath); err == nil && info.IsDir() {
			_ = os.Remove(metadataPath)
			_ = os.Rename(backupPath, metadataPath)
		}
	}
	t.Cleanup(restore)
	memory.mu.Lock()
	memory.saveHook = func() {
		if err := os.Rename(metadataPath, backupPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(metadataPath, 0700); err != nil {
			t.Fatal(err)
		}
	}
	memory.mu.Unlock()
	if s.Execute(Request{Action: "config-sync-save", SyncConnection: &SyncConnectionRequest{Provider: "s3", Config: candidate, Secrets: secrets}}).Status != "error" {
		t.Fatal("failed activation reported success")
	}
	memory.mu.Lock()
	memory.saveHook = nil
	memory.mu.Unlock()
	restore()
	if f := s.prefs.ThemeStyleFallbacks; f == nil || f.Keys["desktop.themeStyle"] != "aurora" {
		t.Fatalf("failed activation lost the fallback record: %+v", f)
	}

	must(t, s, Request{Action: "config-sync-now"})
	remote, err := engine.Read(t.Context(), activeConfig, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if string(remote.Values["desktop.themeStyle"]) != `"aurora"` {
		t.Fatalf("fallback was uploaded after a failed activation: %s", remote.Values["desktop.themeStyle"])
	}
}
