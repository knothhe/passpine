package nativehost

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func manualBrowserFixture(t *testing.T) (string, string, BrowserConfiguration) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "Local App Data"))
	program := filepath.Join(root, "Custom Browser.exe")
	if runtime.GOOS == "darwin" {
		program = filepath.Join(root, "Custom Browser.app", "Contents", "MacOS", "browser")
		if err := os.MkdirAll(filepath.Dir(program), 0700); err != nil {
			t.Fatal(err)
		}
		plist := `<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>test.custom.browser</string><key>CFBundleExecutable</key><string>browser</string><key>CFBundleName</key><string>Custom Browser</string><key>CrProductDirName</key><string>Test Vendor/Custom Browser</string></dict></plist>`
		if err := os.WriteFile(filepath.Join(root, "Custom Browser.app", "Contents", "Info.plist"), []byte(plist), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(program, []byte("fixture, never executed"), 0700); err != nil {
		t.Fatal(err)
	}
	path := program
	if runtime.GOOS == "darwin" {
		path = filepath.Join(root, "Custom Browser.app")
	}
	return root, program, BrowserConfiguration{Name: "用户浏览器", Path: path, Kind: "chromium", Directory: filepath.Join(root, "Browser Data", "NativeMessagingHosts"), RegistryPrefix: `Software\Passpine Test\Custom Browser`}
}

func TestManualBrowserConfigurationPersistsRegistersAndTracksMissingProgram(t *testing.T) {
	root, program, configuration := manualBrowserFixture(t)
	for _, kind := range []string{"chromium", "firefox"} {
		t.Run(kind, func(t *testing.T) {
			dataDir := filepath.Join(root, kind)
			if err := os.MkdirAll(dataDir, 0700); err != nil {
				t.Fatal(err)
			}
			configuration.Kind = kind
			if err := SaveBrowserConfiguration(dataDir, configuration); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadBrowserConfigurations(dataDir)
			if err != nil || len(loaded) != 1 || loaded[0].Name != configuration.Name {
				t.Fatalf("load: %+v %v", loaded, err)
			}
			target := configurationTarget(loaded[0])
			if !browserInstalled(target) {
				t.Fatal("manual executable was not detected")
			}
			if _, err := os.Stat(target.directory); !os.IsNotExist(err) {
				t.Fatal("saving configuration created browser files")
			}
			helper := program
			paths, err := registerTargets(helper, []registrationTarget{target})
			if err != nil || len(paths) != 1 {
				t.Fatalf("register: %v %v", paths, err)
			}
			data, err := os.ReadFile(paths[0])
			if err != nil {
				t.Fatal(err)
			}
			if !manifestMatches(data, manifestFor(helper, target)) {
				t.Fatal("wrong extension authorization")
			}
			if kind == "firefox" && len(manifestFor(helper, target).AllowedOrigins) != 0 {
				t.Fatal("Firefox received Chromium authorization")
			}
			if err := os.Remove(paths[0]); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(target.directory); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := os.Remove(program); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadBrowserConfigurations(filepath.Join(root, "chromium"))
	if err != nil || len(loaded) != 1 {
		t.Fatal("missing browser destroyed saved configuration")
	}
	if browserInstalled(configurationTarget(loaded[0])) {
		t.Fatal("missing executable reported installed")
	}
	for _, item := range RegistrationsWithConfigurations(program, loaded) {
		if item.ID == loaded[0].ID && (item.Registered || item.Status != "browser_missing") {
			t.Fatalf("missing browser status: %+v", item)
		}
	}
}

func TestManualBrowserRemovalPreservesForeignManifestAndRetriesFailure(t *testing.T) {
	root, helper, c := manualBrowserFixture(t)
	if err := SaveBrowserConfiguration(root, c); err != nil {
		t.Fatal(err)
	}
	loaded, _ := LoadBrowserConfigurations(root)
	target := configurationTarget(loaded[0])
	paths, err := registerTargets(helper, []registrationTarget{target})
	if err != nil {
		t.Fatal(err)
	}
	failure := func(registrationTarget, string) error { return errors.New("detach failed") }
	if err := removeBrowserConfiguration(root, helper, loaded[0].ID, failure); err == nil {
		t.Fatal("removal failure was ignored")
	}
	if remaining, _ := LoadBrowserConfigurations(root); len(remaining) != 1 {
		t.Fatal("failure lost retry configuration")
	}
	if _, err := os.Stat(paths[0]); err != nil {
		t.Fatal("failed detach removed manifest")
	}
	data, _ := json.Marshal(manifestFor(filepath.Join(root, "Another Host"), target))
	if err := os.WriteFile(paths[0], data, 0600); err != nil {
		t.Fatal(err)
	}
	detach := func(registrationTarget, string) error { t.Fatal("foreign registration detached"); return nil }
	if err := removeBrowserConfiguration(root, helper, loaded[0].ID, detach); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(paths[0]); string(got) != string(data) {
		t.Fatal("another installation's manifest was changed")
	}
	if remaining, _ := LoadBrowserConfigurations(root); len(remaining) != 0 {
		t.Fatal("configuration not removed")
	}
	if err := SaveBrowserConfiguration(root, c); err != nil {
		t.Fatal(err)
	}
	if _, err := registerTargets(helper, []registrationTarget{target}); err != nil {
		t.Fatal(err)
	}
	if err := removeBrowserConfiguration(root, helper, loaded[0].ID, func(registrationTarget, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths[0]); !os.IsNotExist(err) {
		t.Fatal("owned manifest remains")
	}
}

func TestManualBrowserConfigurationRejectsInvalidInputsAndPreservesCorruption(t *testing.T) {
	root, _, c := manualBrowserFixture(t)
	for name, change := range map[string]func(*BrowserConfiguration){
		"relative executable": func(c *BrowserConfiguration) { c.Path = "browser" },
		"missing executable":  func(c *BrowserConfiguration) { c.Path += " missing" },
		"invalid engine":      func(c *BrowserConfiguration) { c.Kind = "safari" },
		"relative destination": func(c *BrowserConfiguration) {
			c.Directory = "NativeMessagingHosts"
			c.RegistryPrefix = `HKLM\Software\Browser`
		},
		"wrong directory or registry leaf": func(c *BrowserConfiguration) { c.Directory = root; c.RegistryPrefix += `\NativeMessagingHosts` },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := c
			change(&invalid)
			if err := SaveBrowserConfiguration(root, invalid); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
	file := filepath.Join(root, browserConfigurationFile)
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("invalid input wrote configuration")
	}
	if err := os.WriteFile(file, []byte("broken JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveBrowserConfiguration(root, c); err == nil {
		t.Fatal("corrupted configuration was overwritten")
	}
	if got, _ := os.ReadFile(file); string(got) != "broken JSON" {
		t.Fatal("corrupted configuration changed")
	}
}

func TestManualBrowserOverridesDetectionWithoutDuplicateRegistration(t *testing.T) {
	root, _, c := manualBrowserFixture(t)
	c, err := NormalizeBrowserConfiguration(c)
	if err != nil {
		t.Fatal(err)
	}
	original := configurationTarget(c)
	original.id, original.name = "builtin", "Built-in"
	targets := configuredTargets([]registrationTarget{original}, []BrowserConfiguration{c})
	if len(targets) != 1 || targets[0].id != c.ID {
		t.Fatal("duplicate registration destination")
	}
	if err := SaveBrowserConfiguration(root, c); err != nil {
		t.Fatal(err)
	}
	c.Name = "Renamed browser"
	if err := SaveBrowserConfiguration(root, c); err != nil {
		t.Fatal(err)
	}
	loaded, _ := LoadBrowserConfigurations(root)
	if len(loaded) != 1 || loaded[0].Name != c.Name {
		t.Fatal("editing duplicated the browser")
	}
	c.Kind = "firefox"
	if err := SaveBrowserConfiguration(root, c); err == nil {
		t.Fatal("conflicting engine accepted")
	}
}

func TestManualBrowserRejectsRegistrationDirectoryReplacedWithAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix directory alias regression; Windows symlink creation requires privileges")
	}
	root, helper, c := manualBrowserFixture(t)
	c, err := NormalizeBrowserConfiguration(c)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "Other Data")
	if err := os.MkdirAll(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Dir(c.Directory)); err != nil {
		t.Fatal(err)
	}
	if _, err := registerTargets(helper, []registrationTarget{configurationTarget(c)}); err == nil {
		t.Fatal("registration followed a replaced parent")
	}
	if _, err := os.Stat(filepath.Join(other, "NativeMessagingHosts")); !os.IsNotExist(err) {
		t.Fatal("registration wrote through alias")
	}
}
