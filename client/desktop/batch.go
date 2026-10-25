package desktop

import (
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/vault"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// batchState is the list's selection mode. Selection holds entry paths, never
// secrets, and moving or trashing reads nothing beyond the latest revision.
type batchState struct {
	active            bool
	selected          map[string]bool
	anchor            string
	destination       string
	chosen            bool
	folderQuery       string
	folderCreation    *string
	revealFolder      string
	expanded          map[string]bool
	moveMode          string
	prefixInitialized bool
	prefixFrom        string
	prefixTo          string
	list              components.ListState
	progress          *batchProgress
	result            *batchResult
	// pending is the organization action waiting for the vault to unlock; selection
	// itself needs only entry paths, so it works while browsing locked.
	pending string
}

type batchProgress struct {
	cancelled   atomic.Bool
	action      string
	done, total int
}

type batchFailure struct {
	entry, destination, code string
}

// batchResult keeps unfinished entries of the last operation for retrying.
type batchResult struct {
	action, destination string
	prefix              bool
	completed           int
	failures            []batchFailure
}

func (f batchFailure) retryable() bool {
	return f.code != "source_remains" && f.code != "already_exists"
}

func (a *application) enterBatch() {
	if a.editor.open || a.state.Locked && !a.state.Settings.BrowseWhileLocked {
		return
	}
	a.batch = batchState{active: true, selected: map[string]bool{}, expanded: map[string]bool{}}
	a.dropDetail()
}

func (a *application) exitBatch() {
	if a.batch.progress != nil {
		return
	}
	if a.pendingUnlock.action == "batch" {
		a.pendingUnlock = unlockAction{}
	}
	a.batch = batchState{}
}

// requestBatchUnlock keeps the selection and destination, and runs the action
// once the vault unlocks. Cancelling returns to the selection panel.
func (a *application) requestBatchUnlock(action string) {
	if a.busy {
		return
	}
	a.batch.pending = action
	a.beginUnlock("batch")
}

func (a *application) cancelBatchUnlock() {
	a.batch.pending = ""
	if a.pendingUnlock.action == "batch" {
		a.pendingUnlock = unlockAction{}
	}
}

func (a *application) resumeBatchAction() bool {
	action := a.batch.pending
	a.batch.pending = ""
	if !a.batch.active || a.page != "entries" {
		return false
	}
	switch action {
	case "move":
		a.startBatchMove()
	case "trash":
		a.startBatchTrash()
	case "retry":
		a.retryBatch()
	case "create-folder":
		a.createBatchFolder()
	default:
		return false
	}
	return true
}

// batchUnlockDescription tells what unlocking will do, next to the unlock form.
func (a *application) batchUnlockDescription() string {
	switch a.batch.pending {
	case "create-folder":
		return a.t("解锁后将新建文件夹“%s”。", strings.TrimSpace(a.batch.folderQuery))
	case "trash":
		return a.t("解锁后将 %d 项移到回收站。", len(a.selectedBatchEntries()))
	case "retry":
		retryable := 0
		if a.batch.result != nil {
			for _, failure := range a.batch.result.failures {
				if failure.retryable() {
					retryable++
				}
			}
		}
		return a.t("解锁后将重试 %d 项。", retryable)
	}
	ready, _, _ := a.batchMovePlan()
	if a.batch.moveMode == "prefix" {
		return a.t("解锁后将修改 %d 项的文件夹前缀。", len(ready))
	}
	return a.t("解锁后将 %d 项移动到“%s”。", len(ready), a.batchFolderLabel(a.batch.destination))
}

func (a *application) resetBatchDestination() {
	a.batch.destination, a.batch.chosen, a.batch.folderQuery = "", false, ""
}

func (a *application) createBatchFolder() {
	if a.busy || !a.batch.active || a.closing || a.updates.preparing {
		return
	}
	folder := strings.TrimSpace(a.batch.folderQuery)
	if vault.ValidatePath(folder) != nil {
		a.errorText = a.t("文件夹路径无效，请使用 / 分隔层级，并避免空名称和特殊字符。")
		return
	}
	if a.state.Locked {
		a.requestBatchUnlock("create-folder")
		return
	}
	// The request belongs to this selection session, even if the user leaves
	// and re-enters selection mode before it completes.
	a.batch.folderCreation = &folder
	a.call(app.Request{Action: "create-folder", VaultID: a.state.VaultID, Destination: folder}, func(data app.Data) {
		if !a.batch.active || a.page != "entries" || a.batch.folderCreation != &folder {
			return
		}
		a.batch.folderCreation = nil
		a.chooseBatchFolder(data.Destination)
		a.batch.revealFolder = data.Destination
		a.batch.result = nil
		// A new descendant must remain visible even under a collapsed parent.
		parts := strings.Split(data.Destination, "/")
		for i := 1; i < len(parts); i++ {
			a.batch.expanded[strings.Join(parts[:i], "/")] = true
		}
		a.notice = a.t("已创建并选中文件夹“%s”", data.Destination)
	})
}

// selectedBatchEntries returns the chosen entries that still exist, so entries
// removed by a sync or the browser simply drop out of the selection.
func (a *application) selectedBatchEntries() []app.Entry {
	entries := make([]app.Entry, 0, len(a.batch.selected))
	a.entries()
	for _, entry := range a.sortedEntries {
		if a.batch.selected[entry.Path] {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (a *application) toggleBatchEntry(path string, mods ui.Modifiers) {
	if a.batch.progress != nil {
		return
	}
	a.batch.result = nil
	if mods&ui.Shift != 0 && a.batch.anchor != "" {
		from, to := -1, -1
		for i, entry := range a.filtered {
			if entry.Path == a.batch.anchor {
				from = i
			}
			if entry.Path == path {
				to = i
			}
		}
		if from >= 0 && to >= 0 {
			for i := min(from, to); i <= max(from, to); i++ {
				a.batch.selected[a.filtered[i].Path] = true
			}
			return
		}
	}
	if a.batch.selected[path] {
		delete(a.batch.selected, path)
	} else {
		a.batch.selected[path] = true
	}
	a.batch.anchor = path
}

// toggleBatchAll selects every visible entry, or clears them when all are chosen.
func (a *application) toggleBatchAll() {
	all := len(a.filtered) > 0
	for _, entry := range a.filtered {
		all = all && a.batch.selected[entry.Path]
	}
	for _, entry := range a.filtered {
		if all {
			delete(a.batch.selected, entry.Path)
		} else {
			a.batch.selected[entry.Path] = true
		}
	}
	a.batch.result = nil
}

func batchDestinationPath(folder, name string) string {
	if folder == "" {
		return name
	}
	return folder + "/" + name
}

// batchMovePlan mirrors the service's refusal to overwrite: entries whose new
// path already exists, or that would collide with each other, are skipped.
func (a *application) batchMovePlan() (ready, conflicts, unchanged []app.Entry) {
	if !a.batchMoveConfigured() {
		return nil, nil, nil
	}
	chosen := a.selectedBatchEntries()
	targets := map[string]int{}
	for _, entry := range chosen {
		targets[strings.ToLower(a.batchEntryDestination(entry))]++
	}
	for _, entry := range chosen {
		if entry.Path == a.batchEntryDestination(entry) {
			unchanged = append(unchanged, entry)
			continue
		}
		target := strings.ToLower(a.batchEntryDestination(entry))
		exists := false
		for _, other := range a.sortedEntries {
			if other.Path != entry.Path && strings.ToLower(other.Path) == target {
				exists = true
				break
			}
		}
		if exists || targets[target] > 1 {
			conflicts = append(conflicts, entry)
		} else {
			ready = append(ready, entry)
		}
	}
	return ready, conflicts, unchanged
}

// runBatch sends one request per entry so progress is visible, then refreshes
// the list once. It stops when the session or vault changes underneath it.
func (a *application) runBatch(action string, requests []app.Request, done func([]app.Response)) {
	if a.busy || a.closing || len(requests) == 0 || a.updates.preparing {
		return
	}
	a.busy = true
	a.activeRequestAction = action
	a.beginOperationFeedback()
	a.lastInput = time.Now()
	a.batch.progress = &batchProgress{action: action, total: len(requests)}
	progress, vaultID := a.batch.progress, a.state.VaultID
	generation := a.generation
	go func() {
		responses := make([]app.Response, 0, len(requests))
		for _, request := range requests {
			if progress.cancelled.Load() {
				break
			}
			r := a.service.Execute(request)
			responses = append(responses, r)
			a.runOnMain(func() {
				if generation == a.generation && a.batch.progress != nil {
					a.batch.progress.done++
					a.invalidate()
				}
			})
			if r.Code == "locked" || r.Code == "vault_changed" || r.Code == "not_configured" {
				break
			}
		}
		if progress.cancelled.Load() && len(responses) < len(requests) {
			responses = append(responses, app.Response{Status: "error", Code: "locked"})
		}
		list := a.service.Execute(app.Request{Action: "list"})
		a.runOnMain(func() {
			if a.batch.progress != progress {
				return
			}
			a.batch.progress = nil
			if generation != a.generation {
				// Locking cancels further writes, but the worker must drain before
				// another action can start. Its paths/result still belong to this
				// selection; never apply metadata from another vault or unlock a
				// newer locked UI with a snapshot taken just before the lock.
				if !progress.cancelled.Load() || a.state.VaultID != vaultID || list.Data.VaultID != vaultID {
					a.busy, a.backgroundBusy = false, false
					a.activeRequestAction = ""
					a.queueMetadataRefresh(false)
					return
				}
				list.Data.Locked = a.state.Locked || list.Data.Locked
				if list.Data.Locked && !list.Data.Settings.BrowseWhileLocked {
					list.Data.Entries, list.Data.Folders = nil, nil
					a.cacheReady = false
				}
				generation = a.generation
			}
			a.receiveResponse(generation, list, func(app.Data) { done(responses) })
			// Completion messages can include folder paths. A lock that hides
			// names must also suppress feedback produced by these late callbacks.
			if a.state.Locked && !a.state.Settings.BrowseWhileLocked {
				a.clearNotifications()
			}
		})
	}()
	a.invalidate()
}

func (a *application) startBatchMove() {
	if a.busy {
		return
	}
	ready, conflicts, unchanged := a.batchMovePlan()
	if len(ready) == 0 {
		return
	}
	if a.state.Locked {
		a.requestBatchUnlock("move")
		return
	}
	destination := a.batch.destination
	prefix := a.batch.moveMode == "prefix"
	if prefix {
		destination = strings.TrimSpace(a.batch.prefixTo)
	}
	requests := make([]app.Request, 0, len(ready))
	for _, entry := range ready {
		requests = append(requests, app.Request{Action: "move", VaultID: a.state.VaultID, Entry: entry.Path, Destination: a.batchEntryDestination(entry)})
	}
	a.runBatch("move", requests, func(responses []app.Response) {
		result := batchResult{action: "move", destination: destination, prefix: prefix}
		selected := map[string]bool{}
		for _, entry := range conflicts {
			selected[entry.Path] = true
		}
		if prefix {
			for _, entry := range unchanged {
				selected[entry.Path] = true
			}
		}
		for i, request := range requests {
			r := batchResponse(responses, i)
			if r.Status == "ok" {
				result.completed++
				continue
			}
			result.failures = append(result.failures, batchFailure{entry: request.Entry, destination: request.Destination, code: r.Code})
			selected[request.Entry] = true
		}
		a.finishBatch(result, selected)
	})
}

func (a *application) startBatchTrash() {
	if a.busy {
		return
	}
	if a.state.Locked {
		a.requestBatchUnlock("trash")
		return
	}
	chosen := a.selectedBatchEntries()
	requests := make([]app.Request, 0, len(chosen))
	for _, entry := range chosen {
		requests = append(requests, app.Request{Action: "trash", VaultID: a.state.VaultID, Entry: entry.Path})
	}
	a.runBatch("trash", requests, func(responses []app.Response) {
		result := batchResult{action: "trash"}
		selected := map[string]bool{}
		undo := []string{}
		for i := range requests {
			r := batchResponse(responses, i)
			if r.Status == "ok" {
				result.completed++
				undo = append(undo, r.Data.TrashID)
				continue
			}
			result.failures = append(result.failures, batchFailure{entry: requests[i].Entry, code: r.Code})
			selected[requests[i].Entry] = true
		}
		a.finishBatch(result, selected)
		if len(result.failures) == 0 && len(undo) > 0 {
			a.notifyWithAction(a.t("已将 %d 项移到回收站", result.completed), a.t("恢复副本仅保存在此设备"), a.t("撤销"), 8*time.Second, func() { a.undoBatchTrash(undo) })
		}
	})
}

// finishBatch keeps skipped and failed entries selected, so they can be handled
// next; a fully completed operation leaves selection mode with a toast.
func (a *application) finishBatch(result batchResult, remaining map[string]bool) {
	a.resetBatchDestination()
	if len(result.failures) > 0 {
		a.batch.selected, a.batch.result = remaining, &result
		return
	}
	if len(remaining) > 0 {
		a.batch.selected = remaining
	} else {
		a.batch = batchState{}
	}
	if result.action == "move" && result.completed > 0 {
		destination := result.destination
		a.notifyWithAction(a.batchMoveSuccess(result, result.completed), "", a.t("查看"), 0, func() {
			filter := "all"
			if destination != "" {
				filter = "folder:" + destination
			}
			a.exitBatch()
			a.navigateManager(filter)
		})
	}
}

func (a *application) retryBatch() {
	result := a.batch.result
	if result == nil || result.action != "move" || a.busy {
		return
	}
	if a.state.Locked {
		a.requestBatchUnlock("retry")
		return
	}
	requests := []app.Request{}
	for _, failure := range result.failures {
		if failure.retryable() {
			requests = append(requests, app.Request{Action: "move", VaultID: a.state.VaultID, Entry: failure.entry, Destination: failure.destination})
		}
	}
	a.runBatch("move", requests, func(responses []app.Response) {
		failures := []batchFailure{}
		for _, failure := range result.failures {
			if !failure.retryable() {
				failures = append(failures, failure)
			}
		}
		completed := 0
		for i := range requests {
			r := batchResponse(responses, i)
			if r.Status == "ok" {
				completed++
				delete(a.batch.selected, requests[i].Entry)
				continue
			}
			failures = append(failures, batchFailure{entry: requests[i].Entry, destination: requests[i].Destination, code: r.Code})
		}
		result.completed += completed
		result.failures = failures
		if len(failures) == 0 {
			a.batch.result = nil
		}
		if completed > 0 {
			a.notice = a.batchMoveSuccess(*result, completed)
		}
	})
}

func (a *application) undoBatchTrash(ids []string) {
	requests := make([]app.Request, 0, len(ids))
	for _, id := range ids {
		requests = append(requests, app.Request{Action: "restore", VaultID: a.state.VaultID, ID: id})
	}
	a.runBatch("restore", requests, func(responses []app.Response) {
		restored := map[string]bool{}
		for _, r := range responses {
			if r.Status == "ok" && r.Data.Entry != "" {
				restored[r.Data.Entry] = true
			}
		}
		if len(restored) > 0 {
			a.batch = batchState{active: true, selected: restored, expanded: map[string]bool{}}
			a.dropDetail()
		}
		if len(restored) < len(ids) {
			a.errorText = a.t("部分条目未能还原，可以在回收站中查看。")
			return
		}
		a.notice = a.t("已撤销")
	})
}

func (a *application) batchFolderLabel(folder string) string {
	if folder == "" {
		return a.t("密码库根目录")
	}
	return folder
}

func (a *application) batchFailureReason(code string) string {
	switch code {
	case "source_remains":
		return a.t("已复制到目标文件夹，但原条目仍在。请核对后删除多余的一份。")
	case "already_exists":
		return a.t("目标文件夹已有同名条目。")
	case "vault_busy":
		return a.t("密码库正在同步，可以重试。")
	case "revision_conflict":
		return a.t("条目刚被其他操作修改，可以重试。")
	case "locked":
		return a.t("密码库已锁定，解锁后可以重试。")
	}
	return a.t("操作未完成，可以重试。")
}

// batchNames lists up to three distinct names; entry names are vault data and
// only the separator and the overflow template come from the catalog.
func (a *application) batchNames(entries []app.Entry) string {
	names := []string{}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !seen[entry.Name] {
			seen[entry.Name] = true
			names = append(names, entry.Name)
		}
	}
	separator := a.t("、")
	if len(names) <= 3 {
		return strings.Join(names, separator)
	}
	return a.t("%s 等 %d 项", strings.Join(names[:2], separator), len(names)-2)
}

// batchCheck draws the check box shared by entry rows and "select all".
func batchCheck(c *ui.Context, checked, mixed bool) {
	t := c.Theme()
	box := ui.Box(c).Size(16, 16).Radius(4).Shrink(0).PassThrough()
	if checked || mixed {
		box.Background(t.Accent)
	} else {
		box.Background(t.Surface).Border(1, t.Border.Mix(t.Text, 0.25))
	}
	box.DrawOver(func(p *ui.Painter, r ui.Rect) {
		var path ui.Path
		switch {
		case checked:
			path.MoveTo(r.X+r.W*0.22, r.Y+r.H*0.52).LineTo(r.X+r.W*0.42, r.Y+r.H*0.71).LineTo(r.X+r.W*0.78, r.Y+r.H*0.31)
		case mixed:
			path.MoveTo(r.X+r.W*0.25, r.Y+r.H*0.5).LineTo(r.X+r.W*0.75, r.Y+r.H*0.5)
		default:
			return
		}
		p.StrokePath(&path, 2, t.AccentText)
	})
}

func (a *application) batchSelectAllRow(c *ui.Context) {
	all, some := len(a.filtered) > 0, false
	for _, entry := range a.filtered {
		all = all && a.batch.selected[entry.Path]
		some = some || a.batch.selected[entry.Path]
	}
	checked := all
	c.Key("batch-select-all")
	row := ui.CheckboxBase(c, &checked).Label(a.t("全选")).Tooltip(a.t("全选当前列表")).
		Padding(4, 9).Gap(12).AlignItems(ui.Center).Radius(c.Theme().Radius).Disabled(a.busy || len(a.filtered) == 0)
	row.Children(func() {
		batchCheck(c, all, some && !all)
		components.Text(c, a.t("全选")).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
	})
	if row.Changed() {
		a.toggleBatchAll()
	}
}

func (a *application) batchEntryRow(c *ui.Context, entry app.Entry) {
	t := c.Theme()
	checked := a.batch.selected[entry.Path]
	c.Key(entry.Path)
	row := ui.CheckboxBase(c, &checked).Label(a.t("选择条目 %s", entry.Path)).FillWidth().MinHeight(60).
		Padding(8, 9).Gap(12).AlignItems(ui.Center).Radius(t.Radius).Disabled(a.busy)
	if a.batch.selected[entry.Path] {
		row.Background(t.Selection)
	} else if row.Hovered() {
		row.Background(t.Text.Alpha(.04))
	}
	row.Children(func() {
		batchCheck(c, a.batch.selected[entry.Path], false)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			components.Text(c, entry.Name).SingleLine().Ellipsis("…").FontSize(scaledFontSize(c, 13)).Bold()
			components.Text(c, entry.Path).SingleLine().Ellipsis("…").FontSize(scaledFontSize(c, 11)).TextColor(t.TextMuted)
		})
	})
	if row.Changed() {
		a.toggleBatchEntry(entry.Path, row.ClickModifiers())
	}
}

// batchView presents peer organization actions, each with its own form and
// execution button below the shared selection.
func (a *application) batchView(c *ui.Context) {
	if a.batch.result != nil {
		a.batchResultView(c)
		return
	}
	chosen := a.selectedBatchEntries()
	progress := a.batch.progress
	if len(chosen) == 0 && progress == nil {
		ui.Column(c).Fill().Padding(24).Children(func() {
			ui.Box(c).MinHeight(32)
			ui.Column(c).Margin(20, 0, 0, 0).Gap(6).Children(func() {
				components.Text(c, a.t("选择条目")).FontSize(scaledFontSize(c, 24)).Bold()
				muted(c, a.t("勾选左侧条目后，可以移动到文件夹、修改路径前缀或移到回收站。"))
			})
		})
		return
	}
	count := len(chosen)
	if progress != nil {
		count = progress.total
	}
	ui.Column(c).Key("batch-panel").Fill().MaxWidth(608).Padding(24).Children(func() {
		ui.Box(c).MinHeight(32)
		components.Text(c, a.t("已选 %d 项", count)).Margin(20, 0, 0, 0).FillWidth().FontSize(scaledFontSize(c, 24)).Bold()
		ui.Row(c).Key("batch-actions").Label(a.t("批量操作")).FillWidth().Wrap().Margin(16, 0, 0, 0).AlignItems(ui.Center).Gap(8).Children(func() {
			for _, action := range []struct {
				key, label string
				icon       *ui.SVG
				prefix     bool
			}{{"move", a.t("移动到"), lucide.FolderInput(), false}, {"prefix", a.t("修改前缀"), lucide.Replace(), true}} {
				c.Key(action.key)
				selected := (a.batch.moveMode == "prefix") == action.prefix
				if a.batchActionButton(c, action.label, action.icon, selected, false).Disabled(a.busy).Clicked() {
					a.setBatchMoveMode(action.prefix)
				}
			}
			c.Key("trash")
			trashLabel := a.t("移到回收站")
			trashing := progress != nil && progress.action == "trash"
			if trashing {
				trashLabel = a.t("正在移到回收站 %d/%d", progress.done, progress.total)
			}
			if a.batchActionButton(c, trashLabel, lucide.Trash(), false, true).Disabled(a.busy && !trashing).Clicked() {
				a.startBatchTrash()
			}
		})
		ready, conflicts, unchanged := a.batchMovePlan()
		ui.Column(c).Key("batch-operation").FillWidth().Grow(1).MinHeight(0).Margin(24, 0, 0, 0).Children(func() {
			ui.Row(c).Key("operation-heading").FillWidth().Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(12).Margin(0, 0, 14, 0).Children(func() {
				title := a.t("移动到")
				if a.batch.moveMode == "prefix" {
					title = a.t("修改前缀")
				}
				components.Text(c, title).FontSize(scaledFontSize(c, 14)).Bold()
				label := a.t("移动 %d 项", len(chosen))
				if a.batchMoveConfigured() {
					label = a.t("移动 %d 项", len(ready))
				}
				if a.batch.moveMode == "prefix" {
					label = a.t("修改 %d 项", len(ready))
				}
				moving := progress != nil && progress.action == "move"
				if moving {
					label = a.t("正在移动 %d/%d", progress.done, progress.total)
					if a.batch.moveMode == "prefix" {
						label = a.t("正在修改 %d/%d", progress.done, progress.total)
					}
				}
				// The running action keeps full contrast so its progress stays readable;
				// startBatchMove ignores clicks while busy.
				if components.Button(c, label, components.ButtonPrimary).Disabled(!moving && (a.busy || !a.batchMoveConfigured() || len(ready) == 0)).Clicked() {
					a.startBatchMove()
				}
			})
			if a.batch.moveMode == "prefix" {
				a.batchPrefixView(c, chosen, ready, conflicts, unchanged)
				return
			}
			search := textBox(c, &a.batch.folderQuery, components.TextBoxOptions{LeadingIcon: lucide.Search()}).FillWidth().Label(a.t("搜索目标文件夹")).Placeholder(a.t("搜索或新建文件夹")).Disabled(a.busy)
			if search.Changed() {
				a.syncBatchFolderQuery()
				a.lastInput = time.Now()
			}
			components.ScrollView(c, components.ScrollViewOptions{}).Key("batch-folders").Label(a.t("选择目标文件夹")).FillWidth().Grow(1).MinHeight(160).Margin(8, 0, 0, 0).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					a.batchFolderOptions(c)
				})
			})
			if a.batch.chosen {
				if len(conflicts) > 0 {
					a.batchNotice(c, true, a.t("%s 移到“%s”后会重名，将跳过。", a.batchNames(conflicts), a.batchFolderLabel(a.batch.destination)))
				}
				if len(unchanged) > 0 {
					a.batchNotice(c, false, a.t("%d 项已在此文件夹中。", len(unchanged)))
				}
				if len(ready) == 0 {
					a.batchNotice(c, false, a.t("没有可移动的条目，请换一个文件夹。"))
				}
			}
		})
	})
}

