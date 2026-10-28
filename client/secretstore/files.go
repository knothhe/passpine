package secretstore

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

func privateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrInvalid
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return ErrInvalid
	}
	return nil
}

func readPrivateFile(path string, limit int64) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, ErrUnavailable
	}
	if !info.Mode().IsRegular() || info.Size() > limit || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, true, ErrInvalid
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, true, ErrUnavailable
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, true, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, true, ErrUnavailable
	}
	if int64(len(data)) > limit {
		return nil, true, ErrInvalid
	}
	return data, true, nil
}

func atomicPrivateFile(path string, data []byte) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return ErrInvalid
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrUnavailable
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".sync-secrets-*")
	if err != nil {
		return ErrUnavailable
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return ErrUnavailable
	}
	if err = replacePrivateFile(file.Name(), path); err != nil {
		return ErrUnavailable
	}
	return nil
}

func acquireFileLock(path string) (func(), error) {
	if err := privateDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err == nil && !info.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, ErrUnavailable
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, ErrUnavailable
	}
	opened, err := file.Stat()
	actual, statErr := os.Lstat(path)
	if err != nil || statErr != nil || !actual.Mode().IsRegular() || !os.SameFile(opened, actual) || runtime.GOOS != "windows" && opened.Mode().Perm()&0077 != 0 {
		file.Close()
		return nil, ErrInvalid
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		locked, err := tryFileLock(file)
		if err != nil {
			file.Close()
			return nil, ErrUnavailable
		}
		if locked {
			// A waiter must still be locking the path every other process
			// will open, rather than an inode unlinked during the wait.
			actual, err := os.Lstat(path)
			if err != nil || !actual.Mode().IsRegular() || !os.SameFile(opened, actual) || runtime.GOOS != "windows" && actual.Mode().Perm()&0077 != 0 {
				releaseFileLock(file)
				file.Close()
				return nil, ErrInvalid
			}
			return func() { releaseFileLock(file); file.Close() }, nil
		}
		if time.Now().After(deadline) {
			file.Close()
			return nil, ErrUnavailable
		}
		time.Sleep(10 * time.Millisecond)
	}
}
