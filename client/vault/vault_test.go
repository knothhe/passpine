package vault

import (
	"bytes"
	"context"
	"encoding/base32"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
)

const testPassphrase = "test-only long passphrase"

func newFixture(t *testing.T) *Vault {
	t.Helper()
	root := t.TempDir()
	v, e := New(Config{Directory: filepath.Join(root, "store"), KeyFile: filepath.Join(root, "keys", "secret.asc"), RecoveryDirectory: filepath.Join(root, "recovery")})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(v.Lock)
	if _, e = v.Create("Fixture", "fixture@example.invalid", testPassphrase); e != nil {
		t.Fatal(e)
	}
	return v
}

func mustSave(t *testing.T, v *Vault, name, raw, rev string) Document {
	t.Helper()
	d, e := v.Save(name, raw, rev)
	if e != nil {
		t.Fatal(e)
	}
	return d
}

func TestEncryptedStoreLifecycle(t *testing.T) {
	v := newFixture(t)
	raw := "a-unique-plaintext-secret\nusername: alice@example.invalid\nurl: https://example.invalid\ncustom: preserve me\n\nA private note\n"
	doc := mustSave(t, v, "Work/example", raw, "")
	if doc.Password != "a-unique-plaintext-secret" || doc.Username != "alice@example.invalid" || doc.Notes != "A private note" {
		t.Fatalf("parsed fields missing: %#v", doc)
	}
	ciphertext, e := os.ReadFile(filepath.Join(v.config.Directory, "Work/example.gpg"))
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(ciphertext, []byte("a-unique-plaintext-secret")) {
		t.Fatal("plaintext leaked into encrypted file")
	}
	info, _ := os.Stat(filepath.Join(v.config.Directory, "Work/example.gpg"))
	if info.Mode().Perm() != 0600 {
		t.Fatalf("insecure file mode: %v", info.Mode())
	}
	read, e := v.Read("Work/example")
	if e != nil || read.Raw != raw || read.Revision != doc.Revision {
		t.Fatalf("roundtrip: %v", e)
	}
	if _, e = v.Save("Work/example", "overwrite", ""); !errors.Is(e, ErrExists) {
		t.Fatalf("create clobbered existing entry: %v", e)
	}
	updated := mustSave(t, v, "Work/example", "new-password\nusername: Alice", doc.Revision)
	if _, e = v.Save("Work/example", "stale", doc.Revision); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale write accepted: %v", e)
	}
	if e = v.Move("Work/example", "Personal/example", updated.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = v.Read("Work/example"); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("source not removed: %v", e)
	}
	entries, e := v.List()
	if e != nil || len(entries) != 1 || entries[0].Path != "Personal/example" {
		t.Fatalf("list: %v %#v", e, entries)
	}
	v.Lock()
	if _, e = v.Read("Personal/example"); !errors.Is(e, ErrLocked) {
		t.Fatal("locked read succeeded")
	}
	if _, e = v.List(); !errors.Is(e, ErrLocked) {
		t.Fatal("locked list leaked names")
	}
	if e = v.Unlock("wrong-passphrase"); !errors.Is(e, ErrPassphrase) {
		t.Fatalf("bad password accepted: %v", e)
	}
	if v.Status().Unlocked {
		t.Fatal("wrong password unlocked vault")
	}
	if e = v.Unlock(testPassphrase); e != nil {
		t.Fatal(e)
	}
	read, e = v.Read("Personal/example")
	if e != nil || read.Password != "new-password" {
		t.Fatal("unlock roundtrip failed", e)
	}
}

