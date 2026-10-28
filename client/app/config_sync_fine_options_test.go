package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestConfigSyncOptionsMigrateEarlierFilesAndPreserveExplicitChoices(t *testing.T) {
	for _, raw := range []string{
		`{"intervalMinutes":15,"syncOnChange":false,"desktop":true,"browser":false}`,
		`{"intervalMinutes":15,"syncOnChange":false,"desktop":true,"browser":false,"desktopFields":{"language":false}}`,
	} {
		var options ConfigSyncOptions
		if err := json.Unmarshal([]byte(raw), &options); err != nil {
			t.Fatal(err)
		}
		if options.Mode != "automatic" || !options.SyncOnStart || !options.NotifyErrors || options.IntervalMinutes != 15 || options.SyncOnChange || !options.Desktop || options.Browser || !options.DesktopFields.Appearance || !options.DesktopFields.FontScale || !options.BrowserFields.Language {
			t.Fatalf("legacy options did not inherit new defaults: %+v", options)
		}
	}
	s, memory, engine, _, _, _ := newSyncAppSpace(t)
	legacy := []byte(`{"version":1,"enabled":false,"profiles":{},"options":{"intervalMinutes":15,"syncOnChange":false,"desktop":true,"browser":false}}`)
	if err := os.WriteFile(filepath.Join(s.directory, "config-sync.json"), legacy, 0600); err != nil {
		t.Fatal(err)
	}
	reopened := newSyncServiceFixture(t, s.directory, memory, engine)
	options := must(t, reopened, Request{Action: "config-sync-status"}).ConfigSync.Options
	if options != (ConfigSyncOptions{Mode: "automatic", IntervalMinutes: 15, SyncOnStart: true, NotifyErrors: true, Desktop: true, DesktopFields: SyncFieldSelection{true, true, true}, BrowserFields: SyncFieldSelection{true, true, true}}) {
		t.Fatalf("saved legacy file did not migrate: %+v", options)
	}
	options.Mode, options.SyncOnStart, options.NotifyErrors = "manual", false, false
	options.DesktopFields, options.BrowserFields = SyncFieldSelection{}, SyncFieldSelection{}
	saveSyncOptionsFixture(t, reopened, options)
	third := newSyncServiceFixture(t, s.directory, memory, engine)
	if got := must(t, third, Request{Action: "config-sync-status"}).ConfigSync.Options; got != options {
		t.Fatalf("explicit false choices were replaced by defaults: %+v", got)
	}
	invalid := options
	invalid.Mode = "unknown"
	if out := reopened.Execute(Request{Action: "config-sync-options", SyncOptions: &invalid}); out.Code != "invalid_sync_options" || out.Data.ConfigSync.Options != options {
		t.Fatal("invalid scheduling mode changed saved options")
	}
}

func TestConfigSyncManualModeStagesBrowserAfterRestartWithoutAutomaticCloudAccess(t *testing.T) {
	s, memory, engine, config, secrets, bucket := newSyncAppSpace(t)
	saveSyncFixture(t, s, config, secrets)
	options := DefaultConfigSyncOptions()
	options.Mode = "manual"
	saveSyncOptionsFixture(t, s, options)
	reopened := newSyncServiceFixture(t, s.directory, memory, engine)
	if !must(t, reopened, Request{Action: "config-sync-status"}).ConfigSync.Ready {
		t.Fatal("manual bridge was not ready after restart")
	}
	_, _, requests := bucket.snapshot()
	memory.mu.Lock()
	loads := memory.loads
	memory.mu.Unlock()
	reopened.configSync.next = time.Time{}
	reopened.scheduleConfigSync()
	if reopened.configSync.queued {
		t.Fatal("manual mode queued automatic synchronization")
	}
	// Re-check mode after acquiring the lane too: an already queued worker
	// must not reach cloud storage after manual mode has been saved.
	out := reopened.executeConfigSync(Request{Action: "config-sync-now"}, "automatic")
	if out.Status != "ok" || out.Data.ConfigSync.AttemptSequence != 0 {
		t.Fatal("a skipped automatic worker counted as a cloud attempt")
	}
	font := 120
	patch, _ := browserSyncWire(t, reopened, Request{Action: "config-sync-browser", SyncSpaceID: out.Data.ConfigSync.SpaceID, SyncPreferences: &BrowserSyncPreferences{FontScale: &font}})
	if patch.Status != "ok" || patch.Data.SyncedBrowserPreferences == nil || *patch.Data.SyncedBrowserPreferences.FontScale != font || !reopened.configSync.dirty {
		t.Fatalf("manual mode did not stage browser preferences: %s", patch.Code)
	}
	if _, _, after := bucket.snapshot(); after != requests {
		t.Fatal("manual mode contacted cloud storage automatically")
	}
	memory.mu.Lock()
	afterLoads := memory.loads
	memory.mu.Unlock()
	if afterLoads != loads {
		t.Fatal("manual mode loaded sync credentials automatically")
	}
	must(t, reopened, Request{Action: "config-sync-now"})
	remote, err := engine.Read(t.Context(), config, secrets)
	if err != nil || string(remote.Values["browser.fontScale"]) != "120" {
		t.Fatal("manual cloud sync did not upload staged browser preferences", err)
	}
}

