package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// DisplayDirectory identifies a vault by its directory without a long home prefix.
func DisplayDirectory(directory string) string {
	directory = filepath.Clean(directory)
	if home, err := os.UserHomeDir(); err == nil {
		if relative, err := filepath.Rel(home, directory); err == nil {
			if relative == "." {
				return "~"
			}
			if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return "~" + string(filepath.Separator) + relative
			}
		}
	}
	return directory
}

// Automatic directory labels are useful on the desktop, but browser metadata
// must not expose the vault's local path. Explicit user labels stay intact.
func browserVaultName(data Data) string {
	for _, info := range data.Vaults {
		if info.ID != data.VaultID {
			continue
		}
		// Older preferences have no customization flag. Preserve their named
		// vaults unless the label is the generated directory representation.
		if info.NameCustomized || info.Name != DisplayDirectory(info.Directory) {
			return data.VaultName
		}
		name := filepath.Base(info.Directory)
		if name == "." || name == "/" || name == "\\" {
			return "Passpine"
		}
		return name
	}
	return data.VaultName
}

func (s *Service) migrateVaultNames() bool {
	changed := false
	for i := range s.prefs.Vaults {
		info := &s.prefs.Vaults[i]
		// The original onboarding filled this label automatically for imports.
		// Explicit names saved by the current client have provenance and are
		// preserved even when they match that former placeholder exactly.
		if !info.NameCustomized && (info.Name == "我的密码库" || info.Name == "") {
			info.Name = DisplayDirectory(info.Directory)
			changed = true
		}
	}
	return changed
}

func (s *Service) renameVault(id, name string) error {
	name = strings.TrimSpace(name)
	if len([]rune(name)) > 200 || strings.ContainsFunc(name, unicode.IsControl) {
		return errors.New("密码库名称不能超过 200 字或包含控制字符")
	}
	if id == "" {
		id = s.prefs.Current
	}
	for i := range s.prefs.Vaults {
		if s.prefs.Vaults[i].ID == id {
			previous := s.prefs.Vaults[i]
			customized := name != ""
			if !customized {
				name = DisplayDirectory(previous.Directory)
			}
			s.prefs.Vaults[i].Name = name
			s.prefs.Vaults[i].NameCustomized = customized
			if err := s.savePreferences(); err != nil {
				s.prefs.Vaults[i] = previous
				return err
			}
			return nil
		}
	}
	return errors.New("unknown vault")
}
