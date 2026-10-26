package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/knothhe/passpine/client/nativehost"
)

func TestSeedDevelopmentVaultIsIdempotentAndKeepsCurrentVault(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "app")
	s, err := New(Options{DataDir: dataDir, DeviceUnlock: &fixtureDevice{}})
	if err != nil {
		t.Fatal(err)
	}
	selected := must(t, s, Request{Action: "create", Directory: filepath.Join(root, "own"), Passphrase: "own-vault-passphrase"}).VaultID
	if _, _, err := SeedDevelopmentVault(dataDir); !errors.Is(err, nativehost.ErrAlreadyRunning) {
		t.Fatalf("seeding while the client runs = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	directory, created, err := SeedDevelopmentVault(dataDir)
	if err != nil || !created || directory != filepath.Join(dataDir, "test-vault") {
		t.Fatalf("seed = %q %v %v", directory, created, err)
	}
	if _, _, err := SeedDevelopmentVault(dataDir); err != nil {
		t.Fatal(err)
	}
	if _, created, err = SeedDevelopmentVault(dataDir); err != nil || created {
		t.Fatalf("second seed created=%v err=%v", created, err)
	}
	settings, current, vaults, err := CLIPreferences(dataDir)
	if err != nil || current != selected || len(vaults) != 2 {
		t.Fatalf("current=%q vaults=%v settings=%v err=%v", current, vaults, settings, err)
	}
	store, err := OpenCLIVault(dataDir, directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Vault.Close()
	if err := store.Vault.Unlock(DemoPassphrase); err != nil {
		t.Fatal(err)
	}
	if doc, err := store.Vault.Read("工作/GitHub"); err != nil || doc.Username != "alex@example.com" || doc.OTP == "" {
		t.Fatalf("fixture entry = %+v %v", doc, err)
	}
	if _, err := exec.LookPath("git"); err == nil {
		if _, err := os.Stat(filepath.Join(directory, ".git")); err != nil {
			t.Fatalf("test vault has no Git history: %v", err)
		}
	}
}