func TestConfigSyncStartupDisabledWaitsIntervalAndDuplicateBridgePatchDoesNotExpedite(t *testing.T) {
	s, memory, engine, config, secrets, bucket := newSyncAppSpace(t)
	saveSyncFixture(t, s, config, secrets)
	font := 110
	must(t, s, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{FontScale: &font}})
	must(t, s, Request{Action: "config-sync-now"})
	options := DefaultConfigSyncOptions()
	options.SyncOnStart, options.IntervalMinutes = false, 15
	saveSyncOptionsFixture(t, s, options)
	before := time.Now()
	reopened := newSyncServiceFixture(t, s.directory, memory, engine)
	next := reopened.configSync.next
	if delay := next.Sub(before); delay < 15*time.Minute || delay > 15*time.Minute+5*time.Second {
		t.Fatalf("disabled startup did not schedule from launch time: %v", delay)
	}
	_, _, requests := bucket.snapshot()
	reopened.scheduleConfigSync()
	if reopened.configSync.queued {
		t.Fatal("disabled startup queued an immediate cloud check")
	}
	must(t, reopened, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{FontScale: &font}})
	if !reopened.configSync.next.Equal(next) || reopened.configSync.dirty {
		t.Fatal("a repeated bridge snapshot bypassed disabled startup scheduling")
	}
	if _, _, after := bucket.snapshot(); after != requests {
		t.Fatal("disabled startup contacted cloud storage")
	}
	// Actual edits still respect the separate sync-on-change setting.
	font = 120
	must(t, reopened, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{FontScale: &font}})
	if delay := time.Until(reopened.configSync.next); delay < 0 || delay > 3*time.Second {
		t.Fatal("a real preference edit failed to schedule sync on change")
	}
}

