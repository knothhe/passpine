//go:build !windows

package secretstore

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

func tryFileLock(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return false, nil
	}
	return err == nil, err
}

func releaseFileLock(file *os.File) { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN) }

func replacePrivateFile(source, target string) error {
	if err := os.Rename(source, target); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(target))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
