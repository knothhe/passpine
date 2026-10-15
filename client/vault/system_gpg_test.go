package vault

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
)

func isolatedSystemGPG(t *testing.T) func([]byte, ...string) []byte {
	t.Helper()
	binary, err := exec.LookPath("gpg")
	if err != nil {
		t.Skip("GnuPG is not installed")
	}
	home, err := os.MkdirTemp("/tmp", "passpine-system-gpg-")
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
	return func(input []byte, args ...string) []byte {
		t.Helper()
		cmd := exec.Command(binary, append([]string{"--homedir", home, "--batch", "--yes", "--no-tty", "--pinentry-mode", "loopback"}, args...)...)
		cmd.Stdin = bytes.NewReader(input)
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("isolated GPG fixture failed: %v", err)
		}
		return output
	}
}

func systemGPGTarget(t *testing.T, source *Vault) *Vault {
	t.Helper()
	target, err := New(Config{Directory: source.config.Directory, KeyFile: filepath.Join(t.TempDir(), "keys/secret.pgp")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(target.Lock)
	return target
}

func TestSystemGPGImportsMatchedRecipientsAndNestedKeys(t *testing.T) {
	run := isolatedSystemGPG(t)
	source := newFixture(t)
	ids, _ := source.Identities()
	first := ids[0].Fingerprint
	second, err := New(Config{Directory: filepath.Join(t.TempDir(), "store")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(second.Lock)
	identity, err := second.Create("Nested", "nested@example.invalid", testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	secondBackup, err := second.ExportSecret()
	if err != nil {
		t.Fatal(err)
	}
	defer wipe(secondBackup)
	if _, err = source.ImportKeys(secondBackup, testPassphrase); err != nil {
		t.Fatal(err)
	}
	publicOnly, err := openpgp.NewEntity("Sharing", "", "sharing@example.invalid", cryptoConfig())
	if err != nil {
		t.Fatal(err)
	}
	var public bytes.Buffer
	if err = publicOnly.Serialize(&public); err != nil {
		t.Fatal(err)
	}
	if _, err = source.ImportKeys(public.Bytes(), testPassphrase); err != nil {
		t.Fatal(err)
	}
	// A short ID, a public-only sharing recipient, and a distinct nested policy.
	if err = os.WriteFile(filepath.Join(source.config.Directory, ".gpg-id"), []byte("0x"+first[len(first)-8:]+"\n"+fingerprint(publicOnly)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(source.config.Directory, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source.config.Directory, "nested/.gpg-id"), []byte(identity.Fingerprint+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	mustSave(t, source, "root-entry", "root fixture secret", "")
	mustSave(t, source, "nested/entry", "nested fixture secret", "")
	backup, err := source.ExportSecret()
	if err != nil {
		t.Fatal(err)
	}
	defer wipe(backup)
	run(backup, "--import")
	unrelated := newFixture(t)
	unrelatedBackup, err := unrelated.ExportSecret()
	if err != nil {
		t.Fatal(err)
	}
	defer wipe(unrelatedBackup)
	run(unrelatedBackup, "--import")
	before, err := os.ReadFile(filepath.Join(source.config.Directory, ".gpg-id"))
	if err != nil {
		t.Fatal(err)
	}
	target := systemGPGTarget(t, source)
	matched, err := target.ImportSystemGPG(testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 3 {
		t.Fatalf("imported unrelated keys or lost public sharing recipients: %d", len(matched))
	}
	for _, entry := range []string{"root-entry", "nested/entry"} {
		if _, err = target.Read(entry); err != nil {
			t.Fatalf("cannot decrypt %s after automatic matching: %v", entry, err)
		}
	}
	after, err := os.ReadFile(filepath.Join(source.config.Directory, ".gpg-id"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("automatic matching changed the existing recipient policy")
	}
	stored, err := os.ReadFile(target.config.KeyFile)
	if err != nil {
		t.Fatal(err)
	}
	defer wipe(stored)
	keys, err := parseKeys(stored)
	if err != nil {
		t.Fatal(err)
	}
	if err = decryptKeys(keys, "wrong-passphrase", true); !errors.Is(err, ErrPassphrase) {
		t.Fatal("the stored private key copy is not passphrase protected", err)
	}
	target.Lock()
	t.Setenv("GNUPGHOME", t.TempDir())
	if err = target.Unlock(testPassphrase); err != nil {
		t.Fatal("local unlock still depends on the system keyring", err)
	}
}

func TestSystemGPGFailureDoesNotInstallKeys(t *testing.T) {
	for _, scenario := range []string{"missing", "public-only", "wrong-passphrase", "ambiguous-email"} {
		t.Run(scenario, func(t *testing.T) {
			run := isolatedSystemGPG(t)
			source := newFixture(t)
			backup, err := source.ExportSecret()
			if err != nil {
				t.Fatal(err)
			}
			defer wipe(backup)
			ids, _ := source.Identities()
			switch scenario {
			case "public-only":
				keys, err := parseKeys(backup)
				if err != nil {
					t.Fatal(err)
				}
				var public bytes.Buffer
				if err = keys[0].Serialize(&public); err != nil {
					t.Fatal(err)
				}
				run(public.Bytes(), "--import")
			case "wrong-passphrase", "ambiguous-email":
				run(backup, "--import")
			}
			if scenario == "ambiguous-email" {
				other := newFixture(t)
				otherBackup, err := other.ExportSecret()
				if err != nil {
					t.Fatal(err)
				}
				defer wipe(otherBackup)
				run(otherBackup, "--import")
				if err = os.WriteFile(filepath.Join(source.config.Directory, ".gpg-id"), []byte("fixture@example.invalid\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			target := systemGPGTarget(t, source)
			passphrase := testPassphrase
			if scenario == "wrong-passphrase" {
				passphrase = "incorrect-system-passphrase"
			}
			_, err = target.ImportSystemGPG(passphrase)
			if err == nil {
				t.Fatalf("accepted %s system GPG keys (%s)", scenario, ids[0].Fingerprint)
			}
			if scenario == "ambiguous-email" && !strings.Contains(err.Error(), "ambiguous") {
				t.Fatal("ambiguous selector was not rejected before importing secrets", err)
			}
			if target.Status().Configured || target.Status().Unlocked {
				t.Fatal("failed system import installed keys or opened a session")
			}
		})
	}
}

func TestSystemGPGImportTokenCannotReviveLockedSession(t *testing.T) {
	source := newFixture(t)
	backup, err := source.ExportSecret()
	if err != nil {
		t.Fatal(err)
	}
	defer wipe(backup)
	target := systemGPGTarget(t, source)
	token := target.generationToken()
	target.Lock()
	if _, err = target.importKeys(backup, testPassphrase, token); !errors.Is(err, ErrCancelled) {
		t.Fatal("an import started before locking reopened the session", err)
	}
	if target.Status().Configured || target.Status().Unlocked {
		t.Fatal("cancelled import wrote a key file")
	}
}

func TestSystemGPGRejectsOversizedExport(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "fixture-gpg")
	if err := os.WriteFile(filename, []byte("#!/bin/sh\nhead -c 16777217 /dev/zero\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if data, err := runSystemGPG(ctx, filename, nil, "--export"); err == nil || len(data) != 0 {
		t.Fatal("accepted an oversized GPG key export")
	}
}
