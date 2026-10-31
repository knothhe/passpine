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
	"unicode/utf8"

	"github.com/knothhe/passpine/client/internal/clisession"
	"github.com/knothhe/passpine/client/nativehost"
	"github.com/knothhe/passpine/client/vault"
)

func TestDesktopInvalidMetadataIsRejectedBeforeSending(t *testing.T) {
	h := newHarness(t)
	var requests atomic.Int32
	server, err := nativehost.StartWithFrameLimit(clisession.Directory(h.data), clisession.MaxFrame, func(json.RawMessage) json.RawMessage {
		requests.Add(1)
		return json.RawMessage(`{"protocol":1,"status":"error","code":"unexpected_request"}`)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	c := h.command("")
	defer c.close()
	store, err := c.open()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Vault.Unlock(testPassphrase); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Vault.Save("original", "original fixture\n", ""); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(h.store, "original.gpg")
	before, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	invalid := "entry-" + string([]byte{0xff})
	remote := &desktopVault{
		client:    nativehost.NewRelayClientWithFrameLimit(clisession.Directory(h.data), clisession.MaxFrame),
		directory: h.store, session: strings.Repeat("a", 64),
	}
	defer remote.client.Close()
	if _, err := remote.Save(invalid, "new fixture\n", ""); !errors.Is(err, vault.ErrPath) {
		t.Fatalf("invalid entry save = %v", err)
	}
	if _, err := remote.Read(invalid); !errors.Is(err, vault.ErrPath) {
		t.Fatalf("invalid entry read = %v", err)
	}
	if err := remote.Move("original", invalid, "fixture-revision"); !errors.Is(err, vault.ErrPath) {
		t.Fatalf("invalid destination move = %v", err)
	}
	store.Info.Directory = filepath.Join(h.store, "directory-"+string([]byte{0xff}))
	if selected, err := c.tryDesktop(store); !errors.Is(err, vault.ErrPath) || selected != nil {
		t.Fatalf("invalid directory silently selected a backend: remote=%v err=%v", selected != nil, err)
	}
	if requests.Load() != 0 {
		t.Fatal("invalid filesystem metadata crossed the local transport")
	}
	if _, err := os.Stat(filepath.Join(h.store, strings.ToValidUTF8(invalid, "\ufffd")+".gpg")); !os.IsNotExist(err) {
		t.Fatalf("invalid metadata created a replacement-character filename: %v", err)
	}
	after, err := os.ReadFile(original)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejected metadata changed the existing ciphertext")
	}
}

func TestDesktopListRejectsUnrepresentableMetadata(t *testing.T) {
	h := newHarness(t)
	local := h.command("")
	defer local.close()
	store, err := local.open()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Vault.Unlock(testPassphrase); err != nil {
		t.Fatal(err)
	}
	invalid := "entry-" + string([]byte{0xff})
	_, saveErr := store.Vault.Save(invalid, "metadata fixture\n", "")
	files, err := os.ReadDir(h.store)
	if err != nil {
		t.Fatal(err)
	}
	unrepresentable := false
	actualNames := map[string]bool{}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".gpg") {
			name := strings.TrimSuffix(file.Name(), ".gpg")
			unrepresentable = unrepresentable || !utf8.ValidString(name)
			actualNames[name] = true
		}
	}
	s := openDesktopHarness(t, h)
	c := h.command("")
	defer c.close()
	v, err := c.unlock()
	if err != nil {
		t.Fatal(err)
	}
	if unrepresentable {
		entries, err := v.List()
		if !errors.Is(err, vault.ErrPath) || len(entries) != 0 {
			t.Fatalf("shared list returned altered filesystem metadata: entries=%d err=%v", len(entries), err)
		}
	} else {
		// Native UTF-16 filesystems may reject or normalize the byte spelling.
		// List the actual names unchanged while rejecting the invalid spelling.
		t.Logf("native filesystem does not preserve invalid UTF-8 entry names (save error: %v)", saveErr)
		entries, err := v.List()
		if err != nil || len(entries) != len(actualNames) {
			t.Fatalf("native metadata list failed: entries=%d want=%d err=%v", len(entries), len(actualNames), err)
		}
		for _, entry := range entries {
			if !actualNames[entry.Path] || entry.Name != entry.Path || entry.Folder != "" {
				t.Fatal("shared list changed the native filesystem's actual metadata")
			}
		}
		if _, err := v.Read(invalid); !errors.Is(err, vault.ErrPath) {
			t.Fatalf("native metadata spelling escaped the client guard: %v", err)
		}
	}
	if s.IsLocked() {
		t.Fatal("metadata rejection locked the desktop session")
	}
}

func TestDesktopServerRejectsInvalidUTF8JSONMetadata(t *testing.T) {
	h := newHarness(t)
	openDesktopHarness(t, h)
	c := h.command("")
	defer c.close()
	if _, err := c.unlock(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(clisession.Request{
		Protocol: clisession.Protocol, PayloadVersion: clisession.PayloadVersion,
		Action: "read", Directory: h.store, Session: c.remote.session, Path: "invalid-marker",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte("invalid-marker"), []byte{'b', 'a', 'd', '-', 0xff}, 1)
	var response clisession.Response
	if err := json.Unmarshal(c.remote.client.Send(raw), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != "invalid_request" || response.Document != nil || len(response.Entries) != 0 {
		t.Fatal("server normalized invalid UTF-8 metadata before validation")
	}
}