func TestUnsafePathsAndSymlinksCannotEscape(t *testing.T) {
	v := newFixture(t)
	for _, name := range []string{"", "../secret", "/absolute", "a/../b", "a//b", ".git/config", "a\\b", "-option", "a\nname", "CON", "foo.gpg", "a/.gpg-id"} {
		if _, e := v.Save(name, "secret", ""); !errors.Is(e, ErrPath) {
			t.Errorf("accepted unsafe path %q: %v", name, e)
		}
	}
	outside := t.TempDir()
	if e := os.Symlink(outside, filepath.Join(v.config.Directory, "escape")); e != nil {
		t.Fatal(e)
	}
	if _, e := v.Save("escape/password", "must never leave root", ""); e == nil {
		t.Fatal("followed directory symlink")
	}
	if _, e := os.Stat(filepath.Join(outside, "password.gpg")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("wrote outside root")
	}
	if e := os.WriteFile(filepath.Join(outside, "target"), []byte("untouched"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(filepath.Join(outside, "target"), filepath.Join(v.config.Directory, "link.gpg")); e != nil {
		t.Fatal(e)
	}
	if _, e := v.Read("link"); e == nil {
		t.Fatal("followed file symlink")
	}
	if _, e := v.Save("link", "overwrite", ""); e == nil {
		t.Fatal("wrote through file symlink")
	}
	data, _ := os.ReadFile(filepath.Join(outside, "target"))
	if string(data) != "untouched" {
		t.Fatal("target changed")
	}
	if e := os.Mkdir(filepath.Join(v.config.Directory, "policy"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(filepath.Join(v.config.Directory, ".gpg-id"), filepath.Join(v.config.Directory, "policy/.gpg-id")); e != nil {
		t.Fatal(e)
	}
	if _, e := v.Save("policy/password", "secret", ""); e == nil {
		t.Fatal("followed recipient-policy symlink")
	}
}

func TestCiphertextTamperingIsRejected(t *testing.T) {
	v := newFixture(t)
	mustSave(t, v, "example", "secret payload", "")
	name := filepath.Join(v.config.Directory, "example.gpg")
	data, _ := os.ReadFile(name)
	data[len(data)-1] ^= 0xff
	if e := os.WriteFile(name, data, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := v.Read("example"); e == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
}

func TestCrossInstanceConflictAndLock(t *testing.T) {
	v := newFixture(t)
	other, e := New(v.config)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Lock()
	if e = other.Unlock(testPassphrase); e != nil {
		t.Fatal(e)
	}
	doc := mustSave(t, v, "example", "one", "")
	mustSave(t, other, "example", "two", doc.Revision)
	if _, e = v.Save("example", "stale", doc.Revision); !errors.Is(e, ErrConflict) {
		t.Fatalf("cross-instance write was not rejected: %v", e)
	}
	unlock, e := lockStore(v.config.Directory)
	if e != nil {
		t.Fatal(e)
	}
	defer unlock()
	if _, e = other.Save("blocked", "value", ""); !errors.Is(e, ErrBusy) {
		t.Fatalf("cross-process lock was ignored: %v", e)
	}
}

func TestLockInvalidatesInFlightCommit(t *testing.T) {
	v := newFixture(t)
	_, token, e := v.token()
	if e != nil {
		t.Fatal(e)
	}
	v.Lock()
	called := false
	if e = v.commit(token, func() error { called = true; return nil }); !errors.Is(e, ErrCancelled) || called {
		t.Fatal("locked generation committed")
	}
	if e = v.Unlock(testPassphrase); e != nil {
		t.Fatal(e)
	}
	if e = v.validate(token); !errors.Is(e, ErrCancelled) {
		t.Fatal("new unlock revalidated old generation")
	}
}

func TestIdleAndMaximumExpiry(t *testing.T) {
	for _, tc := range []struct {
		name          string
		idle, maximum time.Duration
	}{{"idle", 10 * time.Second, time.Hour}, {"maximum", time.Hour, 10 * time.Second}} {
		t.Run(tc.name, func(t *testing.T) {
			v := newFixture(t)
			v.SetTimeouts(tc.idle, tc.maximum)
			now := time.Now()
			v.session.Lock()
			v.opened = now
			v.touched = now
			v.clock = func() time.Time { return now }
			v.session.Unlock()
			now = now.Add(11 * time.Second)
			if v.Status().Unlocked {
				t.Fatal("expired session remained open")
			}
			if _, e := v.Read("anything"); !errors.Is(e, ErrLocked) {
				t.Fatal("expired session read succeeded")
			}
		})
	}
}

func TestEncryptedDraftAndTrashRecovery(t *testing.T) {
	v := newFixture(t)
	doc := mustSave(t, v, "email/account", "original secret", "")
	draft, e := v.SaveDraftRecord("", doc.Path, "email/renamed", "unsaved secret", doc.Revision)
	if e != nil {
		t.Fatal(e)
	}
	updated, e := v.SaveDraftRecord(draft.ID, doc.Path, "email/renamed", "updated draft secret", doc.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if updated.ID != draft.ID {
		t.Fatal("draft id changed")
	}
	drafts, e := v.Recovery("draft")
	if e != nil || len(drafts) != 1 || drafts[0].Raw != "updated draft secret" || drafts[0].Revision != doc.Revision {
		t.Fatal("draft did not preserve editor state", e)
	}
	trash, e := v.Trash(doc.Path, doc.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = v.Read(doc.Path); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("trash left original", e)
	}
	files, _ := os.ReadDir(v.config.RecoveryDirectory)
	for _, file := range files {
		data, _ := os.ReadFile(filepath.Join(v.config.RecoveryDirectory, file.Name()))
		if bytes.Contains(data, []byte("secret")) || bytes.Contains(data, []byte("email/account")) {
			t.Fatal("plaintext recovery metadata leaked")
		}
	}
	restored, e := v.Restore(trash.ID, "email/restored")
	if e != nil || restored.Raw != "original secret" {
		t.Fatal("restore failed", e)
	}
	if _, e = v.ReadRecovery(trash.ID); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("restored trash record was not removed")
	}
	if e = v.RemoveRecovery(draft.ID); e != nil {
		t.Fatal(e)
	}
	drafts, e = v.Recovery("")
	if e != nil || len(drafts) != 0 {
		t.Fatal("recovery cleanup failed", e)
	}
}

func TestMultipleRecipientsAndNestedPolicy(t *testing.T) {
	v := newFixture(t)
	first, _ := v.Identities()
	second, e := openpgp.NewEntity("Teammate", "", "second@example.invalid", cryptoConfig())
	if e != nil {
		t.Fatal(e)
	}
	var public bytes.Buffer
	if e = second.Serialize(&public); e != nil {
		t.Fatal(e)
	}
	if _, e = v.ImportKeys(public.Bytes(), testPassphrase); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(filepath.Join(v.config.Directory, "shared"), 0700); e != nil {
		t.Fatal(e)
	}
	policy := first[0].Fingerprint + "\n" + fingerprint(second) + "\n"
	if e = os.WriteFile(filepath.Join(v.config.Directory, "shared/.gpg-id"), []byte(policy), 0600); e != nil {
		t.Fatal(e)
	}
	doc := mustSave(t, v, "shared/example", "shared secret", "")
	data, _ := os.ReadFile(filepath.Join(v.config.Directory, "shared/example.gpg"))
	raw, e := decrypt(data, openpgp.EntityList{second})
	if e != nil || string(raw) != "shared secret" {
		t.Fatal("second recipient cannot decrypt", e)
	}
	if e = v.Move("shared/example", "personal/example", doc.Revision); e != nil {
		t.Fatal(e)
	}
	data, _ = os.ReadFile(filepath.Join(v.config.Directory, "personal/example.gpg"))
	if _, e = decrypt(data, openpgp.EntityList{second}); e == nil {
		t.Fatal("move did not re-encrypt under destination policy")
	}
}

func TestBinaryLegacyKeyImport(t *testing.T) {
	v := newFixture(t)
	export, e := v.ExportSecret()
	if e != nil {
		t.Fatal(e)
	}
	keys, e := parseKeys(export)
	if e != nil {
		t.Fatal(e)
	}
	var legacy bytes.Buffer
	for _, k := range keys {
		if e = k.Serialize(&legacy); e != nil {
			t.Fatal(e)
		}
		if e = k.SerializePrivateWithoutSigning(&legacy, cryptoConfig()); e != nil {
			t.Fatal(e)
		}
	}
	dir := t.TempDir()
	target, e := New(Config{Directory: filepath.Join(dir, "store"), KeyFile: filepath.Join(dir, "keys/secret.pgp")})
	if e != nil {
		t.Fatal(e)
	}
	defer target.Lock()
	ids, e := target.ImportKeys(legacy.Bytes(), testPassphrase)
	if e != nil || len(ids) != 1 {
		t.Fatal("legacy binary public+private import", e)
	}
	mustSave(t, target, "imported", "value", "")
	target.Lock()
	if e = target.Unlock(testPassphrase); e != nil {
		t.Fatal(e)
	}
}

func TestMixedPublicAndPrivateSubkeysRemainImportable(t *testing.T) {
	entity, e := openpgp.NewEntity("Mixed keyring", "", "mixed@example.invalid", cryptoConfig())
	if e != nil {
		t.Fatal(e)
	}
	if e = entity.AddSigningSubkey(cryptoConfig()); e != nil {
		t.Fatal(e)
	}
	entity.Subkeys[len(entity.Subkeys)-1].PrivateKey = nil
	data, e := protectedKeyData(openpgp.EntityList{entity}, testPassphrase)
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	v, e := New(Config{Directory: filepath.Join(dir, "store"), KeyFile: filepath.Join(dir, "keys/secret.asc")})
	if e != nil {
		t.Fatal(e)
	}
	defer v.Lock()
	if _, e = v.ImportKeys(data, testPassphrase); e != nil {
		t.Fatal(e)
	}
	doc := mustSave(t, v, "entry", "still decryptable", "")
	read, e := v.Read("entry")
	if e != nil || read.Raw != doc.Raw {
		t.Fatal("mixed private/public subkey roundtrip", e)
	}
	export, e := v.ExportSecret()
	if e != nil {
		t.Fatal(e)
	}
	keys, e := parseKeys(export)
	if e != nil || len(keys[0].Subkeys) != 2 {
		t.Fatal("public signing subkey lost during import", e)
	}
}

func TestTOTPStandardVectors(t *testing.T) {
	for _, tc := range []struct{ algorithm, key, want string }{{"SHA1", "12345678901234567890", "94287082"}, {"SHA256", "12345678901234567890123456789012", "46119246"}, {"SHA512", "1234567890123456789012345678901234567890123456789012345678901234", "90693936"}} {
		secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(tc.key))
		code, e := TOTP("otpauth://totp/test?secret="+secret+"&algorithm="+tc.algorithm+"&digits=8", time.Unix(59, 0))
		if e != nil || code.Code != tc.want || code.Remaining != 1 {
			t.Errorf("%s: %#v %v", tc.algorithm, code, e)
		}
	}
	for _, secret := range []string{"", "bad!", "otpauth://hotp/test?secret=ABC", "otpauth://totp/test?secret=ABC&period=0"} {
		if _, e := TOTP(secret, time.Now()); e == nil {
			t.Errorf("accepted invalid TOTP: %q", secret)
		}
	}
}

func TestPasswordGeneratorCharacterGuarantees(t *testing.T) {
	for i := 0; i < 25; i++ {
		p, e := GeneratePassword(DefaultPasswordOptions())
		if e != nil || len(p) != 24 {
			t.Fatal("generate", e)
		}
		for _, group := range []string{"abcdefghijklmnpqrstuvwxyz", "ABCDEFGHJKLMNPQRSTUVWXYZ", "23456789", "!@#$%^&*()-_=+[]{};:,.?/"} {
			if !strings.ContainsAny(p, group) {
				t.Fatal("missing selected group")
			}
		}
		if strings.ContainsAny(p, "Il1O0o") {
			t.Fatal("ambiguous characters")
		}
	}
	if _, e := GeneratePassword(PasswordOptions{Length: 24}); e == nil {
		t.Fatal("accepted empty alphabet")
	}
}

func TestGitSyncOnlyCommitsEncryptedStore(t *testing.T) {
	if _, e := exec.LookPath("git"); e != nil {
		t.Skip("git unavailable")
	}
	v := newFixture(t)
	ctx := context.Background()
	if e := v.GitInitialize(ctx); e != nil {
		t.Fatal(e)
	}
	mustSave(t, v, "example", "secret", "")
	if e := os.WriteFile(filepath.Join(v.config.Directory, "private.txt"), []byte("never commit plaintext"), 0600); e != nil {
		t.Fatal(e)
	}
	result, e := v.GitSync(ctx)
	if e != nil || result.State != "local" {
		t.Fatal("sync", e)
	}
	tracked, e := gitCommand(ctx, v.config.Directory, "ls-files")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(tracked, "private.txt") || strings.Contains(tracked, ".passpine.lock") || !strings.Contains(tracked, "example.gpg") {
		t.Fatalf("unexpected committed files: %s", tracked)
	}
	if e = os.Mkdir(filepath.Join(v.config.Directory, ".git/rebase-merge"), 0700); e != nil {
		t.Fatal(e)
	}
	if _, e = v.GitSync(ctx); e == nil {
		t.Fatal("sync ignored paused rebase")
	}
}

func TestGitCloneValidatesStoreAndPreservesDestination(t *testing.T) {
	if _, e := exec.LookPath("git"); e != nil {
		t.Skip("git unavailable")
	}
	source := newFixture(t)
	ctx := context.Background()
	if e := source.GitInitialize(ctx); e != nil {
		t.Fatal(e)
	}
	mustSave(t, source, "example", "secret", "")
	if _, e := source.GitSync(ctx); e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(t.TempDir(), "clone")
	if e := Clone(ctx, source.config.Directory, target); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(target, "example.gpg")); e != nil {
		t.Fatal(e)
	}
	if e := Clone(ctx, source.config.Directory, target); !errors.Is(e, ErrExists) {
		t.Fatal("clone overwrote existing destination", e)
	}
	for _, remote := range []string{"https://user:secret@example.invalid/repo", "https://example.invalid/repo?token=secret", "ext::command", "--upload-pack=evil", "ftp://example.invalid/repo"} {
		if ValidateRemote(remote) == nil {
			t.Errorf("unsafe remote accepted: %s", remote)
		}
	}
}

func TestVerifyBackupChecksEntriesAndNestedPolicies(t *testing.T) {
	v := newFixture(t)
	mustSave(t, v, "nested/entry", "secret", "")
	backup, e := v.ExportSecret()
	if e != nil {
		t.Fatal(e)
	}
	v.Lock()
	if e = v.VerifyBackup(backup, testPassphrase); e != nil {
		t.Fatal(e)
	}
	if v.Status().Unlocked {
		t.Fatal("backup verification opened the session")
	}
	if e = v.VerifyBackup(backup, "wrong"); !errors.Is(e, ErrPassphrase) {
		t.Fatal("incorrect backup password accepted", e)
	}
	wrong := newFixture(t)
	wrongBackup, _ := wrong.ExportSecret()
	if e = v.VerifyBackup(wrongBackup, testPassphrase); e == nil {
		t.Fatal("unrelated backup accepted")
	}
	if e = os.WriteFile(filepath.Join(v.config.Directory, "nested/.gpg-id"), []byte("MISSING-RECIPIENT\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = v.VerifyBackup(backup, testPassphrase); e == nil {
		t.Fatal("nested recipient policy was ignored")
	}
}

func TestGitNetworkDoesNotBlockReadsAndOnlyCloseCancelsProcess(t *testing.T) {
	git, e := exec.LookPath("git")
	if e != nil {
		t.Skip("git unavailable")
	}
	v := newFixture(t)
	defer v.Close()
	ctx := context.Background()
	if e = v.GitInitialize(ctx); e != nil {
		t.Fatal(e)
	}
	mustSave(t, v, "entry", "secret", "")
	if _, e = v.GitSync(ctx); e != nil {
		t.Fatal(e)
	}
	if e = v.GitSetRemote(ctx, filepath.Join(t.TempDir(), "remote.git")); e != nil {
		t.Fatal(e)
	}
	wrapperDirectory := t.TempDir()
	marker := filepath.Join(wrapperDirectory, "network-started")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	script := "#!/bin/sh\nfor arg in \"$@\"; do\n if [ \"$arg\" = push ]; then\n  touch " + quote(marker) + "\n  sleep 30\n  exit 0\n fi\ndone\nexec " + quote(git) + " \"$@\"\n"
	if e = os.WriteFile(filepath.Join(wrapperDirectory, "git"), []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", wrapperDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	done := make(chan error, 1)
	go func() { _, err := v.GitSync(ctx); done <- err }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, e = os.Stat(marker); e == nil {
			break
		}
		if time.Now().After(deadline) {
			v.Lock()
			t.Fatal("sync did not reach network phase")
		}
		time.Sleep(10 * time.Millisecond)
	}
	read := make(chan error, 1)
	go func() { _, err := v.Read("entry"); read <- err }()
	select {
	case e = <-read:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		v.Lock()
		t.Fatal("network sync blocked decryption")
	}
	if _, e = v.Save("new", "value", ""); !errors.Is(e, ErrBusy) {
		t.Fatal("write did not reject active Git lock", e)
	}
	v.Lock()
	select {
	case e = <-done:
		t.Fatal("locking the secret session interrupted Git sync", e)
	case <-time.After(100 * time.Millisecond):
	}
	v.Close()
	select {
	case e = <-done:
		if e == nil {
			t.Fatal("close did not cancel network sync")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Git child process remained blocked after close")
	}
}

func TestGnuPGInteroperability(t *testing.T) {
	gpg, e := exec.LookPath("gpg")
	if e != nil {
		t.Skip("GnuPG unavailable; pure Go interoperability tests still run")
	}
	v := newFixture(t)
	// macOS Unix-domain socket paths have a short limit; testing.T's full name
	// beneath /var/folders may leave too little room for GnuPG agent sockets.
	home, e := os.MkdirTemp("/tmp", "passpine-gpg-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	if e = os.Chmod(home, 0700); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if tool, e := exec.LookPath("gpgconf"); e == nil {
			_ = exec.Command(tool, "--homedir", home, "--kill", "gpg-agent").Run()
		}
	})
	run := func(input []byte, args ...string) []byte {
		t.Helper()
		base := []string{"--homedir", home, "--batch", "--yes", "--no-tty", "--pinentry-mode", "loopback"}
		cmd := exec.Command(gpg, append(base, args...)...)
		cmd.Stdin = bytes.NewReader(input)
		out, e := cmd.Output()
		if e != nil {
			if exit, ok := e.(*exec.ExitError); ok {
				t.Fatalf("isolated GnuPG operation %s failed: %v: %s", args[0], e, exit.Stderr)
			}
			t.Fatalf("isolated GnuPG operation %s failed: %v", args[0], e)
		}
		return out
	}
	backup, e := v.ExportSecret()
	if e != nil {
		t.Fatal(e)
	}
	run(backup, "--import")
	doc := mustSave(t, v, "gpg-roundtrip", "interoperable secret\nusername: alice", "")
	plain := run([]byte(testPassphrase+"\n"), "--passphrase-fd", "0", "--decrypt", filepath.Join(v.config.Directory, "gpg-roundtrip.gpg"))
	if string(plain) != doc.Raw {
		t.Fatal("GnuPG could not read Go-generated OpenPGP ciphertext")
	}
	ids, _ := v.Identities()
	encrypted := run([]byte("encrypted by GnuPG"), "--trust-model", "always", "--recipient", ids[0].Fingerprint, "--encrypt")
	if e = os.WriteFile(filepath.Join(v.config.Directory, "from-gpg.gpg"), encrypted, 0600); e != nil {
		t.Fatal(e)
	}
	read, e := v.Read("from-gpg")
	if e != nil || read.Password != "encrypted by GnuPG" {
		t.Fatal("Go could not read GnuPG ciphertext", e)
	}
	exported := run([]byte(testPassphrase+"\n"), "--passphrase-fd", "0", "--armor", "--export-secret-keys", ids[0].Fingerprint)
	if e = v.VerifyBackup(exported, testPassphrase); e != nil {
		t.Fatal("Go could not import/verify GnuPG's protected secret export", e)
	}
	subkeysOnly := run([]byte(testPassphrase+"\n"), "--passphrase-fd", "0", "--armor", "--export-secret-subkeys", ids[0].Fingerprint)
	if e = v.VerifyBackup(subkeysOnly, testPassphrase); e != nil {
		t.Fatal("GnuPG export-secret-subkeys is not usable", e)
	}
	dir := t.TempDir()
	imported, e := New(Config{Directory: filepath.Join(dir, "store"), KeyFile: filepath.Join(dir, "keys/secret.asc")})
	if e != nil {
		t.Fatal(e)
	}
	defer imported.Lock()
	if _, e = imported.ImportKeys(subkeysOnly, testPassphrase); e != nil {
		t.Fatal("GnuPG dummy primary key import failed", e)
	}
	mustSave(t, imported, "entry", "subkeys-only secret", "")
	imported.Lock()
	if e = imported.Unlock(testPassphrase); e != nil {
		t.Fatal("GnuPG subkey-only import did not survive reopen", e)
	}
}
