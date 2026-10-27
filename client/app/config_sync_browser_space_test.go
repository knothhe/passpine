package app

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func browserSyncWire(t *testing.T, s *Service, request Request) (Response, []byte) {
	t.Helper()
	request.Protocol = 1
	request.RequestID = "browser-space-regression"
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	encoded := s.handleBrowser(raw)
	var result Response
	if err = json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.RequestID != request.RequestID {
		t.Fatal("native response lost request identity")
	}
	return result, encoded
}

func TestConfigSyncBrowserWireRejectsMissingAndStaleSpaceWithoutChangingPreferences(t *testing.T) {
	s, _, _, config, secrets, _ := newSyncAppSpace(t)
	first := saveSyncFixture(t, s, config, secrets)
	read, encoded := browserSyncWire(t, s, Request{Action: "config-sync-browser"})
	if read.Status != "ok" || read.Data.ConfigSync == nil || read.Data.ConfigSync.SpaceID != first.ConfigSync.SpaceID {
		t.Fatal("native read did not expose the active opaque space identity")
	}
	oldID := read.Data.ConfigSync.SpaceID
	if decoded, err := hex.DecodeString(oldID); err != nil || len(decoded) != 32 {
		t.Fatal("space identity is not an opaque SHA-256 value")
	}
	for _, private := range []string{config.Endpoint, config.Bucket, config.Prefix, secrets.AccessKeyID, secrets.SecretAccessKey, secrets.Passphrase} {
		if bytes.Contains(encoded, []byte(private)) {
			t.Fatal("native sync status leaked a private connection value")
		}
	}
	dark := "dark"
	current, _ := browserSyncWire(t, s, Request{Action: "config-sync-browser", SyncSpaceID: oldID, SyncPreferences: &BrowserSyncPreferences{Theme: &dark}})
	if current.Status != "ok" || current.Data.SyncedBrowserPreferences == nil || *current.Data.SyncedBrowserPreferences.Theme != dark {
		t.Fatalf("current-space browser patch failed: %s", current.Code)
	}
	secondBucket := &syncTestBucket{}
	server := httptest.NewServer(http.HandlerFunc(secondBucket.serve))
	t.Cleanup(server.Close)
	secondConfig := config
	secondConfig.Endpoint = server.URL
	secondConfig.Prefix = "new-browser-space"
	second := saveSyncFixture(t, s, secondConfig, secrets)
	if second.ConfigSync.SpaceID == oldID || second.ConfigSync.SpaceID == "" {
		t.Fatal("changing the bucket endpoint did not change space identity")
	}
	light := "light"
	must(t, s, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{Theme: &light}})
	before, err := os.ReadFile(filepath.Join(s.directory, "preferences.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, spaceID := range []string{"", oldID, "invalid-space"} {
		result, _ := browserSyncWire(t, s, Request{Action: "config-sync-browser", SyncSpaceID: spaceID, SyncPreferences: &BrowserSyncPreferences{Theme: &dark}})
		if result.Status != "error" || result.Code != "not_ready" {
			t.Fatalf("missing/stale browser space accepted: %q status=%s code=%s", spaceID, result.Status, result.Code)
		}
		if result.Data.SyncedBrowserPreferences == nil || *result.Data.SyncedBrowserPreferences.Theme != light {
			t.Fatal("rejected patch changed the active browser preference")
		}
		after, err := os.ReadFile(filepath.Join(s.directory, "preferences.json"))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("rejected patch rewrote persistent preferences", err)
		}
	}
	accepted, _ := browserSyncWire(t, s, Request{Action: "config-sync-browser", SyncSpaceID: second.ConfigSync.SpaceID, SyncPreferences: &BrowserSyncPreferences{Theme: &dark}})
	if accepted.Status != "ok" || *accepted.Data.SyncedBrowserPreferences.Theme != dark {
		t.Fatalf("new active-space browser patch failed: %s", accepted.Code)
	}
	must(t, s, Request{Action: "config-sync-pause"})
	paused, encoded := browserSyncWire(t, s, Request{Action: "config-sync-browser"})
	if paused.Status != "ok" || paused.Data.ConfigSync == nil || paused.Data.ConfigSync.Enabled || paused.Data.ConfigSync.SpaceID != second.ConfigSync.SpaceID {
		t.Fatal("paused native read lost the saved space identity")
	}
	for _, private := range []string{secondConfig.Endpoint, secondConfig.Bucket, secondConfig.Prefix} {
		if bytes.Contains(encoded, []byte(private)) {
			t.Fatal("paused native status leaked the raw space identity")
		}
	}
	blocked, _ := browserSyncWire(t, s, Request{Action: "config-sync-browser", SyncSpaceID: second.ConfigSync.SpaceID, SyncPreferences: &BrowserSyncPreferences{Theme: &light}})
	if blocked.Status != "error" || blocked.Code != "not_ready" || *blocked.Data.SyncedBrowserPreferences.Theme != dark {
		t.Fatal("paused current-space patch changed browser preferences")
	}
}

func TestConfigSyncBrowserWireSpaceIdentityDoesNotDependOnConnectionMethod(t *testing.T) {
	s, _, _, config, secrets, _ := newSyncAppSpace(t)
	manual := saveSyncFixture(t, s, config, secrets)
	// Only project the same saved data space through the other connection mode;
	// this read-only native action must never discover accounts or load keys.
	s.mu.Lock()
	profile := s.configSync.meta.Profiles["s3"]
	s.configSync.meta.Profiles["r2"] = profile
	s.configSync.meta.Active = "r2"
	s.mu.Unlock()
	result, _ := browserSyncWire(t, s, Request{Action: "config-sync-browser"})
	if result.Status != "ok" || result.Data.ConfigSync == nil || result.Data.ConfigSync.SpaceID != manual.ConfigSync.SpaceID {
		t.Fatal("the same space gained a different identity under OAuth mode")
	}
	s.mu.Lock()
	delete(s.configSync.meta.Profiles, "r2")
	s.configSync.meta.Active = "s3"
	s.mu.Unlock()
}
