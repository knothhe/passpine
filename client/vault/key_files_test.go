package vault

import (
	"bytes"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

func TestImportKeyFilesPreservesPrivateKeysAndSharingRecipients(t *testing.T) {
	source := newFixture(t)
	private, err := source.ExportSecret()
	if err != nil {
		t.Fatal(err)
	}
	defer wipe(private)
	own, err := parseKeys(private)
	if err != nil {
		t.Fatal(err)
	}
	other, err := openpgp.NewEntity("Shared recipient", "", "shared@example.invalid", cryptoConfig())
	if err != nil {
		t.Fatal(err)
	}
	var public bytes.Buffer
	for _, key := range []*openpgp.Entity{own[0], other} {
		if err = key.Serialize(&public); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = source.ImportKeys(public.Bytes(), testPassphrase); err != nil {
		t.Fatal(err)
	}
	policy := fingerprint(own[0]) + "\n" + hex.EncodeToString(other.PrimaryKey.Fingerprint) + "\n"
	if err = os.MkdirAll(filepath.Join(source.config.Directory, "shared"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source.config.Directory, "shared/.gpg-id"), []byte(policy), 0600); err != nil {
		t.Fatal(err)
	}
	mustSave(t, source, "shared/example", "shared-password", "")
	block, err := armor.Decode(bytes.NewReader(private))
	if err != nil {
		t.Fatal(err)
	}
	binaryPrivate, err := io.ReadAll(block.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer wipe(binaryPrivate)
	var armoredPublic bytes.Buffer
	w, err := armor.Encode(&armoredPublic, openpgp.PublicKeyType, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(public.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name            string
		private, public []byte
	}{
		{"armored-private-binary-public", private, public.Bytes()},
		{"binary-private-armored-public", binaryPrivate, armoredPublic.Bytes()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			target, err := New(Config{Directory: source.config.Directory, KeyFile: filepath.Join(root, "secret.pgp"), RecoveryDirectory: filepath.Join(root, "recovery")})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(target.Lock)
			// The nested recipient policy must be complete before any keys are saved.
			if _, err = target.ImportKeyFiles(tc.private, nil, testPassphrase); err == nil {
				t.Fatal("accepted a keyring missing the nested sharing recipient")
			}
			if _, err = os.Stat(target.config.KeyFile); !os.IsNotExist(err) {
				t.Fatal("failed import installed a partial keyring", err)
			}
			ids, err := target.ImportKeyFiles(tc.private, tc.public, testPassphrase)
			if err != nil {
				t.Fatal(err)
			}
			secrets := 0
			for _, id := range ids {
				if id.HasSecret {
					secrets++
				}
			}
			if len(ids) != 2 || secrets != 1 {
				t.Fatal("duplicate public export replaced private material or imported sharing secrets")
			}
			target.Lock()
			if err = target.Unlock(testPassphrase); err != nil {
				t.Fatal(err)
			}
			got, err := target.Read("shared/example")
			if err != nil || got.Password != "shared-password" {
				t.Fatal("imported keys cannot read the shared store after relocking", err)
			}
			name := "shared/" + tc.name
			mustSave(t, target, name, "new-shared-password", "")
			// Confirm new writes retain encryption for the public-only recipient.
			ciphertext, err := os.ReadFile(filepath.Join(source.config.Directory, name+".gpg"))
			if err != nil {
				t.Fatal(err)
			}
			plain, err := decrypt(ciphertext, openpgp.EntityList{other})
			if err != nil || string(plain) != "new-shared-password" {
				t.Fatal("new entry omitted the sharing recipient", err)
			}
			wipe(plain)
		})
	}
}
