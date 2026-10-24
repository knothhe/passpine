// Package commandlink manages only links owned by the current application.
package commandlink

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/knothhe/passpine/client/buildinfo"
)

const Alias = "pass"

var ErrUnsupported = errors.New("command links are unsupported on Windows")

type ConflictError struct{ Path string }

func (e *ConflictError) Error() string { return e.Path }

type Entry struct {
	Name, Path               string
	Exists, Owned, Removable bool
}

type Status struct{ Main, Alias Entry }

type Manager struct{ Executable, Directory, Name string }

// Current resolves the running application, including launches through links.
// It never creates directories or reads any vault or credentials.
func Current(directory string) (Manager, error) {
	if runtime.GOOS == "windows" {
		return Manager{}, ErrUnsupported
	}
	executable, err := os.Executable()
	if err == nil {
		executable, err = filepath.EvalSymlinks(executable)
	}
	if err != nil {
		return Manager{}, err
	}
	if directory == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Manager{}, err
		}
		directory = filepath.Join(home, ".local", "bin")
	}
	directory, err = filepath.Abs(directory)
	name := "passpine-dev"
	if buildinfo.IsRelease() {
		name = "passpine"
	}
	return Manager{executable, directory, name}, err
}

func (m Manager) Inspect(name string) (Entry, error) {
	entry := Entry{Name: name, Path: filepath.Join(m.Directory, name)}
	if runtime.GOOS == "windows" {
		return entry, ErrUnsupported
	}
	info, err := os.Lstat(entry.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return entry, nil
	}
	if err != nil {
		return entry, err
	}
	entry.Exists = true
	resolved, err := filepath.EvalSymlinks(entry.Path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return entry, err
	}
	entry.Owned = err == nil && resolved == m.Executable
	entry.Removable = entry.Owned && info.Mode()&os.ModeSymlink != 0
	return entry, nil
}

func (m Manager) Status() (Status, error) {
	main, mainErr := m.Inspect(m.Name)
	alias, aliasErr := m.Inspect(Alias)
	return Status{main, alias}, errors.Join(mainErr, aliasErr)
}

// InstallLink never overwrites a file, directory, or another application's link.
func (m Manager) InstallLink(name string) (bool, error) {
	entry, err := m.Inspect(name)
	if err != nil || entry.Owned {
		return false, err
	}
	if entry.Exists {
		return false, &ConflictError{entry.Path}
	}
	if err := os.MkdirAll(m.Directory, 0o755); err != nil {
		return false, err
	}
	if err := os.Symlink(m.Executable, entry.Path); err != nil {
		return false, err
	}
	return true, nil
}

func (m Manager) Install(includeAlias bool) error {
	names := []string{m.Name}
	if includeAlias {
		names = append(names, Alias)
	}
	// Check all destinations before changing either command.
	for _, name := range names {
		entry, err := m.Inspect(name)
		if err != nil {
			return err
		}
		if entry.Exists && !entry.Owned {
			return &ConflictError{entry.Path}
		}
	}
	for _, name := range names {
		if _, err := m.InstallLink(name); err != nil {
			return err
		}
	}
	return nil
}

// RemoveLink preserves installed binaries, foreign links and other channels.
func (m Manager) RemoveLink(name string) (bool, error) {
	entry, err := m.Inspect(name)
	if err != nil || !entry.Removable {
		return false, err
	}
	return true, os.Remove(entry.Path)
}

func (m Manager) Uninstall() error {
	for _, name := range []string{Alias, m.Name} {
		if _, err := m.RemoveLink(name); err != nil {
			return err
		}
	}
	return nil
}
