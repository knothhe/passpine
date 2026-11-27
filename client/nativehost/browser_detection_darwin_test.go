package nativehost

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/knothhe/passpine/client/i18n"
)

func macBrowserBundleFixture(t *testing.T, bundleID string, installed bool) (string, string) {
	t.Helper()
	appPath := filepath.Join(t.TempDir(), "Browser With Spaces.app")
	executable := filepath.Join(appPath, "Contents", "MacOS", "browser")
	if err := os.MkdirAll(filepath.Dir(executable), 0700); err != nil {
		t.Fatal(err)
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
	<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
	<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>%s</string><key>CFBundleExecutable</key><string>browser</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>`, bundleID)
	if err := os.WriteFile(filepath.Join(appPath, "Contents", "Info.plist"), []byte(plist), 0600); err != nil {
		t.Fatal(err)
	}
	if installed {
		if err := os.WriteFile(executable, []byte("fixture"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return appPath, executable
}

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
		{"Kaze installed", "kaze", "dev.kite.browser", true, true},
		{"wrong identity", "chrome", "other.application", true, false},
		{"missing executable", "chrome", "com.google.Chrome", false, false},
		{"ego lite installed", "ego-lite", "com.citrolabs.ego.lite", true, true},
		{"ego lite wrong identity", "ego-lite", "com.citrolabs.ego", true, false},
		{"ego lite missing executable", "ego-lite", "com.citrolabs.ego.lite", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			identifier := browserDiscovery.stringCreate(0, browserBundleID(test.browserID), 0x08000100)
			defer browserDiscovery.release(identifier)
			appPath, executable := macBrowserBundleFixture(t, test.identifier, test.executable)
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

func TestMacManualBrowserWithoutProductDirectory(t *testing.T) {
	for _, test := range []struct {
		bundleID, kind, product, wantErr string
	}{
		{"dev.kite.browser", "chromium", "Kite", ""},
		{"com.google.Chrome", "chromium", "Google/Chrome", ""},
		{"org.mozilla.firefox", "firefox", "Mozilla", ""},
		{"org.mozilla.firefox", "chromium", "", i18n.BrowserDestinationInvalid},
		{"unknown.browser", "chromium", "", i18n.BrowserDestinationUnknown},
	} {
		t.Run(test.bundleID+"/"+test.kind, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", root)
			appPath, _ := macBrowserBundleFixture(t, test.bundleID, true)
			c := BrowserConfiguration{Path: appPath, Kind: test.kind}
			got, err := NormalizeBrowserConfiguration(c)
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Fatalf("expected %q, got %v", test.wantErr, err)
				}
				return
			}
			want := filepath.Join(root, "Library", "Application Support", test.product, "NativeMessagingHosts")
			if err != nil || got.Directory != want {
				t.Fatalf("directory=%q, error=%v, want %q", got.Directory, err, want)
			}
			c.Directory = filepath.Join(root, "Custom", "NativeMessagingHosts")
			if got, err := NormalizeBrowserConfiguration(c); err != nil || got.Directory != c.Directory {
				t.Fatalf("explicit directory=%q, error=%v", got.Directory, err)
			}
		})
	}
}
