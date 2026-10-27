package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/knothhe/passpine/client/cloudsync"
)

func TestConfigSyncR2SaveWaitsForNewCredentials(t *testing.T) {
	s, _, api, _ := oauthFixture(t)
	authorizeOAuthCandidate(t, s)
	bucket := &syncTestBucket{}
	var mu sync.Mutex
	var probes, objectRequests int
	var credential string
	ready := false
	s.configSync.engine = cloudsync.New(&http.Client{Transport: syncReviewRoundTripper(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2" {
			probes++
			_, signed, _ := strings.Cut(r.Header.Get("Authorization"), "Credential=")
			key, _, _ := strings.Cut(signed, "/")
			if credential == "" {
				credential = key
			} else if credential != key {
				t.Error("readiness retry changed the provisioned credentials")
			}
			if probes == 1 {
				response := httptest.NewRecorder()
				response.WriteHeader(http.StatusForbidden)
				return response.Result(), nil
			}
			ready = true
		} else {
			objectRequests++
			if !ready {
				t.Error("save accessed the configuration object before credentials became ready")
			}
		}
		response := httptest.NewRecorder()
		bucket.serve(response, r)
		return response.Result(), nil
	})})
	response := s.Execute(Request{Action: "config-sync-save", SyncConnection: &SyncConnectionRequest{
		Provider: "r2", AccountID: oauthTestAccount, Jurisdiction: "default", Bucket: "chosen-bucket",
		Config: cloudsync.Config{Prefix: "settings"}, Secrets: cloudsync.Secrets{Passphrase: "test encryption passphrase"},
	}})
	if response.Status != "ok" {
		t.Fatalf("one save did not wait for new R2 credentials: status=%s code=%s", response.Status, response.Code)
	}
	mu.Lock()
	gotProbes, gotObjects := probes, objectRequests
	mu.Unlock()
	if gotProbes != 2 || gotObjects == 0 {
		t.Fatalf("save did not probe before synchronization: probes=%d objectRequests=%d", gotProbes, gotObjects)
	}
	creates, deletes, revokes, _ := api.counts()
	if creates != 1 || deletes != 0 || revokes != 0 {
		t.Fatalf("readiness retry changed token ownership: creates=%d deletes=%d revokes=%d", creates, deletes, revokes)
	}
	s.mu.Lock()
	active, candidate := s.configSync.meta.Active, s.configSync.meta.Candidate
	s.mu.Unlock()
	if active != "r2" || candidate != nil || response.Data.ConfigSync == nil || !response.Data.ConfigSync.Ready {
		t.Fatal("one save did not activate the prepared R2 connection")
	}
}

func newR2ReadinessFixture(transport http.RoundTripper) (*Service, syncProfile, syncSecret) {
	p := syncProfile{
		SecretRef: "readiness-candidate",
		Config:    cloudsync.Config{Endpoint: "https://r2.example.test", Region: "auto", Bucket: "test-bucket", Prefix: "settings", PathStyle: true},
	}
	v := syncSecret{Secrets: cloudsync.Secrets{AccessKeyID: "readiness-key", SecretAccessKey: "readiness-secret", Passphrase: "test encryption passphrase"}}
	s := &Service{}
	s.configSync.meta.Candidate = &p
	s.configSync.engine = cloudsync.New(&http.Client{Transport: transport})
	return s, p, v
}

func TestConfigSyncR2ReadinessPersistentDenialIsBounded(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				started := time.Now()
				var attempts []time.Duration
				s, p, v := newR2ReadinessFixture(syncReviewRoundTripper(func(r *http.Request) (*http.Response, error) {
					if r.Method != http.MethodGet || r.URL.Query().Get("list-type") != "2" {
						t.Fatal("readiness changed the remote object")
					}
					attempts = append(attempts, time.Since(started))
					response := httptest.NewRecorder()
					response.WriteHeader(status)
					return response.Result(), nil
				}))
				err := s.testPreparedSyncConnection(context.Background(), "r2", p, v)
				var failure *cloudsync.Error
				if !errors.As(err, &failure) || failure.Code != "unauthorized" || failure.Status != status {
					t.Fatalf("persistent rejection lost its error: %v", err)
				}
				want := []time.Duration{0, time.Second, 3 * time.Second, 7 * time.Second, 15 * time.Second, 30 * time.Second, 45 * time.Second, time.Minute}
				if len(attempts) != len(want) {
					t.Fatalf("persistent rejection probes=%d want=%d", len(attempts), len(want))
				}
				for i := range want {
					if attempts[i] != want[i] {
						t.Fatalf("probe %d elapsed=%s want=%s", i, attempts[i], want[i])
					}
				}
				if s.configSync.meta.Candidate == nil || s.configSync.meta.Active != "" {
					t.Fatal("persistent rejection changed candidate ownership")
				}
			})
		})
	}
}

func TestConfigSyncR2ReadinessCancellationInterruptsWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		attempts := 0
		s, p, v := newR2ReadinessFixture(syncReviewRoundTripper(func(*http.Request) (*http.Response, error) {
			attempts++
			response := httptest.NewRecorder()
			response.WriteHeader(http.StatusForbidden)
			return response.Result(), nil
		}))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- s.testPreparedSyncConnection(ctx, "r2", p, v) }()
		synctest.Wait()
		if attempts != 1 {
			t.Fatalf("unexpected probes before cancellation: %d", attempts)
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel did not interrupt readiness: %v", err)
		}
		if attempts != 1 || time.Since(started) != 0 {
			t.Fatalf("cancel waited or retried: probes=%d elapsed=%s", attempts, time.Since(started))
		}
	})
}

func TestConfigSyncR2ReadinessDoesNotRetryUnrelatedFailures(t *testing.T) {
	for _, test := range []struct {
		name      string
		provider  string
		candidate string
		status    int
		network   bool
		wantCode  string
	}{
		{name: "manual S3", provider: "s3", candidate: "matching", status: http.StatusForbidden, wantCode: "unauthorized"},
		{name: "saved R2 connection", provider: "r2", status: http.StatusForbidden, wantCode: "unauthorized"},
		{name: "unrelated candidate", provider: "r2", candidate: "other", status: http.StatusForbidden, wantCode: "unauthorized"},
		{name: "missing bucket", provider: "r2", candidate: "matching", status: http.StatusNotFound, wantCode: "remote_error"},
		{name: "server failure", provider: "r2", candidate: "matching", status: http.StatusInternalServerError, wantCode: "remote_error"},
		{name: "network failure", provider: "r2", candidate: "matching", network: true, wantCode: "network"},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				started := time.Now()
				attempts := 0
				s, p, v := newR2ReadinessFixture(syncReviewRoundTripper(func(*http.Request) (*http.Response, error) {
					attempts++
					if test.network {
						return nil, errors.New("isolated transport failure")
					}
					response := httptest.NewRecorder()
					response.WriteHeader(test.status)
					return response.Result(), nil
				}))
				switch test.candidate {
				case "":
					s.configSync.meta.Candidate = nil
				case "other":
					other := p
					other.SecretRef = "unrelated-candidate"
					s.configSync.meta.Candidate = &other
				}
				err := s.testPreparedSyncConnection(context.Background(), test.provider, p, v)
				if cloudsync.ErrorCode(err) != test.wantCode {
					t.Fatalf("failure code=%s want=%s", cloudsync.ErrorCode(err), test.wantCode)
				}
				if attempts != 1 || time.Since(started) != 0 {
					t.Fatalf("unrelated failure retried: probes=%d elapsed=%s", attempts, time.Since(started))
				}
			})
		})
	}
}
