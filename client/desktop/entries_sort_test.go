package desktop

import (
	"reflect"
	"testing"

	"github.com/knothhe/passpine-neo/client/app"
)

func TestEntriesSortByFullPath(t *testing.T) {
	a := newApplication(nil)
	a.state.Entries = []app.Entry{
		{Path: "work/Alpha", Name: "Alpha", Folder: "work"},
		{Path: "personal/Shared", Name: "Shared", Folder: "personal"},
		{Path: "Alpha/Zulu", Name: "Zulu", Folder: "Alpha"},
		{Path: "work/Shared", Name: "Shared", Folder: "work"},
		{Path: "Beta/Shared", Name: "Shared", Folder: "Beta"},
		{Path: "account", Name: "account"},
	}
	want := []string{"account", "Alpha/Zulu", "Beta/Shared", "personal/Shared", "work/Alpha", "work/Shared"}
	assertEntryPaths(t, a.entries(), want)
	// Reusing the cached results must preserve the full-path order.
	assertEntryPaths(t, a.entries(), want)
	if a.state.Entries[0].Path != "work/Alpha" {
		t.Fatal("sorting changed the backend snapshot")
	}
}

func TestEntrySearchPreservesFullPathOrder(t *testing.T) {
	a := newApplication(nil)
	a.state.Entries = []app.Entry{
		{Path: "work/account-Alpha", Name: "account-Alpha", Folder: "work"},
		{Path: "personal/account-Shared", Name: "account-Shared", Folder: "personal"},
		{Path: "Alpha/account-Zulu", Name: "account-Zulu", Folder: "Alpha"},
		{Path: "work/account-Shared", Name: "account-Shared", Folder: "work"},
		{Path: "Beta/account-Shared", Name: "account-Shared", Folder: "Beta"},
		{Path: "Alpha/unrelated", Name: "unrelated", Folder: "Alpha"},
	}
	a.entries()
	a.query = " ACCOUNT "
	want := []string{"Alpha/account-Zulu", "Beta/account-Shared", "personal/account-Shared", "work/account-Alpha", "work/account-Shared"}
	assertEntryPaths(t, a.entries(), want)
	assertEntryPaths(t, a.entries(), want)
	a.query = "shared"
	assertEntryPaths(t, a.entries(), []string{"Beta/account-Shared", "personal/account-Shared", "work/account-Shared"})
	a.query = ""
	assertEntryPaths(t, a.entries(), []string{"Alpha/account-Zulu", "Alpha/unrelated", "Beta/account-Shared", "personal/account-Shared", "work/account-Alpha", "work/account-Shared"})
}

func assertEntryPaths(t *testing.T, entries []app.Entry, want []string) {
	t.Helper()
	got := make([]string, len(entries))
	for i, entry := range entries {
		got[i] = entry.Path
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entry paths = %q, want %q", got, want)
	}
}
