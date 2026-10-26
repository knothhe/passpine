package vault

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const maxFileSize = 16 << 20
const maxPlaintextSize = 8 << 20

// Open reparse points themselves and deny delete sharing. Holding every parent
// handle prevents a directory from being renamed or replaced during an operation.
func windowsOpen(path string, access, disposition uint32, directory bool) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	var attributes *windows.SecurityAttributes
	if disposition == windows.CREATE_NEW || disposition == windows.OPEN_ALWAYS {
		user, err := windows.GetCurrentProcessToken().GetTokenUser()
		if err != nil {
			return nil, err
		}
		sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + user.User.Sid.String() + ")")
		if err != nil {
			return nil, err
		}
		attributes = &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	}
	h, err := windows.CreateFile(p, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, attributes, disposition, flags, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	var info windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(h, &info)
	if err != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		windows.CloseHandle(h)
		if err != nil {
			return nil, err
		}
		return nil, ErrPath
	}
	return os.NewFile(uintptr(h), path), nil
}

func windowsParent(root, name string, create bool) (string, func(), error) {
	if !filepath.IsLocal(name) || strings.ContainsAny(name, ":") {
		return "", nil, ErrPath
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", nil, err
	}
	var handles []*os.File
	closeAll := func() {
		for i := len(handles) - 1; i >= 0; i-- {
			handles[i].Close()
		}
	}
	// Pin ancestors of root too, including user-selected vault locations.
	volume := filepath.VolumeName(absolute)
	current := volume + string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(absolute, current), string(filepath.Separator))
	relative := filepath.Dir(name)
	if relative != "." {
		parts = append(parts, strings.Split(relative, string(filepath.Separator))...)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.Contains(part, ":") {
			closeAll()
			return "", nil, ErrPath
		}
		current = filepath.Join(current, part)
		if create {
			if err = os.Mkdir(current, 0700); err != nil && !errors.Is(err, os.ErrExist) {
				closeAll()
				return "", nil, err
			}
		}
		f, e := windowsOpen(current, windows.GENERIC_READ, windows.OPEN_EXISTING, true)
		if e != nil {
			closeAll()
			return "", nil, e
		}
		handles = append(handles, f)
	}
	return filepath.Join(current, filepath.Base(name)), closeAll, nil
}

func safeMkdirAll(root, name string) error {
	// windowsParent creates and pins every parent. The final child is only a
	// path argument; no placeholder file is opened or written.
	_, closeAll, err := windowsParent(root, filepath.Join(filepath.FromSlash(name), ".passpine-folder"), true)
	if err != nil {
		return err
	}
	closeAll()
	return nil
}

func safeRemoveEmptyFolder(root, name string) (bool, error) {
	path, closeAll, err := windowsParent(root, name, false)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer closeAll()
	f, err := windowsOpen(path, windows.FILE_READ_ATTRIBUTES, windows.OPEN_EXISTING, true)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	f.Close()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	// Unlike os.Remove, RemoveDirectory cannot delete a regular file swapped
	// into this path, and a concurrently added child makes removal fail.
	err = windows.RemoveDirectory(p)
	if errors.Is(err, windows.ERROR_DIR_NOT_EMPTY) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return false, nil
	}
	return err == nil, err
}

func safeRead(root, name string) ([]byte, error) {
	path, closeAll, err := windowsParent(root, name, false)
	if err != nil {
		return nil, err
	}
	defer closeAll()
	f, err := windowsOpen(path, windows.GENERIC_READ, windows.OPEN_EXISTING, false)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > maxFileSize {
		return nil, ErrPath
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if len(data) > maxFileSize {
		wipe(data)
		return nil, ErrPath
	}
	return data, err
}

func safeStat(root, name string) (os.FileInfo, error) {
	path, closeAll, err := windowsParent(root, name, false)
	if err != nil {
		return nil, err
	}
	defer closeAll()
	f, err := windowsOpen(path, windows.FILE_READ_ATTRIBUTES, windows.OPEN_EXISTING, false)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > maxFileSize {
		return nil, ErrPath
	}
	return stat, nil
}

func safeWrite(root, name string, data []byte, replace bool) error {
	path, closeAll, err := windowsParent(root, name, true)
	if err != nil {
		return err
	}
	defer closeAll()
	existing, err := windowsOpen(path, windows.FILE_READ_ATTRIBUTES, windows.OPEN_EXISTING, false)
	if err == nil {
		existing.Close()
		if !replace {
			return ErrExists
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temp := filepath.Join(filepath.Dir(path), ".passpine-tmp-"+randomID())
	f, err := windowsOpen(temp, windows.GENERIC_WRITE, windows.CREATE_NEW, false)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	from, err := windows.UTF16PtrFromString(temp)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	flags := uint32(windows.MOVEFILE_WRITE_THROUGH)
	if replace {
		flags |= windows.MOVEFILE_REPLACE_EXISTING
	}
	err = windows.MoveFileEx(from, to, flags)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
		return ErrExists
	}
	return err
}

func safeRemove(root, name string) error {
	path, closeAll, err := windowsParent(root, name, false)
	if err != nil {
		return err
	}
	defer closeAll()
	f, err := windowsOpen(path, windows.FILE_READ_ATTRIBUTES, windows.OPEN_EXISTING, false)
	if err != nil {
		return err
	}
	f.Close()
	// Parent handles prevent redirection; Remove deletes a replacement link itself.
	return os.Remove(path)
}

func lockStore(root string) (func(), error) {
	path, closeAll, err := windowsParent(root, ".passpine.lock", false)
	if err != nil {
		return nil, err
	}
	f, err := windowsOpen(path, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.OPEN_ALWAYS, false)
	if err != nil {
		closeAll()
		return nil, err
	}
	overlapped := new(windows.Overlapped)
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlapped)
	if err != nil {
		f.Close()
		closeAll()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return func() { windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, overlapped); f.Close(); closeAll() }, nil
}