func TestConfigSyncGranularSelectionsPreserveUnselectedDataAndImportNewlyEnabledFields(t *testing.T) {
	s, memory, engine, config, secrets, _ := newSyncAppSpace(t)
	remoteValues := syncTestValues("desktop.theme", `"dark"`, "desktop.themeStyle", `"onevra"`, "desktop.fontScale", "120", "desktop.language", `"zh-CN"`, "browser.theme", `"dark"`, "browser.themeStyle", `"onevra"`, "browser.fontScale", "120", "browser.language", `"zh-CN"`, "future.setting", "true")
	if _, err := engine.Sync(t.Context(), config, secrets, remoteValues, nil); err != nil {
		t.Fatal(err)
	}
	options := DefaultConfigSyncOptions()
	options.DesktopFields = SyncFieldSelection{Appearance: true}
	options.BrowserFields = SyncFieldSelection{FontScale: true}
	saveSyncOptionsFixture(t, s, options)
	data := saveSyncFixture(t, s, config, secrets)
	if data.Settings.Theme != "dark" || data.Settings.ThemeStyle != "onevra" || data.Settings.FontScale != 100 || data.Settings.Language != "system" || data.SyncedBrowserPreferences == nil || data.SyncedBrowserPreferences.Theme != nil || *data.SyncedBrowserPreferences.FontScale != 120 || data.SyncedBrowserPreferences.Language != nil {
		t.Fatal("fine selections applied unselected remote fields")
	}
	wire, _ := browserSyncWire(t, s, Request{Action: "config-sync-browser"})
	if !reflect.DeepEqual(wire.Data.ConfigSync.Fields, []string{"fontScale"}) || wire.Data.SyncedBrowserPreferences == nil || wire.Data.SyncedBrowserPreferences.Theme != nil || wire.Data.SyncedBrowserPreferences.Language != nil || *wire.Data.SyncedBrowserPreferences.FontScale != 120 {
		t.Fatal("browser bridge did not project selected fields only")
	}
	settings := data.Settings
	settings.Theme, settings.FontScale, settings.Language = "light", 80, "en"
	must(t, s, Request{Action: "settings", Settings: &settings})
	font, theme := 110, "light"
	stale, _ := browserSyncWire(t, s, Request{Action: "config-sync-browser", SyncSpaceID: data.ConfigSync.SpaceID, SyncPreferences: &BrowserSyncPreferences{FontScale: &font, Theme: &theme}})
	if stale.Status != "error" || stale.Code != "not_ready" || *s.configSync.browser.FontScale != 120 {
		t.Fatal("a stale mixed patch changed selected or unselected cached preferences")
	}
	must(t, s, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{FontScale: &font}})
	must(t, s, Request{Action: "config-sync-now"})
	remote, err := engine.Read(t.Context(), config, secrets)
	if err != nil || string(remote.Values["desktop.theme"]) != `"light"` || string(remote.Values["desktop.fontScale"]) != "120" || string(remote.Values["desktop.language"]) != `"zh-CN"` || string(remote.Values["browser.fontScale"]) != "110" || string(remote.Values["browser.theme"]) != `"dark"` || string(remote.Values["browser.language"]) != `"zh-CN"` || string(remote.Values["future.setting"]) != "true" {
		t.Fatal("sync changed unselected remote fields", err)
	}
	reopened := newSyncServiceFixture(t, s.directory, memory, engine)
	options.DesktopFields.FontScale, options.BrowserFields.Appearance = true, true
	saveSyncOptionsFixture(t, reopened, options)
	data = must(t, reopened, Request{Action: "config-sync-now"})
	if data.Settings.FontScale != 120 || data.Settings.Language != "en" || data.SyncedBrowserPreferences == nil || *data.SyncedBrowserPreferences.Theme != "dark" || *data.SyncedBrowserPreferences.FontScale != 110 || data.SyncedBrowserPreferences.Language != nil {
		t.Fatal("first enabling individual fields after restart did not import just those cloud fields")
	}
}

func TestConfigSyncGranularExcludedBaselineRetainsConflictsAfterRestart(t *testing.T) {
	s, memory, engine, config, secrets, _ := newSyncAppSpace(t)
	data := saveSyncFixture(t, s, config, secrets)
	options := DefaultConfigSyncOptions()
	options.DesktopFields.Language = false
	saveSyncOptionsFixture(t, s, options)
	remote, err := engine.Read(t.Context(), config, secrets)
	if err != nil {
		t.Fatal(err)
	}
	edit := copySyncValues(remote.Values)
	edit["desktop.language"] = jsonValue("zh-CN")
	if _, err = engine.Sync(t.Context(), config, secrets, edit, remote.Values); err != nil {
		t.Fatal(err)
	}
	settings := data.Settings
	settings.Language = "en"
	must(t, s, Request{Action: "settings", Settings: &settings})
	must(t, s, Request{Action: "config-sync-now"})
	reopened := newSyncServiceFixture(t, s.directory, memory, engine)
	options.DesktopFields.Language = true
	saveSyncOptionsFixture(t, reopened, options)
	out := reopened.Execute(Request{Action: "config-sync-now"})
	if out.Status != "error" || out.Code != "conflict" || !reflect.DeepEqual(out.Data.ConfigSync.ConflictKeys, []string{"desktop.language"}) || out.Data.Settings.Language != "en" {
		t.Fatal("fine selection lost independent local and cloud changes")
	}
}

