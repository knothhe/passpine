package nativehost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func registrationFixture(t *testing.T) (string, []registrationTarget) {
	t.Helper()
	helper := filepath.Join(t.TempDir(), "passpine-host")
	if err := os.WriteFile(helper, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	return helper, registrationTargets(t.TempDir(), "darwin")
}

func TestRegistrationsReportEachBrowserWithoutWriting(t *testing.T) {
	helper, targets := registrationFixture(t)
	got := inspectRegistrations(helper, targets)
	if len(got) != 7 {
		t.Fatalf("expected seven browser registrations, got %d", len(got))
	}
	for _, item := range got {
		if item.Registered || item.Status != "missing" || item.Name == "" || item.ExtensionID == "" {
			t.Fatalf("incorrect missing registration: %+v", item)
		}
		if _, err := os.Stat(filepath.Dir(item.ManifestPath)); !os.IsNotExist(err) {
			t.Fatal("status check created a browser directory")
		}
	}
	if _, err := registerTargets(helper, targets); err != nil {
		t.Fatal(err)
	}
	for _, item := range inspectRegistrations(helper, targets) {
		if !item.Registered || item.Status != "registered" {
			t.Fatalf("registration not recognized: %+v", item)
		}
		if item.ID == "firefox" && item.ExtensionID != FirefoxID {
			t.Fatal("Firefox registration uses the wrong extension identity")
		}
	}
}

func TestRegistrationRejectsStaleOrExpandedIdentitiesAndRepairs(t *testing.T) {
	helper, targets := registrationFixture(t)
	targets = targets[:1]
	if _, err := registerTargets(helper, targets); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(targets[0].directory, Name+".json")
	for name, change := range map[string]func(*hostManifest){
		"moved helper":           func(m *hostManifest) { m.Path += ".old" },
		"wrong host":             func(m *hostManifest) { m.Name += ".other" },
		"wrong type":             func(m *hostManifest) { m.Type = "http" },
		"extra origin":           func(m *hostManifest) { m.AllowedOrigins = append(m.AllowedOrigins, "chrome-extension://attacker/") },
		"extra Firefox identity": func(m *hostManifest) { m.AllowedExtensions = []string{FirefoxID} },
	} {
		t.Run(name, func(t *testing.T) {
			manifest := manifestFor(helper, targets[0])
			change(&manifest)
			data, _ := json.Marshal(manifest)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if got := inspectRegistrations(helper, targets)[0]; got.Registered || got.Status != "invalid" {
				t.Fatalf("unsafe or stale registration accepted: %+v", got)
			}
			if _, err := registerTargets(helper, targets); err != nil {
				t.Fatal(err)
			}
			if !inspectRegistrations(helper, targets)[0].Registered {
				t.Fatal("repair did not restore registration")
			}
		})
	}
	if err := os.Remove(helper); err != nil {
		t.Fatal(err)
	}
	if got := inspectRegistrations(helper, targets)[0]; got.Registered || got.Status != "helper_missing" {
		t.Fatalf("missing helper reported ready: %+v", got)
	}
}

func TestRegistrationRequiresExecutableHelperAndRegularManifest(t *testing.T) {
	helper, targets := registrationFixture(t)
	targets = targets[:1]
	if _, err := registerTargets(helper, targets); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(helper, 0600); err != nil {
		t.Fatal(err)
	}
	if HelperAvailable(helper) || inspectRegistrations(helper, targets)[0].Registered {
		t.Fatal("non-executable helper reported ready")
	}
	path := filepath.Join(targets[0].directory, Name+".json")
	if err := os.Rename(path, path+".target"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path+".target", path); err != nil {
		t.Fatal(err)
	}
	if got := inspectRegistrations(helper, targets)[0]; got.Registered || got.Status != "invalid" {
		t.Fatal("symlinked manifest reported ready")
	}
	if _, err := registerTargets(helper, targets); err == nil {
		t.Fatal("registration replaced an unsafe manifest")
	}
}

func TestRegistrationContinuesAfterOneBrowserFails(t *testing.T) {
	helper, targets := registrationFixture(t)
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	targets[0].directory = filepath.Join(blocked, "NativeMessagingHosts")
	paths, err := registerTargets(helper, targets)
	if err == nil || len(paths) != len(targets)-1 {
		t.Fatalf("failure prevented other registrations: paths=%d, error=%v", len(paths), err)
	}
	if !strings.Contains(err.Error(), "Chrome:") {
		t.Fatal("registration failure does not identify the affected browser")
	}
}

func TestRegistrationDetectsInstalledBrowsersWithoutTrustingSupportDirectories(t *testing.T) {
	helper, targets := registrationFixture(t)
	// An earlier installation can leave a valid host manifest behind even after
	// its browser is removed. It must not put that browser back into the list.
	if _, err := registerTargets(helper, targets[1:2]); err != nil {
		t.Fatal(err)
	}
	installed := map[string]bool{"chrome": true}
	detect := func() []registrationTarget {
		return installedTargets(targets, func(target registrationTarget) bool { return installed[target.id] })
	}
	got := inspectRegistrations(helper, detect())
	if len(got) != 1 || got[0].ID != "chrome" || got[0].Registered {
		t.Fatalf("residual browser configuration was treated as an installed browser: %+v", got)
	}
	if _, err := registerBrowserTargets(helper, detect(), ""); err != nil {
		t.Fatal(err)
	}
	for _, target := range targets[2:] {
		if _, err := os.Stat(target.directory); !os.IsNotExist(err) {
			t.Fatalf("repair created an uninstalled browser's directory: %s", target.id)
		}
	}
	installed["firefox"] = true
	if _, err := registerBrowserTargets(helper, detect(), ""); err != nil {
		t.Fatal(err)
	}
	if got = inspectRegistrations(helper, detect()); len(got) != 2 || !got[1].Registered || got[1].ID != "firefox" {
		t.Fatalf("redetection did not register the newly installed browser: %+v", got)
	}
	delete(installed, "chrome")
	if got = inspectRegistrations(helper, detect()); len(got) != 1 || got[0].ID != "firefox" {
		t.Fatalf("removed browser remains in registration status: %+v", got)
	}
}

func TestSelectedBrowserRegistrationDoesNotWriteOtherBrowsers(t *testing.T) {
	helper, targets := registrationFixture(t)
	paths, err := registerBrowserTargets(helper, targets, "firefox")
	if err != nil || len(paths) != 1 {
		t.Fatalf("selected registration failed: %v, %v", paths, err)
	}
	for _, item := range inspectRegistrations(helper, targets) {
		if item.Registered != (item.ID == "firefox") {
			t.Fatalf("selected registration touched another browser: %+v", item)
		}
	}
	if _, err := registerBrowserTargets(helper, targets, "unsupported"); err == nil {
		t.Fatal("unsupported browser was accepted")
	}
	if _, err := registerBrowserTargets(helper, nil, "chrome"); err == nil {
		t.Fatal("uninstalled browser was accepted")
	}
	if paths, err := registerBrowserTargets(helper, nil, ""); err != nil || len(paths) != 0 {
		t.Fatalf("no-browser repair should not create registrations: %v, %v", paths, err)
	}
}

func TestRegistrationPreservesEquivalentValidManifest(t *testing.T) {
	helper, targets := registrationFixture(t)
	targets = targets[:1]
	if _, err := registerTargets(helper, targets); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(targets[0].directory, Name+".json")
	compact, _ := json.Marshal(manifestFor(helper, targets[0]))
	if err := os.WriteFile(path, compact, 0600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Unix(42, 0)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if _, err := registerTargets(helper, targets); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.ModTime().Equal(oldTime) {
		t.Fatalf("valid manifest was rewritten: %v", err)
	}
}
