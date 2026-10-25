//go:build !windows

package vault

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func safeRestoreEmptyFolder(root, name string) error {
	fd, err := directoryFD(root, filepath.Dir(name), false)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if err = unix.Mkdirat(fd, filepath.Base(name), 0700); errors.Is(err, unix.EEXIST) {
		// Pin and inspect the existing leaf too; a symlink is never accepted.
		leaf, err := unix.Openat(fd, filepath.Base(name), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return ErrPath
		}
		return unix.Close(leaf)
	} else if err != nil {
		return err
	}
	return unix.Fsync(fd)
}
