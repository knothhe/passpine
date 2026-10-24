//go:build !windows

package vault

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCreateFolderNeverFollowsSymlinks(t *testing.T) {
	v := newFixture(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(v.config.Directory, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := v.CreateFolder("linked/child"); err == nil {
		t.Fatal("created a folder through a symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "child")); !os.IsNotExist(err) {
		t.Fatal("creation escaped the vault", err)
	}
	_, folders, err := v.ListMetadataWithFolders()
	if err != nil || slices.Contains(folders, "linked") {
		t.Fatal("folder listing exposed a symlink", folders, err)
	}
}

func TestCleanupEmptyFoldersPreservesSymlinksAndTheirTargets(t *testing.T) {
	v := newFixture(t)
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(outside, "keep-empty"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(v.config.Directory, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if removed, err := v.CleanupEmptyFolders(); err != nil || removed != 0 {
		t.Fatal("cleanup touched a symlink", removed, err)
	}
	if deleted, err := safeRemoveEmptyFolder(v.config.Directory, "linked"); deleted || !errors.Is(err, ErrPath) {
		t.Fatal("directory removal accepted a replacement symlink", deleted, err)
	}
	if deleted, err := safeRemoveEmptyFolder(v.config.Directory, "linked/keep-empty"); deleted || !errors.Is(err, ErrPath) {
		t.Fatal("directory removal followed a parent symlink", deleted, err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("cleanup removed the link", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep-empty")); err != nil {
		t.Fatal("cleanup escaped the vault", err)
	}
}

func TestCleanupEmptyFoldersReportsPartialPermissionFailure(t *testing.T) {
	v := newFixture(t)
	for _, name := range []string{"a-blocked/child", "z-empty"} {
		if err := v.CreateFolder(name); err != nil {
			t.Fatal(err)
		}
	}
	blocked := filepath.Join(v.config.Directory, "a-blocked")
	if err := os.Chmod(blocked, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(blocked, 0700) })
	removed, err := v.CleanupEmptyFolders()
	if !errors.Is(err, os.ErrPermission) || removed != 1 {
		t.Fatalf("permission failure did not report completed work: %d %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(blocked, "child")); err != nil {
		t.Fatal("failed cleanup changed the protected directory", err)
	}
}
