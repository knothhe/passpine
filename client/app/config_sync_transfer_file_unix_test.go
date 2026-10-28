//go:build !windows

package app

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
)

func TestConfigSyncTransferFileRejectsNamedPipeBeforeOpening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config-transfer.pipe")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadConfigSyncTransferFile(path); !errors.Is(err, ErrInvalidConfigSyncTransfer) {
		t.Fatal("import accepted a named pipe instead of a regular file", err)
	}
}
