package desktop

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// Preview status of a selected entry. Every status except ready is skipped.
const (
	batchMoveReady     = "ready"
	batchMoveExists    = "exists"
	batchMoveDuplicate = "duplicate"
	batchMoveUnchanged = "unchanged"
)

// batchPlan mirrors the service's refusal to overwrite: entries whose new path
// already exists, collides with another selected entry or stays the same are
// skipped, and stay selected after the move.
type batchPlan struct {
	ready, skipped []app.Entry
	status, target map[string]string
}

func (a *application) chooseBatchFolder(folder string) {
	a.batch.destination, a.batch.folderQuery, a.batch.chosen = folder, folder, true
	a.batch.result = nil
}

// Only an exact existing path is a destination. A partial search or a new path
// must never silently keep the previously chosen destination active.
func (a *application) syncBatchFolderQuery() {
	folder := strings.TrimSpace(a.batch.folderQuery)
	a.batch.destination, a.batch.chosen = "", false
	if folder != "" && containsFolder(a.folders, folder) {
		a.batch.destination, a.batch.chosen = folder, true
	}
	a.batch.result = nil
}

func (a *application) batchMoveConfigured() bool {
	return a.batch.chosen
}

func (a *application) batchMovePlan() batchPlan {
	plan := batchPlan{status: map[string]string{}, target: map[string]string{}}
	if !a.batchMoveConfigured() {
		return plan
	}
	chosen := a.selectedBatchEntries()
	target := a.batchMoveTarget(chosen)
	existing := make(map[string]int, len(a.sortedEntries))
	for _, entry := range a.sortedEntries {
		existing[strings.ToLower(entry.Path)]++
	}
	targets := make(map[string]int, len(chosen))
	for _, entry := range chosen {
		plan.target[entry.Path] = target(entry)
		targets[strings.ToLower(plan.target[entry.Path])]++
	}
	for _, entry := range chosen {
		destination := plan.target[entry.Path]
		key := strings.ToLower(destination)
		// A case-only rename of the entry itself is not an existing target.
		others := existing[key]
		if key == strings.ToLower(entry.Path) {
			others--
		}
		status := batchMoveReady
		switch {
		case destination == entry.Path:
			status = batchMoveUnchanged
		case others > 0:
			status = batchMoveExists
		case targets[key] > 1:
			status = batchMoveDuplicate
		}
		plan.status[entry.Path] = status
		if status == batchMoveReady {
			plan.ready = append(plan.ready, entry)
		} else {
			plan.skipped = append(plan.skipped, entry)
		}
	}
	return plan
}

func (a *application) batchMoveSuccess(result batchResult, count int) string {
	return a.t("已将 %d 项移动到“%s”", count, a.batchFolderLabel(result.destination))
}

// A vault/session error stops the sequence. Keep unsent entries in the result
// with the same recoverable reason so a later retry cannot lose the selection.
func batchResponse(responses []app.Response, index int) app.Response {
	if index < len(responses) {
		return responses[index]
	}
	code := "operation_failed"
	if len(responses) > 0 {
		code = responses[len(responses)-1].Code
	}
	return app.Response{Status: "error", Code: code}
}

// batchPreviewView lists every original and new path. Skipped entries come
// first, so their reasons stay visible without scrolling.
func (a *application) batchPreviewView(c *ui.Context, plan batchPlan) {
	t := c.Theme()
	components.Text(c, a.t("路径预览")).Margin(16, 0, 6, 0).FontSize(scaledFontSize(c, 11)).TextColor(t.TextMuted)
	if !a.batchMoveConfigured() {
		muted(c, a.t("选择目标文件夹后，这里会列出每一项的新路径。"))
		return
	}
	summary := a.t("将移动 %d 项", len(plan.ready))
	if len(plan.skipped) > 0 {
		summary = a.t("将移动 %d 项，跳过 %d 项", len(plan.ready), len(plan.skipped))
	}
	components.Text(c, summary).FontSize(scaledFontSize(c, 12)).Bold()
	if len(plan.ready) == 0 {
		a.batchNotice(c, false, a.t("没有可移动的条目，请换一个目标文件夹或保留方式。"))
	}
	rows := append(append(make([]app.Entry, 0, len(plan.skipped)+len(plan.ready)), plan.skipped...), plan.ready...)
	a.batch.preview.Key = func(i int) any { return rows[i].Path }
	c.Key("batch-preview")
	components.List(c, &a.batch.preview, len(rows), func(i int) {
		entry := rows[i]
		status := plan.status[entry.Path]
		ui.Column(c).FillWidth().Padding(7, 0).Gap(2).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			components.Text(c, entry.Path).FillWidth().Wrap().FontSize(scaledFontSize(c, 11)).TextColor(t.TextMuted)
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Start).Children(func() {
				components.Icon(c, lucide.ArrowRight()).Size(12, 12).Margin(3, 0, 0, 0).Shrink(0).TextColor(t.TextMuted)
				target := components.Text(c, plan.target[entry.Path]).Grow(1).MinWidth(0).Wrap().FontSize(scaledFontSize(c, 11))
				if status != batchMoveReady {
					target.TextColor(t.TextMuted)
				}
			})
			reason, icon, color := "", lucide.TriangleAlert(), t.Warning
			switch status {
			case batchMoveExists:
				reason = a.t("目标位置已有同名条目，跳过")
			case batchMoveDuplicate:
				reason = a.t("与其他所选条目重名，跳过")
			case batchMoveUnchanged:
				reason, icon, color = a.t("路径不变，跳过"), lucide.Info(), t.TextMuted
			default:
				return
			}
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Start).Children(func() {
				components.Icon(c, icon).Size(12, 12).Margin(3, 0, 0, 0).Shrink(0).TextColor(color)
				components.Text(c, reason).Grow(1).MinWidth(0).Wrap().FontSize(scaledFontSize(c, 11)).TextColor(color)
			})
		})
	}).Label(a.t("原路径与新路径")).FillWidth().Grow(1).MinHeight(96).Margin(8, 0, 0, 0)
}

// A lock keeps the operation lease busy until its current request has returned.
// The atomic flag is the only UI state read by the worker.
func (a *application) stopBatchForLock() bool {
	if a.batch.progress == nil {
		return false
	}
	a.batch.progress.cancelled.Store(true)
	return true
}
