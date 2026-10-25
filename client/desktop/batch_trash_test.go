package desktop

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func clickBatchTrashDialogButton(t *testing.T, a *application, view *ui.Tester, label string) {
	t.Helper()
	confirmation := a.batch.trashConfirmation
	if confirmation == nil {
		t.Fatal("batch trash confirmation is missing")
	}
	panel := visualBounds(t, view, a.t("将 %d 项移到回收站？", len(confirmation.entries)))
	button := visualBoundsIn(t, view, label, panel)
	view.ClickAt(button.X+button.W/2, button.Y+button.H/2)
}

func TestBatchTrashRequiresConfirmationAndCancellationKeepsDraft(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		t.Run(language, func(t *testing.T) {
			a, backend, settle := batchFixture(t, nil)
			a.state.Settings.Language = language
			a.enterBatch()
			a.batch.selected["开发/Cloudflare"], a.batch.selected["工作/Figma"] = true, true
			a.chooseBatchFolder("家庭/设备")
			a.batch.prefixTo = "unfinished/prefix"
			view := ui.NewTester(testView(a), 860, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			view.Advance(time.Second)
			if err := view.Click(a.t("移到回收站")); err != nil {
				t.Fatal(err)
			}
			if a.batch.trashConfirmation == nil || a.busy || len(backend.actions("trash")) != 0 || len(backend.actions("unlock")) != 0 {
				t.Fatal("opening trash confirmation started authentication or a write")
			}
			if !view.HasText(a.t("将 %d 项移到回收站？", 2)) || !view.HasText(a.t("将在本机保存加密恢复副本，之后可从恢复中心还原。")) {
				t.Fatal("confirmation omitted its count or recovery explanation")
			}
			if language == "en" && !view.HasText("Move 2 entries to Trash?") {
				t.Fatal("English confirmation fell back to untranslated text")
			}
			view.Advance(time.Second)
			managerResizeScreenshot(t, view, "batch-trash-confirmation-"+language)
			clickBatchTrashDialogButton(t, a, view, a.t("取消"))
			if a.batch.trashConfirmation != nil || len(a.batch.selected) != 2 || !a.batch.chosen || a.batch.destination != "家庭/设备" || a.batch.prefixTo != "unfinished/prefix" || len(backend.actions("trash")) != 0 {
				t.Fatal("cancelling trash discarded the selection/draft or sent a write")
			}
			if err := view.Click(a.t("移到回收站")); err != nil {
				t.Fatal(err)
			}
			clickBatchTrashDialogButton(t, a, view, a.t("移到回收站"))
			settle()
			view.Frame()
			if a.batch.trashConfirmation != nil || a.batch.active || len(backend.actions("trash")) != 2 {
				t.Fatal("confirming did not trash each selected entry exactly once")
			}
		})
	}
}

func TestBatchTrashConfirmationPrecedesUnlock(t *testing.T) {
	a, backend, settle := batchFixture(t, nil)
	backend.locked, a.state.Locked = true, true
	a.enterBatch()
	a.batch.selected["开发/Cloudflare"] = true
	view := ui.NewTester(testView(a), 1180, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	if err := view.Click(a.t("移到回收站")); err != nil {
		t.Fatal(err)
	}
	if a.pendingUnlock.action != "" || len(backend.actions("unlock")) != 0 || len(backend.actions("trash")) != 0 {
		t.Fatal("authentication started before the user confirmed trash")
	}
	clickBatchTrashDialogButton(t, a, view, a.t("移到回收站"))
	if a.batch.trashConfirmation != nil || a.pendingUnlock.action != "batch" || a.batch.pending != "trash" || len(backend.actions("trash")) != 0 {
		t.Fatal("confirmation did not hand off to the existing unlock continuation")
	}
	backend.locked, a.state.Locked = false, false
	if !a.resumeBatchAction() {
		t.Fatal("unlock did not resume the confirmed trash operation")
	}
	settle()
	if len(backend.actions("trash")) != 1 || a.batch.trashConfirmation != nil {
		t.Fatal("confirmed unlock required another dialog or repeated trash")
	}
}

func TestBatchTrashConfirmationRejectsStaleContext(t *testing.T) {
	for _, changed := range []string{"selection", "vault", "session", "navigation", "busy"} {
		t.Run(changed, func(t *testing.T) {
			a, backend, _ := batchFixture(t, nil)
			a.enterBatch()
			a.batch.selected["开发/Cloudflare"], a.batch.selected["工作/Figma"] = true, true
			view := ui.NewTester(testView(a), 1180, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			a.confirmBatchTrash()
			view.Frame()
			if a.batch.trashConfirmation == nil {
				t.Fatal("confirmation did not open")
			}
			switch changed {
			case "selection":
				delete(a.batch.selected, "工作/Figma")
			case "vault":
				a.state.VaultID = "another-vault"
			case "session":
				a.generation++
			case "navigation":
				a.page = "settings"
			case "busy":
				a.busy = true
			}
			view.Frame()
			if a.batch.trashConfirmation != nil || len(backend.actions("trash")) != 0 || len(backend.actions("unlock")) != 0 {
				t.Fatal("stale confirmation stayed active or executed an operation")
			}
		})
	}
}

func TestBatchTrashConfirmationIgnoresUnavailableEntry(t *testing.T) {
	for _, blocked := range []string{"empty", "busy", "closing", "update", "inactive"} {
		t.Run(fmt.Sprint(blocked), func(t *testing.T) {
			a, backend, _ := batchFixture(t, nil)
			a.enterBatch()
			if blocked != "empty" {
				a.batch.selected["开发/Cloudflare"] = true
			}
			switch blocked {
			case "busy":
				a.busy = true
			case "closing":
				a.closing = true
			case "update":
				a.updates.preparing = true
			case "inactive":
				a.batch.active = false
			}
			a.confirmBatchTrash()
			if a.batch.trashConfirmation != nil || len(backend.actions("trash")) != 0 {
				t.Fatal("an unavailable action opened confirmation or wrote entries")
			}
		})
	}
}
