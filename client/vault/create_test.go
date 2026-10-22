package vault

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCreatePreservesExistingStoreWithoutRootPolicy(t *testing.T) {
	source := newFixture(t)
	mustSave(t, source, "entry", "original-password", "")
	ciphertext, err := os.ReadFile(filepath.Join(source.config.Directory, "entry.gpg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name string
		path string
		data []byte
	}{
		{"entry", "nested/entry.gpg", ciphertext},
		{"nested-policy", "nested/.gpg-id", []byte("existing-recipient\n")},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			v, err := New(Config{Directory: filepath.Join(root, "store"), KeyFile: filepath.Join(root, "keys", "secret.asc")})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(v.Close)
			original := filepath.Join(v.config.Directory, filepath.FromSlash(fixture.path))
			if err = os.MkdirAll(filepath.Dir(original), 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(original, fixture.data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = v.Create("New identity", "new@example.invalid", testPassphrase); err == nil {
				t.Fatal("installed an unrelated private key over existing store contents")
			}
			if v.Status().Configured || v.Status().Unlocked {
				t.Fatal("rejected creation configured or unlocked the vault")
			}
			for _, file := range []string{v.config.KeyFile, filepath.Join(v.config.Directory, ".gpg-id")} {
				if _, err = os.Stat(file); !os.IsNotExist(err) {
					t.Fatal("rejected creation wrote private key or recipient policy", err)
				}
			}
			unchanged, err := os.ReadFile(original)
			if err != nil || !bytes.Equal(unchanged, fixture.data) {
				t.Fatal("rejected creation changed existing store contents", err)
			}
		})
	}
}
