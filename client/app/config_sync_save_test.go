package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/knothhe/passpine/client/cloudsync"
)

type syncReviewRoundTripper func(*http.Request) (*http.Response, error)

func (f syncReviewRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestConfigSyncRejectedReplacementPreservesSavedConnection(t *testing.T) {
	for _, target := range []string{"same-space", "new-space"} {
		for _, fault := range []string{"wrong-phrase", "invalid-document", "write-forbidden"} {
			t.Run(target+"/"+fault, func(t *testing.T) {
				s, memory, engine, oldConfig, oldSecrets, _ := newSyncAppSpace(t)
				data := saveSyncFixture(t, s, oldConfig, oldSecrets)
				if target == "same-space" && fault == "wrong-phrase" {
					must(t, s, Request{Action: "config-sync-pause"})
				}
				settings := data.Settings
				settings.Theme = "dark"
				if fault == "invalid-document" {
					settings.Theme = data.Settings.Theme
					settings.FontScale = 110
				}
				must(t, s, Request{Action: "settings", Settings: &settings})
				s.mu.Lock()
				oldProfile := s.configSync.meta.Profiles["s3"]
				oldEnabled := s.configSync.meta.Enabled
				oldSynced := s.configSync.meta.LastSyncedAt
				s.mu.Unlock()
				memory.mu.Lock()
				oldRecord := append([]byte(nil), memory.values[oldProfile.SecretRef]...)
				memory.mu.Unlock()
				var oldValue syncSecret
				if err := json.Unmarshal(oldRecord, &oldValue); err != nil {
					t.Fatal(err)
				}
				config := oldConfig
				if target == "new-space" {
					bucket := &syncTestBucket{}
					server := httptest.NewServer(http.HandlerFunc(bucket.serve))
					t.Cleanup(server.Close)
					config.Endpoint = server.URL
				}
				requestSecrets := oldSecrets
				requestSecrets.SecretAccessKey = "replacement-only-secret-marker"
				wantCode := ""
				switch fault {
				case "wrong-phrase":
					if target == "new-space" {
						if _, err := engine.Sync(context.Background(), config, oldSecrets, syncTestValues("desktop.theme", `"light"`), nil); err != nil {
							t.Fatal(err)
						}
					}
					requestSecrets.Passphrase = "wrong replacement phrase marker"
					wantCode = "decrypt_failed"
				case "invalid-document":
					remote := copySyncValues(oldValue.Baseline)
					remote["desktop.theme"] = json.RawMessage(`"unsupported-theme"`)
					if _, err := engine.Sync(context.Background(), config, oldSecrets, remote, oldValue.Baseline); err != nil {
						t.Fatal(err)
					}
					wantCode = "invalid_document"
				case "write-forbidden":
					var writes atomic.Int32
					s.configSync.engine = cloudsync.New(&http.Client{Transport: syncReviewRoundTripper(func(r *http.Request) (*http.Response, error) {
						if r.Method == http.MethodPut {
							writes.Add(1)
							return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
						}
						return http.DefaultTransport.RoundTrip(r)
					})})
					t.Cleanup(func() {
						if writes.Load() == 0 {
							t.Error("save never checked candidate write permission")
						}
					})
					wantCode = "unauthorized"
				}
				response := s.Execute(Request{Action: "config-sync-save", SyncConnection: &SyncConnectionRequest{Provider: "s3", Config: config, Secrets: requestSecrets}})
				if response.Status != "error" || response.Code != wantCode {
					t.Fatalf("candidate rejection status=%s code=%s want=%s", response.Status, response.Code, wantCode)
				}
				s.mu.Lock()
				saved, exists := s.configSync.meta.Profiles["s3"]
				active, enabled, synced := s.configSync.meta.Active, s.configSync.meta.Enabled, s.configSync.meta.LastSyncedAt
				s.mu.Unlock()
				if !exists || !reflect.DeepEqual(saved, oldProfile) || active != "s3" || enabled != oldEnabled || synced != oldSynced {
					t.Fatal("failed replacement activated a candidate or changed the saved connection baseline")
				}
				memory.mu.Lock()
				kept, exists := memory.values[oldProfile.SecretRef]
				equal := bytes.Equal(kept, oldRecord)
				memory.mu.Unlock()
				if !exists || !equal {
					t.Fatal("failed replacement deleted or changed the original credential/phrase/baseline")
				}
				if response.Data.Settings != settings {
					t.Fatal("failed connection save changed current local preferences")
				}
			})
		}
	}
}

func TestConfigSyncRejectedR2CandidateRemainsRetryable(t *testing.T) {
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
	saved, savedSecret := prepareOAuthCandidate(t, s, "chosen-bucket")
	s.mu.Lock()
	local := s.localSyncValues(nil)
	s.mu.Unlock()
	initial, err := engine.Sync(context.Background(), saved.Config, savedSecret.Secrets, local, nil)
	if err != nil {
		t.Fatal(err)
	}
	savedSecret.Baseline = initial.Values
	activateOAuthCandidate(t, s, saved, savedSecret)
	memory.mu.Lock()
	oldRecord := append([]byte(nil), memory.values[saved.SecretRef]...)
	memory.mu.Unlock()
	authorizeOAuthCandidate(t, s)
	candidate, candidateSecret := prepareOAuthCandidate(t, s, "other-bucket")
	if _, err = engine.Sync(context.Background(), candidate.Config, candidateSecret.Secrets, local, nil); err != nil {
		t.Fatal(err)
	}
	_, deletes, revokes, _ := api.counts()
	request := &SyncConnectionRequest{Provider: "r2", AccountID: oauthTestAccount, Jurisdiction: "default", Bucket: "other-bucket", Config: candidate.Config, Secrets: cloudsync.Secrets{Passphrase: "wrong candidate encryption phrase"}}
	response := s.Execute(Request{Action: "config-sync-save", SyncConnection: request})
	if response.Status != "error" || response.Code != "decrypt_failed" {
		t.Fatalf("expected R2 phrase rejection, got %s %s", response.Status, response.Code)
	}
	s.mu.Lock()
	draft := s.configSync.meta.Candidate
	active := s.configSync.meta.Profiles["r2"]
	s.mu.Unlock()
	if draft == nil || draft.SecretRef != candidate.SecretRef || draft.Connection == nil || draft.Connection.ID != candidate.Connection.ID {
		t.Fatal("failed R2 save discarded the candidate required for retry")
	}
	if !reflect.DeepEqual(active, saved) {
		t.Fatal("failed R2 candidate replaced the previous saved target")
	}
	memory.mu.Lock()
	equal := bytes.Equal(memory.values[saved.SecretRef], oldRecord)
	memory.mu.Unlock()
	if !equal {
		t.Fatal("failed R2 save deleted original sync phrase or baseline")
	}
	_, newDeletes, newRevokes, _ := api.counts()
	if newDeletes != deletes || newRevokes != revokes {
		t.Fatal("failed R2 save revoked the saved target or draft")
	}
	request.Secrets.Passphrase = candidateSecret.Secrets.Passphrase
	response = s.Execute(Request{Action: "config-sync-save", SyncConnection: request})
	if response.Status != "ok" {
		t.Fatalf("corrected candidate could not retry: %s", response.Code)
	}
	s.mu.Lock()
	draft = s.configSync.meta.Candidate
	active = s.configSync.meta.Profiles["r2"]
	s.mu.Unlock()
	if draft != nil || active.Connection == nil || active.Connection.ID != candidate.Connection.ID {
		t.Fatal("successful retry did not activate the existing SDK candidate")
	}
}
