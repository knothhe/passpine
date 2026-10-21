package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func gitTestService(t *testing.T) *Service {
	t.Helper()
	s, err := New(Options{DataDir: t.TempDir(), Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	must(t, s, Request{Action: "git-init"})
	return s
}

func TestAutomaticGitSyncDefersAndCoalescesChanges(t *testing.T) {
	s := gitTestService(t)
	s.mu.Lock()
	s.prefs.Settings.AutoSync = true
	s.mu.Unlock()
	for _, name := range []string{"first", "second", "third"} {
		data := must(t, s, Request{Action: "save", Entry: name, Contents: "secret", Mode: "create"})
		if data.GitSyncing {
			t.Fatal("saving immediately started a Git operation")
		}
	}
	changed := make(chan struct{}, 1)
	s.SetOnChange(func() { changed <- struct{}{} })
	s.mu.Lock()
	id, version := s.prefs.Current, s.entriesVersion
	if !s.gitSyncPending || !s.lastSync.IsZero() || s.autoSyncDueLocked(time.Now()) {
		t.Fatal("rapid changes were not queued until input becomes idle")
	}
	s.lastActivity.Store(time.Now().Add(-automaticGitIdleDelay - time.Second).UnixNano())
	if !s.autoSyncDueLocked(time.Now()) {
		t.Fatal("idle changes were not ready to sync")
	}
	s.mu.Unlock()
	s.runAutomaticGitSync(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gitSyncPending || s.lastSync.IsZero() || s.gitData.GitChanges != 0 {
		t.Fatal("coalesced automatic sync did not commit all changes")
	}
	if s.entriesVersion != version {
		t.Fatal("a local-only commit invalidated entry data")
	}
	select {
	case <-changed:
		t.Fatal("a local-only commit requested a full metadata refresh")
	default:
	}
}

func TestAutomaticGitSyncRechecksActivityAndSettingsBeforeStarting(t *testing.T) {
	for _, scenario := range []string{"input-resumed", "sync-disabled"} {
		t.Run(scenario, func(t *testing.T) {
			s := gitTestService(t)
			s.mu.Lock()
			s.prefs.Settings.AutoSync = true
			s.gitSyncPending, s.gitSyncQueued = true, true
			s.lastActivity.Store(time.Now().Add(-time.Minute).UnixNano())
			id := s.prefs.Current
			if scenario == "sync-disabled" {
				s.prefs.Settings.AutoSync = false
			}
			s.mu.Unlock()
			if scenario == "input-resumed" {
				s.Touch()
			}
			s.runAutomaticGitSync(id)
			s.mu.Lock()
			defer s.mu.Unlock()
			if !s.lastSync.IsZero() || s.gitOperation || s.gitSyncQueued || !s.gitSyncPending {
				t.Fatal("stale automatic request ignored newer input or settings")
			}
		})
	}
}

func TestAutomaticGitSyncRetriesFailuresWithBackoff(t *testing.T) {
	s := gitTestService(t)
	must(t, s, Request{Action: "git-remote", URL: filepath.Join(t.TempDir(), "offline.git")})
	s.mu.Lock()
	s.prefs.Settings.AutoSync = true
	s.gitSyncPending = true
	s.lastActivity.Store(time.Now().Add(-time.Minute).UnixNano())
	id := s.prefs.Current
	s.mu.Unlock()
	s.runAutomaticGitSync(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.gitSyncPending || s.gitError == "" || !s.gitRetryAt.After(time.Now()) {
		t.Fatal("failed automatic sync lost its pending changes or retry delay")
	}
	if s.autoSyncDueLocked(time.Now()) || !s.autoSyncDueLocked(s.gitRetryAt.Add(time.Millisecond)) {
		t.Fatal("failed sync did not respect its retry backoff")
	}
}

func TestManualGitSyncFailurePreservesPendingAutomaticRetry(t *testing.T) {
	s := gitTestService(t)
	must(t, s, Request{Action: "git-remote", URL: filepath.Join(t.TempDir(), "offline.git")})
	s.mu.Lock()
	s.prefs.Settings.AutoSync = true
	s.prefs.Settings.SyncMinutes = 0
	s.mu.Unlock()
	must(t, s, Request{Action: "save", Entry: "pending-manual-retry", Contents: "secret", Mode: "create"})
	if result := s.Execute(Request{Action: "git-sync"}); result.Status != "error" {
		t.Fatal("manual sync unexpectedly reached the offline remote")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.gitSyncPending || s.gitError == "" || !s.gitRetryAt.After(time.Now()) {
		t.Fatal("manual failure discarded pending automatic changes")
	}
	s.lastActivity.Store(time.Now().Add(-time.Minute).UnixNano())
	if s.autoSyncDueLocked(time.Now()) || !s.autoSyncDueLocked(s.gitRetryAt.Add(time.Millisecond)) {
		t.Fatal("manual failure did not preserve an automatic retry with backoff")
	}
}

func TestBrowserActivityIsLightweightAndDoesNotUnlockExpiredSession(t *testing.T) {
	s := gitTestService(t)
	s.active.SetTimeouts(time.Second, time.Second)
	deadline := time.Now().Add(2 * time.Second)
	for s.active.Status().Unlocked && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.active.Status().Unlocked {
		t.Fatal("fixture session did not expire")
	}
	s.mu.Lock()
	cached, records := s.cachedAt, len(s.activity)
	// A touch must not wait for another credential request's mutex.
	done := make(chan json.RawMessage, 1)
	go func() {
		done <- s.handleBrowser(json.RawMessage(`{"protocol":1,"action":"touch","requestId":"activity"}`))
	}()
	select {
	case raw := <-done:
		var response map[string]any
		if err := json.Unmarshal(raw, &response); err != nil || response["status"] != "ok" || response["requestId"] != "activity" {
			t.Fatal("touch was not accepted", string(raw), err)
		}
		if data, ok := response["data"].(map[string]any); !ok || len(data) != 0 {
			t.Fatal("touch returned a snapshot")
		}
	case <-time.After(time.Second):
		s.mu.Unlock()
		t.Fatal("touch waited for the credential request mutex")
	}
	if s.cachedAt != cached || len(s.activity) != records || s.lastActivity.Load() == 0 {
		t.Fatal("touch refreshed metadata, wrote logs, or failed to record input")
	}
	s.mu.Unlock()
	s.Touch()
	if s.active.Status().Unlocked {
		t.Fatal("activity revived an expired session")
	}
	for _, payload := range []string{`"vaultId":"vault"`, `"contents":"secret"`, `"interactive":true`, `"contents":""`, `"unknown":"payload"`} {
		var response Response
		raw := s.handleBrowser(json.RawMessage(`{"protocol":1,"action":"touch",` + payload + `}`))
		if err := json.Unmarshal(raw, &response); err != nil || response.Code != "invalid_request" {
			t.Fatal("touch accepted unrelated fields", string(raw), err)
		}
	}
}

func TestGitRemoteChangesRefreshEntriesButStatusDoesNotNotifyAgain(t *testing.T) {
	s := gitTestService(t)
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	command := func(directory string, args ...string) {
		t.Helper()
		cmd := exec.Command(git, append([]string{"-c", "commit.gpgSign=false", "-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-C", directory}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v %s", args[0], err, output)
		}
	}
	directory := must(t, s, Request{Action: "status"}).Directory
	remote := filepath.Join(t.TempDir(), "remote.git")
	command(directory, "init", "--bare", "--initial-branch=main", remote)
	must(t, s, Request{Action: "git-remote", URL: remote})
	must(t, s, Request{Action: "git-sync"})
	peer := filepath.Join(t.TempDir(), "peer")
	command(directory, "clone", remote, peer)
	if err = os.WriteFile(filepath.Join(peer, "remote-entry.gpg"), []byte("encrypted fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	command(peer, "add", "remote-entry.gpg")
	command(peer, "-c", "user.name=Peer", "-c", "user.email=peer@localhost", "commit", "-m", "Add remote fixture")
	command(peer, "push")
	changes := make(chan struct{}, 1)
	s.SetOnChange(func() { changes <- struct{}{} })
	before := must(t, s, Request{Action: "status"}).EntriesVersion
	data := must(t, s, Request{Action: "git-sync"})
	if data.EntriesVersion <= before {
		t.Fatal("remote working-tree changes did not invalidate the entry cache")
	}
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("remote working-tree changes did not notify the desktop")
	}
	notified := make(chan struct{}, 1)
	s.SetOnGitChange(func() { notified <- struct{}{} })
	must(t, s, Request{Action: "git-status"})
	select {
	case <-notified:
		t.Fatal("Git status read caused a notification feedback loop")
	case <-time.After(50 * time.Millisecond):
	}
}
