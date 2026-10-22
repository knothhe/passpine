package nativehost

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/knothhe/passpine/client/buildinfo"
)

// BrowserRegistration reports native messaging registration, not whether the
// browser extension itself has been installed or enabled by the user.
type BrowserRegistration struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Registered   bool   `json:"registered"`
	Status       string `json:"status"`
	ExtensionID  string `json:"extensionId"`
	ManifestPath string `json:"manifestPath"`
	HelperPath   string `json:"helperPath"`
}

type registrationTarget struct {
	id, name, directory string
	firefox             bool
}

// Browser support directories are not installation evidence: registration
// itself creates them, and browsers can leave them behind after uninstalling.
func installedTargets(targets []registrationTarget, installed func(registrationTarget) bool) []registrationTarget {
	var detected []registrationTarget
	for _, target := range targets {
		if installed(target) {
			detected = append(detected, target)
		}
	}
	return detected
}

type hostManifest struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Path              string   `json:"path"`
	Type              string   `json:"type"`
	AllowedOrigins    []string `json:"allowed_origins,omitempty"`
	AllowedExtensions []string `json:"allowed_extensions,omitempty"`
}

func registrationTargets(home, platform string) []registrationTarget {
	var targets []registrationTarget
	switch platform {
	case "windows":
		targets = []registrationTarget{
			{"chrome", "Chrome", "chrome", false},
			{"edge", "Microsoft Edge", "edge", false},
			{"brave", "Brave", "brave", false},
			{"firefox", "Firefox", "firefox", true},
		}
		home = filepath.Join(home, "AppData", "Local", buildinfo.Name(), "NativeMessagingHosts")
	case "darwin":
		targets = []registrationTarget{
			{"chrome", "Chrome", "Google/Chrome/NativeMessagingHosts", false},
			{"chrome-beta", "Chrome Beta", "Google/Chrome Beta/NativeMessagingHosts", false},
			{"chrome-testing", "Chrome for Testing", "Google/ChromeForTesting/NativeMessagingHosts", false},
			{"chromium", "Chromium", "Chromium/NativeMessagingHosts", false},
			{"edge", "Microsoft Edge", "Microsoft Edge/NativeMessagingHosts", false},
			{"brave", "Brave", "BraveSoftware/Brave-Browser/NativeMessagingHosts", false},
			{"firefox", "Firefox", "Mozilla/NativeMessagingHosts", true},
		}
		home = filepath.Join(home, "Library", "Application Support")
	case "linux":
		targets = []registrationTarget{
			{"chrome", "Chrome", ".config/google-chrome/NativeMessagingHosts", false},
			{"chromium", "Chromium", ".config/chromium/NativeMessagingHosts", false},
			{"edge", "Microsoft Edge", ".config/microsoft-edge/NativeMessagingHosts", false},
			{"brave", "Brave", ".config/BraveSoftware/Brave-Browser/NativeMessagingHosts", false},
			{"firefox", "Firefox", ".mozilla/native-messaging-hosts", true},
		}
	}
	for i := range targets {
		targets[i].directory = filepath.Join(home, targets[i].directory)
	}
	return targets
}

func manifestFor(executable string, target registrationTarget) hostManifest {
	m := hostManifest{Name: Name, Description: buildinfo.Name(), Path: executable, Type: "stdio"}
	if target.firefox {
		m.AllowedExtensions = []string{FirefoxID}
	} else {
		m.AllowedOrigins = []string{"chrome-extension://" + ChromiumID + "/"}
	}
	return m
}

func HelperAvailable(executable string) bool {
	if executable == "" || !filepath.IsAbs(executable) {
		return false
	}
	info, err := os.Stat(executable)
	return err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm()&0111 != 0)
}

