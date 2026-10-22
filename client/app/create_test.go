package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
)

func TestCreateVaultWithoutGPGSurvivesRestartAndBackupRestore(t *testing.T) {
	// A first-time user has neither GnuPG nor an initialized password store.
	// All credentials and key material in this test are disposable fixtures.
	root := t.TempDir()
	gpgHome := filepath.Join(root, "unused-gnupg")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GNUPGHOME", gpgHome)
	t.Setenv("PASSWORD_STORE_DIR", filepath.Join(root, "unused-password-store"))
	dataDir, storeDir := filepath.Join(root, "app"), filepath.Join(root, "store")
	newService := func(dataDir string) *Service {
		t.Helper()
		s, err := New(Options{DataDir: dataDir, DeviceUnlock: &fixtureDevice{}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s
	}

	created := newService(dataDir)
	if state := must(t, created, Request{Action: "status"}); state.Configured {
		t.Fatal("fresh installation already has a vault")
	}
	const passphrase = "first-time-user-passphrase"
	identity := must(t, created, Request{Action: "create", Directory: storeDir, Name: "New user", Email: "new@example.invalid", Passphrase: passphrase})
	if !identity.Configured || identity.Locked || identity.Fingerprint == "" {
		t.Fatal("creating the first private key did not open the vault")
	}
	policy, err := os.ReadFile(filepath.Join(storeDir, ".gpg-id"))
	if err != nil || strings.TrimSpace(string(policy)) != identity.Fingerprint {
		t.Fatal("new store has no matching recipient policy", err)
	}
	keyFile := created.active.Config().KeyFile
	relative, err := filepath.Rel(storeDir, keyFile)
	if err != nil || relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		t.Fatal("private key was stored inside the password store", err)
	}
	assertPrivateFile(t, keyFile)
	assertPrivateFile(t, filepath.Join(storeDir, ".gpg-id"))
	saved := must(t, created, Request{Action: "save", Entry: "personal/example", Contents: "fresh-install-secret\nusername: new-user\n", Mode: "create"})
	ciphertext, err := os.ReadFile(filepath.Join(storeDir, "personal", "example.gpg"))
	if err != nil || bytes.Contains(ciphertext, []byte(saved.Password)) {
		t.Fatal("new vault did not encrypt its first password", err)
	}
	backupFile := filepath.Join(root, "recovery-private-key.asc")
	must(t, created, Request{Action: "export", Destination: backupFile})
	assertPrivateFile(t, backupFile)
	backup, err := os.ReadFile(backupFile)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(backup)
	keys, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(backup))
	if err != nil || len(keys) != 1 || keys[0].PrivateKey == nil || !keys[0].PrivateKey.Encrypted {
		t.Fatal("export is not a protected OpenPGP private key", err)
	}
	for _, subkey := range keys[0].Subkeys {
		if subkey.PrivateKey == nil || !subkey.PrivateKey.Encrypted {
			t.Fatal("generated private subkey was exported without passphrase protection")
		}
	}
	must(t, created, Request{Action: "verify-backup", KeyFile: backupFile, Passphrase: passphrase})
	if err = created.Close(); err != nil {
		t.Fatal(err)
	}

	restarted := newService(dataDir)
	if state := must(t, restarted, Request{Action: "status"}); !state.Configured || !state.Locked || state.Directory != storeDir {
		t.Fatal("restarted client did not retain the locked vault")
	}
	if result := restarted.Execute(Request{Action: "unlock", Passphrase: "wrong-passphrase"}); result.Code != "invalid_passphrase" || !result.Data.Locked {
		t.Fatal("incorrect passphrase unlocked the generated private key")
	}
	must(t, restarted, Request{Action: "unlock", Passphrase: passphrase})
	if got := must(t, restarted, Request{Action: "fetch", Entry: saved.Entry}); got.Contents != saved.Contents {
		t.Fatal("generated key could not decrypt after restarting the client")
	}
	if err = restarted.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(dataDir); err != nil {
		t.Fatal(err)
	}

	// Recover the encrypted store using only the exported key after losing all
	// application data, without a system keyring or the original local key.
	restoredDir := filepath.Join(root, "restored-app")
	restored := newService(restoredDir)
	if result := restored.Execute(Request{Action: "open", Directory: storeDir, Mode: "key-file", KeyFile: backupFile, Passphrase: "wrong-passphrase"}); result.Code != "invalid_passphrase" || result.Data.Configured {
		t.Fatal("failed recovery installed a private key")
	}
	recovered := must(t, restored, Request{Action: "open", Directory: storeDir, Mode: "key-file", KeyFile: backupFile, Passphrase: passphrase})
	if recovered.Locked || recovered.Fingerprint != identity.Fingerprint {
		t.Fatal("restored key does not match the generated identity")
	}
	if got := must(t, restored, Request{Action: "fetch", Entry: saved.Entry}); got.Contents != saved.Contents {
		t.Fatal("recovery key could not decrypt the original store")
	}
	if _, err = os.Stat(gpgHome); !os.IsNotExist(err) {
		t.Fatal("creating or recovering a vault accessed the system GPG home", err)
	}
}
