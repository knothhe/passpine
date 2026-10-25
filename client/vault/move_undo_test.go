package vault

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
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
	if err != nil || restored.Path != original.Path || restored.Revision != original.Revision {
		t.Fatal("unchanged move could not be undone", err)
	}
	if got, err := v.Read(restored.Path); err != nil || got.Raw != original.Raw {
		t.Fatal("undo changed the entry contents", err)
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

func TestMoveKeepsCiphertextWithoutUnlockUnderSamePolicy(t *testing.T) {
	v := newFixture(t)
	doc := mustSave(t, v, "work/account", "locked-move-secret", "")
	before, err := os.ReadFile(filepath.Join(v.config.Directory, "work/account.gpg"))
	if err != nil {
		t.Fatal(err)
	}
	v.Lock()
	moved, err := v.MoveWithRevision(doc.Path, "archive/team/account", doc.Revision)
	if err != nil || moved.Path != "archive/team/account" || moved.Revision != doc.Revision || moved.Raw != "" {
		t.Fatal("locked move under the same policy failed or exposed contents", err)
	}
	after, err := os.ReadFile(filepath.Join(v.config.Directory, "archive/team/account.gpg"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("locked move changed the ciphertext", err)
	}
	if _, err := os.Stat(filepath.Join(v.config.Directory, "work/account.gpg")); !os.IsNotExist(err) {
		t.Fatal("locked move kept its source", err)
	}
	if _, err := v.MoveWithRevision(moved.Path, doc.Path, "stale"); !errors.Is(err, ErrConflict) {
		t.Fatal("locked move accepted a stale revision", err)
	}
	if err := os.WriteFile(filepath.Join(v.config.Directory, "archive/other.gpg"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := v.MoveWithRevision(moved.Path, "archive/other", moved.Revision); !errors.Is(err, ErrExists) {
		t.Fatal("locked move overwrote an existing entry", err)
	}
}

func TestLockedMoveToAnotherPolicyRequiresUnlock(t *testing.T) {
	v := newFixture(t)
	doc := mustSave(t, v, "work/account", "policy-secret", "")
	if err := os.MkdirAll(filepath.Join(v.config.Directory, "shared"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v.config.Directory, "shared/.gpg-id"), []byte("other-recipient\n"), 0600); err != nil {
		t.Fatal(err)
	}
	v.Lock()
	if _, err := v.MoveWithRevision(doc.Path, "shared/account", doc.Revision); !errors.Is(err, ErrLocked) {
		t.Fatal("locked move skipped re-encryption for another policy", err)
	}
	if _, err := os.Stat(filepath.Join(v.config.Directory, "shared/account.gpg")); !os.IsNotExist(err) {
		t.Fatal("rejected move wrote its destination", err)
	}
	if revision, err := v.Revision(doc.Path); err != nil || revision != doc.Revision {
		t.Fatal("rejected move changed its source", err)
	}
}