// Registrations only reads manifests and executable metadata. A status check
// must never create browser directories or silently repair a broken manifest.
func Registrations(executable string) []BrowserRegistration {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	path, err := filepath.Abs(executable)
	if err != nil || executable == "" {
		path = ""
	}
	items := inspectRegistrations(path, installedTargets(platformRegistrationTargets(home), browserInstalled))
	for i := range items {
		if items[i].Registered && !platformRegistrationMatches(items[i]) {
			items[i].Registered, items[i].Status = false, "missing"
		}
	}
	return items
}

func inspectRegistrations(executable string, targets []registrationTarget) []BrowserRegistration {
	available := HelperAvailable(executable)
	registrations := make([]BrowserRegistration, 0, len(targets))
	for _, target := range targets {
		item := BrowserRegistration{ID: target.id, Name: target.name, ExtensionID: ChromiumID, ManifestPath: filepath.Join(target.directory, Name+".json"), HelperPath: executable, Status: "missing"}
		if target.firefox {
			item.ExtensionID = FirefoxID
		}
		info, err := os.Lstat(item.ManifestPath)
		if err == nil {
			item.Status = "invalid"
			if info.Mode().IsRegular() && info.Size() <= 64<<10 {
				if b, err := os.ReadFile(item.ManifestPath); err == nil && manifestMatches(b, manifestFor(executable, target)) {
					item.Status = "helper_missing"
					if available {
						item.Status, item.Registered = "registered", true
					}
				}
			}
		} else if !os.IsNotExist(err) {
			item.Status = "invalid"
		}
		registrations = append(registrations, item)
	}
	return registrations
}

func manifestMatches(data []byte, expected hostManifest) bool {
	var actual hostManifest
	if json.Unmarshal(data, &actual) != nil || actual.Name != expected.Name || actual.Path != expected.Path || actual.Type != expected.Type {
		return false
	}
	return sameStrings(actual.AllowedOrigins, expected.AllowedOrigins) && sameStrings(actual.AllowedExtensions, expected.AllowedExtensions)
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func Register(executable string) ([]string, error) {
	return RegisterBrowser(executable, "")
}

// RegisterBrowser registers only the selected installed browser. An empty ID
// redetects and repairs installed browsers, matching the previous client.
func RegisterBrowser(executable, browserID string) ([]string, error) {
	path, err := filepath.Abs(executable)
	if err != nil {
		return nil, err
	}
	if !HelperAvailable(path) {
		return nil, errors.New("native helper executable is missing or not executable")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	targets := platformRegistrationTargets(home)
	if len(targets) == 0 {
		return nil, errors.New("automatic host registration is unsupported on this platform")
	}
	targets = installedTargets(targets, browserInstalled)
	paths, err := registerBrowserTargets(path, targets, browserID)
	if err != nil {
		return paths, err
	}
	for _, target := range targets {
		if browserID == "" || target.id == browserID {
			if err := registerPlatformHost(target); err != nil {
				return paths, err
			}
		}
	}
	return paths, nil
}

func registerBrowserTargets(executable string, targets []registrationTarget, browserID string) ([]string, error) {
	if browserID == "" {
		return registerTargets(executable, targets)
	}
	for _, target := range targets {
		if target.id == browserID {
			return registerTargets(executable, []registrationTarget{target})
		}
	}
	return nil, errors.New("selected browser is not installed or supported")
}

func registerTargets(executable string, targets []registrationTarget) ([]string, error) {
	installed := []string{}
	var failures []error
	for i, status := range inspectRegistrations(executable, targets) {
		if status.Registered {
			installed = append(installed, status.ManifestPath)
			continue
		}
		target := targets[i]
		b, _ := json.MarshalIndent(manifestFor(executable, target), "", "  ")
		if err := os.MkdirAll(target.directory, 0700); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", target.name, err))
			continue
		}
		if err := writePrivate(status.ManifestPath, b); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", target.name, err))
			continue
		}
		installed = append(installed, status.ManifestPath)
	}
	return installed, errors.Join(failures...)
}

// CheckRegistration is retained for callers needing a single summary flag.
func CheckRegistration(executable string) bool {
	for _, browser := range Registrations(executable) {
		if browser.Registered {
			return true
		}
	}
	return false
}
