//go:build !windows

package app

import (
	"os"
	"testing"
)

func assertPrivateFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("file is not private", err)
	}
}
