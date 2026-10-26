package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCleanupUndoReceiptIsOpaqueLockedAndConsumedOnce(t *testing.T) {
	s := testService(t)
	before := must(t, s, Request{Action: "create-folder", Destination: "private-folder/有 空格/child"})
	settings := before.Settings
	settings.BrowseWhileLocked = false
	must(t, s, Request{Action: "settings", Settings: &settings})
	must(t, s, Request{Action: "lock"})
	cleaned := must(t, s, Request{Action: "cleanup-folders", VaultID: before.VaultID})
	if cleaned.RemovedFolders != 3 || cleaned.UndoID == "" || len(cleaned.Folders) != 0 || !cleaned.Locked {
		t.Fatalf("missing locked metadata-only undo receipt: %#v", cleaned)
	}
	encoded, err := json.Marshal(cleaned)
	if err != nil || strings.Contains(string(encoded), "private-folder") || strings.Contains(string(encoded), "有 空格") {
		t.Fatal("cleanup receipt exposed private folder paths", string(encoded), err)
	}
	if noop := must(t, s, Request{Action: "cleanup-folders", VaultID: before.VaultID}); noop.RemovedFolders != 0 || noop.UndoID != "" {
		t.Fatal("no-op cleanup produced another receipt")
	}
	for _, request := range []Request{
		{Action: "undo-cleanup-folders", ID: cleaned.UndoID, VaultID: "stale-vault"},
		{Action: "undo-cleanup-folders", ID: cleaned.UndoID},
		{Action: "undo-cleanup-folders", ID: "unknown", VaultID: before.VaultID},
	} {
		if response := s.Execute(request); response.Status != "error" {
			t.Fatalf("undo accepted an unaffiliated receipt: %#v", response)
		}
	}
	restored := must(t, s, Request{Action: "undo-cleanup-folders", ID: cleaned.UndoID, VaultID: before.VaultID})
	if restored.RestoredFolders != 3 || restored.RemainingFolders != 0 || restored.UndoID != "" || !restored.Locked || len(restored.Entries) != 0 || len(restored.Folders) != 0 || restored.EntriesVersion <= cleaned.EntriesVersion {
		t.Fatalf("locked undo exposed metadata or missed its result: %#v", restored)
	}
	if _, err := os.Stat(filepath.Join(before.Directory, "private-folder/有 空格/child")); err != nil {
		t.Fatal("undo did not recreate removed directories", err)
	}
	if again := s.Execute(Request{Action: "undo-cleanup-folders", ID: cleaned.UndoID, VaultID: before.VaultID}); again.Status != "error" || again.Data.UndoID != "" {
		t.Fatal("completed receipt was reusable", again)
	}
	for _, action := range []string{"cleanup-folders", "undo-cleanup-folders"} {
		raw := s.handleBrowser([]byte(`{"protocol":1,"action":"` + action + `","id":"` + cleaned.UndoID + `"}`))
		if !strings.Contains(string(raw), `"unsupported_action"`) || strings.Contains(string(raw), `"undoId"`) {
			t.Fatal("browser accepted a desktop folder undo capability", string(raw))
		}
	}
}

func TestCleanupUndoPartialFailureKeepsRetryReceiptAndUserFiles(t *testing.T) {
	s := testService(t)
	before := must(t, s, Request{Action: "create-folder", Destination: "blocked/child"})
	must(t, s, Request{Action: "create-folder", Destination: "good"})
	settings := before.Settings
	settings.BrowseWhileLocked = false
	must(t, s, Request{Action: "settings", Settings: &settings})
	must(t, s, Request{Action: "lock"})
	cleaned := must(t, s, Request{Action: "cleanup-folders", VaultID: before.VaultID})
	blocked := filepath.Join(before.Directory, "blocked")
	if err := os.WriteFile(blocked, []byte("concurrent user data"), 0600); err != nil {
		t.Fatal(err)
	}
	partial := s.Execute(Request{Action: "undo-cleanup-folders", ID: cleaned.UndoID, VaultID: before.VaultID})
	if partial.Status != "error" || partial.Data.RestoredFolders != 1 || partial.Data.RemainingFolders != 2 || partial.Data.UndoID != cleaned.UndoID || !partial.Data.Locked || len(partial.Data.Folders) != 0 {
		t.Fatalf("partial undo discarded its retry capability: %#v", partial)
	}
	if strings.Contains(partial.Message, "blocked") || strings.Contains(partial.Message, before.Directory) {
		t.Fatal("failed metadata undo exposed a private path", partial.Message)
	}
	if data, err := os.ReadFile(blocked); err != nil || string(data) != "concurrent user data" {
		t.Fatal("undo overwrote the concurrent file", err)
	}
	s.mu.Lock()
	pending := append([]string(nil), s.folderUndos[cleaned.UndoID].folders...)
	s.mu.Unlock()
	if !slices.Equal(pending, []string{"blocked", "blocked/child"}) {
		t.Fatal("receipt retained completed work or lost failed paths", pending)
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	retry := must(t, s, Request{Action: "undo-cleanup-folders", ID: cleaned.UndoID, VaultID: before.VaultID})
	if retry.RestoredFolders != 2 || retry.RemainingFolders != 0 || retry.UndoID != "" || !retry.Locked {
		t.Fatalf("retry did not complete the remaining metadata-only work: %#v", retry)
	}
}

func TestCleanupUndoReceiptExpiresAndCannotFollowVaultReopen(t *testing.T) {
	for _, invalidate := range []string{"expiry", "reopen"} {
		t.Run(invalidate, func(t *testing.T) {
			s := testService(t)
			before := must(t, s, Request{Action: "create-folder", Destination: "empty"})
			cleaned := must(t, s, Request{Action: "cleanup-folders", VaultID: before.VaultID})
			if invalidate == "expiry" {
				s.mu.Lock()
				receipt := s.folderUndos[cleaned.UndoID]
				receipt.expires = time.Now().Add(-time.Second)
				s.folderUndos[cleaned.UndoID] = receipt
				s.mu.Unlock()
			} else {
				must(t, s, Request{Action: "switch", ID: before.VaultID})
			}
			response := s.Execute(Request{Action: "undo-cleanup-folders", ID: cleaned.UndoID, VaultID: before.VaultID})
			if response.Status != "error" || response.Data.UndoID != "" {
				t.Fatalf("invalidated receipt was still usable: %#v", response)
			}
			if _, err := os.Stat(filepath.Join(before.Directory, "empty")); !os.IsNotExist(err) {
				t.Fatal("invalidated receipt changed the filesystem", err)
			}
		})
	}
}