func TestConfigSyncLegacyPendingNamespaceMigratesToIndividualFirstImports(t *testing.T) {
	s, memory, engine, config, secrets, _ := newSyncAppSpace(t)
	if _, err := engine.Sync(t.Context(), config, secrets, syncTestValues("desktop.theme", `"dark"`, "desktop.language", `"zh-CN"`), nil); err != nil {
		t.Fatal(err)
	}
	options := DefaultConfigSyncOptions()
	options.Desktop = false
	saveSyncOptionsFixture(t, s, options)
	saveSyncFixture(t, s, config, secrets)
	profile := s.configSync.meta.Profiles["s3"]
	stored, err := s.loadSyncSecret(profile.SecretRef)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a credential record written by the earlier namespace-only
	// implementation, before per-field pending markers existed.
	stored.PendingDesktop, stored.PendingKeys = true, nil
	if err := s.storeSyncSecret(profile.SecretRef, stored); err != nil {
		t.Fatal(err)
	}
	reopened := newSyncServiceFixture(t, s.directory, memory, engine)
	options.Desktop, options.DesktopFields = true, SyncFieldSelection{Language: true}
	saveSyncOptionsFixture(t, reopened, options)
	data := must(t, reopened, Request{Action: "config-sync-now"})
	if data.Settings.Language != "zh-CN" || data.Settings.Theme != "system" {
		t.Fatal("legacy pending namespace did not import just the enabled field")
	}
	options.DesktopFields.Appearance = true
	saveSyncOptionsFixture(t, reopened, options)
	data = must(t, reopened, Request{Action: "config-sync-now"})
	if data.Settings.Theme != "dark" || data.Settings.Language != "zh-CN" {
		t.Fatal("legacy pending namespace lost the remaining field's first import")
	}
}

func TestConfigSyncPartialBrowserDocumentAppliesOnlyPresentSelectedFields(t *testing.T) {
	s, _, engine, config, secrets, _ := newSyncAppSpace(t)
	if _, err := engine.Sync(t.Context(), config, secrets, syncTestValues("browser.language", `"zh-CN"`), nil); err != nil {
		t.Fatal(err)
	}
	options := DefaultConfigSyncOptions()
	options.BrowserFields = SyncFieldSelection{Language: true}
	saveSyncOptionsFixture(t, s, options)
	data := saveSyncFixture(t, s, config, secrets)
	if data.SyncedBrowserPreferences == nil || *data.SyncedBrowserPreferences.Language != "zh-CN" || data.SyncedBrowserPreferences.FontScale != nil || data.SyncedBrowserPreferences.Theme != nil {
		t.Fatal("partial browser document did not preserve absent cached preferences")
	}
	remote, err := engine.Read(t.Context(), config, secrets)
	if err != nil || remote.Values["browser.theme"] != nil || remote.Values["browser.fontScale"] != nil {
		t.Fatal("partial browser document uploaded unselected defaults", err)
	}
}

func TestConfigSyncAttemptEventsCountCloudAttemptsWithoutPollingOrLocalChanges(t *testing.T) {
	s, memory, _, config, secrets, _ := newSyncAppSpace(t)
	data := saveSyncFixture(t, s, config, secrets)
	if data.ConfigSync.AttemptSequence != 1 || data.ConfigSync.AttemptErrorCode != "" {
		t.Fatal("successful initial connection did not publish an attempt event")
	}
	options := DefaultConfigSyncOptions()
	options.NotifyErrors = false
	for _, request := range []Request{{Action: "config-sync-status"}, {Action: "config-sync-options", SyncOptions: &options}, {Action: "config-sync-test", SyncConnection: &SyncConnectionRequest{Provider: "s3", Config: config, Secrets: secrets}}} {
		if got := must(t, s, request).ConfigSync; got.AttemptSequence != 1 || got.AttemptErrorCode != "" {
			t.Fatal("polling, local save or read-only test changed attempt event")
		}
	}
	memory.mu.Lock()
	memory.loadErr = os.ErrPermission
	memory.mu.Unlock()
	out := s.Execute(Request{Action: "config-sync-now"})
	if out.Status != "error" || out.Data.ConfigSync.AttemptSequence != 2 || out.Data.ConfigSync.AttemptErrorCode != out.Code || out.Data.ConfigSync.AttemptErrorCode == "" {
		t.Fatal("failed cloud attempt did not publish a controlled error event")
	}
	failure := out.Data.ConfigSync.AttemptErrorCode
	// Successful non-attempt actions may clear current presentation errors;
	// they must not erase the failure that the UI still needs to observe.
	must(t, s, Request{Action: "config-sync-pause"})
	status := must(t, s, Request{Action: "config-sync-status"}).ConfigSync
	if status.AttemptSequence != 2 || status.AttemptErrorCode != failure {
		t.Fatal("non-attempt action erased the latest cloud event")
	}
	memory.mu.Lock()
	memory.loadErr = nil
	memory.mu.Unlock()
	must(t, s, Request{Action: "config-sync-resume"})
	status = must(t, s, Request{Action: "config-sync-now"}).ConfigSync
	if status.AttemptSequence != 3 || status.AttemptErrorCode != "" || status.Options.NotifyErrors {
		t.Fatal("successful recovery did not clear the cloud error event or changed notification preference")
	}
}
