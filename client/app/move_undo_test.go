package app

import "testing"

func TestBatchMoveReturnsRevisionAndRejectsUnsafeUndo(t *testing.T) {
	s := testService(t)
	original := must(t, s, Request{Action: "save", Entry: "work/account", Contents: "first-secret", Mode: "create"})
	edited := must(t, s, Request{Action: "save", Entry: original.Entry, Contents: "latest-secret", Revision: original.Revision, Mode: "edit"})
	// A normal list-based move has no revision and carries the latest edit.
	moved := must(t, s, Request{Action: "move", Entry: original.Entry, Destination: "archive/account"})
	actual := must(t, s, Request{Action: "fetch", Entry: moved.Entry})
	if moved.Revision == "" || moved.Revision != actual.Revision || actual.Contents != edited.Contents {
		t.Fatal("move did not preserve the latest contents and report the destination revision")
	}
	if moved.Contents != "" || moved.Password != "" || moved.Username != "" || moved.Notes != "" {
		t.Fatal("move response exposed decrypted contents")
	}
	// The same revision can move back only while the original path is free.
	replacement := must(t, s, Request{Action: "save", Entry: original.Entry, Contents: "new-original-entry", Mode: "create"})
	blocked := s.Execute(Request{Action: "move", Entry: moved.Entry, Destination: original.Entry, Revision: moved.Revision})
	if blocked.Code != "already_exists" {
		t.Fatalf("undo accepted an occupied original path: %s", blocked.Code)
	}
	for _, want := range []Data{replacement, actual} {
		got := must(t, s, Request{Action: "fetch", Entry: want.Entry})
		if got.Contents != want.Contents || got.Revision != want.Revision {
			t.Fatal("rejected undo changed an entry")
		}
	}
	must(t, s, Request{Action: "trash", Entry: replacement.Entry})
	later := must(t, s, Request{Action: "save", Entry: moved.Entry, Contents: "later-destination-edit", Revision: moved.Revision, Mode: "edit"})
	blocked = s.Execute(Request{Action: "move", Entry: moved.Entry, Destination: original.Entry, Revision: moved.Revision})
	if blocked.Code != "revision_conflict" || blocked.Data.Revision != "" {
		t.Fatalf("undo accepted an edited destination or returned a usable revision: %s", blocked.Code)
	}
	got := must(t, s, Request{Action: "fetch", Entry: moved.Entry})
	if got.Contents != later.Contents || got.Revision != later.Revision {
		t.Fatal("rejected undo moved or changed the later edit")
	}
	if absent := s.Execute(Request{Action: "fetch", Entry: original.Entry}); absent.Status != "error" {
		t.Fatal("rejected undo recreated the original path")
	}
}
