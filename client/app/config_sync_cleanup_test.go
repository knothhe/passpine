package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/knothhe/passpine/client/cloudsync"
	"github.com/knothhe/passpine/client/secretstore"
	"github.com/knothhe/r2connect"
)

func cleanupReplacementFixture(t *testing.T) (*Service, *oauthMemorySecrets, *oauthAPI, syncProfile, syncProfile, *SyncConnectionRequest) {
	t.Helper()
	s, memory, api, _ := oauthFixture(t)
	chosen, other := &syncTestBucket{}, &syncTestBucket{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/chosen-bucket") {
			chosen.serve(w, r)
			return
		}
		other.serve(w, r)
	}))
	t.Cleanup(server.Close)
	destination, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	engine := cloudsync.New(&http.Client{Transport: syncReviewRoundTripper(func(request *http.Request) (*http.Response, error) {
		forward := request.Clone(request.Context())
		address := *request.URL
		address.Scheme, address.Host = destination.Scheme, destination.Host
		forward.URL = &address
		forward.Host = request.URL.Host
		return http.DefaultTransport.RoundTrip(forward)
	})})
	s.configSync.engine = engine
	authorizeOAuthCandidate(t, s)
	saved, value := prepareOAuthCandidate(t, s, "chosen-bucket")
	s.mu.Lock()
	local := s.localSyncValues(nil)
	s.mu.Unlock()
	initial, err := engine.Sync(context.Background(), saved.Config, value.Secrets, local, nil)
	if err != nil {
		t.Fatal(err)
	}
	value.Baseline = initial.Values
	activateOAuthCandidate(t, s, saved, value)
	authorizeOAuthCandidate(t, s)
	candidate, candidateValue := prepareOAuthCandidate(t, s, "other-bucket")
	request := &SyncConnectionRequest{Provider: "r2", AccountID: oauthTestAccount, Jurisdiction: "default", Bucket: "other-bucket", Config: candidate.Config, Secrets: cloudsync.Secrets{Passphrase: candidateValue.Secrets.Passphrase}}
	return s, memory, api, saved, candidate, request
}

func TestConfigSyncCommittedR2SaveReportsCleanupWarningAndRecovers(t *testing.T) {
	s, memory, api, saved, candidate, request := cleanupReplacementFixture(t)
	memory.mu.Lock()
	oldRecord := append([]byte(nil), memory.values[saved.SecretRef]...)
	memory.mu.Unlock()
	api.mu.Lock()
	api.deleteStatus = http.StatusInternalServerError
	api.mu.Unlock()
	response := s.Execute(Request{Action: "config-sync-save", SyncConnection: request})
	if response.Status != "ok" || response.Code != "" {
		t.Fatalf("committed connection reported save failure: %s %s", response.Status, response.Code)
	}
	if response.Data.ConfigSync == nil || !response.Data.ConfigSync.Ready || response.Data.ConfigSync.WarningCode != "oauth_cleanup_pending" {
		t.Fatal("committed save did not expose pending cleanup warning")
	}
	s.mu.Lock()
	active := s.configSync.meta.Profiles["r2"]
	draft := s.configSync.meta.Candidate
	pending := append([]syncProfile(nil), s.configSync.meta.PendingCleanup...)
	s.mu.Unlock()
	if active.Connection == nil || active.Connection.ID != candidate.Connection.ID || draft != nil {
		t.Fatal("committed save did not retain the new active target")
	}
	if len(pending) != 1 || pending[0].SecretRef != saved.SecretRef || pending[0].RemoteClean {
		t.Fatal("old token cleanup ownership was lost or marked complete")
	}
	memory.mu.Lock()
	retained := bytes.Equal(oldRecord, memory.values[saved.SecretRef])
	memory.mu.Unlock()
	if !retained {
		t.Fatal("old local record was removed before its token could be cleaned")
	}
	_, deletes, revokes, _ := api.counts()
	if deletes == 0 || revokes != 0 {
		t.Fatal("cleanup did not target the retired token or revoked shared authorization")
	}
	api.mu.Lock()
	api.deleteStatus = 0
	api.mu.Unlock()
	response = s.Execute(Request{Action: "config-sync-now"})
	if response.Status != "ok" {
		t.Fatalf("cleanup recovery failed: %s", response.Code)
	}
	if response.Data.ConfigSync.WarningCode != "" {
		t.Fatal("completed cleanup warning did not clear")
	}
	s.mu.Lock()
	remaining := len(s.configSync.meta.PendingCleanup)
	active = s.configSync.meta.Profiles["r2"]
	s.mu.Unlock()
	if remaining != 0 || active.Connection == nil || active.Connection.ID != candidate.Connection.ID {
		t.Fatal("cleanup recovery changed the saved target or retained pending records")
	}
	memory.mu.Lock()
	_, oldExists := memory.values[saved.SecretRef]
	_, newExists := memory.values[candidate.SecretRef]
	memory.mu.Unlock()
	if oldExists || !newExists {
		t.Fatal("cleanup recovery removed the current credential or retained the retired app record")
	}
	connections := s.configSync.manager.State().Connections
	if len(connections) != 1 || connections[0].ID != candidate.Connection.ID {
		t.Fatal("SDK cleanup removed the live connection")
	}
}

