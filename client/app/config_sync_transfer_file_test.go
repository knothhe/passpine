package app

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestConfigSyncTransferFileRoundTripIsPrivateAndPreservesExistingFile(t *testing.T) {
	options, config, data := transferFixture(t)
	directory := t.TempDir()
	path := filepath.Join(directory, "同步 config with spaces.json")
	if err := WriteConfigSyncTransferFile(path, data); err != nil {
		t.Fatal(err)
	}
	assertPrivateFile(t, path)
	decoded, target, err := ReadConfigSyncTransferFile(path)
	if err != nil || decoded != options || target == nil || *target != config {
		t.Fatal("file round trip changed preferences or connection", err)
	}
	options.Mode = "automatic"
	changed, err := EncodeConfigSyncTransfer(options, &config)
	if err != nil {
		t.Fatal(err)
	}
	if err = WriteConfigSyncTransferFile(path, changed); !errors.Is(err, os.ErrExist) {
		t.Fatal("export replaced an existing selected file", err)
	}
	stored, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(stored, data) {
		t.Fatal("refused replacement changed the existing file", err)
	}
	assertTransferTemporaryFilesRemoved(t, directory)
}

func TestConfigSyncTransferFileRejectsInvalidInputsWithoutWriting(t *testing.T) {
	_, _, data := transferFixture(t)
	directory := t.TempDir()
	path := filepath.Join(directory, "invalid transfer.json")
	if err := WriteConfigSyncTransferFile(path, []byte(`{"private":"unexpected"}`)); !errors.Is(err, ErrInvalidConfigSyncTransfer) {
		t.Fatal("invalid transfer was written", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid export created its destination", err)
	}
	for _, relative := range []string{"", "config.json", filepath.Join("relative", "config.json")} {
		if err := WriteConfigSyncTransferFile(relative, data); !errors.Is(err, ErrInvalidConfigSyncTransfer) {
			t.Fatalf("relative export path was accepted: %q, %v", relative, err)
		}
		if _, _, err := ReadConfigSyncTransferFile(relative); !errors.Is(err, ErrInvalidConfigSyncTransfer) {
			t.Fatalf("relative import path was accepted: %q, %v", relative, err)
		}
	}
	if _, _, err := ReadConfigSyncTransferFile(directory); !errors.Is(err, ErrInvalidConfigSyncTransfer) {
		t.Fatal("import accepted a directory", err)
	}
	if _, _, err := ReadConfigSyncTransferFile(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing file did not report a file error", err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte(" "), MaxConfigSyncTransferBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadConfigSyncTransferFile(path); !errors.Is(err, ErrInvalidConfigSyncTransfer) {
		t.Fatal("import accepted a file exceeding the transfer size limit", err)
	}
	assertTransferTemporaryFilesRemoved(t, directory)
}

func TestConfigSyncTransferFileRejectsSymlinkAndPreservesItsTarget(t *testing.T) {
	_, _, data := transferFixture(t)
	directory := t.TempDir()
	path := filepath.Join(directory, "original.json")
	link := filepath.Join(directory, "linked.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("creating a Windows test symlink requires developer mode or symbolic-link privilege")
		}
		t.Fatal(err)
	}
	if _, _, err := ReadConfigSyncTransferFile(link); !errors.Is(err, ErrInvalidConfigSyncTransfer) {
		t.Fatal("import followed a symbolic link", err)
	}
	if err := WriteConfigSyncTransferFile(link, data); !errors.Is(err, os.ErrExist) {
		t.Fatal("export replaced a symbolic link", err)
	}
	stored, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(stored, data) {
		t.Fatal("rejected symlink export changed its target", err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("rejected export removed the symbolic link", err)
	}
	assertTransferTemporaryFilesRemoved(t, directory)
}

type configSyncGrowingReader struct {
	read int
}

func (reader *configSyncGrowingReader) Read(data []byte) (int, error) {
	for i := range data {
		data[i] = ' '
	}
	reader.read += len(data)
	return len(data), nil
}

func TestConfigSyncTransferReadBoundsDataThatGrowsAfterStat(t *testing.T) {
	_, _, data := transferFixture(t)
	growth := &configSyncGrowingReader{}
	options, target, err := readConfigSyncTransfer(io.MultiReader(bytes.NewReader(data), growth))
	if !errors.Is(err, ErrInvalidConfigSyncTransfer) || target != nil || !reflect.DeepEqual(options, ConfigSyncOptions{}) {
		t.Fatal("an unbounded growing input produced partial import data", err)
	}
	if growth.read != MaxConfigSyncTransferBytes+1-len(data) {
		t.Fatalf("input growth was not bounded to one byte past the limit: %d", growth.read)
	}
}

func TestConfigSyncTransferFailedPublicationRemovesTemporaryFile(t *testing.T) {
	_, _, data := transferFixture(t)
	directory := t.TempDir()
	// An overlong final basename fails publication after the temporary file
	// has been written. This uses native file APIs on each supported target.
	path := filepath.Join(directory, string(bytes.Repeat([]byte("x"), 1024)))
	if err := WriteConfigSyncTransferFile(path, data); err == nil {
		t.Fatal("filesystem unexpectedly accepted the overlong final filename")
	}
	assertTransferTemporaryFilesRemoved(t, directory)
}

func assertTransferTemporaryFilesRemoved(t *testing.T, directory string) {
	t.Helper()
	temporary, err := filepath.Glob(filepath.Join(directory, ".passpine-*"))
	if err != nil || len(temporary) != 0 {
		t.Fatal("transfer operation retained a temporary file", temporary, err)
	}
}
