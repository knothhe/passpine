package vault

import (
	"errors"
	"sort"
	"strings"
)

// CreateFolder creates a relative folder and any missing parents without
// following symlinks. Existing directories are reused, and no marker files or
// recipient policies are added to the pass store. Like cleanup, this
// metadata-only operation needs no decrypted keys or unlocked session.
func (v *Vault) CreateFolder(name string) error {
	if err := ValidatePath(name); err != nil {
		return err
	}
	v.operations.Lock()
	defer v.operations.Unlock()
	unlock, err := lockStore(v.config.Directory)
	if err != nil {
		return err
	}
	defer unlock()
	return safeMkdirAll(v.config.Directory, name)
}

// CleanupEmptyFolders removes visible, empty directories from deepest to
// shallowest. Hidden directories, symlinks, files and the vault root are kept.
// A directory containing any file (including .gpg-id) is never empty.
// This metadata-only operation needs no decrypted keys or unlocked session.
func (v *Vault) CleanupEmptyFolders() (int, error) {
	removed, err := v.CleanupEmptyFoldersWithPaths()
	return len(removed), err
}

// CleanupEmptyFoldersWithPaths also returns the directories actually removed,
// including completed work before an error. Callers must keep these paths local
// rather than exposing them when browsing a locked vault is disabled.
func (v *Vault) CleanupEmptyFoldersWithPaths() ([]string, error) {
	v.operations.Lock()
	defer v.operations.Unlock()
	unlock, err := lockStore(v.config.Directory)
	if err != nil {
		return nil, err
	}
	defer unlock()
	_, folders, err := v.listMetadataWithFolders()
	if err != nil {
		return nil, err
	}
	var removed []string
	// WalkDir visits parents before children, so reversing the snapshot also
	// cleans up parents made empty by removing their last empty descendant.
	for i := len(folders) - 1; i >= 0; i-- {
		deleted, err := safeRemoveEmptyFolder(v.config.Directory, folders[i])
		if deleted {
			removed = append(removed, folders[i])
		}
		if err != nil {
			return removed, err
		}
	}
	return removed, nil
}

// RestoreEmptyFolders recreates only the recorded directories, shallowest
// first. It never creates an unrecorded parent or replaces existing files or
// links. Existing directories are already restored. Failed paths are returned
// so a retry does not replay successful work. No unlocked session is needed.
func (v *Vault) RestoreEmptyFolders(folders []string) (restored int, remaining []string, err error) {
	seen := make(map[string]bool, len(folders))
	for _, folder := range folders {
		if err := ValidatePath(folder); err != nil {
			return 0, append([]string(nil), folders...), err
		}
		if !seen[folder] {
			seen[folder] = true
			remaining = append(remaining, folder)
		}
	}
	sort.Slice(remaining, func(i, j int) bool {
		if a, b := strings.Count(remaining[i], "/"), strings.Count(remaining[j], "/"); a != b {
			return a < b
		}
		return remaining[i] < remaining[j]
	})
	v.operations.Lock()
	defer v.operations.Unlock()
	unlock, err := lockStore(v.config.Directory)
	if err != nil {
		return 0, remaining, err
	}
	defer unlock()
	pending := make([]string, 0, len(remaining))
	for _, folder := range remaining {
		if restoreErr := safeRestoreEmptyFolder(v.config.Directory, folder); restoreErr != nil {
			pending = append(pending, folder)
			err = errors.Join(err, restoreErr)
		} else {
			restored++
		}
	}
	return restored, pending, err
}
