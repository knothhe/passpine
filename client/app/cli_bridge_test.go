package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/knothhe/passpine/client/internal/clisession"
	"github.com/knothhe/passpine/client/nativehost"
)

func cliRequest(t *testing.T, s *Service, r clisession.Request) clisession.Response {
	t.Helper()
	r.Protocol = clisession.Protocol
	r.PayloadVersion = clisession.PayloadVersion
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var out clisession.Response
	if err := json.Unmarshal(s.handleCLI(raw), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func cliSession(t *testing.T, s *Service) clisession.Request {
	t.Helper()
	r := clisession.Request{PayloadVersion: clisession.PayloadVersion, Directory: must(t, s, Request{Action: "status"}).Directory, Action: "status"}
	out := cliRequest(t, s, r)
	if out.Status != "ok" || len(out.Session) != 64 {
		t.Fatalf("CLI session: status=%s code=%s", out.Status, out.Code)
	}
	r.Session = out.Session
	return r
}

func TestCLIBridgeReusesDesktopSessionForOperations(t *testing.T) {
	s := testService(t)
	r := cliSession(t, s)
	changed := make(chan struct{}, 8)
	s.SetOnChange(func() { changed <- struct{}{} })
	initialVersion := must(t, s, Request{Action: "status"}).EntriesVersion
	r.Action, r.Path, r.Contents = "save", "work/example", []byte("fixture-secret\nusername: alex\n")
	saved := cliRequest(t, s, r)
	if saved.Status != "ok" || saved.Document == nil || !bytes.Equal(saved.Document.Raw, r.Contents) || saved.Document.Revision == "" {
		t.Fatal("CLI did not save through the unlocked desktop session")
	}
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("CLI mutation did not notify the desktop")
	}
	if got := must(t, s, Request{Action: "list"}); len(got.Entries) != 1 || got.EntriesVersion <= initialVersion {
		t.Fatal("desktop metadata did not refresh after CLI save")
	}
	r.Action, r.Contents = "read", nil
	read := cliRequest(t, s, r)
	if read.Status != "ok" || read.Document == nil || !bytes.Equal(read.Document.Raw, saved.Document.Raw) {
		t.Fatal("CLI could not read its saved contents")
	}
	r.Action, r.Path = "list", ""
	if list := cliRequest(t, s, r); list.Status != "ok" || len(list.Entries) != 1 || list.Entries[0].Path != "work/example" {
		t.Fatal("CLI list did not use desktop vault")
	}
	r.Action, r.Path, r.Destination, r.Revision = "move", "work/example", "work/renamed", saved.Document.Revision
	if moved := cliRequest(t, s, r); moved.Status != "ok" {
		t.Fatalf("CLI move failed: %s", moved.Code)
	}
	r.Action, r.Path, r.Destination, r.Revision = "read", "work/renamed", "", ""
	moved := cliRequest(t, s, r)
	if moved.Document == nil {
		t.Fatalf("moved entry not readable: %s", moved.Code)
	}
	r.Action, r.Revision = "trash", moved.Document.Revision
	if deleted := cliRequest(t, s, r); deleted.Status != "ok" || deleted.RecoveryID == "" || deleted.Document != nil {
		t.Fatal("CLI trash did not preserve recoverable deletion")
	}
	if got := must(t, s, Request{Action: "list"}); len(got.Entries) != 0 {
		t.Fatal("desktop still lists deleted entry")
	}
}

func TestCLIBridgeRejectsOtherVaultAndExpiredSessions(t *testing.T) {
	s := testService(t)
	r := cliSession(t, s)
	r.Action, r.Path = "read", "fixture"
	other := r
	other.Action, other.Path, other.Session = "status", "", ""
	other.Directory = t.TempDir()
	if got := cliRequest(t, s, other); got.Code != "vault_changed" {
		t.Fatalf("other directory: %s", got.Code)
	}
	must(t, s, Request{Action: "lock"})
	status := r
	status.Action, status.Path, status.Session = "status", "", ""
	if got := cliRequest(t, s, status); got.Code != "locked" {
		t.Fatalf("locked status: %s", got.Code)
	}
	must(t, s, Request{Action: "unlock", Passphrase: "test-passphrase-123"})
	if got := cliRequest(t, s, r); got.Code != "cancelled" || got.Document != nil {
		t.Fatal("locking and unlocking revived an old CLI session")
	}
	r = cliSession(t, s)
	first := must(t, s, Request{Action: "status"}).VaultID
	must(t, s, Request{Action: "create", Directory: filepath.Join(t.TempDir(), "second"), Passphrase: "second-vault-passphrase"})
	must(t, s, Request{Action: "switch", ID: first})
	must(t, s, Request{Action: "unlock", Passphrase: "test-passphrase-123"})
	r.Action = "list"
	if got := cliRequest(t, s, r); got.Code != "cancelled" {
		t.Fatal("switching back revived an old CLI session")
	}
}

func TestCLIBridgeMoveRetainsRevisionAndDestinationGuards(t *testing.T) {
	s := testService(t)
	source := must(t, s, Request{Action: "save", Entry: "source", Contents: "source-secret", Mode: "create"})
	must(t, s, Request{Action: "save", Entry: "destination", Contents: "existing-secret", Mode: "create"})
	r := cliSession(t, s)
	r.Action, r.Path, r.Destination, r.Revision = "move", "source", "new-destination", "stale"
	if got := cliRequest(t, s, r); got.Code != "revision_conflict" {
		t.Fatalf("stale move revision was accepted: %s", got.Code)
	}
	r.Destination, r.Revision = "destination", source.Revision
	if got := cliRequest(t, s, r); got.Code != "already_exists" {
		t.Fatalf("move overwrote existing destination: %s", got.Code)
	}
	if got := must(t, s, Request{Action: "fetch", Entry: "source"}); got.Contents != "source-secret" {
		t.Fatal("rejected move removed the source")
	}
	if got := must(t, s, Request{Action: "fetch", Entry: "destination"}); got.Contents != "existing-secret" {
		t.Fatal("rejected move changed the destination")
	}
}

func TestCLIDirectoryIdentityUsesNativeFilesystem(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "Vault")
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	if !sameDirectory(original, original+string(os.PathSeparator)+".") {
		t.Fatal("directory alias did not preserve identity")
	}
	if sameDirectory(original, filepath.Join(root, "missing")) {
		t.Fatal("missing directory matched a vault")
	}
	// macOS and Windows commonly resolve this spelling to the same directory;
	// Linux commonly permits a separate directory. Assert the native behavior.
	alternate := filepath.Join(root, "VAULT")
	left, err := os.Stat(original)
	if err != nil {
		t.Fatal(err)
	}
	right, err := os.Stat(alternate)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(alternate, 0700); err != nil {
			t.Fatal(err)
		}
		right, err = os.Stat(alternate)
	}
	if err != nil {
		t.Fatal(err)
	}
	if got, want := sameDirectory(original, alternate), os.SameFile(left, right); got != want {
		t.Fatalf("case alias identity=%v, native identity=%v", got, want)
	}
}

