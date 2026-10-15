package vault

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
)

func rotationFixture(t *testing.T) *Vault {
	t.Helper()
	config := cryptoConfig()
	config.Time = func() time.Time { return time.Now().Add(-time.Hour) }
	entity, e := openpgp.NewEntity("Rotating identity", "", "rotation@example.invalid", config)
	if e != nil {
		t.Fatal(e)
	}
	data, e := protectedKeyData(openpgp.EntityList{entity}, testPassphrase)
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	v, e := New(Config{Directory: filepath.Join(dir, "store"), KeyFile: filepath.Join(dir, "keys/secret.asc")})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(v.Lock)
	if _, e = v.ImportKeys(data, testPassphrase); e != nil {
		t.Fatal(e)
	}
	return v
}

func rotateEncryptionKey(t *testing.T, v *Vault, publicOnly bool) {
	t.Helper()
	keys, _, e := v.token()
	if e != nil {
		t.Fatal(e)
	}
	config := cryptoConfig()
	config.Time = func() time.Time { return time.Now().Add(-time.Minute) }
	if e = keys[0].AddEncryptionSubkey(config); e != nil {
		t.Fatal(e)
	}
	if publicOnly {
		keys[0].Subkeys[len(keys[0].Subkeys)-1].PrivateKey = nil
	}
	data, e := protectedKeyData(keys, testPassphrase)
	if e != nil {
		t.Fatal(e)
	}
	// Restore the live fixture's key state before exercising the public import.
	if e = decryptKeys(keys, testPassphrase, true); e != nil {
		t.Fatal(e)
	}
	if _, e = v.ImportKeys(data, testPassphrase); e != nil {
		t.Fatal(e)
	}
}

func TestMissingCurrentPrivateSubkeyCannotDestroyReadableEntry(t *testing.T) {
	v := rotationFixture(t)
	doc := mustSave(t, v, "old-entry", "keep this secret", "")
	rotateEncryptionKey(t, v, true)
	keys, _, e := v.token()
	if e != nil {
		t.Fatal(e)
	}
	selected, ok := keys[0].EncryptionKey(time.Now())
	if !ok || selected.PrivateKey != nil {
		t.Fatal("fixture did not select the public-only current subkey")
	}
	if _, e = v.Save(doc.Path, "unreadable replacement", doc.Revision); e == nil {
		t.Fatal("accepted an update that cannot be decrypted locally")
	}
	if _, e = v.Save("new-entry", "unreadable new value", ""); e == nil {
		t.Fatal("accepted a new entry that cannot be decrypted locally")
	}
	if _, e = os.Stat(filepath.Join(v.config.Directory, "new-entry.gpg")); !os.IsNotExist(e) {
		t.Fatal("failed save wrote an unreadable entry")
	}
	if _, e = v.Trash(doc.Path, doc.Revision); e == nil {
		t.Fatal("deleted source before verifying the recovery recipient")
	}
	if _, e = v.SaveDraft(doc.Path, "unreadable draft"); e == nil {
		t.Fatal("accepted a draft that cannot be recovered")
	}
	read, e := v.Read(doc.Path)
	if e != nil || read.Raw != doc.Raw || read.Revision != doc.Revision {
		t.Fatal("original readable ciphertext was changed", e)
	}
	records, e := v.Recovery("")
	if e != nil || len(records) != 0 {
		t.Fatal("failed operations left unreadable recovery records", e)
	}
}

