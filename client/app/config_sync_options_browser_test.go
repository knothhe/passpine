package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigSyncOptionsBrowserWireDisablesBridgeAndRejectsStalePatch(t *testing.T) {
	s, _, _, config, secrets, _ := newSyncAppSpace(t)
	data := saveSyncFixture(t, s, config, secrets)
	theme := "light"
	must(t, s, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{Theme: &theme}})
	options := DefaultConfigSyncOptions()
	options.Browser = false
	must(t, s, Request{Action: "config-sync-options", SyncOptions: &options})
	before, err := os.ReadFile(filepath.Join(s.directory, "preferences.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, patch := range []bool{false, true} {
		r := Request{Action: "config-sync-browser"}
		if patch {
			dark := "dark"
			r.SyncSpaceID = data.ConfigSync.SpaceID
			r.SyncPreferences = &BrowserSyncPreferences{Theme: &dark}
		}
		result, encoded := browserSyncWire(t, s, r)
		if patch && (result.Status != "error" || result.Code != "not_ready") || !patch && result.Status != "ok" {
			t.Fatalf("unexpected disabled browser result: patch=%t status=%s code=%s", patch, result.Status, result.Code)
		}
		if result.Data.ConfigSync == nil || result.Data.ConfigSync.Enabled || result.Data.ConfigSync.Ready || result.Data.ConfigSync.SpaceID != data.ConfigSync.SpaceID || result.Data.SyncedBrowserPreferences != nil {
			t.Fatal("disabled bridge exposed browser preferences or lost its opaque space identity")
		}
		var wire struct {
			Data struct {
				ConfigSync map[string]json.RawMessage `json:"configSync"`
			} `json:"data"`
		}
		if err := json.Unmarshal(encoded, &wire); err != nil {
			t.Fatal(err)
		}
		if len(wire.Data.ConfigSync) != 4 || wire.Data.ConfigSync["enabled"] == nil || wire.Data.ConfigSync["ready"] == nil || wire.Data.ConfigSync["spaceId"] == nil || string(wire.Data.ConfigSync["fields"]) != "[]" {
			t.Fatalf("browser status exposed desktop synchronization options or connection details: %s", encoded)
		}
		for _, private := range []string{config.Endpoint, config.Bucket, config.Prefix, secrets.AccessKeyID, secrets.SecretAccessKey, secrets.Passphrase} {
			if bytes.Contains(encoded, []byte(private)) {
				t.Fatal("disabled browser response exposed a private connection value")
			}
		}
	}
	after, err := os.ReadFile(filepath.Join(s.directory, "preferences.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("a disabled browser request changed persistent preferences", err)
	}
	desktop := must(t, s, Request{Action: "config-sync-status"})
	if !desktop.ConfigSync.Enabled || !desktop.ConfigSync.Ready || desktop.ConfigSync.Options.Browser || *desktop.SyncedBrowserPreferences.Theme != "light" {
		t.Fatal("browser-only disable changed the desktop connection or cached preference")
	}
}

func TestConfigSyncOptionsBrowserReenableAfterRestartImportsUnappliedRemoteValues(t *testing.T) {
	for _, initialExcluded := range []bool{false, true} {
		name := "previously-synced"
		if initialExcluded {
			name = "excluded-on-first-connection"
		}
		t.Run(name, func(t *testing.T) {
			s, memory, engine, config, secrets, _ := newSyncAppSpace(t)
			options := DefaultConfigSyncOptions()
			if initialExcluded {
				options.Browser = false
				must(t, s, Request{Action: "config-sync-options", SyncOptions: &options})
			}
			saveSyncFixture(t, s, config, secrets)
			if !initialExcluded {
				theme := "light"
				must(t, s, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{Theme: &theme}})
				must(t, s, Request{Action: "config-sync-now"})
				options.Browser = false
				must(t, s, Request{Action: "config-sync-options", SyncOptions: &options})
			}
			base, err := engine.Read(t.Context(), config, secrets)
			if err != nil {
				t.Fatal(err)
			}
			remote := copySyncValues(base.Values)
			for key, value := range syncTestValues("browser.theme", `"dark"`, "browser.themeStyle", `"onevra"`, "browser.fontScale", `120`, "browser.language", `"zh-CN"`) {
				remote[key] = value
			}
			if _, err = engine.Sync(t.Context(), config, secrets, remote, base.Values); err != nil {
				t.Fatal(err)
			}
			disabled := must(t, s, Request{Action: "config-sync-now"})
			if initialExcluded && disabled.SyncedBrowserPreferences != nil || !initialExcluded && (disabled.SyncedBrowserPreferences == nil || *disabled.SyncedBrowserPreferences.Theme != "light") {
				t.Fatal("disabled synchronization imported remote browser values")
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened := newSyncServiceFixture(t, s.directory, memory, engine)
			status := must(t, reopened, Request{Action: "config-sync-status"})
			if status.ConfigSync.Options.Browser {
				t.Fatal("restart lost the disabled browser option")
			}
			options.Browser = true
			must(t, reopened, Request{Action: "config-sync-options", SyncOptions: &options})
			data := must(t, reopened, Request{Action: "config-sync-now"})
			if data.SyncedBrowserPreferences == nil || *data.SyncedBrowserPreferences.Theme != "dark" || *data.SyncedBrowserPreferences.ThemeStyle != "onevra" || *data.SyncedBrowserPreferences.FontScale != 120 || *data.SyncedBrowserPreferences.Language != "zh-CN" {
				t.Fatal("reenabling browser synchronization lost the unapplied remote values")
			}
			wire, _ := browserSyncWire(t, reopened, Request{Action: "config-sync-browser"})
			if wire.Status != "ok" || wire.Data.ConfigSync == nil || !wire.Data.ConfigSync.Enabled || !wire.Data.ConfigSync.Ready || wire.Data.SyncedBrowserPreferences == nil || *wire.Data.SyncedBrowserPreferences.Theme != "dark" {
				t.Fatal("reenabled bridge did not expose its current imported preferences")
			}
			must(t, reopened, Request{Action: "config-sync-now"})
			cloud, err := engine.Read(t.Context(), config, secrets)
			if err != nil || string(cloud.Values["browser.theme"]) != `"dark"` {
				t.Fatal("a subsequent synchronization restored the old cached browser preference", err)
			}
		})
	}
}

func TestConfigSyncOptionsBrowserCannotInjectLocalOptions(t *testing.T) {
	s, _, _, config, secrets, _ := newSyncAppSpace(t)
	saveSyncFixture(t, s, config, secrets)
	before, err := os.ReadFile(filepath.Join(s.directory, "config-sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	options := DefaultConfigSyncOptions()
	options.IntervalMinutes, options.Desktop = 60, false
	for _, action := range []string{"config-sync-options", "config-sync-browser", "list"} {
		result, _ := browserSyncWire(t, s, Request{Action: action, SyncOptions: &options})
		if result.Status != "error" || result.Code != "unsupported_action" && result.Code != "invalid_request" {
			t.Fatalf("browser injected local options via %s: %s/%s", action, result.Status, result.Code)
		}
	}
	after, err := os.ReadFile(filepath.Join(s.directory, "config-sync.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejected browser option injection rewrote local options", err)
	}
}

func TestConfigSyncOptionsBrowserNewlyEnabledMissingFieldsWaitForExtensionValues(t *testing.T) {
	s, _, engine, config, secrets, _ := newSyncAppSpace(t)
	if _, err := engine.Sync(t.Context(), config, secrets, syncTestValues("browser.language", `"en"`), nil); err != nil {
		t.Fatal(err)
	}
	options := DefaultConfigSyncOptions()
	options.BrowserFields = SyncFieldSelection{Language: true}
	must(t, s, Request{Action: "config-sync-options", SyncOptions: &options})
	saveSyncFixture(t, s, config, secrets)
	options.BrowserFields = DefaultConfigSyncOptions().BrowserFields
	must(t, s, Request{Action: "config-sync-options", SyncOptions: &options})
	read, _ := browserSyncWire(t, s, Request{Action: "config-sync-browser"})
	if read.Data.SyncedBrowserPreferences == nil || read.Data.SyncedBrowserPreferences.Theme != nil || read.Data.SyncedBrowserPreferences.ThemeStyle != nil || read.Data.SyncedBrowserPreferences.FontScale != nil || *read.Data.SyncedBrowserPreferences.Language != "en" {
		t.Fatal("newly enabled absent fields exposed synthetic client defaults to the extension")
	}
	must(t, s, Request{Action: "config-sync-now"})
	remote, err := engine.Read(t.Context(), config, secrets)
	if err != nil || remote.Values["browser.theme"] != nil || remote.Values["browser.themeStyle"] != nil || remote.Values["browser.fontScale"] != nil {
		t.Fatal("newly enabled absent fields uploaded synthetic defaults before the extension supplied its preferences", err)
	}
	theme, style, scale := "light", "onevra", 120
	patch, _ := browserSyncWire(t, s, Request{Action: "config-sync-browser", SyncSpaceID: read.Data.ConfigSync.SpaceID, SyncPreferences: &BrowserSyncPreferences{Theme: &theme, ThemeStyle: &style, FontScale: &scale}})
	if patch.Status != "ok" || patch.Data.SyncedBrowserPreferences == nil || patch.Data.SyncedBrowserPreferences.Theme == nil || *patch.Data.SyncedBrowserPreferences.Theme != theme {
		t.Fatal("newly enabled fields did not accept the actual extension preferences")
	}
	must(t, s, Request{Action: "config-sync-now"})
	remote, err = engine.Read(t.Context(), config, secrets)
	if err != nil || string(remote.Values["browser.theme"]) != `"light"` || string(remote.Values["browser.themeStyle"]) != `"onevra"` || string(remote.Values["browser.fontScale"]) != "120" {
		t.Fatal("newly enabled fields did not upload the supplied extension preferences", err)
	}
}
