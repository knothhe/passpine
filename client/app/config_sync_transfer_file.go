package app

import (
	"io"
	"os"
	"path/filepath"

	"github.com/knothhe/passpine/client/cloudsync"
)

// ReadConfigSyncTransferFile reads a user-selected regular file without following
// symlinks. The read remains bounded if the file grows after its initial stat.
func ReadConfigSyncTransferFile(path string) (ConfigSyncOptions, *cloudsync.Config, error) {
	invalid := func(err error) (ConfigSyncOptions, *cloudsync.Config, error) {
		return ConfigSyncOptions{}, nil, err
	}
	if !filepath.IsAbs(path) {
		return invalid(ErrInvalidConfigSyncTransfer)
	}
	before, err := os.Lstat(path)
	if err != nil {
		return invalid(err)
	}
	if !before.Mode().IsRegular() || before.Size() > MaxConfigSyncTransferBytes {
		return invalid(ErrInvalidConfigSyncTransfer)
	}
	file, err := os.Open(path)
	if err != nil {
		return invalid(err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return invalid(err)
	}
	// A path swapped between Lstat and Open cannot authorize another file.
	if !opened.Mode().IsRegular() || opened.Size() > MaxConfigSyncTransferBytes || !os.SameFile(before, opened) {
		return invalid(ErrInvalidConfigSyncTransfer)
	}
	return readConfigSyncTransfer(file)
}

func readConfigSyncTransfer(reader io.Reader) (ConfigSyncOptions, *cloudsync.Config, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxConfigSyncTransferBytes+1))
	if err != nil {
		return ConfigSyncOptions{}, nil, err
	}
	return DecodeConfigSyncTransfer(data)
}

// WriteConfigSyncTransferFile publishes a private file atomically and refuses
// to replace any existing target, including a symlink. Failed writes remove
// only their temporary file and leave the selected destination unchanged.
func WriteConfigSyncTransferFile(path string, data []byte) error {
	if !filepath.IsAbs(path) {
		return ErrInvalidConfigSyncTransfer
	}
	if _, _, err := DecodeConfigSyncTransfer(data); err != nil {
		return err
	}
	return writePrivate(path, data, true)
}