func (a *application) batchActionButton(c *ui.Context, label string, icon *ui.SVG, selected, danger bool) ui.Element {
	color := c.Theme().Text
	if danger {
		color = c.Theme().Danger
	}
	button := components.Button(c, "", components.ButtonGhost).Label(label).Selected(selected).MinHeight(34).Padding(6, 10).Gap(6).AlignItems(ui.Center)
	if selected {
		button.Background(c.Theme().Selection)
	}
	button.Children(func() {
		components.Icon(c, icon).Size(managerGlyphSize(c), managerGlyphSize(c)).TextColor(color)
		components.Text(c, label).FontSize(scaledFontSize(c, 12)).TextColor(color).SingleLine()
	})
	return button
}

func (a *application) batchNotice(c *ui.Context, warning bool, text string) {
	t := c.Theme()
	color, icon := t.TextMuted, lucide.Info()
	if warning {
		color, icon = t.Warning, lucide.TriangleAlert()
	}
	ui.Row(c).FillWidth().Margin(10, 0, 0, 0).Padding(10, 12).Gap(8).AlignItems(ui.Start).Radius(t.Radius).Background(color.Alpha(.10)).Children(func() {
		components.Icon(c, icon).Size(14, 14).Shrink(0).TextColor(color)
		components.Text(c, text).Grow(1).MinWidth(0).Wrap().FontSize(scaledFontSize(c, 11)).TextColor(t.Text)
	})
}

