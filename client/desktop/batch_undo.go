package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
)

// Undo holds inverse requests for confirmed successes only. Move revisions are
// those written by the service; undo never substitutes a newer entry revision.
type batchUndoState struct {
	vaultID          string
	generation       uint64
	requests         []app.Request
	selection        map[string]bool
	failures         []batchFailure
	forward          *batchResult
	completed        int
	running, visible bool
}

func (a *application) newBatchUndo() *batchUndoState {
	a.batchUndo = nil
	return &batchUndoState{vaultID: a.state.VaultID, generation: a.generation, selection: map[string]bool{}}
}

func (undo *batchUndoState) record(request app.Request, response app.Response) {
	if response.Status != "ok" {
		return
	}
	switch request.Action {
	case "move":
		if response.Data.Revision != "" {
			undo.requests = append(undo.requests, app.Request{Action: "move", VaultID: undo.vaultID, Entry: request.Destination, Destination: request.Entry, Revision: response.Data.Revision})
		}
	case "trash":
		if response.Data.TrashID != "" {
			undo.requests = append(undo.requests, app.Request{Action: "restore", VaultID: undo.vaultID, ID: response.Data.TrashID, Destination: request.Entry})
		}
	}
}

func (a *application) batchUndoCurrent(undo *batchUndoState) bool {
	return undo != nil && a.batchUndo == undo && undo.vaultID == a.state.VaultID && undo.generation == a.generation && len(undo.requests) > 0
}

func (a *application) offerBatchUndo(result batchResult, remaining map[string]bool) {
	undo := result.undo
	if undo == nil || len(undo.requests) == 0 {
		return
	}
	undo.generation = a.generation
	for path := range remaining {
		undo.selection[path] = true
	}
	a.batchUndo = undo
	message, description := a.batchMoveSuccess(result, result.completed), ""
	if result.action == "trash" {
		message, description = a.t("已将 %d 项移到回收站", result.completed), a.t("恢复副本仅保存在此设备")
	}
	if len(result.failures) > 0 {
		description = a.t("%d 项未完成，可重试或撤销已完成的操作。", len(result.failures))
	}
	a.notifyWithAction(message, description, a.t("撤销"), 8*time.Second, func() { a.startBatchUndo(undo) })
}

func (a *application) startBatchUndo(undo *batchUndoState) {
	if !a.batchUndoCurrent(undo) || undo.running || a.busy || a.closing || a.editor.open || a.updates.preparing || a.setup {
		return
	}
	a.page = "entries"
	a.dropDetail()
	a.batch.active = true
	if a.batch.expanded == nil {
		a.batch.expanded = map[string]bool{}
	}
	undo.visible = true
	if a.state.Locked {
		a.requestBatchUnlock("undo")
		return
	}
	requests := append([]app.Request(nil), undo.requests...)
	if a.batch.result != nil && a.batch.result.undo == undo {
		undo.forward = a.batch.result
	}
	undo.running = true
	undo.failures = nil
	a.batch.result = nil
	a.runBatch("undo", requests, func(responses []app.Response) {
		undo.running = false
		if a.batchUndo != undo || a.state.VaultID != undo.vaultID || a.generation != undo.generation {
			return
		}
		undo.requests = nil
		for index, request := range requests {
			response := batchResponse(responses, index)
			if response.Status == "ok" {
				undo.completed++
				undo.selection[request.Destination] = true
				continue
			}
			undo.requests = append(undo.requests, request)
			undo.failures = append(undo.failures, batchFailure{entry: request.Destination, destination: request.Entry, code: response.Code})
		}
		a.batch.selected = undo.selection
		if len(undo.requests) == 0 {
			a.batchUndo = nil
			a.restoreForwardBatchResult(undo)
			a.notice = a.t("已撤销")
			return
		}
		a.notifyErrorWithAction(a.t("已撤销 %d 项，%d 项未完成", undo.completed, len(undo.requests)), a.t("未完成的条目保持原样，可重试撤销。"), a.t("重试撤销"), func() { a.startBatchUndo(undo) })
	})
}

func (a *application) restoreForwardBatchResult(undo *batchUndoState) {
	if undo.forward != nil {
		undo.forward.completed = max(0, undo.forward.completed-undo.completed)
		undo.forward.undo = nil
		a.batch.result = undo.forward
		undo.forward = nil
	}
}

func (a *application) batchUndoView(c *ui.Context, undo *batchUndoState) {
	components.ScrollView(c, components.ScrollViewOptions{}).Key("batch-undo-result").Fill().Children(func() {
		ui.Column(c).FillWidth().MaxWidth(608).Padding(24).Gap(12).Children(func() {
			ui.Box(c).MinHeight(32)
			title := a.t("撤销操作")
			if undo.running && a.batch.progress != nil {
				title = a.t("正在撤销 %d/%d", a.batch.progress.done, a.batch.progress.total)
			} else if len(undo.failures) > 0 {
				title = a.t("已撤销 %d 项，%d 项未完成", undo.completed, len(undo.requests))
			}
			components.Text(c, title).FillWidth().Wrap().FontSize(scaledFontSize(c, 24)).Bold()
			for _, failure := range undo.failures {
				ui.Column(c).Key(failure.entry).FillWidth().Gap(4).Children(func() {
					components.Text(c, failure.entry).FillWidth().Wrap().FontSize(scaledFontSize(c, 12))
					message := a.t("暂时无法还原，可以重试撤销。")
					if failure.code == "already_exists" {
						message = a.t("原路径已有条目，未覆盖。")
					} else if failure.code == "revision_conflict" {
						message = a.t("目标条目已更改，未还原。")
					}
					muted(c, message)
				})
			}
			if !undo.running {
				ui.Row(c).Gap(8).Children(func() {
					if components.Button(c, a.t("重试撤销"), components.ButtonPrimary).Disabled(a.busy || !a.batchUndoCurrent(undo)).Clicked() {
						a.startBatchUndo(undo)
					}
					if components.Button(c, a.t("完成"), components.ButtonOutline).Disabled(a.busy).Clicked() {
						a.batchUndo = nil
						a.clearNotifications()
						a.restoreForwardBatchResult(undo)
						if a.batch.result == nil {
							a.exitBatch()
						}
					}
				})
			}
		})
	})
}
