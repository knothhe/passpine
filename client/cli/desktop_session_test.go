package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/internal/clisession"
	"github.com/knothhe/passpine/client/nativehost"
	"github.com/knothhe/passpine/client/vault"
)

// A test must never invoke real platform authentication or enroll a real key.
type noDesktopDeviceUnlock struct{}

func (noDesktopDeviceUnlock) Available() bool { return false }
func (noDesktopDeviceUnlock) Enroll(context.Context, string, []byte) ([]byte, error) {
	return nil, errors.New("device unlock is disabled in this fixture")
}
func (noDesktopDeviceUnlock) Unlock(context.Context, string, []byte) ([]byte, error) {
	return nil, errors.New("device unlock is disabled in this fixture")
}

func openDesktopHarness(t *testing.T, h *harness) *app.Service {
	t.Helper()
	s, err := app.New(app.Options{DataDir: h.data, DeviceUnlock: noDesktopDeviceUnlock{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	desktopExecute(t, s, app.Request{Action: "open", Directory: h.store, Name: "CLI fixture", Passphrase: testPassphrase})
	return s
}

func desktopExecute(t *testing.T, s *app.Service, r app.Request) app.Data {
	t.Helper()
	out := s.Execute(r)
	if out.Status != "ok" {
		t.Fatalf("desktop %s failed: %s", r.Action, out.Code)
	}
	return out.Data
}

func runDesktopCommand(t *testing.T, h *harness, input string, args ...string) string {
	t.Helper()
	c := h.command(input)
	defer c.close()
	run, ok := commands[args[0]]
	if !ok {
		run = (*command).show
	} else {
		args = args[1:]
	}
	if err := run(c, args); err != nil {
		t.Fatalf("CLI %v failed: %v", args, err)
	}
	if c.store != nil && c.store.Vault.Status().Unlocked {
		t.Fatal("CLI unlocked its own private key instead of using the desktop session")
	}
	return c.stdout.(*bytes.Buffer).String()
}

func TestCommandsReuseUnlockedDesktopSession(t *testing.T) {
	h := newHarness(t)
	s := openDesktopHarness(t, h)
	settings := app.DefaultSettings()
	settings.DeviceUnlock = false
	settings.BrowseWhileLocked = false
	desktopExecute(t, s, app.Request{Action: "settings", Settings: &settings})

	runDesktopCommand(t, h, "secret\n", "insert", "-e", "web/mail")
	if got := runDesktopCommand(t, h, "", "show", "web/mail"); got != "secret\n" {
		t.Fatalf("shared-session show = %q", got)
	}
	if got := runDesktopCommand(t, h, "", "ls"); !strings.Contains(got, "mail") {
		t.Fatalf("listing with locked browsing disabled = %q", got)
	}
	runDesktopCommand(t, h, "pw\nuser: alice\notpauth://totp/fixture?secret=JBSWY3DPEHPK3PXP\n", "insert", "-m", "bank")
	if got := runDesktopCommand(t, h, "", "grep", "alice"); got != "bank:\nuser: alice\n" {
		t.Fatalf("shared-session grep = %q", got)
	}
	if got := runDesktopCommand(t, h, "", "otp", "bank"); len(strings.TrimSpace(got)) != 6 {
		t.Fatalf("shared-session OTP = %q", got)
	}
	runDesktopCommand(t, h, "", "generate", "-n", "generated", "20")
	if got := runDesktopCommand(t, h, "", "show", "generated"); len(got) != 21 {
		t.Fatalf("generated password length = %d", len(got))
	}
	runDesktopCommand(t, h, "", "generate", "-i", "bank", "12")
	if got := runDesktopCommand(t, h, "", "show", "bank"); !strings.HasSuffix(got, "\nuser: alice\notpauth://totp/fixture?secret=JBSWY3DPEHPK3PXP\n") {
		t.Fatal("in-place generation lost the existing metadata")
	}
	runDesktopCommand(t, h, "", "cp", "web/mail", "copied")
	runDesktopCommand(t, h, "", "mv", "web/mail", "moved")
	runDesktopCommand(t, h, "", "rm", "-f", "moved")
	if got := runDesktopCommand(t, h, "", "show", "copied"); got != "secret\n" {
		t.Fatalf("shared-session copy = %q", got)
	}
	if _, err := os.Stat(filepath.Join(h.store, "moved.gpg")); !os.IsNotExist(err) {
		t.Fatalf("shared-session deletion did not remove ciphertext: %v", err)
	}
	if s.IsLocked() {
		t.Fatal("closing CLI commands locked the desktop session")
	}
}

func TestDesktopSessionInitialFallbackSelection(t *testing.T) {
	h := newHarness(t)
	checkFallback := func(label string, c *command) {
		t.Helper()
		defer c.close()
		store, err := c.open()
		if err != nil {
			t.Fatal(err)
		}
		remote, err := c.tryDesktop(store)
		if remote != nil {
			defer remote.client.Close()
		}
		if err != nil || remote != nil {
			t.Fatalf("%s should keep independent CLI unlocking: remote=%v err=%v", label, remote != nil, err)
		}
		if store.Vault.Status().Unlocked {
			t.Fatalf("%s unexpectedly unlocked local keys", label)
		}
	}
	checkFallback("desktop not running", h.command(""))
	s := openDesktopHarness(t, h)
	desktopExecute(t, s, app.Request{Action: "lock"})
	locked := h.command("")
	defer locked.close()
	store, err := locked.open()
	if err != nil {
		t.Fatal(err)
	}
	remote, err := locked.tryDesktop(store)
	if remote != nil {
		defer remote.client.Close()
	}
	if err != nil || remote == nil || !remote.needsUnlock || remote.session == "" {
		t.Fatalf("locked desktop should offer shared authentication: selected=%v err=%v", remote != nil, err)
	}
	if !s.IsLocked() || store.Vault.Status().Unlocked {
		t.Fatal("passive handshake unlocked the desktop or CLI")
	}
	desktopExecute(t, s, app.Request{Action: "unlock", Passphrase: testPassphrase})
	other := newHarness(t)
	c := other.command("")
	c.dataDir = h.data // Same desktop endpoint, a different PASSWORD_STORE_DIR.
	checkFallback("different selected vault", c)
	other = newHarness(t)
	c = h.command("")
	c.dataDir = other.data // Same store, another channel/application directory.
	checkFallback("isolated application data directory", c)
}

func TestDesktopSessionSupportsLargeEntry(t *testing.T) {
	h := newHarness(t)
	openDesktopHarness(t, h)
	contents := strings.Repeat("fixture<&>\x00\n", 100000)
	runDesktopCommand(t, h, contents, "insert", "-m", "large")
	if got := runDesktopCommand(t, h, "", "show", "large"); got != contents {
		t.Fatalf("large shared-session entry round trip changed contents (length=%d, want=%d)", len(got), len(contents))
	}
}

func TestSelectedDesktopSessionDoesNotFallBackAfterInvalidation(t *testing.T) {
	for _, action := range []string{"lock", "lock-and-unlock", "switch", "close"} {
		t.Run(action, func(t *testing.T) {
			h := newHarness(t)
			s := openDesktopHarness(t, h)
			desktopExecute(t, s, app.Request{Action: "save", Entry: "note", Contents: "original\n", Mode: "create"})
			c := h.command("")
			defer c.close()
			v, err := c.unlock()
			if err != nil {
				t.Fatal(err)
			}
			doc, err := v.Read("note")
			if err != nil {
				t.Fatal(err)
			}
			switch action {
			case "lock", "lock-and-unlock":
				desktopExecute(t, s, app.Request{Action: "lock"})
				if action == "lock-and-unlock" {
					desktopExecute(t, s, app.Request{Action: "unlock", Passphrase: testPassphrase})
				}
			case "switch":
				desktopExecute(t, s, app.Request{Action: "create", Directory: filepath.Join(t.TempDir(), "other store"), Name: "Other", Passphrase: testPassphrase})
			case "close":
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := v.Read("note"); err == nil {
				t.Fatal("invalidated shared session returned plaintext")
			}
			if _, err := v.Save("note", "stale change\n", doc.Revision); err == nil {
				t.Fatal("invalidated shared session accepted a write")
			}
			if c.store.Vault.Status().Unlocked {
				t.Fatal("CLI silently fell back to independently unlocked keys")
			}
			if err := c.store.Vault.Unlock(testPassphrase); err != nil {
				t.Fatal(err)
			}
			current, err := c.store.Vault.Read("note")
			if err != nil || current.Raw != "original\n" {
				t.Fatalf("invalidated operation changed ciphertext: %v", err)
			}
		})
	}
}

func TestDesktopMutationTransportFailureIsNotReplayed(t *testing.T) {
	h := newHarness(t)
	var saves atomic.Int32
	var activeServer atomic.Pointer[nativehost.Server]
	var server *nativehost.Server
	var err error
	server, err = nativehost.StartWithFrameLimit(clisession.Directory(h.data), clisession.MaxFrame, func(raw json.RawMessage) json.RawMessage {
		var request clisession.Request
		if err := json.Unmarshal(raw, &request); err != nil {
			panic(err)
		}
		response := clisession.Response{Protocol: clisession.Protocol, Status: "ok", Session: "fixture-session"}
		switch request.Action {
		case "status":
		case "read":
			response.Status, response.Code = "error", "not_found"
		case "save":
			saves.Add(1)
			// The operation was accepted, but its response is lost. Reconnecting
			// or independently unlocking must never repeat the mutation.
			_ = activeServer.Load().Close()
		default:
			response.Status, response.Code = "error", "invalid_request"
		}
		encoded, _ := json.Marshal(response)
		return encoded
	})
	if err != nil {
		t.Fatal(err)
	}
	activeServer.Store(server)
	defer server.Close()
	c := h.command("")
	defer c.close()
	if err := c.write("new", "fixture-content"); err == nil {
		t.Fatal("a lost mutation response was reported as success")
	}
	if saves.Load() != 1 {
		t.Fatalf("mutation executed %d times", saves.Load())
	}
	if c.store.Vault.Status().Unlocked {
		t.Fatal("lost mutation response fell back to local unlock")
	}
}

func TestDesktopGrepStopsWhenSessionIsInvalidated(t *testing.T) {
	h := newHarness(t)
	var reads atomic.Int32
	server, err := nativehost.StartWithFrameLimit(clisession.Directory(h.data), clisession.MaxFrame, func(raw json.RawMessage) json.RawMessage {
		var request clisession.Request
		if err := json.Unmarshal(raw, &request); err != nil {
			panic(err)
		}
		response := clisession.Response{Protocol: clisession.Protocol, Status: "ok", Session: "fixture-session"}
		switch request.Action {
		case "status":
		case "list":
			response.Entries = []vault.Entry{{Path: "first"}, {Path: "second"}}
		case "read":
			reads.Add(1)
			response.Status, response.Code = "error", "cancelled"
		default:
			response.Status, response.Code = "error", "invalid_request"
		}
		encoded, _ := json.Marshal(response)
		return encoded
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	c := h.command("")
	defer c.close()
	if err := c.grep([]string{"fixture"}); !errors.Is(err, vault.ErrCancelled) {
		t.Fatalf("grep with invalidated shared session = %v", err)
	}
	if reads.Load() != 1 {
		t.Fatalf("grep continued reading after invalidation: reads=%d", reads.Load())
	}
	if c.stdout.(*bytes.Buffer).Len() != 0 {
		t.Fatal("invalidated grep printed stale matches")
	}
}

func TestDesktopEditRoundTripAndRejectsUnchanged(t *testing.T) {
	h := newHarness(t)
	s := openDesktopHarness(t, h)
	desktopExecute(t, s, app.Request{Action: "save", Entry: "note", Contents: "one\n", Mode: "create"})
	editor := installDesktopEditor(t, h)
	t.Setenv("PASSPINE_TEST_CLI_EDITOR", "1")
	marker := filepath.Join(t.TempDir(), "edited-file")
	t.Setenv("PASSPINE_TEST_CLI_EDITOR_FILE", marker)
	assertCleaned := func() {
		t.Helper()
		file, err := os.ReadFile(marker)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(string(file)); !os.IsNotExist(err) {
			t.Fatalf("desktop editor plaintext was not removed: %v", err)
		}
	}
	h.editor = editor + " -test.run=^TestCLIEditorProcess$ -- replace"
	runDesktopCommand(t, h, "", "edit", "note")
	assertCleaned()
	if got := runDesktopCommand(t, h, "", "show", "note"); got != "two\n" {
		t.Fatalf("shared-session edit = %q", got)
	}
	h.editor = editor + " -test.run=^TestCLIEditorProcess$ -- unchanged"
	c := h.command("")
	defer c.close()
	if err := c.edit([]string{"note"}); err != exitError(1) {
		t.Fatalf("unchanged shared-session edit = %v", err)
	}
	assertCleaned()
	if c.store.Vault.Status().Unlocked {
		t.Fatal("unchanged edit independently unlocked CLI keys")
	}
}

func TestDesktopEditRejectsSaveAfterLockAndCleansPlaintext(t *testing.T) {
	h := newHarness(t)
	s := openDesktopHarness(t, h)
	desktopExecute(t, s, app.Request{Action: "save", Entry: "note", Contents: "original\n", Mode: "create"})
	installDesktopEditor(t, h)
	marker := filepath.Join(t.TempDir(), "editor-ready")
	release := filepath.Join(t.TempDir(), "editor-release")
	t.Setenv("PASSPINE_TEST_DESKTOP_EDITOR", "1")
	t.Setenv("PASSPINE_TEST_DESKTOP_EDITOR_READY", marker)
	t.Setenv("PASSPINE_TEST_DESKTOP_EDITOR_RELEASE", release)
	c := h.command("")
	defer c.close()
	done := make(chan error, 1)
	go func() { done <- c.edit([]string{"note"}) }()
	// Always release the isolated helper even when a fixture assertion fails.
	t.Cleanup(func() { _ = os.WriteFile(release, nil, 0o600) })
	deadline := time.Now().Add(20 * time.Second)
	var file []byte
	for time.Now().Before(deadline) {
		var err error
		file, err = os.ReadFile(marker)
		if err == nil && len(file) != 0 {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("edit ended before starting the native editor: %v", err)
		case <-time.After(5 * time.Millisecond):
		}
	}
	if len(file) == 0 {
		t.Fatal("native editor did not start")
	}
	desktopExecute(t, s, app.Request{Action: "lock"})
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, vault.ErrLocked) && !errors.Is(err, vault.ErrCancelled) {
			t.Fatalf("edit after lock returned %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("native editor did not finish")
	}
	if _, err := os.Stat(string(file)); !os.IsNotExist(err) {
		t.Fatalf("plaintext editor file remains after cancelled save: %v", err)
	}
	desktopExecute(t, s, app.Request{Action: "unlock", Passphrase: testPassphrase})
	if got := runDesktopCommand(t, h, "", "show", "note"); got != "original\n" {
		t.Fatal("expired editor changed the password store")
	}
}

func installDesktopEditor(t *testing.T, h *harness) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	tools := filepath.Join(t.TempDir(), "editor tools with spaces")
	if err := os.Mkdir(tools, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "passpine-desktop-test-editor"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(tools, name), binary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	h.editor = name + " -test.run=^TestCLIDesktopEditorProcess$ --"
	return name
}

func TestCLIDesktopEditorProcess(t *testing.T) {
	if os.Getenv("PASSPINE_TEST_DESKTOP_EDITOR") != "1" {
		return
	}
	file := os.Args[len(os.Args)-1]
	if err := os.WriteFile(os.Getenv("PASSPINE_TEST_DESKTOP_EDITOR_READY"), []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(os.Getenv("PASSPINE_TEST_DESKTOP_EDITOR_RELEASE")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("editor release timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := os.WriteFile(file, []byte("edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(os.Stdout, "fixture editor completed")
}
