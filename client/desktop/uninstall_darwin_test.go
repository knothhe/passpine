package desktop

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDevelopmentUninstallHandlesAbsentAndRegisteredLoginServices(t *testing.T) {
	permission := errors.New("operation not permitted")
	for _, test := range []struct {
		name   string
		status int
		called bool
		failed bool
	}{
		{"not registered", 0, false, false},
		{"enabled", 1, true, true},
		{"requires approval", 2, true, true},
		{"not found in ad-hoc bundle", 3, false, false},
		{"unknown status", 4, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			err := removeDevelopmentLoginItem(test.status, func() error { called = true; return permission })
			if called != test.called || (err != nil) != test.failed {
				t.Fatalf("called=%v error=%v", called, err)
			}
			if called && !errors.Is(err, permission) {
				t.Fatal("registered service removal error was suppressed")
			}
		})
	}
	called := false
	if err := removeDevelopmentLoginItem(1, func() error { called = true; return nil }); err != nil || !called {
		t.Fatalf("enabled service was not removed: %v", err)
	}
}

// Opt in with a built bundle. The copy gets a unique system identity so this
// regression never unregisters a real installed application's login service.
func TestDevelopmentUninstallWithoutLoginServiceInRealBundle(t *testing.T) {
	source := os.Getenv("PASSPINE_TEST_CLIENT_APP")
	if source == "" {
		t.Skip("set PASSPINE_TEST_CLIENT_APP to a built Dev app bundle")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, "Uninstall Probe.app")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	run := func(command string, args ...string) []byte {
		t.Helper()
		output, err := exec.CommandContext(ctx, command, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", command, err, output)
		}
		return output
	}
	run("/usr/bin/ditto", source, bundle)
	run("/usr/libexec/PlistBuddy", "-c", fmt.Sprintf("Set CFBundleIdentifier com.knothhe.passpine.uninstall-test.%s", rand.Text()), filepath.Join(bundle, "Contents/Info.plist"))
	run("/usr/bin/codesign", "--force", "--deep", "--sign", "-", bundle)
	run("/usr/bin/codesign", "--verify", "--deep", "--strict", bundle)
	directory := filepath.Join(root, "data")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(directory, "preserve-user-data")
	contents := []byte("isolated uninstall regression fixture")
	if err := os.WriteFile(sentinel, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(bundle, "Contents/MacOS/passpine")
	if channel := run(executable, "--build-channel"); string(bytes.TrimSpace(channel)) != "dev" {
		t.Fatal("regression fixture must be a Dev build")
	}
	for attempt := 0; attempt < 2; attempt++ {
		command := exec.CommandContext(ctx, executable, "--quit-for-uninstall")
		command.Env = append(os.Environ(), "HOME="+root, "PASSPINE_DEV_HOME="+directory)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("uninstall preparation %d: %v\n%s", attempt+1, err, output)
		}
		if bytes.Contains(output, []byte("MyGo's own strings are in English")) {
			t.Fatalf("native Chinese locale is missing: %s", output)
		}
	}
	if after, err := os.ReadFile(sentinel); err != nil || !bytes.Equal(after, contents) {
		t.Fatalf("user data changed: %v", err)
	}
}
