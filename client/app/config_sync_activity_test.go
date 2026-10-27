package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/knothhe/passpine/client/cloudsync"
	"github.com/knothhe/r2connect"
)

func TestConfigSyncActivityRecordsFailedSaveThroughExecute(t *testing.T) {
	private := "private-save-request-marker"
	requests := 0
	engine := cloudsync.New(&http.Client{Transport: syncReviewRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(private)), Request: request}, nil
	})})
	s := newSyncServiceFixture(t, "", &syncMemorySecrets{values: map[string][]byte{}}, engine)
	result := s.Execute(Request{RequestID: private, Action: "config-sync-save", SyncConnection: &SyncConnectionRequest{
		Provider: "s3",
		Config:   cloudsync.Config{Endpoint: "https://" + private + ".example", Bucket: private, Prefix: private, PathStyle: true},
		Secrets:  cloudsync.Secrets{AccessKeyID: private, SecretAccessKey: private, Passphrase: private},
	}})
	if result.Status != "error" || result.Code != "unauthorized" || requests != 1 {
		t.Fatalf("save did not reach and report the rejected object request: status=%s code=%s requests=%d", result.Status, result.Code, requests)
	}
	data, err := os.ReadFile(filepath.Join(s.directory, "activity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), private) {
		t.Fatal("failed save activity persisted request data, credentials or response text")
	}
	var records []Activity
	if err = json.Unmarshal(data, &records); err != nil || len(records) != 1 {
		t.Fatalf("failed save did not persist one activity record: count=%d error=%v", len(records), err)
	}
	if record := records[0]; record.Action != "config-sync-save" || record.Source != "desktop" || record.Outcome != "error" || record.ErrorCode != "unauthorized" || record.HTTPStatus != http.StatusForbidden {
		t.Fatalf("failed save activity lost its controlled diagnostics: %+v", record)
	}
}

func TestConfigSyncActivityPersistsOnlyControlledDiagnostics(t *testing.T) {
	private := "private-sync-diagnostic-marker"
	for _, test := range []struct {
		name   string
		err    error
		code   string
		status int
	}{
		{"object access", fmt.Errorf("%s: %w", private, &cloudsync.Error{Code: "unauthorized", Status: http.StatusForbidden, Keys: []string{private}}), "unauthorized", http.StatusForbidden},
		{"account access", fmt.Errorf("%s: %w", private, &r2connect.APIError{Operation: private, StatusCode: http.StatusForbidden, OAuthError: private, CFRay: private}), "oauth_permission_required", http.StatusForbidden},
		{"transport", errors.New("response body and credentials: " + private), "connection_failed", 0},
		{"success", nil, "", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &Service{directory: t.TempDir(), prefs: preferences{Settings: DefaultSettings()}}
			s.configSync.meta = syncMetadata{Profiles: map[string]syncProfile{"r2": {Config: cloudsync.Config{Endpoint: "https://" + private, Bucket: private}}}}
			s.configSync.pendingSecrets = map[string]syncSecret{private: {Secrets: cloudsync.Secrets{AccessKeyID: private, SecretAccessKey: private, Passphrase: private}}}
			s.mu.Lock()
			s.logConfigSync("config-sync-save", "desktop", test.err)
			s.mu.Unlock()
			data, err := os.ReadFile(filepath.Join(s.directory, "activity.json"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), private) {
				t.Fatal("activity persisted private configuration, credentials, response data or error text")
			}
			var records []Activity
			if err = json.Unmarshal(data, &records); err != nil || len(records) != 1 {
				t.Fatalf("expected one persisted activity record: count=%d error=%v", len(records), err)
			}
			record := records[0]
			outcome := "ok"
			if test.err != nil {
				outcome = "error"
			}
			if record.Action != "config-sync-save" || record.Source != "desktop" || record.Outcome != outcome || record.ErrorCode != test.code || record.HTTPStatus != test.status {
				t.Fatalf("incorrect controlled diagnostics: %+v", record)
			}
			if _, err = time.Parse(time.RFC3339, record.Time); err != nil {
				t.Fatal("activity timestamp is invalid")
			}
			if test.code == "" && strings.Contains(string(data), "errorCode") || test.status == 0 && strings.Contains(string(data), "httpStatus") {
				t.Fatal("empty optional diagnostics were persisted")
			}
		})
	}
}

func TestConfigSyncActivitySkipsPollingUnknownActionsAndDisabledRecording(t *testing.T) {
	s := &Service{directory: t.TempDir(), prefs: preferences{Settings: DefaultSettings()}}
	s.mu.Lock()
	for _, action := range []string{"config-sync-status", "config-sync-browser", "config-sync-private-user-input"} {
		s.logConfigSync(action, "desktop", errors.New("private error"))
	}
	s.prefs.Settings.RecordActivity = false
	s.logConfigSync("config-sync-save", "desktop", &cloudsync.Error{Code: "unauthorized", Status: http.StatusForbidden})
	s.mu.Unlock()
	if len(s.activity) != 0 {
		t.Fatal("polling, unknown actions or disabled recording created activity")
	}
	if _, err := os.Stat(filepath.Join(s.directory, "activity.json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected activity file: %v", err)
	}
}

func TestConfigSyncActivityUsesExistingRetention(t *testing.T) {
	s := &Service{directory: t.TempDir(), prefs: preferences{Settings: DefaultSettings()}}
	s.prefs.Settings.ActivityMaxEntries = 2
	s.activity = []Activity{{Time: time.Now().Add(-8 * 24 * time.Hour).Format(time.RFC3339), Action: "expired"}}
	s.mu.Lock()
	for _, action := range []string{"config-sync-oauth-start", "config-sync-oauth-buckets", "config-sync-save"} {
		s.logConfigSync(action, "desktop", nil)
	}
	s.mu.Unlock()
	data, err := os.ReadFile(filepath.Join(s.directory, "activity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var records []Activity
	if err = json.Unmarshal(data, &records); err != nil || len(records) != 2 || records[0].Action != "config-sync-save" || records[1].Action != "config-sync-oauth-buckets" {
		t.Fatalf("sync diagnostics did not retain the existing activity limits: %s error=%v", data, err)
	}
}
