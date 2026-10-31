package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/internal/clisession"
	"github.com/knothhe/passpine/client/nativehost"
	"github.com/knothhe/passpine/client/vault"
)

// Terminal fixtures exercise the real retry/read path without touching the
// controlling terminal, real credentials or a platform authentication prompt.
func sharedUnlockTerminal(t *testing.T, c *command, input string) (*atomic.Int32, string) {
	t.Helper()
	in, err := os.CreateTemp(t.TempDir(), "passphrase-input")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close() })
	if _, err := in.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	out, err := os.CreateTemp(t.TempDir(), "terminal-output")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = out.Close() })
	calls := &atomic.Int32{}
	c.openTerminal = func() (*os.File, *os.File, error) {
		calls.Add(1)
		return in, out, nil
	}
	return calls, out.Name()
}

func TestCLIPassphraseUnlocksDesktopAndNextCommandReusesSession(t *testing.T) {
	h := newHarness(t)
	s := openDesktopHarness(t, h)
	desktopExecute(t, s, app.Request{Action: "save", Entry: "note", Contents: "fixture-content\n", Mode: "create"})
	desktopExecute(t, s, app.Request{Action: "lock"})
	c := h.command("")
	calls, terminalOutput := sharedUnlockTerminal(t, c, "wrong-fixture-passphrase\n"+testPassphrase+"\n")
	defer c.close()
	if err := c.show([]string{"note"}); err != nil {
		t.Fatalf("shared passphrase authentication failed: %v", err)
	}
	if calls.Load() != 1 || c.remote == nil || c.remote.needsUnlock || c.store.Vault.Status().Unlocked {
		t.Fatal("CLI did not authenticate through the selected desktop session")
	}
	if s.IsLocked() || desktopExecute(t, s, app.Request{Action: "status"}).Locked {
		t.Fatal("successful CLI passphrase authentication left the desktop locked")
	}
	if got := c.stdout.(*bytes.Buffer).String(); got != "fixture-content\n" {
		t.Fatal("CLI did not read the desktop vault after authentication")
	}
	feedback, err := os.ReadFile(terminalOutput)
	if err != nil || bytes.Count(feedback, []byte("Incorrect private key passphrase.")) != 1 {
		t.Fatalf("wrong passphrase did not receive one retry: %v", err)
	}
	c.close()
	if s.IsLocked() {
		t.Fatal("CLI completion locked the newly authenticated desktop session")
	}
	next := h.command("")
	defer next.close()
	next.openTerminal = func() (*os.File, *os.File, error) {
		t.Fatal("next command asked for an already shared passphrase")
		return nil, nil, errors.New("unexpected terminal access")
	}
	if err := next.show([]string{"note"}); err != nil {
		t.Fatalf("next command could not reuse CLI-created desktop session: %v", err)
	}
	if next.remote == nil || next.store.Vault.Status().Unlocked {
		t.Fatal("next command used independent keys")
	}
}

func TestCLIWrongPassphrasesKeepBothProcessesLocked(t *testing.T) {
	h := newHarness(t)
	s := openDesktopHarness(t, h)
	desktopExecute(t, s, app.Request{Action: "save", Entry: "note", Contents: "fixture-content\n", Mode: "create"})
	desktopExecute(t, s, app.Request{Action: "lock"})
	c := h.command("")
	defer c.close()
	_, terminalOutput := sharedUnlockTerminal(t, c, "wrong-one\nwrong-two\nwrong-three\n")
	if err := c.show([]string{"note"}); !errors.Is(err, vault.ErrPassphrase) {
		t.Fatalf("three incorrect passphrases returned %v", err)
	}
	if !s.IsLocked() || c.store.Vault.Status().Unlocked || c.remote == nil {
		t.Fatal("failed authentication unlocked keys or abandoned the desktop backend")
	}
	if c.stdout.(*bytes.Buffer).Len() != 0 {
		t.Fatal("failed authentication emitted credential content")
	}
	feedback, err := os.ReadFile(terminalOutput)
	if err != nil || bytes.Count(feedback, []byte("Incorrect private key passphrase.")) != 3 {
		t.Fatalf("incorrect passphrase retry count changed: %v", err)
	}
}

type sharedUnlockDevice struct {
	unlocks atomic.Int32
	fail    bool
}

func (*sharedUnlockDevice) Available() bool { return true }
func (*sharedUnlockDevice) Enroll(_ context.Context, _ string, secret []byte) ([]byte, error) {
	if !bytes.Equal(secret, []byte(testPassphrase)) {
		return nil, errors.New("unexpected fixture passphrase")
	}
	return []byte(`{"version":1,"ciphertext":"isolated-fixture-envelope"}`), nil
}
func (d *sharedUnlockDevice) Unlock(context.Context, string, []byte) ([]byte, error) {
	d.unlocks.Add(1)
	if d.fail {
		return nil, errors.New("fixture authentication failed")
	}
	return []byte(testPassphrase), nil
}

