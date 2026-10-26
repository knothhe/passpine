package vault

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func safeRestoreEmptyFolder(root, name string) error {
	path, closeAll, err := windowsParent(root, filepath.FromSlash(name), false)
	if err != nil {
		return err
	}
	defer closeAll()
	if err = os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	// Opening the existing/new leaf rejects regular files and reparse points;
	// pinned parents prevent a swap from redirecting directory creation.
	f, err := windowsOpen(path, windows.FILE_READ_ATTRIBUTES, windows.OPEN_EXISTING, true)
	if err != nil {
		return err
	}
	return f.Close()
}
