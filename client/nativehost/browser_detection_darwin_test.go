package nativehost

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMacBrowserDetectionRejectsWrongBundleIdentityAndMissingExecutable(t *testing.T) {
	browserDiscovery.once.Do(loadBrowserDiscovery)
	if !browserDiscovery.available {
		t.Fatal("macOS browser discovery frameworks are unavailable")
	}
	for _, test := range []struct {
		name       string
		browserID  string
		identifier string
		executable bool
		want       bool
	}{
		{"installed", "chrome", "com.google.Chrome", true, true},
		{"wrong identity", "chrome", "other.application", true, false},
		{"missing executable", "chrome", "com.google.Chrome", false, false},
		{"ego lite installed", "ego-lite", "com.citrolabs.ego.lite", true, true},
		{"ego lite wrong identity", "ego-lite", "com.citrolabs.ego", true, false},
		{"ego lite missing executable", "ego-lite", "com.citrolabs.ego.lite", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			identifier := browserDiscovery.stringCreate(0, browserBundleID(test.browserID), 0x08000100)
			defer browserDiscovery.release(identifier)
			appPath := filepath.Join(t.TempDir(), "Browser With Spaces.app")
			executable := filepath.Join(appPath, "Contents", "MacOS", "browser")
			if err := os.MkdirAll(filepath.Dir(executable), 0700); err != nil {
				t.Fatal(err)
			}
			plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>%s</string><key>CFBundleExecutable</key><string>browser</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>`, test.identifier)
			if err := os.WriteFile(filepath.Join(appPath, "Contents", "Info.plist"), []byte(plist), 0600); err != nil {
				t.Fatal(err)
			}
			if test.executable {
				if err := os.WriteFile(executable, []byte("fixture"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			path := browserDiscovery.stringCreate(0, appPath, 0x08000100)
			defer browserDiscovery.release(path)
			url := browserDiscovery.urlCreateWithPath(0, path, 0, true)
			defer browserDiscovery.release(url)
			if got := applicationBundleInstalled(url, identifier); got != test.want {
				t.Fatalf("installed=%v, want %v", got, test.want)
			}
			if test.want {
				if err := os.Remove(executable); err != nil {
					t.Fatal(err)
				}
				if applicationBundleInstalled(url, identifier) {
					t.Fatal("removed browser executable is still reported installed")
				}
			}
		})
	}
	if browserInstalled(registrationTarget{id: "unsupported"}) {
		t.Fatal("unsupported application was detected as a browser")
	}
}

func TestMacManualBrowserInfersProductDirectoryWithoutRegistration(t *testing.T) {
	root, _, configuration := manualBrowserFixture(t)
	t.Setenv("HOME", root)
	configuration.Name, configuration.Directory = "", ""
	got, err := NormalizeBrowserConfiguration(configuration)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "Library", "Application Support", "Test Vendor", "Custom Browser", "NativeMessagingHosts")
	if got.Name != "Custom Browser" || got.Directory != want {
		t.Fatalf("inferred metadata: %+v", got)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatal("metadata inspection created registration")
	}
}