func TestSaveRequiresAtLeastOneLocallyDecryptableRecipient(t *testing.T) {
	v := newFixture(t)
	remote, e := openpgp.NewEntity("Remote recipient", "", "remote@example.invalid", cryptoConfig())
	if e != nil {
		t.Fatal(e)
	}
	var public bytes.Buffer
	if e = remote.Serialize(&public); e != nil {
		t.Fatal(e)
	}
	if _, e = v.ImportKeys(public.Bytes(), testPassphrase); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(filepath.Join(v.config.Directory, "shared"), 0700); e != nil {
		t.Fatal(e)
	}
	policyPath := filepath.Join(v.config.Directory, "shared/.gpg-id")
	if e = os.WriteFile(policyPath, []byte(fingerprint(remote)+"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = v.Save("shared/entry", "remote-only secret", ""); e == nil {
		t.Fatal("accepted a public-only recipient policy")
	}
	ids, e := v.Identities()
	if e != nil {
		t.Fatal(e)
	}
	var local string
	for _, id := range ids {
		if id.HasSecret {
			local = id.Fingerprint
			break
		}
	}
	if e = os.WriteFile(policyPath, []byte(fingerprint(remote)+"\n"+local+"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	mustSave(t, v, "shared/entry", "readable shared secret", "")
	read, e := v.Read("shared/entry")
	if e != nil || read.Raw != "readable shared secret" {
		t.Fatal("mixed local/public recipients rejected", e)
	}
}

func TestOlderKeyImportCannotOrphanDraftsAndTrash(t *testing.T) {
	v := rotationFixture(t)
	oldBackup, e := v.ExportSecret()
	if e != nil {
		t.Fatal(e)
	}
	rotateEncryptionKey(t, v, false)
	doc := mustSave(t, v, "to-delete", "recover this value", "")
	trash, e := v.Trash(doc.Path, doc.Revision)
	if e != nil {
		t.Fatal(e)
	}
	draft, e := v.SaveDraft("unfinished", "keep this draft")
	if e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(v.config.KeyFile)
	if e != nil {
		t.Fatal(e)
	}
	if e = v.VerifyBackup(oldBackup, testPassphrase); e == nil {
		t.Fatal("backup verification ignored encrypted recovery records")
	}
	if _, e = v.ImportKeys(oldBackup, testPassphrase); e == nil {
		t.Fatal("key downgrade discarded private recovery subkey")
	}
	after, e := os.ReadFile(v.config.KeyFile)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("rejected import changed the installed keyring", e)
	}
	for _, record := range []RecoveryRecord{trash, draft} {
		read, e := v.ReadRecovery(record.ID)
		if e != nil || read.Raw != record.Raw {
			t.Fatal("rejected import damaged an existing recovery record", e)
		}
	}
}

func TestParseNotesPreservesParagraphOrder(t *testing.T) {
	for _, tc := range []struct{ name, raw, want string }{
		{"single", "password\nnotes: a single note", "a single note"},
		{"alias", "password\nnote: singular note", "singular note"},
		{"multiline", "password\nnotes: first line\nsecond line\n\nthird paragraph", "first line\nsecond line\n\nthird paragraph"},
		{"ordered", "password\nbefore the note\nnotes: first note\n\nbetween notes\nusername: alice\nnote: second note\nafter the notes", "before the note\nfirst note\n\nbetween notes\nsecond note\nafter the notes"},
		{"duplicate", "password\nnotes: first\nnotes: second\nNOTES: third", "first\nsecond\nthird"},
		{"empty-prefix", "password\nnotes:\nfirst paragraph\n\nnext paragraph", "first paragraph\n\nnext paragraph"},
		{"crlf", "password\r\nnotes: first\r\nsecond", "first\nsecond"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := ParseDocument("entry", tc.raw, "revision")
			if doc.Notes != tc.want {
				t.Fatalf("notes=%q, want=%q", doc.Notes, tc.want)
			}
			if doc.Raw != tc.raw {
				t.Fatal("note parsing rewrote the source document")
			}
		})
	}
}

func TestGitStagesWildcardEntryNamesLiterally(t *testing.T) {
	if _, e := exec.LookPath("git"); e != nil {
		t.Skip("git unavailable")
	}
	v := newFixture(t)
	ctx := context.Background()
	if e := v.GitInitialize(ctx); e != nil {
		t.Fatal(e)
	}
	mustSave(t, v, "a*", "encrypted credential", "")
	directory := filepath.Join(v.config.Directory, "archive.gpg")
	if e := os.Mkdir(directory, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(directory, "private.txt"), []byte("must never be synchronized"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := v.GitSync(ctx); e != nil {
		t.Fatal(e)
	}
	tracked, e := gitCommand(ctx, v.config.Directory, "ls-files", "-z")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(tracked, "private.txt") {
		t.Fatal("wildcard pathspec included an unrelated plaintext file")
	}
	if !strings.Contains(tracked, "a*.gpg\x00") {
		t.Fatal("literal wildcard entry was not committed")
	}
}

func TestGitClonePreservesFileCreatedDuringDownload(t *testing.T) {
	git, e := exec.LookPath("git")
	if e != nil {
		t.Skip("git unavailable")
	}
	source := newFixture(t)
	ctx := context.Background()
	if e = source.GitInitialize(ctx); e != nil {
		t.Fatal(e)
	}
	mustSave(t, source, "entry", "encrypted credential", "")
	if _, e = source.GitSync(ctx); e != nil {
		t.Fatal(e)
	}
	// Existing empty directories remain supported without first deleting them.
	empty := filepath.Join(t.TempDir(), "empty")
	if e = os.Mkdir(empty, 0700); e != nil {
		t.Fatal(e)
	}
	if e = Clone(ctx, source.config.Directory, empty); e != nil {
		t.Fatal("clone into an empty directory failed", e)
	}
	target := filepath.Join(t.TempDir(), "concurrent-file")
	wrapperDirectory := t.TempDir()
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	// Deterministically create the destination after Git has downloaded the
	// checkout, but before Clone attempts its final installation.
	script := "#!/bin/sh\n" + quote(git) + " \"$@\" || exit $?\nfor arg in \"$@\"; do\n if [ \"$arg\" = clone ]; then\n  printf '%s' 'created during download' > " + quote(target) + "\n fi\ndone\n"
	if e = os.WriteFile(filepath.Join(wrapperDirectory, "git"), []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", wrapperDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	if e = Clone(ctx, source.config.Directory, target); e == nil {
		t.Fatal("clone replaced a concurrently created file")
	}
	data, e := os.ReadFile(target)
	if e != nil || string(data) != "created during download" {
		t.Fatal("clone removed or changed the concurrently created file", e)
	}
}
