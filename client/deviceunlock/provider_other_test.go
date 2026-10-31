//go:build !windows && !linux

package deviceunlock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHelperBesideResolvesCommandLink(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "Passpine App.app", "Contents", "MacOS")
	bin := filepath.Join(root, "local bin")
	for _, dir := range []string{bundle, bin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	executable := filepath.Join(bundle, "passpine")
	if err := os.WriteFile(executable, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(bin, "pass")
	if err := os.Symlink(executable, link); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(bundle)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(real, "passpine-device-unlock")
	if got := helperBeside(link); got != want {
		t.Fatalf("helper through link = %q, want %q", got, want)
	}
	if got := helperBeside(filepath.Join(real, "passpine")); got != want {
		t.Fatalf("helper from bundle = %q, want %q", got, want)
	}
	missing := filepath.Join(root, "missing", "passpine")
	if got := helperBeside(missing); got != filepath.Join(root, "missing", "passpine-device-unlock") {
		t.Fatalf("helper for unresolved path = %q", got)
	}
}
