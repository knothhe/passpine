package app

import (
	"testing"

	"github.com/knothhe/passpine/client/secretstore"
)

func TestConfigSyncExplicitSavePreservesPendingBaselineAndNewLocalEdit(t *testing.T) {
	for _, failure := range []struct {
		name string
		err  error
	}{
		{name: "unavailable", err: secretstore.ErrUnavailable},
		{name: "denied", err: secretstore.ErrDenied},
	} {
		t.Run(failure.name, func(t *testing.T) {
			s, _, backend, config, secrets := newSyncSecretSessionFixture(t)
			data := saveSyncFixture(t, s, config, secrets)
			originalTheme := data.Settings.Theme
			settings := data.Settings
			settings.Theme = "dark"
			must(t, s, Request{Action: "settings", Settings: &settings})
			backend.mu.Lock()
			backend.failNextSave = failure.err
			backend.mu.Unlock()
			failed := s.Execute(Request{Action: "config-sync-now"})
			if failed.Code != syncErrorCode(failure.err) || len(s.configSync.pendingSecrets) != 1 {
				t.Fatalf("completed upload did not retain its unsaved baseline: %s", failed.Code)
			}
			// Returning to the older persisted value is a new local edit. The
			// pending baseline must identify the cloud's dark value as unchanged.
			settings.Theme = originalTheme
			must(t, s, Request{Action: "settings", Settings: &settings})
			reconnected := s.Execute(Request{Action: "config-sync-save", SyncConnection: &SyncConnectionRequest{Provider: "s3", Config: config}})
			if reconnected.Status != "ok" {
				t.Fatalf("reconnect failed: %s", reconnected.Code)
			}
			if reconnected.Data.Settings.Theme != originalTheme {
				t.Fatalf("new local edit lost: want %q got %q", originalTheme, reconnected.Data.Settings.Theme)
			}
		})
	}
}