// batchFolderOptions shows an indented tree, or full paths while searching,
// since a deep match needs its parent folders to be told apart.
func (a *application) batchFolderOptions(c *ui.Context) {
	query := strings.ToLower(strings.TrimSpace(a.batch.folderQuery))
	folders := append([]string{""}, a.folders...)
	if query != "" {
		matches := 0
		for _, folder := range folders {
			if strings.Contains(strings.ToLower(a.batchFolderLabel(folder)), query) {
				matches++
				a.batchFolderOption(c, folder, a.batchFolderLabel(folder), 0, false)
			}
		}
		candidate := strings.TrimSpace(a.batch.folderQuery)
		if candidate != "" && !containsFolder(a.folders, candidate) {
			option := ui.ButtonBase(c).Key("batch-create-folder").FillWidth().MinHeight(34).Padding(6, 9).Gap(8).AlignItems(ui.Center).Radius(c.Theme().Radius).Label(a.t("新建文件夹 %s", candidate)).Disabled(a.busy)
			option.Children(func() {
				components.Icon(c, lucide.FolderPlus()).Size(14, 14).Shrink(0)
				components.Text(c, a.t("新建文件夹：%s", candidate)).SingleLine().Ellipsis("…").MinWidth(0).FontSize(scaledFontSize(c, 12))
			})
			if option.Clicked() {
				a.createBatchFolder()
			}
		} else if matches == 0 {
			muted(c, a.t("没有找到文件夹"))
		}
		return
	}
	children := map[string]int{}
	for _, folder := range a.folders {
		if index := strings.LastIndex(folder, "/"); index >= 0 {
			children[folder[:index]]++
		}
	}
	a.batchFolderOption(c, "", a.batchFolderLabel(""), 0, false)
	for _, folder := range a.folders {
		hidden := false
		parts := strings.Split(folder, "/")
		for i := 1; i < len(parts); i++ {
			hidden = hidden || !a.batch.expanded[strings.Join(parts[:i], "/")]
		}
		if hidden {
			continue
		}
		a.batchFolderOption(c, folder, parts[len(parts)-1], len(parts)-1, children[folder] > 0)
	}
}

