package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAutomaticUpdateDownloadDefaultsAndPersistence(t *testing.T) {
	for _, test := range []struct {
		name, contents string
		want           bool
	}{
		{"fresh", "", true},
		{"legacy", `{"settings":{"language":"en"}}`, true},
		{"disabled", `{"settings":{"autoDownloadUpdates":false}}`, false},
		{"enabled", `{"settings":{"autoDownloadUpdates":true}}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if test.contents != "" {
				if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(test.contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			s, err := New(Options{DataDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			settings := must(t, s, Request{Action: "settings"}).Settings
			if settings.AutoDownloadUpdates != test.want {
				t.Fatalf("auto download = %v, want %v", settings.AutoDownloadUpdates, test.want)
			}
			settings.AutoDownloadUpdates = !test.want
			must(t, s, Request{Action: "settings", Settings: &settings})
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := New(Options{DataDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if got := must(t, reopened, Request{Action: "settings"}).Settings.AutoDownloadUpdates; got != !test.want {
				t.Fatalf("restart lost auto download choice: %v", got)
			}
		})
	}
}

// A confirmed restart waits for an operation that is already using the service,
// then refuses new desktop/browser requests until preparation fails or exits.
func TestUpdatePauseDrainsRequestsAndCanResumeAfterFailure(t *testing.T) {
	s, err := New(Options{DataDir: t.TempDir(), Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.updateRequests.RLock()
	paused := make(chan func(), 1)
	go func() { paused <- s.PauseForUpdate() }()
	select {
	case <-paused:
		t.Fatal("installation interrupted in-flight request")
	case <-time.After(25 * time.Millisecond):
	}
	s.updateRequests.RUnlock()
	resume := <-paused
	if got := s.Execute(Request{Action: "settings"}); got.Status != "error" || got.Code != "operation_failed" {
		t.Fatal("new request started during installation")
	}
	if got := s.execute(Request{Action: "list"}, "browser"); got.Status != "error" {
		t.Fatal("browser request started during installation")
	}
	if got := s.executeGit(Request{Action: "git-sync"}, "automatic"); got.Status != "error" {
		t.Fatal("automatic sync started during installation")
	}
	resume()
	if got := s.Execute(Request{Action: "settings"}); got.Status != "ok" {
		t.Fatal("failed installation did not resume service")
	}
}