func TestCLIBridgeRejectsOldSessionAfterDesktopRestart(t *testing.T) {
	s := testService(t)
	r := cliSession(t, s)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(Options{DataDir: s.DataDirectory()})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	must(t, reopened, Request{Action: "unlock", Passphrase: "test-passphrase-123"})
	r.Action = "list"
	if got := cliRequest(t, reopened, r); got.Code != "cancelled" {
		t.Fatal("desktop restart revived an old CLI session")
	}
}

func TestCLIBridgeGenerationRejectsSessionAfterAutomaticLock(t *testing.T) {
	s := testService(t)
	r := cliSession(t, s)
	// Automatic session expiry clears the vault generation without needing a
	// desktop lock request or changing the service cancellation counter.
	epoch := s.cancellation.Load()
	s.active.Lock()
	must(t, s, Request{Action: "unlock", Passphrase: "test-passphrase-123"})
	if s.cancellation.Load() != epoch {
		t.Fatal("fixture changed the cancellation epoch")
	}
	r.Action = "list"
	if got := cliRequest(t, s, r); got.Code != "cancelled" {
		t.Fatal("automatic lock and unlock revived the old CLI generation")
	}
}

func TestCLIBridgeQueuedWriteIsCancelledByLock(t *testing.T) {
	s := testService(t)
	r := cliSession(t, s)
	r.Action, r.Path, r.Contents = "save", "must-not-exist", []byte("fixture-secret")
	r.Protocol = clisession.Protocol
	r.PayloadVersion = clisession.PayloadVersion
	raw, _ := json.Marshal(r)
	result := make(chan json.RawMessage, 1)
	s.mu.Lock()
	go func() { result <- s.handleCLI(raw) }()
	// A queued CLI request holds the update admission lease before taking mu.
	deadline := time.Now().Add(time.Second)
	for s.updateRequests.TryLock() {
		s.updateRequests.Unlock()
		if time.Now().After(deadline) {
			s.mu.Unlock()
			t.Fatal("CLI request did not enter the queue")
		}
		time.Sleep(time.Millisecond)
	}
	epoch := s.cancellation.Load()
	locked := make(chan Response, 1)
	go func() { locked <- s.Execute(Request{Action: "lock"}) }()
	for s.cancellation.Load() == epoch {
		if time.Now().After(deadline) {
			s.mu.Unlock()
			t.Fatal("lock request did not cancel the queued write")
		}
		time.Sleep(time.Millisecond)
	}
	s.mu.Unlock()
	var out clisession.Response
	if err := json.Unmarshal(<-result, &out); err != nil || out.Code != "cancelled" {
		t.Fatalf("queued write was not cancelled: %s", out.Code)
	}
	if response := <-locked; response.Status != "ok" {
		t.Fatal("lock request failed")
	}
	must(t, s, Request{Action: "unlock", Passphrase: "test-passphrase-123"})
	if _, err := s.active.Read(r.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("queued CLI write committed after the lock request")
	}
}