func (a *application) batchFolderOption(c *ui.Context, folder, label string, depth int, collapsible bool) {
	t := c.Theme()
	chosen := a.batch.chosen && a.batch.destination == folder
	ui.Row(c).Key("folder:"+folder).FillWidth().Padding(0, 0, 0, float32(depth*20)).Gap(2).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(20, 28).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			if !collapsible {
				return
			}
			open := a.batch.expanded[folder]
			icon, toggleLabel := lucide.ChevronDown(), a.t("收起“%s”", folder)
			if !open {
				icon, toggleLabel = lucide.ChevronRight(), a.t("展开“%s”", folder)
			}
			toggle := ui.ButtonBase(c).Key("toggle").Size(20, 28).AlignItems(ui.Center).Justify(ui.Center).Radius(4).Label(toggleLabel).Expanded(open).Disabled(a.busy)
			toggle.Children(func() { components.Icon(c, icon).Size(12, 12).TextColor(t.TextMuted) })
			if toggle.Clicked() {
				if open {
					delete(a.batch.expanded, folder)
				} else {
					a.batch.expanded[folder] = true
				}
			}
		})
		option := ui.ButtonBase(c).Key("option").Grow(1).MinWidth(0).MinHeight(34).Padding(6, 9).Gap(8).AlignItems(ui.Center).Radius(t.Radius).
			Label(a.t("选择文件夹 %s", a.batchFolderLabel(folder))).Selected(chosen).Disabled(a.busy).TextColor(t.Text)
		if chosen {
			option.Background(t.Selection)
			if a.batch.revealFolder == folder {
				option.ScrollIntoView()
				a.batch.revealFolder = ""
			}
		} else if option.Hovered() {
			option.Background(t.Text.Alpha(.04))
		}
		option.Children(func() {
			components.Icon(c, lucide.Folder()).Size(14, 14).Shrink(0).TextColor(t.TextMuted)
			text := components.Text(c, label).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(scaledFontSize(c, 12)).TextColor(t.Text)
			if chosen {
				text.Bold()
				components.Icon(c, lucide.Check()).Size(14, 14).Shrink(0).TextColor(t.Text)
			}
		})
		if option.Clicked() {
			a.chooseBatchFolder(folder)
		}
	})
}

