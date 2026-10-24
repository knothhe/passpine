package vault

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestCreateFolderCreatesParentsAndListsEmptyFolders(t *testing.T) {
	v := newFixture(t)
	for _, folder := range []string{"test/a/b/c", "test/a/b/c", "test/有 空格/child"} {
		if err := v.CreateFolder(folder); err != nil {
			t.Fatal(err)
		}
	}
	entries, folders, err := v.ListMetadataWithFolders()
	want := []string{"test", "test/a", "test/a/b", "test/a/b/c", "test/有 空格", "test/有 空格/child"}
	if err != nil || len(entries) != 0 || !slices.Equal(folders, want) {
		t.Fatalf("empty folder metadata: entries=%v folders=%v err=%v", entries, folders, err)
	}
	for _, folder := range want {
		info, err := os.Stat(filepath.Join(v.config.Directory, filepath.FromSlash(folder)))
		if err != nil || !info.IsDir() {
			t.Fatalf("folder %q was not created: %v", folder, err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
			t.Fatalf("folder %q permissions = %v", folder, info.Mode())
		}
	}
	children, err := os.ReadDir(filepath.Join(v.config.Directory, "test/a/b/c"))
	if err != nil || len(children) != 0 {
		t.Fatalf("creating a folder should not add marker or policy files: %v %v", children, err)
	}
	doc := mustSave(t, v, "source", "folder-test-secret", "")
	if err := v.Move(doc.Path, "test/a/b/c/source", doc.Revision); err != nil {
		t.Fatal(err)
	}
	if moved, err := v.Read("test/a/b/c/source"); err != nil || moved.Raw != doc.Raw {
		t.Fatalf("moving into the new folder changed the entry: %v", err)
	}
}

func TestCreateFolderRejectsInvalidLockedAndBlockedPaths(t *testing.T) {
	v := newFixture(t)
	for _, folder := range []string{"", "/absolute", "rejected/../escape", "rejected//child", "rejected/.git/child", `rejected\child`, "rejected/CON/child", "rejected/child/"} {
		if err := v.CreateFolder(folder); !errors.Is(err, ErrPath) {
			t.Fatalf("accepted invalid folder %q: %v", folder, err)
		}
	}
	if _, err := os.Stat(filepath.Join(v.config.Directory, "rejected")); !os.IsNotExist(err) {
		t.Fatal("invalid path created a parent directory", err)
	}
	blocked := filepath.Join(v.config.Directory, "blocked")
	if err := os.WriteFile(blocked, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := v.CreateFolder("blocked/child"); err == nil {
		t.Fatal("created a directory through a regular file")
	}
	if data, err := os.ReadFile(blocked); err != nil || string(data) != "keep" {
		t.Fatal("failed creation changed the existing file", err)
	}
	v.Lock()
	if err := v.CreateFolder("locked/child"); !errors.Is(err, ErrLocked) {
		t.Fatal("locked vault accepted folder creation", err)
	}
	if _, err := os.Stat(filepath.Join(v.config.Directory, "locked")); !os.IsNotExist(err) {
		t.Fatal("locked creation wrote a directory", err)
	}
}

func TestCleanupEmptyFoldersAfterMovingEntryToParent(t *testing.T) {
	v := newFixture(t)
	doc := mustSave(t, v, "test/a/b/c/d", "keep-this-secret", "")
	if err := v.Move(doc.Path, "test/a/b/d", doc.Revision); err != nil {
		t.Fatal(err)
	}
	removed, err := v.CleanupEmptyFolders()
	if err != nil || removed != 1 {
		t.Fatalf("cleanup should remove only c: removed=%d err=%v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(v.config.Directory, "test/a/b/c")); !os.IsNotExist(err) {
		t.Fatal("empty source folder remains", err)
	}
	if moved, err := v.Read("test/a/b/d"); err != nil || moved.Raw != doc.Raw {
		t.Fatal("cleanup changed the moved entry", err)
	}
}

func TestCleanupEmptyFoldersPreservesFilesPoliciesAndHiddenDirectories(t *testing.T) {
	v := newFixture(t)
	for _, folder := range []string{"empty/a/b", "policy/unused", "hidden/unused", "notes"} {
		if err := v.CreateFolder(folder); err != nil {
			t.Fatal(err)
		}
	}
	for _, folder := range []string{".git/empty", ".private/empty"} {
		if err := os.MkdirAll(filepath.Join(v.config.Directory, filepath.FromSlash(folder)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{"policy/.gpg-id": "keep-recipient", "hidden/.keep": "keep-hidden-file", "notes/README": "keep-note"}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(v.config.Directory, filepath.FromSlash(name)), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if removed, err := v.CleanupEmptyFolders(); err != nil || removed != 5 {
		t.Fatalf("cleanup should remove the empty chain and two empty children: %d %v", removed, err)
	}
	for name, want := range files {
		if data, err := os.ReadFile(filepath.Join(v.config.Directory, filepath.FromSlash(name))); err != nil || string(data) != want {
			t.Fatal("cleanup changed a file", name, err)
		}
	}
	for _, folder := range []string{".git/empty", ".private/empty"} {
		if info, err := os.Stat(filepath.Join(v.config.Directory, filepath.FromSlash(folder))); err != nil || !info.IsDir() {
			t.Fatal("cleanup removed a hidden directory", folder, err)
		}
	}
	if removed, err := v.CleanupEmptyFolders(); err != nil || removed != 0 {
		t.Fatalf("repeated cleanup should be a no-op: %d %v", removed, err)
	}
}

func TestCleanupEmptyFoldersRequiresUnlockedSessionAndStoreLock(t *testing.T) {
	v := newFixture(t)
	if err := v.CreateFolder("keep/empty"); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockStore(v.config.Directory)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := v.CleanupEmptyFolders()
	unlock()
	if removed != 0 || !errors.Is(err, ErrBusy) {
		t.Fatalf("cleanup ignored another writer: %d %v", removed, err)
	}
	v.Lock()
	if removed, err := v.CleanupEmptyFolders(); removed != 0 || !errors.Is(err, ErrLocked) {
		t.Fatalf("locked cleanup was accepted: %d %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(v.config.Directory, "keep/empty")); err != nil {
		t.Fatal("rejected cleanup removed a directory", err)
	}
}

func TestEmptyFolderRemovalPreservesNewFilesAndSkipsMissingFolders(t *testing.T) {
	root := t.TempDir()
	if err := safeMkdirAll(root, "raced"); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "raced", "new-file")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if removed, err := safeRemoveEmptyFolder(root, "raced"); removed || err != nil {
		t.Fatalf("nonempty folder must be skipped: %v %v", removed, err)
	}
	if removed, err := safeRemoveEmptyFolder(root, "raced/new-file"); removed || err == nil {
		t.Fatalf("a regular file must never be removed: %v %v", removed, err)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "keep" {
		t.Fatal("concurrent file was lost", err)
	}
	if removed, err := safeRemoveEmptyFolder(root, "missing/child"); removed || err != nil {
		t.Fatalf("already removed folder should be skipped: %v %v", removed, err)
	}
}
