package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
)

func TestOpenVaultWithKeyFilesWithoutSystemGPG(t *testing.T) {
	source := testService(t)
	saved := must(t, source, Request{Action: "save", Entry: "example", Contents: "manual-import-secret", Mode: "create"})
	private, err := source.active.ExportSecret()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(private)
	keys, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(private))
	if err != nil {
		t.Fatal(err)
	}
	var public bytes.Buffer
	if err = keys[0].Serialize(&public); err != nil {
		t.Fatal(err)
	}
	keyDir := t.TempDir()
	privateFile, publicFile := filepath.Join(keyDir, "private.asc"), filepath.Join(keyDir, "public.pgp")
	if err = os.WriteFile(privateFile, private, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(publicFile, public.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	invalidFile, emptyFile, symlinkFile := filepath.Join(keyDir, "invalid.asc"), filepath.Join(keyDir, "empty.asc"), filepath.Join(keyDir, "linked.asc")
	if err = os.WriteFile(invalidFile, []byte("not an OpenPGP key"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(emptyFile, nil, 0600); err != nil {
		t.Fatal(err)
	}
	symlinkErr := os.Symlink(privateFile, symlinkFile)
	if symlinkErr != nil && !(runtime.GOOS == "windows" && errors.Is(symlinkErr, syscall.Errno(1314))) {
		t.Fatal(symlinkErr)
	}
	// Any attempted GPG use fails and leaves a marker, including on machines
	// where findSystemGPG could otherwise find a Homebrew installation.
	marker := filepath.Join(keyDir, "gpg-called")
	t.Setenv("PASSPINE_TEST_GPG_MARKER", marker)
	if err = os.WriteFile(filepath.Join(keyDir, "gpg"), []byte("#!/bin/sh\nprintf called > \"$PASSPINE_TEST_GPG_MARKER\"\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", keyDir)
	t.Setenv("GNUPGHOME", t.TempDir())

	for _, tc := range []struct {
		name, private, public, passphrase, wantError string
	}{
		{"private-only", privateFile, "", "test-passphrase-123", ""},
		{"separate-public", privateFile, publicFile, "test-passphrase-123", ""},
		{"missing-private", "", publicFile, "test-passphrase-123", "请先选择私钥文件。"},
		{"missing-both", "", "", "test-passphrase-123", "请先选择私钥文件。"},
		{"unreadable-private", filepath.Join(keyDir, "missing"), "", "test-passphrase-123", "无法读取私钥文件，请检查文件路径和访问权限。"},
		{"unreadable-public", privateFile, filepath.Join(keyDir, "missing"), "test-passphrase-123", "无法读取公钥文件，请检查文件路径和访问权限。"},
		{"public-in-private", publicFile, "", "test-passphrase-123", "私钥文件中没有可用的私钥。"},
		{"private-in-public", privateFile, privateFile, "test-passphrase-123", "公钥文件包含私钥，请在私钥文件中导入。"},
		{"invalid-private", invalidFile, "", "test-passphrase-123", "私钥文件不是有效的 OpenPGP 密钥文件。"},
		{"invalid-public", privateFile, invalidFile, "test-passphrase-123", "公钥文件不是有效的 OpenPGP 密钥文件。"},
		{"empty-public", privateFile, emptyFile, "test-passphrase-123", "无法读取公钥文件，请检查文件路径和访问权限。"},
		{"symlink-private", symlinkFile, "", "test-passphrase-123", "无法读取私钥文件，请检查文件路径和访问权限。"},
		{"wrong-passphrase", privateFile, publicFile, "incorrect-passphrase", "私钥口令错误或私钥不可用"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "symlink-private" && symlinkErr != nil {
				t.Skipf("file symlinks require Windows Developer Mode or administrator privilege: %v", symlinkErr)
			}
			opened, err := New(Options{DataDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { opened.Close() })
			directory := source.active.Config().Directory
			r := opened.Execute(Request{Action: "open", Directory: directory, Mode: "key-file", KeyFile: tc.private, PublicKeyFile: tc.public, Passphrase: tc.passphrase})
			if tc.wantError != "" {
				if r.Status != "error" || r.Message != tc.wantError {
					t.Fatalf("unexpected import result: status=%s message=%s", r.Status, r.Message)
				}
				target, err := opened.newVault(directory)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = os.Stat(target.Config().KeyFile); !os.IsNotExist(err) {
					t.Fatal("failed import installed keys", err)
				}
				return
			}
			if r.Status != "ok" || r.Data.Locked || !r.Data.Configured || r.Data.Fingerprint == "" {
				t.Fatalf("manual import failed: status=%s message=%s", r.Status, r.Message)
			}
			must(t, opened, Request{Action: "lock"})
			must(t, opened, Request{Action: "unlock", Passphrase: tc.passphrase})
			if got := must(t, opened, Request{Action: "fetch", Entry: saved.Entry}); got.Contents != saved.Contents {
				t.Fatal("manually imported keys cannot decrypt existing entries")
			}
		})
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("manual key import invoked system GPG", err)
	}
}
