package desktop

import (
	"sync"
	"testing"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/r2connect"
)

func TestConfigurationSyncLateSnapshotKeepsNewerCommittedPreferences(t *testing.T) {
	for _, action := range []string{"config-sync-status", "config-sync-now"} {
		t.Run(action, func(t *testing.T) {
			a, callbacks := configurationSyncFixture()
			a.state.Preview = true
			started, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			var backendMu sync.Mutex
			backendSettings := a.settingsSaved
			configReads := 0
			settingsRequests := make(chan app.Settings, 2)
			a.service = backendFunc(func(request app.Request) app.Response {
				if request.Action == "settings" {
					backendMu.Lock()
					backendSettings = *request.Settings
					saved := backendSettings
					backendMu.Unlock()
					settingsRequests <- saved
					return app.Response{Status: "ok", Data: app.Data{Settings: saved}}
				}
				backendMu.Lock()
				configReads++
				first := configReads == 1
				snapshot := backendSettings
				backendMu.Unlock()
				if first {
					close(started)
					<-release
				}
				return app.Response{Status: "ok", Data: app.Data{Settings: snapshot, ConfigSync: &app.ConfigSyncStatus{Provider: "s3", Enabled: true, Ready: true}}}
			})
			a.manageConfigSync(action, nil)
			<-started
			a.settings.Language = "en"
			a.settingsDirty = true
			a.saveSettingsAutomatically()
			<-settingsRequests
			awaitMain(t, callbacks)
			if a.settingsSaved.Language != "en" {
				t.Fatal("new local preference was not committed first")
			}
			releaseOnce.Do(func() { close(release) })
			awaitMain(t, callbacks)
			if a.settings.Language != "en" || a.settingsSaved.Language != "en" || a.state.Settings.Language != "en" {
				t.Fatal("older sync snapshot replaced the newer saved language")
			}
			for a.configSync.operation != "" {
				awaitMain(t, callbacks)
			}
			// An unrelated later edit must not copy an older language back into
			// the service when it saves the complete Settings value.
			a.settings.CloseToTray = !a.settings.CloseToTray
			a.settingsDirty = true
			a.saveSettingsAutomatically()
			awaitMain(t, callbacks)
			if saved := <-settingsRequests; saved.Language != "en" {
				t.Fatal("stale synchronization snapshot was written back by another setting")
			}
		})
	}
}

func TestConfigurationSyncStatusCannotRestoreAnotherDraftsBucketOptions(t *testing.T) {
	a, callbacks := configurationSyncFixture()
	a.configSync.mode = 0
	a.configSync.r2.account = "first-account"
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	a.service = backendFunc(func(request app.Request) app.Response {
		if request.Action == "config-sync-oauth-buckets" {
			close(started)
			<-release
		}
		return app.Response{Status: "ok", Data: app.Data{ConfigSync: &app.ConfigSyncStatus{OAuthAvailable: true, OAuthStage: "bucket", AccountID: "first-account", Jurisdiction: "default", Buckets: []r2connect.Bucket{{Name: "first-account-bucket", Jurisdiction: "default"}}}}}
	})
	a.manageConfigSync("config-sync-oauth-buckets", a.syncConnectionRequest())
	<-started
	a.configSync.r2.account = "second-account"
	// This is the status refresh queued by the service's settings notification
	// while the original bucket request is still in the desktop worker lane.
	a.refreshConfigSync()
	once.Do(func() { close(release) })
	awaitMain(t, callbacks)
	if len(a.configSync.status.Buckets) != 0 {
		t.Fatal("late bucket result followed a different account draft")
	}
	awaitMain(t, callbacks)
	if len(a.configSync.status.Buckets) != 0 || a.configSync.r2.account != "second-account" {
		t.Fatal("follow-up status restored another account's bucket options")
	}
}
