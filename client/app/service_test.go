package app

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenExistingVaultAutomaticallyUsesSystemGPG(t *testing.T) {
	gpg, err := exec.LookPath("gpg")
	if err != nil {
		t.Skip("GnuPG is not installed")
	}
	home, err := os.MkdirTemp("/tmp", "passpine-app-gpg-")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GNUPGHOME", home)
	t.Cleanup(func() {
		if tool, err := exec.LookPath("gpgconf"); err == nil {
			_ = exec.Command(tool, "--homedir", home, "--kill", "gpg-agent").Run()
		}
		_ = os.RemoveAll(home)
	})
	source := testService(t)
	saved := must(t, source, Request{Action: "save", Entry: "system-gpg", Contents: "fixture-system-secret", Mode: "create"})
	backup, err := source.active.ExportSecret()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(backup)
	cmd := exec.Command(gpg, "--homedir", home, "--batch", "--import")
	cmd.Stdin = bytes.NewReader(backup)
	if err = cmd.Run(); err != nil {
		t.Fatal("cannot initialize isolated GPG fixture", err)
	}
	directory := must(t, source, Request{Action: "status"}).Directory
	if err = os.WriteFile(filepath.Join(directory, ".gpg-id"), []byte("test@example.invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	opened, err := New(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { opened.Close() })
	data := must(t, opened, Request{Action: "open", Directory: directory, Mode: "system-gpg", Passphrase: "test-passphrase-123"})
	if !data.Configured || data.Locked || data.Fingerprint == "" || data.Directory != directory {
		t.Fatal("system GPG open did not configure and unlock the store")
	}
	if got := must(t, opened, Request{Action: "fetch", Entry: saved.Entry}); got.Contents != saved.Contents {
		t.Fatal("system GPG session cannot read the existing ciphertext")
	}
	must(t, opened, Request{Action: "lock"})
	must(t, opened, Request{Action: "unlock", Passphrase: "test-passphrase-123"})
	preview, err := New(Options{DataDir: t.TempDir(), Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { preview.Close() })
	if result := preview.Execute(Request{Action: "open", Directory: directory, Mode: "system-gpg", Passphrase: "test-passphrase-123"}); result.Status != "error" || !strings.Contains(result.Message, "演示模式") {
		t.Fatal("preview accessed the system GPG keyring")
	}
}

func testService(t *testing.T) *Service {
	t.Helper()
	s, e := New(Options{DataDir: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	r := s.Execute(Request{Action: "create", Directory: filepath.Join(t.TempDir(), "vault"), Name: "Test", Email: "test@example.invalid", Passphrase: "test-passphrase-123"})
	if r.Status != "ok" {
		t.Fatal(r.Message)
	}
	return s
}
func must(t *testing.T, s *Service, r Request) Data {
	t.Helper()
	v := s.Execute(r)
	if v.Status != "ok" {
		t.Fatalf("%s: %s", r.Action, v.Message)
	}
	return v.Data
}
func TestSharedSessionLifecycle(t *testing.T) {
	s := testService(t)
	initial := must(t, s, Request{Action: "configure"})
	if initial.Locked || !initial.Configured {
		t.Fatal(initial)
	}
	saved := must(t, s, Request{Action: "save", Entry: "work/example.com", Contents: "secret\nusername: alex\nurl: https://example.com\n", Mode: "create"})
	var copied string
	s.SetClipboard(func(v string, _ int) error { copied = v; return nil })
	request := func(r Request) Response {
		r.Protocol = 1
		r.RequestID = "test"
		b, _ := json.Marshal(r)
		var out Response
		if e := json.Unmarshal(s.handleBrowser(b), &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	got := request(Request{Action: "fetch", VaultID: initial.VaultID, Entry: saved.Entry})
	if got.Data.Password != "secret" || got.RequestID != "test" {
		t.Fatal(got)
	}
	got = request(Request{Action: "copy", VaultID: initial.VaultID, Entry: saved.Entry, Field: "username"})
	if got.Status != "ok" || copied != "alex" {
		t.Fatal(got)
	}
	got = request(Request{Action: "fetch", VaultID: "old-vault", Entry: saved.Entry})
	if got.Code != "vault_changed" || got.Data.Password != "" {
		t.Fatal(got)
	}
	must(t, s, Request{Action: "lock"})
	got = request(Request{Action: "fetch", VaultID: initial.VaultID, Entry: saved.Entry})
	if got.Code != "locked" || got.Data.Contents != "" {
		t.Fatal(got)
	}
	must(t, s, Request{Action: "unlock", Passphrase: "test-passphrase-123"})
	moved := must(t, s, Request{Action: "save", Entry: saved.Entry, Destination: "work/new.example.com", Contents: saved.Contents, Revision: saved.Revision, Mode: "edit"})
	list := must(t, s, Request{Action: "list"})
	if len(list.Entries) != 1 || list.Entries[0].Path != moved.Entry {
		t.Fatal(list.Entries)
	}
	deleted := must(t, s, Request{Action: "delete", Entry: moved.Entry, Revision: moved.Revision})
	must(t, s, Request{Action: "restore", ID: deleted.TrashID, Destination: "restored"})
	if len(must(t, s, Request{Action: "list"}).Entries) != 1 {
		t.Fatal("restore list stale")
	}
	bytes, e := os.ReadFile(filepath.Join(s.directory, "activity.json"))
	if e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"test-passphrase-123", "https://example.com", "new.example.com", "secret"} {
		if strings.Contains(string(bytes), secret) {
			t.Fatalf("activity contains private data %q", secret)
		}
	}
}
func TestBrowserBoundaryAndMetadataOnly(t *testing.T) {
	s := testService(t)
	id := must(t, s, Request{Action: "status"}).VaultID
	for _, r := range []Request{{Protocol: 1, Action: "open", Directory: "/tmp"}, {Protocol: 1, Action: "unlock", Passphrase: "secret"}, {Protocol: 1, Action: "fetch", Entry: "anything"}, {Protocol: 2, Action: "configure"}, {Protocol: 1, Action: "fetch", VaultID: id, Kind: "draft", ID: "x"}} {
		b, _ := json.Marshal(r)
		var got Response
		_ = json.Unmarshal(s.handleBrowser(b), &got)
		if got.Status != "error" {
			t.Fatal(r)
		}
	}
	must(t, s, Request{Action: "save", Entry: "example.com", Contents: "very-secret", Mode: "create"})
	got := s.handleBrowser([]byte(`{"protocol":1,"action":"configure"}`))
	if strings.Contains(string(got), "very-secret") || strings.Contains(string(got), s.directory) {
		t.Fatal("configure leaked secret or local path")
	}
	cfg := s.prefs.Settings
	cfg.BrowseWhileLocked = true
	must(t, s, Request{Action: "settings", Settings: &cfg})
	must(t, s, Request{Action: "lock"})
	if len(must(t, s, Request{Action: "list"}).Entries) != 1 {
		t.Fatal("locked metadata missing")
	}
}
func TestRestoreDefaultsToOriginalPathWithoutOverwriting(t *testing.T) {
	s := testService(t)
	saved := must(t, s, Request{Action: "save", Entry: "work/original", Contents: "original-secret", Mode: "create"})
	deleted := must(t, s, Request{Action: "delete", Entry: saved.Entry, Revision: saved.Revision})
	restored := must(t, s, Request{Action: "restore", ID: deleted.TrashID})
	if restored.Entry != saved.Entry || restored.Contents != saved.Contents {
		t.Fatal("restore did not use the original path")
	}
	deleted = must(t, s, Request{Action: "delete", Entry: restored.Entry, Revision: restored.Revision})
	replacement := must(t, s, Request{Action: "save", Entry: saved.Entry, Contents: "replacement-secret", Mode: "create"})
	if result := s.Execute(Request{Action: "restore", ID: deleted.TrashID}); result.Code != "already_exists" {
		t.Fatalf("restore overwrote an occupied path: %s", result.Code)
	}
	if got := must(t, s, Request{Action: "fetch", Entry: saved.Entry}); got.Contents != replacement.Contents {
		t.Fatal("existing entry changed")
	}
	if got := must(t, s, Request{Action: "recovery", Kind: "trash"}); len(got.Recovery) != 1 || got.Recovery[0].ID != deleted.TrashID {
		t.Fatal("failed restore removed the recovery record")
	}
}
func TestDraftResumeAndEncryptedExport(t *testing.T) {
	s := testService(t)
	saved := must(t, s, Request{Action: "save", Entry: "old", Contents: "old-secret", Mode: "create"})
	draft := must(t, s, Request{Action: "draft", Entry: "old", Destination: "new", Revision: saved.Revision, Contents: "new-secret"})
	restored := must(t, s, Request{Action: "fetch", Kind: "draft", ID: draft.ID})
	if restored.OriginalEntry != "old" || restored.Destination != "new" || restored.Contents != "new-secret" {
		t.Fatal(restored)
	}
	key := filepath.Join(t.TempDir(), "recovery.asc")
	must(t, s, Request{Action: "export", Destination: key})
	must(t, s, Request{Action: "verify-backup", KeyFile: key, Passphrase: "test-passphrase-123"})
	if got := s.Execute(Request{Action: "export", Destination: key}); got.Status != "error" {
		t.Fatal("export overwrote file")
	}
}
func TestPreviewUsesOnlyIsolatedData(t *testing.T) {
	s, e := New(Options{DataDir: t.TempDir(), Preview: true})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	d := must(t, s, Request{Action: "configure"})
	if !d.Preview || d.Locked || len(d.Entries) < 8 || s.server != nil {
		t.Fatal("bad preview")
	}
}

func TestMetadataVersionsAndClipboardRevocation(t *testing.T) {
	s := testService(t)
	saved := must(t, s, Request{Action: "save", Entry: "first", Contents: "one", Mode: "create"})
	initial := must(t, s, Request{Action: "configure"})
	if _, e := s.active.SaveRename(saved.Entry, "second", "two", saved.Revision); e != nil {
		t.Fatal(e)
	}
	external := must(t, s, Request{Action: "configure"})
	if external.EntriesVersion <= initial.EntriesVersion || external.Entries[0].Path != "second" {
		t.Fatal("external changes not observed")
	}
	lease := s.ClipboardLease()
	must(t, s, Request{Action: "lock"})
	written := false
	if e := lease(func() { written = true }); e == nil || written {
		t.Fatal("late clipboard write after lock")
	}
}
func TestBrowserMutationNotifiesNativeUI(t *testing.T) {
	s := testService(t)
	id := must(t, s, Request{Action: "status"}).VaultID
	changed := make(chan struct{}, 1)
	s.SetOnChange(func() { changed <- struct{}{} })
	raw, _ := json.Marshal(Request{Protocol: 1, RequestID: "create", Action: "save", VaultID: id, Entry: "test", Contents: "secret", Mode: "create"})
	var response Response
	_ = json.Unmarshal(s.handleBrowser(raw), &response)
	if response.Status != "ok" {
		t.Fatal(response)
	}
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("no native UI change notification")
	}
}
func TestBrowserSerializationRejectsExpiredSession(t *testing.T) {
	s := testService(t)
	lease := s.ClipboardLease()
	out := Response{Protocol: 1, RequestID: "fetch", Status: "ok", Data: Data{Contents: "raw-secret", Password: "password-secret", Username: "private-username"}}
	must(t, s, Request{Action: "lock"})
	must(t, s, Request{Action: "unlock", Passphrase: "test-passphrase-123"})
	var encoded Response
	if err := json.Unmarshal(encodeBrowserResponse(out, lease), &encoded); err != nil {
		t.Fatal(err)
	}
	if encoded.Status != "error" || encoded.Code != "locked" || encoded.RequestID != out.RequestID || !encoded.Data.Locked {
		t.Fatal("stale browser response was not rejected")
	}
	if encoded.Data.Contents != "" || encoded.Data.Password != "" || encoded.Data.Username != "" {
		t.Fatal("expired session exposed credential data")
	}
}
func TestClosedServiceCannotStartClone(t *testing.T) {
	s := testService(t)
	_ = s.Close()
	if r := s.Execute(Request{Action: "clone", URL: "https://example.invalid/repo", Directory: filepath.Join(t.TempDir(), "clone")}); r.Code != "app_unavailable" {
		t.Fatal(r)
	}
}
