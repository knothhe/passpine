package commandlink

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testManager(t *testing.T) Manager {
	t.Helper()
	root := t.TempDir()
	executable := filepath.Join(root, "Applications with spaces", "passpine")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("test application"), 0o700); err != nil {
		t.Fatal(err)
	}
	executable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	return Manager{Executable: executable, Directory: filepath.Join(root, "commands with spaces"), Name: "passpine-dev"}
}

func TestCommandLinksPlatformContract(t *testing.T) {
	m := testManager(t)
	if runtime.GOOS == "windows" {
		for _, operation := range []func() error{
			func() error { _, err := m.Status(); return err },
			func() error { return m.Install(true) }, m.Uninstall,
		} {
			if err := operation(); !errors.Is(err, ErrUnsupported) {
				t.Fatalf("unsupported operation returned %v", err)
			}
			if _, err := os.Stat(m.Directory); !os.IsNotExist(err) {
				t.Fatalf("unsupported operation created a directory: %v", err)
			}
		}
		if err := os.MkdirAll(m.Directory, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{m.Name, Alias} {
			if err := os.WriteFile(filepath.Join(m.Directory, name), []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		_ = m.Install(true)
		_ = m.Uninstall()
		for _, name := range []string{m.Name, Alias} {
			if data, err := os.ReadFile(filepath.Join(m.Directory, name)); err != nil || string(data) != "keep" {
				t.Fatalf("existing file changed: %q %v", data, err)
			}
		}
		return
	}
	status, err := m.Status()
	if err != nil || status.Main.Exists || status.Alias.Exists {
		t.Fatalf("initial status: %+v %v", status, err)
	}
	if err := m.Install(false); err != nil {
		t.Fatal(err)
	}
	status, err = m.Status()
	if err != nil || !status.Main.Removable || status.Alias.Exists {
		t.Fatalf("unselected alias installed: %+v %v", status, err)
	}
	if err := m.Install(true); err != nil {
		t.Fatal(err)
	}
	if err := m.Install(true); err != nil {
		t.Fatal("repeated installation", err)
	}
	status, err = m.Status()
	if err != nil || !status.Main.Owned || !status.Alias.Removable {
		t.Fatalf("installed status: %+v %v", status, err)
	}
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(); err != nil {
		t.Fatal("repeated removal", err)
	}
	status, err = m.Status()
	if err != nil || status.Main.Exists || status.Alias.Exists {
		t.Fatalf("removed status: %+v %v", status, err)
	}
	if data, err := os.ReadFile(m.Executable); err != nil || string(data) != "test application" {
		t.Fatal("application binary was changed")
	}
}

func TestConflictsPreserveFilesAndOtherChannels(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlink contract; Windows rejection is tested separately")
	}
	for _, kind := range []string{"file", "directory", "foreign-link", "broken-link"} {
		t.Run(kind, func(t *testing.T) {
			m := testManager(t)
			if err := os.MkdirAll(m.Directory, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(m.Directory, Alias)
			other := filepath.Join(t.TempDir(), "other executable")
			if err := os.WriteFile(other, []byte("keep"), 0o700); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "file":
				err = os.WriteFile(path, []byte("keep"), 0o600)
			case "directory":
				err = os.Mkdir(path, 0o755)
			case "broken-link":
				err = os.Symlink(other+"-missing", path)
			default:
				err = os.Symlink(other, path)
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			var conflict *ConflictError
			if err := m.Install(true); !errors.As(err, &conflict) || conflict.Path != path {
				t.Fatalf("conflict = %v", err)
			}
			if _, err := os.Lstat(filepath.Join(m.Directory, m.Name)); !os.IsNotExist(err) {
				t.Fatal("alias conflict left a partial installation")
			}
			if err := m.Uninstall(); err != nil {
				t.Fatal(err)
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("foreign destination was changed")
			}
		})
	}
}

func TestDevelopmentAndReleaseCommandsCoexist(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlink contract; Windows rejection is tested separately")
	}
	dev, release := testManager(t), testManager(t)
	release.Directory, release.Name = dev.Directory, "passpine"
	if err := release.Install(true); err != nil {
		t.Fatal(err)
	}
	if err := dev.Install(false); err != nil {
		t.Fatal(err)
	}
	if err := dev.Install(true); err == nil {
		t.Fatal("Dev replaced the release alias")
	}
	if err := dev.Uninstall(); err != nil {
		t.Fatal(err)
	}
	status, err := release.Status()
	if err != nil || !status.Main.Removable || !status.Alias.Removable {
		t.Fatalf("Dev uninstall changed the release commands: %+v %v", status, err)
	}
	if err := release.Install(false); err != nil {
		t.Fatal(err)
	}
	status, err = release.Status()
	if err != nil || !status.Alias.Removable {
		t.Fatal("installing without the alias deleted an existing alias")
	}
}

func TestMainConflictDoesNotCreateAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows rejection is tested separately")
	}
	m := testManager(t)
	if err := os.MkdirAll(m.Directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Directory, m.Name), []byte("keep"), 0o700); err != nil {
		t.Fatal(err)
	}
	var conflict *ConflictError
	if err := m.Install(true); !errors.As(err, &conflict) {
		t.Fatalf("main conflict = %v", err)
	}
	status, err := m.Status()
	if err != nil || !status.Main.Exists || status.Main.Owned || status.Alias.Exists {
		t.Fatalf("main conflict changed commands: %+v %v", status, err)
	}
}

func TestUninstallPreservesInstallerBinaryAndForeignAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix installed binary; Windows rejection is tested separately")
	}
	m := testManager(t)
	m.Directory, m.Name = filepath.Dir(m.Executable), filepath.Base(m.Executable)
	if err := os.WriteFile(filepath.Join(m.Directory, Alias), []byte("original pass"), 0o700); err != nil {
		t.Fatal(err)
	}
	status, err := m.Status()
	if err != nil || !status.Main.Owned || status.Main.Removable || status.Alias.Owned {
		t.Fatalf("binary status: %+v %v", status, err)
	}
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{m.Name: "test application", Alias: "original pass"} {
		if data, err := os.ReadFile(filepath.Join(m.Directory, name)); err != nil || string(data) != want {
			t.Fatalf("%s was changed", name)
		}
	}
}
