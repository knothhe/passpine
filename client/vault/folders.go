package vault

// CreateFolder creates a relative folder and any missing parents without
// following symlinks. Existing directories are reused, and no marker files or
// recipient policies are added to the pass store.
func (v *Vault) CreateFolder(name string) error {
	if err := ValidatePath(name); err != nil {
		return err
	}
	v.operations.Lock()
	defer v.operations.Unlock()
	_, token, err := v.token()
	if err != nil {
		return err
	}
	unlock, err := lockStore(v.config.Directory)
	if err != nil {
		return err
	}
	defer unlock()
	return v.commit(token, func() error { return safeMkdirAll(v.config.Directory, name) })
}

// CleanupEmptyFolders removes visible, empty directories from deepest to
// shallowest. Hidden directories, symlinks, files and the vault root are kept.
// A directory containing any file (including .gpg-id) is never empty.
func (v *Vault) CleanupEmptyFolders() (int, error) {
	v.operations.Lock()
	defer v.operations.Unlock()
	_, token, err := v.token()
	if err != nil {
		return 0, err
	}
	unlock, err := lockStore(v.config.Directory)
	if err != nil {
		return 0, err
	}
	defer unlock()
	_, folders, err := v.listMetadataWithFolders()
	if err != nil {
		return 0, err
	}
	removed := 0
	// WalkDir visits parents before children, so reversing the snapshot also
	// cleans up parents made empty by removing their last empty descendant.
	for i := len(folders) - 1; i >= 0; i-- {
		err = v.commit(token, func() error {
			deleted, err := safeRemoveEmptyFolder(v.config.Directory, folders[i])
			if deleted {
				removed++
			}
			return err
		})
		if err != nil {
			return removed, err
		}
	}
	return removed, nil
}
