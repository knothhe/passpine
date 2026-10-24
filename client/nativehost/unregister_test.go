package nativehost

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnregisterPreservesOtherInstallationAndCanRetryPlatformFailure(t *testing.T) {
	directory := t.TempDir()
	directory, _ = filepath.EvalSymlinks(directory)
	target := registrationTarget{id: "chrome", name: "Test browser", directory: directory}
	executable := filepath.Join(directory, "owned-host")
	manifest := filepath.Join(directory, Name+".json")
	write := func(host string) {
		data, _ := json.Marshal(manifestFor(host, target))
		if err := os.WriteFile(manifest, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	called := false
	detach := func(registrationTarget, string) error { called = true; return nil }
	write("another-host")
	if err := unregisterTargets(executable, []registrationTarget{target}, detach); err != nil || called {
		t.Fatal("claimed another installation's registration")
	}
	if _, err := os.Stat(manifest); err != nil {
		t.Fatal("foreign manifest removed")
	}
	write(executable)
	failure := errors.New("registry unavailable")
	if err := unregisterTargets(executable, []registrationTarget{target}, func(registrationTarget, string) error { return failure }); !errors.Is(err, failure) {
		t.Fatal("platform failure not reported")
	}
	if _, err := os.Stat(manifest); err != nil {
		t.Fatal("partial failure lost retry ownership")
	}
	if err := unregisterTargets(executable, []registrationTarget{target}, detach); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manifest); !os.IsNotExist(err) {
		t.Fatal("owned manifest remains")
	}
	if err := unregisterTargets(executable, []registrationTarget{target}, detach); err != nil {
		t.Fatal("repeated unregister failed")
	}
}
