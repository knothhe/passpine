package nativehost

import (
	"path/filepath"
	"testing"
)

func TestWindowsRegistrationUsesRedirectedLocalAppData(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	targets := platformRegistrationTargets(t.TempDir())
	if len(targets) != 4 {
		t.Fatalf("expected four Windows browser targets, got %d", len(targets))
	}
	for _, target := range targets {
		want := filepath.Join(local, "Passpine", "NativeMessagingHosts", target.id)
		if target.directory != want {
			t.Errorf("%s manifest directory = %q, want %q", target.id, target.directory, want)
		}
	}
}
