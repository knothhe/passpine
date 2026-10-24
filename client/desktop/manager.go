package desktop

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func (a *application) entriesTitle() string {
	if strings.HasPrefix(a.filter, "folder:") {
		return strings.TrimPrefix(a.filter, "folder:")
	}
	return a.t("全部条目")
}

// Keep the list and inspector mounted under the same keys when navigation is
// collapsed or presented as a narrow-window overlay. Each owns its scroll state.
func (a *application) managerColumnsView(c *ui.Context) {
	ui.Column(c).Key("manager-frame-inset").Fill().MinWidth(0).Padding(managerFrameInset).Background(c.Theme().Background).Children(func() {
		ui.Row(c).Key("manager-columns").Fill().MinWidth(0).Radius(12).Border(1, c.Theme().Border).Background(c.Theme().Surface).Clip().Children(func() {
			list := ui.Column(c).Key("entry-list-column").Label(a.t("条目列表")).Width(a.managerListWidth(c)).MinWidth(managerListMinWidth).Shrink(1).FillHeight()
			list.Children(func() {
				a.listView(c)
			})
			ui.Column(c).Key("entry-inspector-column").Grow(1).MinWidth(managerDetailMinWidth).FillHeight().Children(func() {
				ui.Column(c).Key("entry-inspector-body").Fill().MinWidth(0).Clip().Children(func() {
					ui.Box(c).Absolute().Left(0).Top(0).Bottom(0).Width(1).Background(c.Theme().Border)
					if a.state.Locked {
						a.unlockView(c)
					} else if a.editor.open {
						a.editorView(c)
					} else if a.batch.active {
						a.batchView(c)
					} else {
						a.detailView(c)
					}
				})
				a.managerResizeHandle(c, list)
			})
		})
	})
}

func (a *application) entriesCountLabel() string {
	if strings.TrimSpace(a.query) != "" {
		return a.t("%d / %d 个条目", len(a.filtered), len(a.state.Entries))
	}
	return a.countLabel(len(a.filtered))
}

func managerGlyphSize(c *ui.Context) float32 { return c.Theme().Rem(4.0 / 3.0) }

// Use the same icon and label columns in scrolling navigation and fixed footers.
func managerNavigationItem(c *ui.Context, key, label, visible string, icon *ui.SVG, selected bool, count string, depth int) ui.Element {
	t := c.Theme()
	row := ui.ButtonBase(c).Key(key).Label(label).FillWidth().MinWidth(0).
		MinHeight(36).Padding(8, 8, 8, float32(8+min(depth, 3)*14)).Gap(8).
		AlignItems(ui.Center).Radius(t.Radius).Selected(selected)
	if selected {
		row.Background(t.Selection)
	} else if row.Hovered() {
		row.Background(t.Text.Alpha(.04))
	}
	row.Children(func() {
		color := t.TextMuted
		if selected {
			color = t.Accent
		}
		components.Icon(c, icon).Size(managerGlyphSize(c), managerGlyphSize(c)).Shrink(0).TextColor(color)
		components.Text(c, visible).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(scaledFontSize(c, 13)).TextColor(t.Text)
		if count != "" {
			components.Text(c, count).Shrink(0).FontSize(scaledFontSize(c, 11)).TextColor(t.TextMuted)
		}
	})
	return row
}

func (a *application) navigationSidebar(c *ui.Context) {
	ui.Column(c).Fill().MinWidth(0).Children(func() {
		ui.Column(c).FillWidth().Padding(24, 20, 12, 20).Children(func() {
			components.Text(c, a.t("密码库")).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
		})
		components.ScrollView(c, components.ScrollViewOptions{}).Key("vault-navigation-scroll").FillWidth().Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(0, 12, 12, 12).Gap(4).Children(func() {
				if managerNavigationItem(c, "all", a.t("全部条目"), a.t("全部条目"), lucide.KeyRound(), a.page == "entries" && a.filter == "all", strconv.Itoa(len(a.state.Entries)), 0).Clicked() {
					a.navigateManager("all")
				}
				if a.state.Configured && !a.setup && (!a.state.Locked || a.state.Settings.BrowseWhileLocked) {
					a.entries()
					components.Text(c, a.t("文件夹")).Margin(16, 8, 4, 8).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
					for _, folder := range a.folders {
						folder := folder
						if strings.Contains(folder, "/") {
							continue
						}
						selected := a.page == "entries" && (a.filter == "folder:"+folder || strings.HasPrefix(a.filter, "folder:"+folder+"/"))
						if managerNavigationItem(c, folder, a.t("文件夹 %s", folder), folder, lucide.Folder(), selected, "", 0).Tooltip(folder).Clicked() {
							a.navigateManager("folder:" + folder)
						}
					}
				}
			})
		})
		ui.Column(c).FillWidth().Shrink(0).Padding(12, 12, 8, 12).Gap(4).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
			if managerNavigationItem(c, "drafts", a.t("加密草稿"), a.t("草稿"), lucide.FilePenLine(), a.page == "recovery" && a.recoveryKind == 1, "", 0).Clicked() {
				a.navigateRecovery(1)
			}
			if managerNavigationItem(c, "trash", a.t("回收站"), a.t("回收站"), lucide.Trash(), a.page == "recovery" && a.recoveryKind == 0, "", 0).Clicked() {
				a.navigateRecovery(0)
			}
		})
	})
}

func (a *application) closeNavigationOverlay() {
	if a.shell != nil && a.shell.LeftOverlayActive() {
		a.shell.SetLeftExpanded(false)
	}
}

func (a *application) navigateManager(filter string) {
	if a.editor.open && (a.page != "entries" || a.filter != filter) {
		a.requestEditorClose(func() { a.navigateManager(filter) })
		return
	}
	if a.page != "entries" || a.filter != filter {
		// Selection belongs to one folder's list; switching folders clears it.
		a.exitBatch()
		a.navigate("entries", filter)
		a.query = ""
		a.dropDetail()
		a.syncSelectedIndex()
		a.list.ScrollTo(0, ui.Start)
	}
	a.closeNavigationOverlay()
}

func (a *application) navigateRecovery(kind int) {
	// Keep an active recovery operation bound to its category. Reads on other
	// pages can finish before the queued recovery navigation starts its read.
	if a.discardOpen || a.busy && a.page == "recovery" {
		return
	}
	if a.editor.open {
		a.requestEditorClose(func() { a.navigateRecovery(kind) })
		return
	}
	if a.page != "recovery" || a.recoveryKind != kind {
		a.exitBatch()
		a.recoveryKind = kind
		a.recovery = nil
		a.recoverySelection, a.restoreDestination = "", ""
		a.navigate("recovery", "")
	}
	a.closeNavigationOverlay()
}
