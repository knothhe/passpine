//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCleanupPermissionFailureStillProvidesUndoForCompletedFolders(t *testing.T) {
	s := testService(t)
	before := must(t, s, Request{Action: "create-folder", Destination: "a-blocked/child"})
	must(t, s, Request{Action: "create-folder", Destination: "z-empty"})
	blocked := filepath.Join(before.Directory, "a-blocked")
	if err := os.Chmod(blocked, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(blocked, 0700) })
	response := s.Execute(Request{Action: "cleanup-folders", VaultID: before.VaultID})
	if response.Status != "error" || response.Data.RemovedFolders != 1 || response.Data.UndoID == "" || strings.Contains(response.Message, before.Directory) || strings.Contains(response.Message, "a-blocked") {
		t.Fatalf("partial cleanup lost its safe receipt: %#v", response)
	}
	s.mu.Lock()
	removed := append([]string(nil), s.folderUndos[response.Data.UndoID].folders...)
	s.mu.Unlock()
	if !slices.Equal(removed, []string{"z-empty"}) {
		t.Fatal("receipt included folders that were not removed", removed)
	}
	restored := must(t, s, Request{Action: "undo-cleanup-folders", ID: response.Data.UndoID, VaultID: before.VaultID})
	if restored.RestoredFolders != 1 || restored.RemainingFolders != 0 {
		t.Fatal("could not undo completed work from partial cleanup", restored)
	}
}
