package desktop

import (
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// Recovery records expose metadata until the user explicitly resumes a draft.
// Keep the selected recovery ID separate from the regular entry and its secrets.
func (a *application) selectedRecovery() (app.RecoveryItem, bool) {
	for _, item := range a.recovery {
		if item.ID == a.recoverySelection {
			return item, true
		}
	}
	a.recoverySelection, a.restoreDestination = "", ""
	if len(a.recovery) == 0 {
		return app.RecoveryItem{}, false
	}
	item := a.recovery[0]
	a.recoverySelection = item.ID
	return item, true
}

func (a *application) selectRecovery(id string) {
	if id == a.recoverySelection {
		return
	}
	for _, item := range a.recovery {
		if item.ID == id {
			a.recoverySelection, a.restoreDestination = id, ""
			return
		}
	}
}

func (a *application) recoveryColumnsView(c *ui.Context, listWidth float32) {
	if a.state.Locked {
		a.unlockView(c)
		return
	}
	selected, hasSelection := a.selectedRecovery()
	ui.Row(c).Key("recovery-columns").Fill().MinWidth(0).MinHeight(0).Children(func() {
		list := ui.Column(c).Key("recovery-list-pane").Width(listWidth).MinWidth(managerListMinWidth).Shrink(1).FillHeight().MinHeight(0).Background(c.Theme().Surface).
			BorderWidth(0, 1, 0, 0).BorderColor(c.Theme().Border)
		list.Children(func() {
			ui.Column(c).FillWidth().Padding(24, 16, 14, 16).Children(func() {
				title := a.t("回收站")
				if a.recoveryKind == 1 {
					title = a.t("加密草稿")
				}
				ui.Row(c).FillWidth().MinHeight(34).AlignItems(ui.Center).Children(func() {
					components.Text(c, title).FontSize(scaledFontSize(c, 18)).Bold()
				})
			})
			components.ScrollView(c, components.ScrollViewOptions{}).Key("recovery-list-scroll").Grow(1).FillWidth().MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Padding(0, 8, 12, 8).Gap(3).Children(func() {
					for _, item := range a.recovery {
						item := item
						active := item.ID == selected.ID
						row := components.Button(c, "", components.ButtonGhost).Key(item.ID).
							Label(a.t("恢复副本 %s", item.Path)).FillWidth().MinWidth(0).MinHeight(66).
							Padding(10, 9).Radius(c.Theme().Radius).Disabled(a.busy)
						if active {
							background := c.Theme().Accent.Alpha(.10)
							if isMonochromePalette(c.Theme()) {
								background = c.Theme().Selection
							}
							row.Background(background)
						}
						row.Children(func() {
							ui.Row(c).FillWidth().MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
								ui.Box(c).Size(34, 34).Shrink(0).Radius(8).Background(c.Theme().Text.Alpha(.05)).
									AlignItems(ui.Center).Justify(ui.Center).Children(func() {
									components.Icon(c, lucide.FileKey()).Size(16, 16).TextColor(c.Theme().TextMuted)
								})
								ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
									components.Text(c, item.Path).SingleLine().Bold().FontSize(scaledFontSize(c, 13)).TextColor(c.Theme().Text)
									components.Text(c, item.CreatedAt).SingleLine().FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
								})
							})
						})
						if row.Clicked() {
							a.selectRecovery(item.ID)
						}
					}
				})
			})
			ui.Row(c).FillWidth().MinHeight(39).Padding(10, 16).Label(a.t("条目数量")).AlignItems(ui.Center).Children(func() {
				components.Text(c, a.countLabel(len(a.recovery))).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
			})
		})
		ui.Column(c).Key("recovery-detail-pane").Grow(1).MinWidth(managerDetailMinWidth).FillHeight().MinHeight(0).Background(c.Theme().Surface).Children(func() {
			ui.Column(c).Key("recovery-detail-body").Fill().MinWidth(0).MinHeight(0).Clip().Children(func() {
				if !hasSelection {
					ui.Column(c).Fill().Padding(24).AlignItems(ui.Center).Justify(ui.Center).Gap(14).Children(func() {
						components.Icon(c, lucide.ArchiveRestore()).Size(36, 36).TextColor(c.Theme().TextMuted)
						muted(c, a.t("目前没有可恢复的内容"))
					})
					return
				}
				a.recoveryDetailView(c, selected)
			})
			a.managerResizeHandle(c, list)
		})
	})
}

func (a *application) recoveryDetailView(c *ui.Context, item app.RecoveryItem) {
	components.ScrollView(c, components.ScrollViewOptions{}).Key("recovery-detail-scroll").Label(a.t("恢复副本详情")).Fill().MinWidth(0).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(808).AlignSelf(ui.Center).Padding(24).Gap(24).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(12).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					ui.Row(c).FillWidth().MinHeight(32).AlignItems(ui.Center).Children(func() {
						components.Text(c, entryName(item.Path)).FontSize(scaledFontSize(c, 22)).FontWeight(500).Wrap()
					})
					muted(c, item.Path)
				})
				if iconButton(c, lucide.Trash(), a.t("永久删除恢复副本"), components.ButtonGhost).
					Tooltip(a.t("永久删除恢复副本")).Disabled(a.busy).Clicked() {
					a.purgeID, a.purgeOpen = item.ID, true
				}
			})
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				components.Text(c, a.t("保存时间")).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
				components.Text(c, item.CreatedAt).FontSize(scaledFontSize(c, 13)).Wrap()
			})
			ui.Column(c).FillWidth().Gap(16).Children(func() {
				label := a.t("恢复")
				if a.recoveryKind == 1 {
					label = a.t("继续编辑")
					if a.editor.open {
						muted(c, a.t("请先保存或关闭当前编辑的条目。"))
					}
				} else {
					ui.Column(c).FillWidth().Disabled(a.busy).Children(func() {
						field(c, a.t("恢复到其他位置（可选）"), &a.restoreDestination, a.t("留空则恢复到原位置"), false)
					})
				}
				if components.Button(c, label, components.ButtonPrimary).AlignSelf(ui.Start).
					Disabled(a.busy || a.recoveryKind == 1 && a.editor.open).Clicked() {
					a.resumeRecovery(item)
				}
			})
		})
	})
}

func (a *application) resumeRecovery(item app.RecoveryItem) {
	if a.busy || a.state.Locked || a.page != "recovery" || item.ID != a.recoverySelection {
		return
	}
	current, kind := a.preferencesLease(), a.recoveryKind
	if kind == 1 {
		if a.editor.open {
			return
		}
		a.call(app.Request{Action: "fetch", Kind: "draft", ID: item.ID}, func(d app.Data) {
			if !current() || a.page != "recovery" || a.recoveryKind != kind || a.recoverySelection != item.ID {
				return
			}
			if a.editor.open {
				a.inform(a.t("请先保存或关闭当前编辑的条目。"))
				return
			}
			path := d.Destination
			if path == "" {
				path = d.Entry
			}
			a.editor = editorState{open: true, path: path, originalPath: d.OriginalEntry, contents: d.Contents, originalContents: d.Contents, revision: d.Revision, draftID: item.ID, draftSaved: true}
			a.editor.readFields()
			a.navigate("entries", "")
		})
		return
	}
	a.call(app.Request{Action: "restore", ID: item.ID, Kind: "trash", Destination: a.restoreDestination}, func(app.Data) {
		if !current() || a.page != "recovery" || a.recoveryKind != kind {
			return
		}
		a.removeRecovery(item.ID)
		a.notice = a.t("已恢复条目")
	})
}