func (a *application) batchResultView(c *ui.Context) {
	result := a.batch.result
	retryable := false
	for _, failure := range result.failures {
		retryable = retryable || failure.retryable() && result.action == "move"
	}
	failures := append([]batchFailure(nil), result.failures...)
	sort.SliceStable(failures, func(i, j int) bool { return failures[i].entry < failures[j].entry })
	components.ScrollView(c, components.ScrollViewOptions{}).Key("batch-result").Fill().Children(func() {
		ui.Column(c).FillWidth().MaxWidth(608).Padding(24).Children(func() {
			ui.Box(c).MinHeight(32)
			components.Icon(c, lucide.TriangleAlert()).Margin(20, 0, 12, 0).Size(22, 22).TextColor(c.Theme().TextMuted)
			title := a.t("%d 项已移动，%d 项未完成", result.completed, len(failures))
			if result.action == "trash" {
				title = a.t("%d 项已移到回收站，%d 项未完成", result.completed, len(failures))
			}
			components.Text(c, title).FillWidth().Wrap().FontSize(scaledFontSize(c, 24)).Bold()
			ui.Column(c).FillWidth().Margin(20, 0, 0, 0).Dividers(1, c.Theme().Border).BorderWidth(1, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
				for _, failure := range failures {
					ui.Column(c).Key(failure.entry).FillWidth().Padding(14, 0).Gap(4).Children(func() {
						components.Text(c, entryName(failure.entry)).FontSize(scaledFontSize(c, 13)).Bold()
						components.Text(c, failure.entry).Wrap().FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
						components.Text(c, a.batchFailureReason(failure.code)).Margin(4, 0, 0, 0).Wrap().FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
					})
				}
			})
			ui.Row(c).Margin(20, 0, 0, 0).Gap(8).Children(func() {
				if retryable && components.Button(c, a.t("重试"), components.ButtonPrimary).Disabled(a.busy).Clicked() {
					a.retryBatch()
				}
				variant := components.ButtonPrimary
				if retryable {
					variant = components.ButtonOutline
				}
				if components.Button(c, a.t("完成"), variant).Disabled(a.busy).Clicked() {
					a.exitBatch()
				}
			})
		})
	})
}
