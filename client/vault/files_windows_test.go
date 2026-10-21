package vault

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsSafeFilesAndLock(t *testing.T) {
	root := t.TempDir()
	unlock, err := lockStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if second, err := lockStore(root); !errors.Is(err, ErrBusy) {
		if second != nil {
			second()
		}
		t.Fatalf("second writer acquired lock: %v", err)
	}
	if err := safeWrite(root, "nested/entry.gpg", []byte("first"), false); err != nil {
		t.Fatal(err)
	}
	if err := safeWrite(root, "nested/entry.gpg", []byte("duplicate"), false); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate write: %v", err)
	}
	if err := safeWrite(root, "nested/entry.gpg", []byte("second"), true); err != nil {
		t.Fatal(err)
	}
	if data, err := safeRead(root, "nested/entry.gpg"); err != nil || string(data) != "second" {
		t.Fatalf("read: %q %v", data, err)
	}
	for _, name := range []string{"../outside", `C:\outside`, "entry.gpg:stream"} {
		if err := safeWrite(root, name, []byte("secret"), false); !errors.Is(err, ErrPath) {
			t.Fatalf("unsafe path %q: %v", name, err)
		}
	}
	path, release, err := windowsParent(root, "nested/entry.gpg", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Dir(path), filepath.Join(root, "moved")); err == nil {
		release()
		t.Fatal("pinned parent was renamed")
	}
	release()
	// New key/ciphertext files must have a protected DACL, independent of inheritance.
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("file DACL is not protected: %v", err)
	}
	if err := safeRemove(root, "nested/entry.gpg"); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsSafeFilesRejectJunction(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	junction := filepath.Join(root, "escape")
	command := exec.Command("cmd.exe", "/c", "mklink", "/J", junction, outside)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create junction: %s %v", output, err)
	}
	if err := safeWrite(root, "escape/password.gpg", []byte("secret"), false); !errors.Is(err, ErrPath) {
		t.Fatalf("junction accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "password.gpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrote outside root: %v", err)
	}
}

func TestWindowsCheckoutPreservesConcurrentDestination(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "checkout"), filepath.Join(root, "destination")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := renameCheckout(source, destination); err == nil {
		t.Fatal("replaced a concurrent destination file")
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "keep" {
		t.Fatalf("destination changed: %q %v", data, err)
	}
}
