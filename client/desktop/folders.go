package desktop

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

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
