package desktop

import (
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/i18n"
	"github.com/knothhe/passpine/client/vault"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
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
		a.syncBatchPrefixSource()
	}
	a.batch.result = nil
}

func (a *application) batchPrefixOptions() []string {
	seen := map[string]bool{}
	for _, entry := range a.selectedBatchEntries() {
		if entry.Folder == "" {
			continue
		}
		parts := strings.Split(entry.Folder, "/")
		for i := range parts {
			seen[strings.Join(parts[:i+1], "/")] = true
		}
	}
	options := make([]string, 0, len(seen))
	for folder := range seen {
		options = append(options, folder)
	}
	sort.Strings(options)
	return options
}

func (a *application) syncBatchPrefixSource() {
	if a.busy || a.batch.progress != nil {
		return
	}
	options := a.batchPrefixOptions()
	if a.batch.prefixInitialized && containsFolder(options, a.batch.prefixFrom) {
		return
	}
	from := ""
	if entries := a.selectedBatchEntries(); len(entries) > 0 {
		from = entries[0].Folder
	}
	if from == "" && len(options) == 1 {
		from = options[0]
	}
	if !a.batch.prefixInitialized || a.batch.prefixTo != "" && a.batch.prefixTo == a.batch.prefixFrom {
		a.batch.prefixTo = from
	}
	a.batch.prefixFrom, a.batch.prefixInitialized = from, true
}

func (a *application) chooseBatchPrefix(from string) {
	if a.busy || !containsFolder(a.batchPrefixOptions(), from) {
		return
	}
	if a.batch.prefixTo != "" && a.batch.prefixTo == a.batch.prefixFrom {
		a.batch.prefixTo = from
	}
	a.batch.prefixFrom, a.batch.prefixInitialized = from, true
	a.batch.result = nil
	a.lastInput = time.Now()
}

func (a *application) copyBatchPrefix() {
	if a.busy || a.closing || a.state.Locked && !a.state.Settings.BrowseWhileLocked || !containsFolder(a.batchPrefixOptions(), a.batch.prefixFrom) {
		return
	}
	a.notice, a.errorText = "", ""
	if err := a.writeClipboard(a.batch.prefixFrom, 0, false); err != nil {
		a.errorText = a.t(i18n.ClipboardWriteFailed)
		return
	}
	a.notice = a.t("已复制")
}

func (a *application) batchPrefixValid() bool {
	from, to := strings.TrimSpace(a.batch.prefixFrom), strings.TrimSpace(a.batch.prefixTo)
	return from != "" && to != "" && vault.ValidatePath(from) == nil && vault.ValidatePath(to) == nil
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

func (a *application) batchPrefixView(c *ui.Context, ready, conflicts, unchanged []app.Entry) {
	muted(c, a.t("仅替换完整文件夹前缀，保留后续层级和条目名称。"))
	options := a.batchPrefixOptions()
	components.Text(c, a.t("原文件夹前缀")).Margin(14, 0, 6, 0).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
	ui.Row(c).Key("prefix-source").FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		if len(options) > 1 {
			selected := a.batch.prefixFrom
			sel := components.Select(c, &selected)
			sel.Trigger.Label(a.t("原文件夹前缀")).Grow(1).MinWidth(0).Disabled(a.busy).Children(func() {
				label := selected
				if label == "" {
					label = a.t("请选择原文件夹前缀。")
				}
				components.Text(c, label).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(scaledFontSize(c, 12))
			})
			sel.Popup(func(panel ui.Element) {
				panel.Width(320).MaxWidth(420).Label(a.t("原文件夹前缀"))
				components.ScrollView(c, components.ScrollViewOptions{}).FillWidth().MaxHeight(240).Children(func() {
					for _, folder := range options {
						sel.Item(folder).Label(a.t("选择原文件夹前缀 %s", folder)).Children(func() { components.Text(c, folder).SingleLine() })
					}
				})
			})
			if sel.Trigger.Changed() {
				a.chooseBatchPrefix(selected)
			}
		} else {
			label := a.batch.prefixFrom
			if label == "" {
				label = a.t("所选条目没有可替换的文件夹前缀。")
			}
			components.Text(c, label).Grow(1).MinWidth(0).Wrap().FontSize(scaledFontSize(c, 12))
		}
		if iconButton(c, lucide.Copy(), a.t("复制原文件夹前缀"), components.ButtonGhost).Tooltip(a.t("复制原文件夹前缀")).Size(28, 28).Disabled(a.busy || a.batch.prefixFrom == "").Clicked() {
			a.copyBatchPrefix()
		}
	})
	components.Text(c, a.t("新文件夹前缀")).Margin(14, 0, 6, 0).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
	c.Key("prefix-to")
	if textBox(c, &a.batch.prefixTo, components.TextBoxOptions{}).FillWidth().Label(a.t("新文件夹前缀")).Placeholder(a.t("输入新的文件夹前缀")).Disabled(a.busy).Changed() {
		a.lastInput = time.Now()
	}
	if !a.batchPrefixValid() {
		message := a.t("新文件夹前缀无效，请使用 / 分隔层级，并避免空名称和特殊字符。")
		if len(options) == 0 {
			message = a.t("所选条目没有可替换的文件夹前缀。")
		} else if a.batch.prefixFrom == "" {
			message = a.t("请选择原文件夹前缀。")
		} else if strings.TrimSpace(a.batch.prefixTo) == "" {
			message = a.t("请输入新文件夹前缀。")
		}
		a.batchNotice(c, false, message)
		return
	}
	components.Text(c, a.t("路径预览")).Margin(18, 0, 8, 0).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
	components.Text(c, a.t("将修改 %d 项，%d 项不修改", len(ready), len(conflicts)+len(unchanged))).FontSize(scaledFontSize(c, 12)).Bold()
	if len(conflicts)+len(unchanged) == 0 {
		muted(c, a.t("所有选中条目都会修改。"))
		return
	}
	components.Text(c, a.t("不修改原因")).Margin(14, 0, 8, 0).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
	var mismatched []app.Entry
	for _, entry := range unchanged {
		if entry.Folder != a.batch.prefixFrom && !strings.HasPrefix(entry.Folder, a.batch.prefixFrom+"/") {
			mismatched = append(mismatched, entry)
		}
	}
	components.ScrollView(c, components.ScrollViewOptions{}).Key("batch-prefix-reasons").Label(a.t("不修改原因")).FillWidth().Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Gap(12).Children(func() {
			for _, reason := range []struct {
				key, message string
				count        int
			}{
				{"conflict", "目标路径已存在或重名：%d 项", len(conflicts)},
				{"mismatch", "前缀不匹配：%d 项", len(mismatched)},
				{"unchanged", "路径未变化：%d 项", len(unchanged) - len(mismatched)},
			} {
				if reason.count == 0 {
					continue
				}
				ui.Column(c).Key(reason.key).FillWidth().Gap(6).Children(func() {
					components.Text(c, a.t(reason.message, reason.count)).FillWidth().Wrap().FontSize(scaledFontSize(c, 12))
					if reason.key == "mismatch" {
						ui.Column(c).FillWidth().Margin(0, 0, 0, 12).Gap(4).Children(func() {
							for _, entry := range mismatched {
								components.Text(c, entry.Path).Key(entry.Path).FillWidth().Wrap().FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
							}
						})
					}
				})
			}
		})
	})
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
