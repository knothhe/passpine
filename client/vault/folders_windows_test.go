package vault

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestRestoreEmptyFoldersRejectsWindowsJunctions(t *testing.T) {
	v := newFixture(t)
	outside := t.TempDir()
	link := filepath.Join(v.config.Directory, "linked")
	if output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
		t.Fatalf("create test junction: %s %v", output, err)
	}
	if restored, pending, err := v.RestoreEmptyFolders([]string{"linked", "linked/child", "safe"}); err == nil || restored != 1 || !slices.Equal(pending, []string{"linked", "linked/child"}) {
		t.Fatal("restore accepted a junction or lost independent work", restored, pending, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "child")); !os.IsNotExist(err) {
		t.Fatal("restore escaped the vault through a junction", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("restore removed the junction", err)
	}
}
