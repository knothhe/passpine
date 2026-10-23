package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/knothhe/passpine/client/vault"
)

// CLIVault is the vault selected for one command-line invocation. It reuses the
// desktop client's key directory, so both share one protected private key.
type CLIVault struct {
	Vault      *vault.Vault
	Info       VaultInfo
	Settings   Settings
	DeviceFile string
}

// CLIPreferences reads the desktop client's preferences without starting the
// application service, its browser endpoint or background Git work.
func CLIPreferences(dataDir string) (Settings, string, []VaultInfo, error) {
	prefs := preferences{Vaults: []VaultInfo{}, Settings: DefaultSettings()}
	b, err := readPrivate(filepath.Join(dataDir, "preferences.json"), 1<<20)
	if err != nil && !os.IsNotExist(err) {
		return prefs.Settings, "", nil, err
	}
	if err == nil {
		if err = json.Unmarshal(b, &prefs); err != nil {
			return DefaultSettings(), "", nil, fmt.Errorf("settings are invalid: %w", err)
		}
	}
	if prefs.Settings.ThemeStyle == "" {
		prefs.Settings.ThemeStyle = defaultThemeStyle()
	}
	prefs.Settings.ThemeStyle = NormalizeThemeStyle(prefs.Settings.ThemeStyle)
	normalizeLifecycleSettings(&prefs.Settings, DefaultSettings())
	normalizeTypographySettings(&prefs.Settings, DefaultSettings())
	if shortcut, err := NormalizeGlobalShortcut(prefs.Settings.GlobalShortcut); err == nil {
		prefs.Settings.GlobalShortcut = shortcut
	}
	if validateSettings(prefs.Settings) != nil {
		prefs.Settings = DefaultSettings()
	}
	return prefs.Settings, prefs.Current, prefs.Vaults, nil
}

// OpenCLIVault selects directory when it is set, like PASSWORD_STORE_DIR, and
// otherwise the vault currently selected in the desktop client.
func OpenCLIVault(dataDir, directory string) (*CLIVault, error) {
	settings, current, vaults, err := CLIPreferences(dataDir)
	if err != nil {
		return nil, err
	}
	var info VaultInfo
	if directory != "" {
		dir, err := filepath.Abs(directory)
		if err != nil {
			return nil, err
		}
		info = VaultInfo{ID: idFor(dir), Name: DisplayDirectory(dir), Directory: dir}
		for _, v := range vaults {
			if v.ID == info.ID || sameDirectory(v.Directory, dir) {
				info = v
				break
			}
		}
	} else {
		for _, v := range vaults {
			if v.ID == current {
				info = v
				break
			}
		}
		if info.ID == "" {
			return nil, vault.ErrNotConfigured
		}
	}
	keyDir := filepath.Join(dataDir, "keys", idFor(info.Directory))
	v, err := vault.New(vault.Config{Directory: info.Directory, KeyFile: filepath.Join(keyDir, "secret.pgp"), RecoveryDirectory: filepath.Join(keyDir, "recovery"), IdleTimeout: time.Duration(settings.IdleMinutes) * time.Minute, MaximumSession: time.Duration(settings.MaxSessionMinutes) * time.Minute})
	if err != nil {
		return nil, err
	}
	out := &CLIVault{Vault: v, Info: info, Settings: settings}
	if settings.DeviceUnlock {
		file := filepath.Join(keyDir, "device-unlock.json")
		if _, err := readPrivate(file, 1<<20); err == nil {
			out.DeviceFile = file
		}
	}
	return out, nil
}

func sameDirectory(a, b string) bool {
	left, errA := filepath.EvalSymlinks(a)
	right, errB := filepath.EvalSymlinks(b)
	return errors.Join(errA, errB) == nil && left == right
}
