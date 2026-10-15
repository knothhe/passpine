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
	identifier := browserDiscovery.stringCreate(0, browserBundleID("chrome"), 0x08000100)
	defer browserDiscovery.release(identifier)
	for _, test := range []struct {
		name       string
		identifier string
		executable bool
		want       bool
	}{
		{"installed", "com.google.Chrome", true, true},
		{"wrong identity", "other.application", true, false},
		{"missing executable", "com.google.Chrome", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			appPath := filepath.Join(t.TempDir(), "Chrome.app")
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