func TestCLIBridgePausesAndClosesWithDesktop(t *testing.T) {
	s := testService(t)
	r := cliSession(t, s)
	r.Action = "list"
	resume := s.PauseForUpdate()
	if got := cliRequest(t, s, r); got.Code != "operation_failed" {
		t.Fatalf("request admitted during update pause: %s", got.Code)
	}
	resume()
	if got := cliRequest(t, s, r); got.Status != "ok" {
		t.Fatal("failed update preparation did not resume CLI requests")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if got := cliRequest(t, s, r); got.Code != "app_unavailable" {
		t.Fatal("closed desktop admitted CLI request")
	}
	for _, directory := range []string{s.DataDirectory(), clisession.Directory(s.DataDirectory())} {
		if _, err := os.Stat(filepath.Join(directory, "browser-endpoint.json")); !os.IsNotExist(err) {
			t.Fatal("closed service left an endpoint published")
		}
	}
}

func TestCLIBridgeRejectsInvalidRequestsAndBrowserAccess(t *testing.T) {
	s := testService(t)
	r := cliSession(t, s)
	for _, raw := range []string{
		`{"protocol":1,"payloadVersion":1,"action":"unlock","directory":"/tmp","passphrase":"secret"}`,
		`{"protocol":1,"payloadVersion":1,"action":"status","directory":"/tmp","extra":true}`,
		`{"protocol":1,"payloadVersion":1,"action":"status","directory":"/tmp","keyFile":"secret"}`,
		`{"protocol":1,"payloadVersion":1,"action":"status","directory":"relative"}`,
		`{"protocol":1,"payloadVersion":1,"action":"status","directory":"/tmp"} {}`,
	} {
		var out clisession.Response
		if err := json.Unmarshal(s.handleCLI([]byte(raw)), &out); err != nil || out.Code != "invalid_request" || out.Document != nil {
			t.Fatalf("invalid request accepted: %s", raw)
		}
	}
	r.Action = "list"
	raw, _ := json.Marshal(r)
	var out Response
	if err := json.Unmarshal(s.handleBrowser(raw), &out); err != nil || out.Status != "error" {
		t.Fatal("browser endpoint accepted CLI session capability")
	}
	r.Action = "status"
	r.Session = ""
	r.Protocol = clisession.Protocol
	r.PayloadVersion = clisession.PayloadVersion
	raw, _ = json.Marshal(r)
	var decoded map[string]any
	_ = json.Unmarshal(raw, &decoded)
	decoded["protocol"] = 2
	raw, _ = json.Marshal(decoded)
	var rejected clisession.Response
	_ = json.Unmarshal(s.handleCLI(raw), &rejected)
	if rejected.Code != "protocol_mismatch" {
		t.Fatal("CLI endpoint accepted incompatible protocol")
	}
}

func TestCLIBridgeTransportSupportsLargeEntryWithoutBrowserChanges(t *testing.T) {
	s := testService(t)
	r := cliSession(t, s)
	r.Action, r.Path = "save", "large"
	r.Contents = []byte(strings.Repeat("x", nativehost.MaxFrame+1))
	r.Protocol = clisession.Protocol
	r.PayloadVersion = clisession.PayloadVersion
	client := nativehost.NewRelayClientWithFrameLimit(clisession.Directory(s.DataDirectory()), clisession.MaxFrame)
	defer client.Close()
	raw, _ := json.Marshal(r)
	var out clisession.Response
	if err := json.Unmarshal(client.Send(raw), &out); err != nil || out.Status != "ok" || out.Document == nil || !bytes.Equal(out.Document.Raw, r.Contents) {
		t.Fatalf("large CLI operation failed: %s", out.Code)
	}
	r.Action, r.Contents = "read", nil
	raw, _ = json.Marshal(r)
	if err := json.Unmarshal(client.Send(raw), &out); err != nil || out.Status != "ok" || out.Document == nil || len(out.Document.Raw) != nativehost.MaxFrame+1 {
		t.Fatalf("large CLI read failed: %s", out.Code)
	}
}

func TestCLIBridgeStartupFailureClosesBrowserEndpoint(t *testing.T) {
	data := t.TempDir()
	if err := os.WriteFile(clisession.Directory(data), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	if service, err := New(Options{DataDir: data}); err == nil || service != nil {
		t.Fatal("startup ignored CLI endpoint failure")
	}
	if _, err := os.Stat(filepath.Join(data, "browser-endpoint.json")); !os.IsNotExist(err) {
		t.Fatal("failed startup left the browser endpoint published")
	}
}
