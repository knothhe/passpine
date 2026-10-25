package desktop

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/vault"
	"github.com/oneo-me/onevra/source/components"
)

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

func (a *application) setBatchMoveMode(prefix bool) {
	a.batch.moveMode = ""
	if prefix {
		a.batch.moveMode = "prefix"
		if !a.batch.prefixInitialized {
			a.batch.prefixFrom = commonBatchFolder(a.selectedBatchEntries())
			a.batch.prefixTo = a.batch.prefixFrom
			a.batch.prefixInitialized = true
		}
	}
	a.batch.result = nil
}

func commonBatchFolder(entries []app.Entry) string {
	if len(entries) == 0 {
		return ""
	}
	parts := strings.Split(entries[0].Folder, "/")
	for _, entry := range entries[1:] {
		other := strings.Split(entry.Folder, "/")
		n := 0
		for n < len(parts) && n < len(other) && parts[n] == other[n] {
			n++
		}
		parts = parts[:n]
	}
	return strings.Join(parts, "/")
}

func (a *application) batchPrefixValid() bool {
	from, to := strings.TrimSpace(a.batch.prefixFrom), strings.TrimSpace(a.batch.prefixTo)
	return from != "" && vault.ValidatePath(from) == nil && (to == "" || vault.ValidatePath(to) == nil)
}

func (a *application) batchMoveConfigured() bool {
	if a.batch.moveMode == "prefix" {
		return a.batchPrefixValid()
	}
	return a.batch.chosen
}

func (a *application) batchEntryDestination(entry app.Entry) string {
	if a.batch.moveMode != "prefix" {
		return batchDestinationPath(a.batch.destination, entry.Name)
	}
	from, to := strings.TrimSpace(a.batch.prefixFrom), strings.TrimSpace(a.batch.prefixTo)
	// Match complete folder segments, never an entry name or a partial segment.
	if from == "" || entry.Folder != from && !strings.HasPrefix(entry.Folder, from+"/") {
		return entry.Path
	}
	return batchDestinationPath(to, strings.TrimPrefix(entry.Path, from+"/"))
}

func (a *application) batchMoveSuccess(result batchResult, count int) string {
	if result.prefix {
		return a.t("已修改 %d 项的文件夹前缀", count)
	}
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

func (a *application) batchPrefixView(c *ui.Context, chosen, ready, conflicts, unchanged []app.Entry) {
	muted(c, a.t("仅替换完整文件夹前缀，保留后续层级和条目名称。"))
	for _, field := range []struct {
		value *string
		label string
		key   string
	}{{&a.batch.prefixFrom, a.t("原文件夹前缀"), "from"}, {&a.batch.prefixTo, a.t("新文件夹前缀"), "to"}} {
		components.Text(c, field.label).Margin(14, 0, 6, 0).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
		c.Key(field.key)
		input := textBox(c, field.value, components.TextBoxOptions{}).FillWidth().Label(field.label).Disabled(a.busy)
		if field.value == &a.batch.prefixTo {
			input.Placeholder(a.t("留空移除前缀"))
		}
		if input.Changed() {
			a.lastInput = time.Now()
		}
	}
	if !a.batchPrefixValid() {
		a.batchNotice(c, false, a.t("请输入有效的原文件夹前缀；新前缀留空可移除原前缀。"))
		return
	}
	components.Text(c, a.t("路径预览")).Margin(18, 0, 8, 0).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
	components.ScrollView(c, components.ScrollViewOptions{}).Key("batch-prefix-preview").Label(a.t("路径预览")).FillWidth().Grow(1).MinHeight(90).Children(func() {
		ui.Column(c).FillWidth().Gap(12).Children(func() {
			for _, entry := range chosen {
				ui.Column(c).Key(entry.Path).FillWidth().Gap(4).Children(func() {
					components.Text(c, entry.Path).FillWidth().Wrap().FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
					components.Text(c, a.batchEntryDestination(entry)).FillWidth().Wrap().FontSize(scaledFontSize(c, 12)).Bold()
				})
			}
		})
	})
	if len(conflicts) > 0 {
		a.batchNotice(c, true, a.t("%d 项目标路径已存在或重名，将跳过。", len(conflicts)))
	}
	if len(unchanged) > 0 {
		a.batchNotice(c, false, a.t("%d 项前缀不匹配或无需更改，将保留。", len(unchanged)))
	}
	if len(ready) == 0 {
		a.batchNotice(c, false, a.t("没有可修改的条目，请检查前缀。"))
	}
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
