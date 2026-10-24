package desktop

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func (a *application) cleanupEmptyFolders() {
	if a.busy || a.closing || a.editor.open || a.batch.active || a.updates.preparing {
		return
	}
	a.closeNavigationOverlay()
	if a.state.Locked {
		a.beginUnlock("cleanup-folders")
		return
	}
	a.call(app.Request{Action: "cleanup-folders", VaultID: a.state.VaultID}, func(data app.Data) {
		a.refreshFolderNavigation()
		if data.RemovedFolders == 0 {
			a.inform(a.t("没有需要清理的空文件夹"))
			return
		}
		a.notice = a.t("已清理 %d 个空文件夹", data.RemovedFolders)
	})
}

func (a *application) refreshFolderNavigation() {
	a.entries()
	if strings.HasPrefix(a.filter, "folder:") && !containsFolder(a.folders, strings.TrimPrefix(a.filter, "folder:")) {
		// Refresh the retained filter without replacing navigation made while
		// cleanup was pending or cancelling that destination's queued reads.
		if a.page != "entries" || a.setup {
			a.filter = "all"
			return
		}
		a.navigateManager("all")
	}
}

func (a *application) folderCleanupButton(c *ui.Context) {
	tooltip := a.t("清理密码库中真正为空的文件夹，保留所有文件及隐藏目录。")
	if a.editor.open || a.batch.active {
		tooltip = a.t("请先完成编辑或退出选择，再清理空文件夹。")
	}
	if iconButton(c, lucide.BrushCleaning(), a.t("清理空文件夹"), components.ButtonGhost).
		Size(24, 24).Padding(5).Tooltip(tooltip).
		Disabled(a.busy && !a.detailReadPending() || a.editor.open || a.batch.active || a.updates.preparing).Clicked() {
		a.cleanupEmptyFolders()
	}
}

// The list filter and editor share Onevra's select, search focus handling,
// option styling and bounded scrolling. Values remain plain vault folder paths.
func (a *application) folderSelect(c *ui.Context, selected, query *string, label, rootLabel string, allowCreate bool) ui.Element {
	a.entries()
	sel := components.Select(c, selected)
	display := *selected
	if display == "" {
		display = rootLabel
	}
	sel.Trigger.Label(label).FillWidth().MinWidth(0).Gap(6).Children(func() {
		components.Icon(c, lucide.Folder()).Size(14, 14).TextColor(c.Theme().TextMuted)
		components.Text(c, display).SingleLine()
	})
	sel.Popup(func(panel ui.Element) {
		panel.Width(280).MaxWidth(360).Label(a.t("%s选项", label))
		if sel.SearchField(query).Label(a.t("搜索文件夹")).Placeholder(a.t("搜索文件夹…")).Changed() {
			a.lastInput = time.Now()
		}
		components.ScrollView(c, components.ScrollViewOptions{}).FillWidth().MaxHeight(240).ScrollbarInsets(2).Children(func() {
			sel.Item("").Label(rootLabel).Children(func() { components.Text(c, rootLabel).SingleLine() })
			matches := 0
			for _, folder := range matchingFolders(a.folders, *query) {
				folder := folder
				matches++
				sel.Item(folder).Label(a.t("选择文件夹 %s", folder)).Children(func() { components.Text(c, folder).SingleLine() })
			}
			candidate := strings.TrimSpace(*query)
			if allowCreate && candidate != "" && !containsFolder(a.folders, candidate) {
				sel.Item(candidate).Label(a.t("新建文件夹 %s", candidate)).Children(func() {
					components.Icon(c, lucide.FolderPlus()).Size(14, 14)
					components.Text(c, a.t("新建文件夹：%s", candidate)).SingleLine()
				})
			} else if matches == 0 && candidate != "" {
				components.Text(c, a.t("没有找到文件夹")).Padding(10).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
			}
		})
	})
	if sel.Trigger.Changed() {
		*query = ""
		a.lastInput = time.Now()
	}
	return sel.Trigger
}

func matchingFolders(folders []string, query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return folders
	}
	matches := make([]string, 0)
	for _, folder := range folders {
		if strings.Contains(strings.ToLower(folder), query) {
			matches = append(matches, folder)
		}
	}
	return matches
}

func containsFolder(folders []string, value string) bool {
	for _, folder := range folders {
		if folder == value {
			return true
		}
	}
	return false
}

func splitEntryPath(path string) (folder, name string) {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i], path[i+1:]
	}
	return "", path
}

func (e *editorState) setPath(folder, name string) {
	e.folder, e.name = folder, name
	e.path = name
	if folder != "" {
		e.path = folder + "/" + name
	}
	e.pathFieldsPath = e.path
	e.draftSaved, e.draftAttempted = false, false
}

func (e *editorState) syncPathFields() {
	if e.pathFieldsPath != e.path {
		e.folder, e.name = splitEntryPath(e.path)
		e.pathFieldsPath = e.path
	}
}
