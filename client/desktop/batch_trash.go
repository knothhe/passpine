package desktop

import (
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
)

// Confirmation belongs to an exact selection and vault session. Authentication
// remains a later step, so opening or cancelling never unlocks or writes.
type batchTrashConfirmation struct {
	open       bool
	entries    []app.Entry
	vaultID    string
	generation uint64
}

func (a *application) confirmBatchTrash() {
	if a.busy || a.closing || a.updates.preparing || !a.batch.active || a.page != "entries" || a.batch.trashConfirmation != nil {
		return
	}
	entries := a.selectedBatchEntries()
	if len(entries) == 0 {
		return
	}
	a.batch.trashConfirmation = &batchTrashConfirmation{
		open: true, entries: entries, vaultID: a.state.VaultID, generation: a.generation,
	}
}

func (a *application) batchTrashConfirmationCurrent(confirmation *batchTrashConfirmation) bool {
	return !a.busy && !a.closing && !a.updates.preparing && a.batch.active && a.page == "entries" &&
		confirmation.vaultID == a.state.VaultID && confirmation.generation == a.generation &&
		slices.EqualFunc(confirmation.entries, a.selectedBatchEntries(), func(left, right app.Entry) bool {
			return left.Path == right.Path
		})
}

func (a *application) batchTrashDialog(c *ui.Context) {
	confirmation := a.batch.trashConfirmation
	if confirmation == nil {
		return
	}
	if !a.batchTrashConfirmationCurrent(confirmation) {
		a.batch.trashConfirmation = nil
		return
	}
	c.Key("batch-trash-confirmation")
	switch components.AlertDialog(c, &confirmation.open, components.AlertDialogOptions{
		Title:        a.t("将 %d 项移到回收站？", len(confirmation.entries)),
		Message:      a.t("将在本机保存加密恢复副本，之后可从恢复中心还原。"),
		ConfirmLabel: a.t("移到回收站"), CancelLabel: a.t("取消"),
	}) {
	case components.DialogConfirmed:
		a.batch.trashConfirmation = nil
		if a.batchTrashConfirmationCurrent(confirmation) {
			a.startBatchTrash()
		}
	case components.DialogCancelled:
		a.batch.trashConfirmation = nil
	}
}
