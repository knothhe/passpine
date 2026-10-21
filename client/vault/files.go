//go:build !windows

package vault

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const maxFileSize = 16 << 20
const maxPlaintextSize = 8 << 20

// directoryFD pins each component using openat(O_NOFOLLOW). Renaming or swapping
// a directory for a symlink cannot redirect a subsequent operation out of root.
func directoryFD(root, relative string, create bool) (int, error) {
	fd, e := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if e != nil {
		return -1, e
	}
	if relative == "." || relative == "" {
		return fd, nil
	}
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		if part == "" || part == "." || part == ".." {
			unix.Close(fd)
			return -1, ErrPath
		}
		if create {
			e = unix.Mkdirat(fd, part, 0700)
			if e != nil && !errors.Is(e, unix.EEXIST) {
				unix.Close(fd)
				return -1, e
			}
		}
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		unix.Close(fd)
		if e != nil {
			return -1, fmt.Errorf("%w: %w", ErrPath, e)
		}
		fd = next
	}
	return fd, nil
}

func safeRead(root, name string) ([]byte, error) {
	fd, e := directoryFD(root, filepath.Dir(name), false)
	if e != nil {
		return nil, e
	}
	defer unix.Close(fd)
	f, e := openAt(fd, filepath.Base(name), unix.O_RDONLY, 0)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	s, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !s.Mode().IsRegular() || s.Size() > maxFileSize {
		return nil, errors.New("invalid or oversized file")
	}
	b, e := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if len(b) > maxFileSize {
		wipe(b)
		return nil, errors.New("file exceeds size limit")
	}
	return b, e
}

func safeStat(root, name string) (os.FileInfo, error) {
	fd, e := directoryFD(root, filepath.Dir(name), false)
	if e != nil {
		return nil, e
	}
	defer unix.Close(fd)
	f, e := openAt(fd, filepath.Base(name), unix.O_RDONLY, 0)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !stat.Mode().IsRegular() || stat.Size() > maxFileSize {
		return nil, ErrPath
	}
	return stat, nil
}

func openAt(fd int, name string, flags int, mode uint32) (*os.File, error) {
	n, e := unix.Openat(fd, name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW, mode)
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(n), name), nil
}

func safeWrite(root, name string, data []byte, replace bool) error {
	fd, e := directoryFD(root, filepath.Dir(name), true)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	name = filepath.Base(name)
	var stat unix.Stat_t
	e = unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if e == nil {
		if stat.Mode&unix.S_IFMT != unix.S_IFREG {
			return ErrPath
		}
		if !replace {
			return ErrExists
		}
	} else if !errors.Is(e, unix.ENOENT) {
		return e
	}
	tmp := ".passpine-tmp-" + randomID()
	f, e := openAt(fd, tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer unix.Unlinkat(fd, tmp, 0)
	_, e = f.Write(data)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if replace {
		e = unix.Renameat(fd, tmp, fd, name)
	} else {
		e = unix.Linkat(fd, tmp, fd, name, 0)
		if errors.Is(e, unix.EEXIST) {
			return ErrExists
		}
	}
	if e != nil {
		return e
	}
	return unix.Fsync(fd)
}

func safeRemove(root, name string) error {
	fd, e := directoryFD(root, filepath.Dir(name), false)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if e = unix.Fstatat(fd, filepath.Base(name), &stat, unix.AT_SYMLINK_NOFOLLOW); e != nil {
		return e
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return ErrPath
	}
	if e = unix.Unlinkat(fd, filepath.Base(name), 0); e != nil {
		return e
	}
	return unix.Fsync(fd)
}

func lockStore(root string) (func(), error) {
	fd, e := directoryFD(root, "", false)
	if e != nil {
		return nil, e
	}
	defer unix.Close(fd)
	f, e := openAt(fd, ".passpine.lock", unix.O_RDWR|unix.O_CREAT, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, ErrBusy
	}
	return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN); _ = f.Close() }, nil
}
