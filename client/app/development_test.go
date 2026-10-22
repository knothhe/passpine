package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/knothhe/passpine/client/buildinfo"
)

func TestDevelopmentRejectsExplicitReleaseHomeBeforeCreatingFiles(t *testing.T) {
	if buildinfo.IsRelease() {
		t.Skip("development data boundary")
	}
	directory := filepath.Join(t.TempDir(), "release-home")
	t.Setenv("PASSPINE_HOME", directory)
	for _, preview := range []bool{false, true} {
		service, err := New(Options{DataDir: directory, Preview: preview})
		if service != nil || err == nil {
			t.Fatal("development opened release application data")
		}
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("rejected development startup created or touched release data")
	}
}

func TestPreviewWithExplicitFixtureRemainsAvailable(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "isolated-preview")
	t.Setenv("PASSPINE_DEV_HOME", directory)
	service, err := New(Options{DataDir: directory, Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if !service.preview || service.DataDirectory() != directory {
		t.Fatal("isolated preview ignored the fixture directory")
	}
}
