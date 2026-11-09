package nativehost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/knothhe/passpine/client/buildinfo"
	"github.com/knothhe/passpine/client/i18n"
)

// BrowserConfiguration is local to this device and never synced or returned to
// browser extensions. Directory is a native-messaging directory, not a profile.
type BrowserConfiguration struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Path           string `json:"path"`
	Kind           string `json:"kind"`
	Directory      string `json:"directory,omitempty"`
	RegistryPrefix string `json:"registryPrefix,omitempty"`
}

const browserConfigurationFile = "browsers.json"

type browserMetadata struct{ name, bundleID, productDirectory string }

func browserConfigurationID(c BrowserConfiguration) string {
	destination := filepath.Clean(c.Directory)
	if runtime.GOOS == "windows" {
		destination = strings.ToLower(c.RegistryPrefix)
	}
	sum := sha256.Sum256([]byte(destination))
	return "custom-" + hex.EncodeToString(sum[:16])
}

func validBrowserConfiguration(c BrowserConfiguration) bool {
	if c.Name == "" || len(c.Name) > 200 || strings.ContainsAny(c.Name, "\x00\r\n") || !filepath.IsAbs(c.Path) || strings.ContainsRune(c.Path, 0) || (c.Kind != "chromium" && c.Kind != "firefox") {
		return false
	}
	if runtime.GOOS == "windows" {
		local, err := os.UserCacheDir()
		if err != nil || !filepath.IsAbs(local) {
			return false
		}
		parts := strings.Split(c.RegistryPrefix, `\`)
		if len(parts) < 2 || !strings.EqualFold(parts[0], "Software") {
			return false
		}
		for _, part := range parts {
			if part == "" || part != strings.TrimSpace(part) || part == "." || part == ".." || strings.ContainsAny(part, "/\x00\r\n") || strings.EqualFold(part, "NativeMessagingHosts") {
				return false
			}
		}
	} else if !filepath.IsAbs(c.Directory) || strings.ContainsRune(c.Directory, 0) || (filepath.Base(c.Directory) != "NativeMessagingHosts" && filepath.Base(c.Directory) != "native-messaging-hosts") {
		return false
	}
	return c.ID == browserConfigurationID(c)
}

func LoadBrowserConfigurations(dataDir string) ([]BrowserConfiguration, error) {
	file := filepath.Join(dataDir, browserConfigurationFile)
	info, err := os.Lstat(file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return nil, errors.New(i18n.BrowserConfigurationUnreadable)
	}
	data, err := os.ReadFile(file)
	var configurations []BrowserConfiguration
	if err != nil || json.Unmarshal(data, &configurations) != nil || len(configurations) > 32 {
		return nil, errors.New(i18n.BrowserConfigurationUnreadable)
	}
	seen := map[string]bool{}
	for _, c := range configurations {
		if !validBrowserConfiguration(c) || seen[c.ID] {
			return nil, errors.New(i18n.BrowserConfigurationUnreadable)
		}
		seen[c.ID] = true
	}
	return configurations, nil
}

func saveBrowserConfigurations(dataDir string, configurations []BrowserConfiguration) error {
	data, err := json.MarshalIndent(configurations, "", "  ")
	if err != nil || len(data) > 64<<10 || len(configurations) > 32 {
		return errors.New(i18n.BrowserConfigurationSaveFailed)
	}
	if err := writePrivate(filepath.Join(dataDir, browserConfigurationFile), data); err != nil {
		return errors.New(i18n.BrowserConfigurationSaveFailed)
	}
	return nil
}

func NormalizeBrowserConfiguration(c BrowserConfiguration) (BrowserConfiguration, error) {
	c.Name = strings.TrimSpace(c.Name)
	if !filepath.IsAbs(c.Path) {
		return c, errors.New(i18n.BrowserPathInvalid)
	}
	c.Path = filepath.Clean(c.Path)
	if runtime.GOOS == "darwin" {
		metadata, err := browserApplicationMetadata(c.Path)
		if err != nil {
			return c, errors.New(i18n.BrowserPathInvalid)
		}
		if c.Name == "" {
			c.Name = metadata.name
		}
		if c.Directory == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return c, err
			}
			if c.Kind == "chromium" && metadata.productDirectory != "" {
				product := filepath.Clean(metadata.productDirectory)
				if filepath.IsAbs(product) || product == ".." || strings.HasPrefix(product, ".."+string(filepath.Separator)) {
					return c, errors.New(i18n.BrowserDestinationInvalid)
				}
				c.Directory = filepath.Join(home, "Library", "Application Support", product, "NativeMessagingHosts")
			} else if c.Kind == "firefox" && metadata.bundleID == "org.mozilla.firefox" {
				c.Directory = filepath.Join(home, "Library", "Application Support", "Mozilla", "NativeMessagingHosts")
			}
		}
	} else if !HelperAvailable(c.Path) || runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(c.Path), ".exe") {
		return c, errors.New(i18n.BrowserPathInvalid)
	}
	if c.Name == "" {
		c.Name = strings.TrimSuffix(filepath.Base(c.Path), filepath.Ext(c.Path))
	}
	if c.Directory != "" {
		c.Directory = filepath.Clean(c.Directory)
		if runtime.GOOS != "windows" && filepath.IsAbs(c.Directory) {
			// Store the actual location even when the user selected an alias.
			// Cleanup verifies every parent and must not follow a changed symlink.
			resolved, err := resolveBrowserDirectory(c.Directory)
			if err != nil {
				return c, errors.New(i18n.BrowserDestinationInvalid)
			}
			c.Directory = resolved
		}
	}
	if runtime.GOOS == "windows" {
		c.Directory = ""
	} else {
		c.RegistryPrefix = ""
	}
	c.ID = browserConfigurationID(c)
	if !validBrowserConfiguration(c) {
		return c, errors.New(i18n.BrowserDestinationInvalid)
	}
	return c, nil
}

// Saving only records the configuration. Registration remains an explicit action.
func SaveBrowserConfiguration(dataDir string, c BrowserConfiguration) error {
	previousID := c.ID
	configurations, err := LoadBrowserConfigurations(dataDir)
	if err != nil {
		return err
	}
	c, err = NormalizeBrowserConfiguration(c)
	if err != nil {
		return err
	}
	if previousID != "" && previousID != c.ID {
		return errors.New(i18n.BrowserDestinationInvalid)
	}
	for i, previous := range configurations {
		if previous.ID == c.ID {
			if previous.Kind != c.Kind {
				return errors.New(i18n.BrowserDestinationInvalid)
			}
			configurations[i] = c
			return saveBrowserConfigurations(dataDir, configurations)
		}
	}
	// The same directory cannot simultaneously hold Chromium and Firefox manifests.
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	for _, target := range platformRegistrationTargets(home) {
		if sameBrowserDestination(target, configurationTarget(c)) && target.firefox != (c.Kind == "firefox") {
			return errors.New(i18n.BrowserDestinationInvalid)
		}
	}
	return saveBrowserConfigurations(dataDir, append(configurations, c))
}

func RemoveBrowserConfiguration(dataDir, helper, id string) error {
	return removeBrowserConfiguration(dataDir, helper, id, unregisterPlatformHost)
}

func removeBrowserConfiguration(dataDir, helper, id string, detach func(registrationTarget, string) error) error {
	configurations, err := LoadBrowserConfigurations(dataDir)
	if err != nil {
		return err
	}
	for i, c := range configurations {
		if c.ID != id {
			continue
		}
		if err := unregisterTargets(helper, []registrationTarget{configurationTarget(c)}, detach); err != nil {
			return errors.New(i18n.BrowserConfigurationRemoveFailed)
		}
		return saveBrowserConfigurations(dataDir, append(configurations[:i], configurations[i+1:]...))
	}
	return errors.New(i18n.BrowserConfigurationMissing)
}

func configurationTarget(c BrowserConfiguration) registrationTarget {
	directory := c.Directory
	if runtime.GOOS == "windows" {
		local, _ := os.UserCacheDir()
		directory = filepath.Join(local, buildinfo.Name(), "NativeMessagingHosts", c.ID)
		// Reuse the built-in manifest location when a manual path overrides its
		// detection, so updating the registry never strands the earlier manifest.
		home, _ := os.UserHomeDir()
		for _, target := range platformRegistrationTargets(home) {
			if strings.EqualFold(targetRegistryPath(target), c.RegistryPrefix+`\NativeMessagingHosts\`+Name) {
				directory = target.directory
				break
			}
		}
		if resolved, err := resolveBrowserDirectory(directory); err == nil {
			directory = resolved
		}
	}
	return registrationTarget{id: c.ID, name: c.Name, directory: directory, firefox: c.Kind == "firefox", configuration: &c}
}

func sameBrowserDestination(a, b registrationTarget) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(targetRegistryPath(a), targetRegistryPath(b))
	}
	return filepath.Clean(a.directory) == filepath.Clean(b.directory)
}

func configuredTargets(targets []registrationTarget, configurations []BrowserConfiguration) []registrationTarget {
	for _, c := range configurations {
		target := configurationTarget(c)
		replaced := false
		for i, previous := range targets {
			if sameBrowserDestination(previous, target) {
				targets[i], replaced = target, true
				break
			}
		}
		if !replaced {
			targets = append(targets, target)
		}
	}
	return targets
}

func configuredBrowserInstalled(c BrowserConfiguration) bool {
	if runtime.GOOS == "darwin" {
		_, err := browserApplicationMetadata(c.Path)
		return err == nil
	}
	return HelperAvailable(c.Path) && (runtime.GOOS != "windows" || strings.EqualFold(filepath.Ext(c.Path), ".exe"))
}

func resolveBrowserDirectory(directory string) (string, error) {
	if _, err := os.Lstat(directory); err == nil {
		resolved, err := filepath.EvalSymlinks(directory)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", errors.New("not a directory")
		}
		return resolved, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(directory)
	if parent == directory {
		return "", os.ErrNotExist
	}
	resolved, err := resolveBrowserDirectory(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(directory)), nil
}

func defaultBrowserConfigurations() ([]BrowserConfiguration, error) {
	directory, err := DataDir()
	if err != nil {
		return nil, err
	}
	return LoadBrowserConfigurations(directory)
}