func TestCLIDeviceAuthenticationUnlocksDesktopWithPassphraseFallback(t *testing.T) {
	for _, failed := range []bool{false, true} {
		label := "system verification succeeds"
		if failed {
			label = "system verification fails and passphrase succeeds"
		}
		t.Run(label, func(t *testing.T) {
			h := newHarness(t)
			provider := &sharedUnlockDevice{fail: failed}
			s, err := app.New(app.Options{DataDir: h.data, DeviceUnlock: provider})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			desktopExecute(t, s, app.Request{Action: "open", Directory: h.store, Passphrase: testPassphrase})
			desktopExecute(t, s, app.Request{Action: "save", Entry: "note", Contents: "fixture-content\n", Mode: "create"})
			if !desktopExecute(t, s, app.Request{Action: "status"}).DeviceUnlockEnrolled {
				t.Fatal("fixture system authentication was not enrolled")
			}
			desktopExecute(t, s, app.Request{Action: "lock"})
			c := h.command("")
			defer c.close()
			if failed {
				sharedUnlockTerminal(t, c, testPassphrase+"\n")
			} else {
				c.openTerminal = func() (*os.File, *os.File, error) {
					t.Fatal("successful desktop system authentication prompted for a passphrase")
					return nil, nil, errors.New("unexpected terminal access")
				}
			}
			if err := c.show([]string{"note"}); err != nil {
				t.Fatalf("desktop authentication failed: %v", err)
			}
			if provider.unlocks.Load() != 1 || s.IsLocked() || c.store.Vault.Status().Unlocked || c.remote == nil {
				t.Fatal("system authentication was not performed by the desktop backend")
			}
			if failed && c.stderr.(*bytes.Buffer).Len() == 0 {
				t.Fatal("system authentication failure did not explain passphrase fallback")
			}
		})
	}
}

func TestCLISharedUnlockCancelledWhileReadingPassphraseDoesNotFallBack(t *testing.T) {
	h := newHarness(t)
	s := openDesktopHarness(t, h)
	desktopExecute(t, s, app.Request{Action: "lock"})
	c := h.command("")
	defer c.close()
	sharedUnlockTerminal(t, c, testPassphrase+"\n")
	open := c.openTerminal
	c.openTerminal = func() (*os.File, *os.File, error) {
		// A fresh lock must invalidate the challenge already selected by CLI.
		desktopExecute(t, s, app.Request{Action: "lock"})
		return open()
	}
	if _, err := c.unlock(); !errors.Is(err, vault.ErrCancelled) {
		t.Fatalf("authentication against a cancelled challenge returned %v", err)
	}
	if !s.IsLocked() || c.store.Vault.Status().Unlocked || c.remote == nil {
		t.Fatal("cancelled shared unlock fell back or revived desktop keys")
	}
}

func TestCLISharedUnlockTransportFailureIsNotReplayed(t *testing.T) {
	h := newHarness(t)
	var attempts atomic.Int32
	var active atomic.Pointer[nativehost.Server]
	server, err := nativehost.StartWithFrameLimit(clisession.Directory(h.data), clisession.MaxFrame, func(raw json.RawMessage) json.RawMessage {
		var r clisession.Request
		if err := json.Unmarshal(raw, &r); err != nil {
			panic(err)
		}
		out := clisession.Response{Protocol: clisession.Protocol, Status: "error", Code: "locked", Session: strings.Repeat("a", 64), CanUnlock: true}
		if r.Action == "unlock" {
			attempts.Add(1)
			_ = active.Load().Close()
		}
		encoded, _ := json.Marshal(out)
		return encoded
	})
	if err != nil {
		t.Fatal(err)
	}
	active.Store(server)
	defer server.Close()
	c := h.command("")
	defer c.close()
	sharedUnlockTerminal(t, c, testPassphrase+"\n")
	if _, err := c.unlock(); !errors.Is(err, clisession.ErrUnavailable) {
		t.Fatalf("lost authentication response returned %v", err)
	}
	if attempts.Load() != 1 || c.store.Vault.Status().Unlocked || c.remote == nil {
		t.Fatal("uncertain authentication was retried or fell back to independent keys")
	}
}

func TestCLIWithoutDesktopKeepsIndependentPassphraseUnlock(t *testing.T) {
	h := newHarness(t)
	c := h.command("")
	defer c.close()
	sharedUnlockTerminal(t, c, testPassphrase+"\n")
	if _, err := c.unlock(); err != nil {
		t.Fatalf("offline CLI passphrase authentication failed: %v", err)
	}
	if c.remote != nil || !c.store.Vault.Status().Unlocked {
		t.Fatal("offline command did not independently unlock its vault")
	}
}

func TestCLIOlderLockedDesktopRequiresRestartWithoutIndependentUnlock(t *testing.T) {
	for _, test := range []struct {
		name      string
		canUnlock bool
		session   string
	}{
		{name: "old desktop without an unlock challenge"},
		{name: "unsupported desktop with a session", session: strings.Repeat("a", 64)},
		{name: "invalid unlock challenge", canUnlock: true, session: "incomplete"},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t)
			var unlockRequests atomic.Int32
			server, err := nativehost.StartWithFrameLimit(clisession.Directory(h.data), clisession.MaxFrame, func(raw json.RawMessage) json.RawMessage {
				var request clisession.Request
				if err := json.Unmarshal(raw, &request); err != nil {
					panic(err)
				}
				if request.Action == "unlock" {
					unlockRequests.Add(1)
				}
				encoded, _ := json.Marshal(clisession.Response{
					Protocol: clisession.Protocol, Status: "error", Code: "locked",
					CanUnlock: test.canUnlock, Session: test.session,
				})
				return encoded
			})
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			c := h.command("")
			defer c.close()
			var terminalCalls atomic.Int32
			c.openTerminal = func() (*os.File, *os.File, error) {
				terminalCalls.Add(1)
				return nil, nil, errors.New("unexpected terminal access")
			}
			_, err = c.unlock()
			want := "The desktop client does not support shared unlocking yet. Restart the updated client and try again."
			if err == nil || c.describe(err) != want {
				t.Fatalf("older desktop did not request restart: %v", err)
			}
			if terminalCalls.Load() != 0 || unlockRequests.Load() != 0 || c.store.Vault.Status().Unlocked || c.remote != nil {
				t.Fatal("older locked desktop triggered independent or unsupported authentication")
			}
		})
	}
}
