package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/internal/clisession"
	"github.com/knothhe/passpine/client/nativehost"
)

func TestDesktopSessionPreservesBinaryContents(t *testing.T) {
	h := newHarness(t)
	openDesktopHarness(t, h)
	local := h.command("")
	defer local.close()
	store, err := local.open()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Vault.Unlock(testPassphrase); err != nil {
		t.Fatal(err)
	}
	contents := []byte{'x', 0xff, 0x00, 0xc3, 0x28, 0xfe, '\n'}
	if _, err := store.Vault.Save("binary", string(contents), ""); err != nil {
		t.Fatal(err)
	}
	read := h.command("")
	defer read.close()
	if err := read.show([]string{"binary"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read.stdout.(*bytes.Buffer).Bytes(), contents) || read.remote == nil {
		t.Fatal("desktop read changed non-UTF-8 or NUL entry bytes")
	}
	write := h.command(string(contents))
	defer write.close()
	if err := write.insert([]string{"-m", "binary-copy"}); err != nil {
		t.Fatal(err)
	}
	if write.remote == nil || write.store.Vault.Status().Unlocked {
		t.Fatal("binary insert did not use the shared desktop session")
	}
	saved, err := store.Vault.Read("binary-copy")
	if err != nil || !bytes.Equal([]byte(saved.Raw), contents) {
		t.Fatalf("desktop save changed the original entry bytes: %v", err)
	}
}

func TestCLIPreservesBinaryPassphraseWhenUnlockingDesktop(t *testing.T) {
	h := &harness{t: t, data: t.TempDir(), store: filepath.Join(t.TempDir(), "vault")}
	s, err := app.New(app.Options{DataDir: h.data, DeviceUnlock: noDesktopDeviceUnlock{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	passphrase := append([]byte("isolated-binary-passphrase-"), 0xff, 0x00, 0xfe)
	defer clear(passphrase)
	desktopExecute(t, s, app.Request{Action: "create", Directory: h.store, Passphrase: string(passphrase)})
	desktopExecute(t, s, app.Request{Action: "save", Entry: "note", Contents: "fixture-content\n", Mode: "create"})
	desktopExecute(t, s, app.Request{Action: "lock"})
	c := h.command("")
	defer c.close()
	sharedUnlockTerminal(t, c, string(passphrase)+"\n")
	if err := c.show([]string{"note"}); err != nil {
		t.Fatalf("binary passphrase did not reach desktop authentication intact: %v", err)
	}
	if s.IsLocked() || c.remote == nil || c.store.Vault.Status().Unlocked || c.stdout.(*bytes.Buffer).String() != "fixture-content\n" {
		t.Fatal("binary passphrase did not establish the shared desktop session")
	}
}

func TestDesktopSessionRejectsLegacyAndFuturePayloadVersions(t *testing.T) {
	h := newHarness(t)
	s := openDesktopHarness(t, h)
	client := nativehost.NewRelayClientWithFrameLimit(clisession.Directory(h.data), clisession.MaxFrame)
	defer client.Close()
	for _, version := range []int{0, clisession.PayloadVersion + 1} {
		request := map[string]any{"protocol": clisession.Protocol, "action": "status", "directory": h.store}
		if version != 0 {
			request["payloadVersion"] = version
		}
		raw, _ := json.Marshal(request)
		var out clisession.Response
		if err := json.Unmarshal(client.Send(raw), &out); err != nil || out.Code != "cli_protocol_mismatch" || out.Session != "" || out.Document != nil {
			t.Fatalf("incompatible payload version %d was accepted: %v", version, err)
		}
	}
	// Even text that happens to be valid base64 must not be interpreted as
	// byte-preserving contents without agreeing on the payload format first.
	request := map[string]any{"protocol": clisession.Protocol, "action": "save", "directory": h.store, "path": "must-not-exist", "contents": "c2VjcmV0", "session": strings.Repeat("a", 64)}
	raw, _ := json.Marshal(request)
	var out clisession.Response
	if err := json.Unmarshal(client.Send(raw), &out); err != nil || out.Code != "cli_protocol_mismatch" {
		t.Fatalf("legacy text was interpreted using the new byte encoding: %v", err)
	}
	if _, err := os.Stat(filepath.Join(h.store, "must-not-exist.gpg")); !os.IsNotExist(err) || s.IsLocked() {
		t.Fatal("incompatible payload changed the vault or its session")
	}
}

func TestCLIRequestsRestartForLegacyTextPayloadEndpoint(t *testing.T) {
	h := newHarness(t)
	var calls atomic.Int32
	server, err := nativehost.StartWithFrameLimit(clisession.Directory(h.data), clisession.MaxFrame, func(raw json.RawMessage) json.RawMessage {
		calls.Add(1)
		// The previous private endpoint strictly decoded this older shape.
		var legacy struct {
			Protocol  int    `json:"protocol"`
			Action    string `json:"action"`
			Directory string `json:"directory"`
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&legacy) == nil {
			t.Error("new CLI omitted its byte-encoding version")
		}
		encoded, _ := json.Marshal(clisession.Response{Protocol: clisession.Protocol, Status: "error", Code: "invalid_request"})
		return encoded
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	c := h.command("")
	defer c.close()
	c.openTerminal = func() (*os.File, *os.File, error) {
		t.Error("incompatible desktop triggered independent authentication")
		return nil, nil, errors.New("unexpected terminal")
	}
	if _, err := c.unlock(); err == nil || c.describe(err) != "The desktop client does not support shared unlocking yet. Restart the updated client and try again." {
		t.Fatalf("incompatible desktop did not request restart: %v", err)
	}
	if calls.Load() != 1 || c.remote != nil || c.store.Vault.Status().Unlocked {
		t.Fatal("incompatible desktop was retried or independently unlocked")
	}
}