type cleanupDeleteDenial struct {
	*oauthMemorySecrets
	denyNext bool
}

func (m *cleanupDeleteDenial) Delete(key string) error {
	m.mu.Lock()
	if m.denied {
		m.mu.Unlock()
		return secretstore.ErrDenied
	}
	if m.denyNext {
		m.denyNext = false
		m.denied = true
		m.operations++
		m.mu.Unlock()
		return secretstore.ErrDenied
	}
	m.mu.Unlock()
	return m.oauthMemorySecrets.Delete(key)
}

func TestConfigSyncCommittedSaveDenialStopsAutomaticScheduling(t *testing.T) {
	s, memory, api, saved, candidate, request := cleanupReplacementFixture(t)
	denial := &cleanupDeleteDenial{oauthMemorySecrets: memory, denyNext: true}
	s.configSync.store = denial
	// Reload only this test manager against the same committed public SDK state
	// so the simulated native denial applies inside SDK Discard as well.
	s.configSync.manager = r2connect.NewManager(s.configSync.client, r2connect.ManagerOptions{Secrets: denial, State: r2connect.FileStateStore{Path: filepath.Join(s.directory, "r2connect-state.json")}, TokenNamePrefix: "Passpine-"})
	if err := s.configSync.manager.LoadError(); err != nil {
		t.Fatal(err)
	}
	response := s.Execute(Request{Action: "config-sync-save", SyncConnection: request})
	if response.Status != "ok" || response.Code != "" {
		t.Fatalf("post-commit keyring refusal was reported as save failure: %s %s", response.Status, response.Code)
	}
	if !denial.Denied() || response.Data.ConfigSync.WarningCode != "oauth_cleanup_pending" {
		t.Fatal("post-commit refusal did not retain the denial/warning")
	}
	s.mu.Lock()
	active := s.configSync.meta.Profiles["r2"]
	pending := append([]syncProfile(nil), s.configSync.meta.PendingCleanup...)
	s.mu.Unlock()
	if active.Connection == nil || active.Connection.ID != candidate.Connection.ID || len(pending) != 1 || pending[0].SecretRef != saved.SecretRef {
		t.Fatal("post-commit refusal lost new target or retired cleanup ownership")
	}
	memory.mu.Lock()
	operations := memory.operations
	memory.mu.Unlock()
	_, deletes, revokes, refreshes := api.counts()
	// Holding the lane makes a wrongly queued worker observable before it can
	// finish. A cleared presentation error cannot bypass the store's own latch.
	s.configSyncMu.Lock()
	s.mu.Lock()
	s.configSync.next = time.Time{}
	s.configSync.status.ErrorCode = ""
	s.mu.Unlock()
	s.scheduleConfigSync()
	s.mu.Lock()
	queued := s.configSync.queued
	s.mu.Unlock()
	s.configSyncMu.Unlock()
	if queued {
		t.Fatal("automatic synchronization was queued after native credential refusal")
	}
	memory.mu.Lock()
	after := memory.operations
	memory.mu.Unlock()
	_, afterDeletes, afterRevokes, afterRefreshes := api.counts()
	if after != operations || deletes != afterDeletes || revokes != afterRevokes || refreshes != afterRefreshes {
		t.Fatal("automatic synchronization reached keyring or Cloudflare after refusal")
	}
}
