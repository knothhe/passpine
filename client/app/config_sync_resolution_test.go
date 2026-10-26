package app

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/knothhe/passpine/client/cloudsync"
)

// Each case reaches a real three-way conflict through the isolated S3 fixture.
// The browser namespace first receives its own persisted initial baseline.
func resolutionConflictFixture(t *testing.T, namespace string) (*Service, *cloudsync.Engine, cloudsync.Config, cloudsync.Secrets, *syncTestBucket) {
	t.Helper()
	s, _, engine, config, secrets, bucket := newSyncAppSpace(t)
	saveSyncFixture(t, s, config, secrets)
	if namespace == "browser" {
		theme := "system"
		must(t, s, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{Theme: &theme}})
		must(t, s, Request{Action: "config-sync-now"})
	}
	base, err := engine.Read(t.Context(), config, secrets)
	if err != nil {
		t.Fatal(err)
	}
	setResolutionTheme(t, s, namespace, "light")
	remote := copySyncValues(base.Values)
	remote[namespace+".theme"] = json.RawMessage(`"dark"`)
	if _, err = engine.Sync(t.Context(), config, secrets, remote, base.Values); err != nil {
		t.Fatal(err)
	}
	result := s.Execute(Request{Action: "config-sync-now"})
	if result.Status != "error" || result.Code != "conflict" {
		t.Fatalf("fixture did not create %s conflict: %s/%s", namespace, result.Status, result.Code)
	}
	return s, engine, config, secrets, bucket
}

func setResolutionTheme(t *testing.T, s *Service, namespace, theme string) {
	t.Helper()
	if namespace == "browser" {
		must(t, s, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{Theme: &theme}})
		return
	}
	data := must(t, s, Request{Action: "config-sync-status"})
	settings := data.Settings
	settings.Theme = theme
	must(t, s, Request{Action: "settings", Settings: &settings})
}

func resolvedTheme(t *testing.T, data Data, namespace string) string {
	t.Helper()
	if namespace == "desktop" {
		return data.Settings.Theme
	}
	if data.SyncedBrowserPreferences == nil || data.SyncedBrowserPreferences.Theme == nil {
		t.Fatal("resolved browser preferences are missing")
	}
	return *data.SyncedBrowserPreferences.Theme
}

func TestConfigSyncRemoteResolutionAppliesAndDoesNotReuploadLosingValue(t *testing.T) {
	for _, namespace := range []string{"desktop", "browser"} {
		t.Run(namespace, func(t *testing.T) {
			s, engine, config, secrets, bucket := resolutionConflictFixture(t, namespace)
			_, beforePuts, _ := bucket.snapshot()
			data := must(t, s, Request{Action: "config-sync-resolve", SyncConnection: &SyncConnectionRequest{Resolution: "remote"}})
			if got := resolvedTheme(t, data, namespace); got != "dark" {
				t.Fatalf("cloud resolution kept losing local %s theme %q", namespace, got)
			}
			data = must(t, s, Request{Action: "config-sync-now"})
			if got := resolvedTheme(t, data, namespace); got != "dark" {
				t.Fatalf("next sync restored losing local theme %q", got)
			}
			remote, err := engine.Read(t.Context(), config, secrets)
			if err != nil {
				t.Fatal(err)
			}
			if string(remote.Values[namespace+".theme"]) != `"dark"` {
				t.Fatalf("next sync overwrote selected cloud value: %s", remote.Values[namespace+".theme"])
			}
			_, afterPuts, _ := bucket.snapshot()
			if afterPuts != beforePuts {
				t.Fatal("choosing cloud or a no-op follow-up sync uploaded the losing local value")
			}
		})
	}
}

func TestConfigSyncRemoteResolutionPreservesNewLocalEditWhileReadingCloud(t *testing.T) {
	for _, namespace := range []string{"desktop", "browser"} {
		t.Run(namespace, func(t *testing.T) {
			s, engine, config, secrets, bucket := resolutionConflictFixture(t, namespace)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			t.Cleanup(func() { once.Do(func() { close(release) }) })
			bucket.mu.Lock()
			bucket.getHook = func() { close(entered); <-release }
			bucket.mu.Unlock()
			done := make(chan Response, 1)
			go func() {
				done <- s.Execute(Request{Action: "config-sync-resolve", SyncConnection: &SyncConnectionRequest{Resolution: "remote"}})
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("resolution did not reach isolated cloud read")
			}
			setResolutionTheme(t, s, namespace, "system")
			once.Do(func() { close(release) })
			var result Response
			select {
			case result = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("resolution did not finish after cloud read")
			}
			if result.Status != "ok" {
				t.Fatalf("resolution failed: %s", result.Code)
			}
			if got := resolvedTheme(t, result.Data, namespace); got != "system" {
				t.Fatalf("resolution erased genuine in-flight edit %q", got)
			}
			remote, err := engine.Read(context.Background(), config, secrets)
			if err != nil {
				t.Fatal(err)
			}
			if string(remote.Values[namespace+".theme"]) != `"dark"` {
				t.Fatal("in-flight local edit was mixed into the earlier resolution request")
			}
			must(t, s, Request{Action: "config-sync-now"})
			remote, err = engine.Read(t.Context(), config, secrets)
			if err != nil {
				t.Fatal(err)
			}
			if string(remote.Values[namespace+".theme"]) != `"system"` {
				t.Fatal("genuine in-flight edit was not retained for the next sync")
			}
		})
	}
}
