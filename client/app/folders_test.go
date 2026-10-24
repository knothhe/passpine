package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCreateFolderPersistsEmptyHierarchyAndRefreshesMetadata(t *testing.T) {
	s := testService(t)
	initial := must(t, s, Request{Action: "list"})
	created := must(t, s, Request{Action: "create-folder", VaultID: initial.VaultID, Destination: "test/a/b/c"})
	want := []string{"test", "test/a", "test/a/b", "test/a/b/c"}
	if created.Destination != "test/a/b/c" || !slices.Equal(created.Folders, want) || len(created.Entries) != 0 || created.EntriesVersion <= initial.EntriesVersion {
		t.Fatalf("creation did not refresh empty folders: %#v", created)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(Options{DataDir: s.directory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { restarted.Close() })
	state := must(t, restarted, Request{Action: "list"})
	if !state.Locked || !slices.Equal(state.Folders, want) {
		t.Fatal("empty folders disappeared after restart", state.Folders)
	}
	if err := os.Mkdir(filepath.Join(state.Directory, "external"), 0700); err != nil {
		t.Fatal(err)
	}
	refreshed := must(t, restarted, Request{Action: "list"})
	if !slices.Contains(refreshed.Folders, "external") || refreshed.EntriesVersion <= state.EntriesVersion {
		t.Fatal("external folder creation did not invalidate metadata", refreshed.Folders)
	}
	settings := refreshed.Settings
	settings.BrowseWhileLocked = false
	hidden := must(t, restarted, Request{Action: "settings", Settings: &settings})
	if len(hidden.Folders) != 0 || len(hidden.Entries) != 0 {
		t.Fatal("locked metadata exposed folders with browsing disabled")
	}
	denied := restarted.Execute(Request{Action: "create-folder", VaultID: state.VaultID, Destination: "locked/child"})
	if denied.Status != "error" || denied.Code != "locked" || len(denied.Data.Folders) != 0 {
		t.Fatalf("locked creation was not rejected: %#v", denied)
	}
}

func TestCreateFolderRejectsStaleVaultAndReportsInvalidPath(t *testing.T) {
	s := testService(t)
	for _, test := range []struct{ vault, destination, code string }{
		{"stale-vault", "stale/child", "vault_changed"},
		{"", "invalid/../child", "invalid_path"},
	} {
		result := s.Execute(Request{Action: "create-folder", VaultID: test.vault, Destination: test.destination})
		if result.Status != "error" || result.Code != test.code || result.Message == "" {
			t.Fatalf("missing creation error: %#v", result)
		}
	}
	for _, parent := range []string{"stale", "invalid"} {
		if _, err := os.Stat(filepath.Join(s.active.Config().Directory, parent)); !os.IsNotExist(err) {
			t.Fatal("rejected request created folders", err)
		}
	}
}

func TestCleanupEmptyFoldersRefreshesMetadataAndKeepsMovedEntry(t *testing.T) {
	s := testService(t)
	entry := must(t, s, Request{Action: "save", Entry: "test/a/b/c/d", Contents: "keep-this-secret", Mode: "create"})
	must(t, s, Request{Action: "move", Entry: entry.Entry, Destination: "test/a/b/d"})
	before := must(t, s, Request{Action: "list"})
	rejected := s.Execute(Request{Action: "cleanup-folders", VaultID: "stale-vault"})
	if rejected.Code != "vault_changed" || !slices.Contains(rejected.Data.Folders, "test/a/b/c") {
		t.Fatal("stale cleanup request changed the vault")
	}
	cleaned := must(t, s, Request{Action: "cleanup-folders", VaultID: before.VaultID})
	if cleaned.RemovedFolders != 1 || slices.Contains(cleaned.Folders, "test/a/b/c") || cleaned.EntriesVersion <= before.EntriesVersion {
		t.Fatalf("cleanup did not refresh folders and count: %#v", cleaned)
	}
	if moved := must(t, s, Request{Action: "fetch", Entry: "test/a/b/d"}); moved.Contents != entry.Contents {
		t.Fatal("cleanup changed the entry")
	}
	if again := must(t, s, Request{Action: "cleanup-folders", VaultID: before.VaultID}); again.RemovedFolders != 0 {
		t.Fatal("repeated cleanup reported removed folders")
	}
	must(t, s, Request{Action: "create-folder", Destination: "keep/empty"})
	must(t, s, Request{Action: "lock"})
	if denied := s.Execute(Request{Action: "cleanup-folders", VaultID: before.VaultID}); denied.Code != "locked" || !slices.Contains(denied.Data.Folders, "keep/empty") {
		t.Fatal("locked cleanup was not rejected")
	}
	for _, action := range []string{"configure", "cleanup-folders"} {
		raw := s.handleBrowser([]byte(`{"protocol":1,"action":"` + action + `"}`))
		if action == "cleanup-folders" && !strings.Contains(string(raw), `"unsupported_action"`) {
			t.Fatal("browser boundary allowed desktop folder cleanup")
		}
		if strings.Contains(string(raw), `"folders"`) {
			t.Fatal("browser response exposed desktop folder metadata")
		}
	}
}
