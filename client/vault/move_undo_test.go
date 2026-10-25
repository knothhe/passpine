package vault

import (
	"errors"
	"os"
	"testing"
)

func TestMoveWithRevisionSupportsUndoWithoutOverwritingLaterChanges(t *testing.T) {
	v := newFixture(t)
	original := mustSave(t, v, "work/account", "original-secret", "")
	moved, err := v.MoveWithRevision(original.Path, "archive/account", original.Revision)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := v.Revision(moved.Path)
	if err != nil || moved.Revision == "" || moved.Revision != actual || moved.Path != "archive/account" {
		t.Fatal("move did not return its actual destination revision", err)
	}
	restored, err := v.MoveWithRevision(moved.Path, original.Path, moved.Revision)
	if err != nil || restored.Path != original.Path || restored.Raw != original.Raw {
		t.Fatal("unchanged move could not be undone", err)
	}
	moved, err = v.MoveWithRevision(restored.Path, moved.Path, restored.Revision)
	if err != nil {
		t.Fatal(err)
	}
	edited := mustSave(t, v, moved.Path, "later-edit", moved.Revision)
	if _, err := v.MoveWithRevision(moved.Path, original.Path, moved.Revision); !errors.Is(err, ErrConflict) {
		t.Fatal("undo accepted a stale destination revision", err)
	}
	if _, err := v.Read(original.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rejected undo created its old path", err)
	}
	replacement := mustSave(t, v, original.Path, "new-original-entry", "")
	if _, err := v.MoveWithRevision(moved.Path, original.Path, edited.Revision); !errors.Is(err, ErrExists) {
		t.Fatal("undo overwrote an entry recreated at the original path", err)
	}
	for _, want := range []Document{replacement, edited} {
		got, err := v.Read(want.Path)
		if err != nil || got.Raw != want.Raw || got.Revision != want.Revision {
			t.Fatal("rejected undo changed a later entry", err)
		}
	}
}
