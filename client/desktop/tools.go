package desktop

import (
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine-neo/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

type generatorState struct {
	length                                  float64
	lower, upper, numbers, symbols, exclude bool
	password                                string
}

func (a *application) generate(done func(string)) {
	g := a.generator
	a.call(app.Request{Action: "generate", Length: int(g.length), Lowercase: g.lower, Uppercase: g.upper, Numbers: g.numbers, Symbols: g.symbols, ExcludeAmbiguous: g.exclude}, func(d app.Data) {
		a.generator.password = d.Password
		if done != nil {
			done(d.Password)
		}
	})
}
func (a *application) recoveryType() string {
	if a.recoveryKind == 1 {
		return "draft"
	}
	return "trash"
}
func (a *application) loadRecovery() {
	if a.state.Locked {
		return
	}
	a.call(app.Request{Action: "recovery", Kind: a.recoveryType()}, func(d app.Data) { a.recovery = d.Recovery })
}
func (a *application) recoveryView(c *ui.Context) {
	ui.Column(c).Fill().Padding(30).Gap(20).Children(func() {
		section(c, "恢复中心", "", nil)
		old := a.recoveryKind
		ui.Column(c).FillWidth().Disabled(a.busy).Children(func() {
			components.Tabs(c, &a.recoveryKind, "回收站", "加密草稿")
		})
		if old != a.recoveryKind {
			a.loadRecovery()
		}
		if len(a.recovery) == 0 {
			ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(14).Children(func() {
				components.Icon(c, lucide.ArchiveRestore).Size(36, 36).TextColor(c.Theme().TextMuted)
				muted(c, "目前没有可恢复的内容")
			})
			return
		}
		field(c, "恢复到其他位置（可选）", &a.restoreDestination, "留空则恢复到原位置", false)
		components.ScrollView(c, components.ScrollViewOptions{}).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Gap(10).Children(func() {
				for _, item := range a.recovery {
					item := item
					ui.Row(c).FillWidth().Padding(14).Gap(10).Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(8).AlignItems(ui.Center).Children(func() {
						components.Icon(c, lucide.FileKey).Size(22, 22).TextColor(c.Theme().Accent)
						ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
							components.Text(c, item.Path).Bold().Wrap()
							components.Text(c, item.CreatedAt).FontSize(11).TextColor(c.Theme().TextMuted)
						})
						label := "恢复"
						if a.recoveryKind == 1 {
							label = "继续编辑"
						}
						if components.Button(c, label, components.ButtonOutline).Disabled(a.busy).Clicked() {
							if a.recoveryKind == 1 {
								a.call(app.Request{Action: "fetch", Kind: "draft", ID: item.ID}, func(d app.Data) {
									path := d.Destination
									if path == "" {
										path = d.Entry
									}
									a.editor = editorState{open: true, path: path, originalPath: d.OriginalEntry, contents: d.Contents, originalContents: d.Contents, revision: d.Revision, draftID: item.ID, draftSaved: true}
									a.editor.readFields()
								})
							} else {
								a.call(app.Request{Action: "restore", ID: item.ID, Kind: "trash", Destination: a.restoreDestination}, func(d app.Data) { a.removeRecovery(item.ID); a.notice = "已恢复条目" })
							}
						}
						if iconButton(c, lucide.Trash(), "永久删除恢复副本", components.ButtonGhost).Clicked() {
							a.purgeID = item.ID
							a.purgeOpen = true
						}
					})
				}
			})
		})
	})
}

func (a *application) settingsView(c *ui.Context) {
	a.shortcut.visible = false
	if !a.settingsLoaded {
		a.settings = a.state.Settings
		a.settingsSaved = a.state.Settings
		a.settingsObserved = a.state.Settings
		a.settingsLoaded = true
	}
	width, _ := c.Size()
	if a.shell == nil {
		width -= 310
	} else if a.shell.LeftExpanded() {
		width -= a.shell.LeftWidth()
	}
	selectValue := func(c *ui.Context, value *string, label string, values, labels []string) {
		selectBox := components.Select(c, value)
		display := *value
		for i, v := range values {
			if v == *value {
				display = labels[i]
			}
		}
		selectBox.Trigger.Label(label).MinWidth(155).AlignSelf(ui.Center).Children(func() { components.Text(c, display) })
		selectBox.Popup(func(ui.Element) {
			for i, v := range values {
				v, label := v, labels[i]
				selectBox.Item(v).Label(label).Children(func() { components.Text(c, label) })
			}
		})
	}
	button := func(c *ui.Context, label, action string) {
		disabled := a.busy
		if action == "git-sync" || action == "git-init" {
			disabled = a.gitActionPending || a.state.GitSyncing
		}
		if components.Button(c, label, components.ButtonOutline).AlignSelf(ui.Center).Disabled(disabled).Clicked() {
			a.call(app.Request{Action: action}, func(app.Data) { a.notice = label + "完成" })
		}
	}
	general := []components.SettingsItem{
		{ID: "theme", Title: "外观", Build: func(c *ui.Context) {
			selectValue(c, &a.settings.Theme, "外观", []string{"system", "light", "dark"}, []string{"跟随系统", "浅色", "深色"})
		}},
		{ID: "close", Title: "关闭窗口后继续运行", Build: func(c *ui.Context) {
			settingSwitch(c, &a.settings.CloseToTray, "关闭后继续运行")
		}},
		{ID: "shortcut", Title: "全局搜索快捷键", Description: "点击录制后按下组合键，Esc 取消。保存后立即生效。", Build: a.shortcutControls},
	}
	security := []components.SettingsItem{
		{ID: "device", Title: "使用 Touch ID 或 Mac 登录密码解锁", Description: a.deviceUnlockDescription(), Build: func(c *ui.Context) {
			ui.Column(c).AlignSelf(ui.Center).Disabled(!a.state.DeviceUnlockAvailable && !a.state.DeviceUnlockEnrolled).Children(func() {
				settingSwitch(c, &a.settings.DeviceUnlock, "使用 Touch ID 或 Mac 登录密码解锁")
			})
		}},
		{ID: "idle", Title: "空闲自动锁定", Build: func(c *ui.Context) {
			durationSegments(c, &a.settings.IdleMinutes, "空闲自动锁定", []int{1, 5, 10, 15, 30, 60}, "分钟")
		}},
		{ID: "max", Title: "最长解锁时间", Build: func(c *ui.Context) {
			durationSegments(c, &a.settings.MaxSessionMinutes, "最长解锁时间", []int{15, 30, 60, 120, 480}, "分钟")
		}},
		{ID: "clipboard", Title: "清理剪贴板", Build: func(c *ui.Context) {
			durationSegments(c, &a.settings.ClipboardSeconds, "清理剪贴板", []int{15, 30, 60, 120}, "秒")
		}},
		{ID: "locked", Title: "锁定时允许列出名称", Build: func(c *ui.Context) {
			settingSwitch(c, &a.settings.BrowseWhileLocked, "锁定时允许列出名称")
		}},
	}
	vaultItems := []components.SettingsItem{
		{ID: "recovery", Title: "恢复中心", Build: func(c *ui.Context) {
			if components.Button(c, "打开恢复中心", components.ButtonOutline).AlignSelf(ui.Center).Clicked() {
				a.navigate("recovery", "")
			}
		}},
		{ID: "directory", Title: a.state.VaultName, Description: a.state.Directory, Build: func(c *ui.Context) {
			if components.Button(c, "切换密码库", components.ButtonOutline).AlignSelf(ui.Center).Clicked() {
				a.openSetup()
			}
		}},
		{ID: "export", Title: "导出恢复私钥", Build: func(c *ui.Context) {
			if components.Button(c, "导出…", components.ButtonOutline).AlignSelf(ui.Center).Clicked() {
				a.exportKey()
			}
		}},
		{ID: "verify", Title: "验证恢复备份", Build: func(c *ui.Context) {
			if components.Button(c, "选择并验证…", components.ButtonOutline).AlignSelf(ui.Center).Clicked() {
				a.verifyBackup()
			}
		}},
	}
	syncItems := []components.SettingsItem{
		{ID: "remote", Title: "Git 远端地址", Build: func(c *ui.Context) {
			ui.Column(c).Width(320).MaxWidth(420).AlignSelf(ui.Center).Gap(8).Children(func() {
				components.TextBox(c, &a.gitRemote, components.TextBoxOptions{}).FillWidth().Disabled(!a.state.GitInitialized || a.state.GitSyncing).Label("Git 远端地址").Placeholder("git@github.com:you/store.git")
			})
		}},
		{ID: "auto", Title: "自动同步", Build: func(c *ui.Context) {
			settingSwitch(c, &a.settings.AutoSync, "自动同步")
		}},
		{ID: "interval", Title: "定时同步间隔", Build: func(c *ui.Context) {
			durationSegments(c, &a.settings.SyncMinutes, "定时同步间隔", []int{0, 5, 15, 30, 60}, "分钟")
		}},
		{ID: "git", Title: "Git 同步状态", Description: a.gitStatusDescription(), Build: func(c *ui.Context) {
			ui.Row(c).Gap(8).AlignSelf(ui.Center).AlignItems(ui.Center).Children(func() {
				if a.state.GitInitialized {
					button(c, "立即同步", "git-sync")
				} else {
					button(c, "初始化 Git", "git-init")
				}
				if iconButton(c, lucide.RefreshCw(), "刷新 Git 状态", components.ButtonGhost).Disabled(a.state.GitRefreshing || a.state.GitSyncing).Clicked() {
					a.callBackground(app.Request{Action: "git-status"}, nil)
				}
			})
		}},
	}
	browserItems := []components.SettingsItem{
		{ID: "browser", Title: "连接浏览器扩展", Build: func(c *ui.Context) { button(c, "注册浏览器连接", "browser-install") }},
	}
	activityItems := []components.SettingsItem{
		{ID: "activity", Title: "本机操作记录", Build: func(c *ui.Context) {
			settingSwitch(c, &a.settings.RecordActivity, "本机操作记录")
		}},
		{ID: "history", Title: "查看操作记录", Build: func(c *ui.Context) {
			if components.Button(c, "打开操作记录", components.ButtonOutline).AlignSelf(ui.Center).Clicked() {
				a.navigate("activity", "")
			}
		}},
	}
	ui.Column(c).Fill().Children(func() {
		components.Settings(c, &a.settingsState, []components.SettingsCategory{
			{ID: "general", Title: "通用", Groups: []components.SettingsGroup{{ID: "appearance", Items: general}, {ID: "activity", Title: "操作记录", Items: activityItems}}},
			{ID: "security", Title: "安全", Groups: []components.SettingsGroup{{ID: "session", Items: security}}},
			{ID: "vault", Title: "密码库", Groups: []components.SettingsGroup{{ID: "backup", Items: vaultItems}, {ID: "sync", Title: "Git 同步", Items: syncItems}}},
			{ID: "browser", Title: "浏览器", Groups: []components.SettingsGroup{{ID: "integration", Items: browserItems}}},
		}, components.SettingsOptions{Narrow: width < 700, Title: "设置", SearchLabel: "搜索设置", SearchPlaceholder: "搜索设置…", CategoryLabel: "设置分类", ResultsTitle: "搜索结果", EmptyText: "没有找到相关设置"})
		if a.settingsSaving || a.gitRemoteSaving || time.Now().Before(a.settingsSavedUntil) {
			ui.Row(c).FillWidth().Padding(8, 24).Children(func() {
				label := "已保存"
				if a.settingsSaving || a.gitRemoteSaving {
					label = "正在保存…"
				}
				muted(c, label)
			})
		}
	})
	if a.shortcut.recording && !a.shortcut.visible {
		a.setShortcutRecording(false)
	}
	if a.settings != a.settingsObserved {
		a.settingsObserved = a.settings
		a.settingsDirty = true
		a.saveSettingsAutomatically()
	}
	if a.gitRemote != a.gitRemoteObserved {
		a.gitRemoteObserved = a.gitRemote
		a.gitRemoteVaultID = a.state.VaultID
		a.gitRemoteSaveAt = time.Now().Add(700 * time.Millisecond)
	}
}

func (a *application) exportKey() {
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.window, Title: "导出受口令保护的恢复私钥", DefaultPath: "passpine-recovery.asc", Filters: []mygo.FileFilter{{Name: "OpenPGP 私钥", Extensions: []string{"asc"}}}, CreateDirectories: true})
		mygo.RunOnMain(func() {
			if err != nil {
				a.errorText = err.Error()
			} else if path != "" {
				a.call(app.Request{Action: "export", Destination: path}, func(app.Data) { a.notice = "恢复私钥已导出。请使用验证备份确认可恢复。" })
			}
			a.invalidate()
		})
	}()
}
func (a *application) verifyBackup() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.window, Title: "选择要验证的恢复私钥", Filters: []mygo.FileFilter{{Name: "OpenPGP 私钥", Extensions: []string{"asc", "pgp", "gpg", "key"}}}})
		mygo.RunOnMain(func() {
			if err != nil {
				a.errorText = err.Error()
			} else if len(paths) > 0 {
				a.backupPath = paths[0]
				a.backupPassphrase = ""
				a.backupOpen = true
			}
			a.invalidate()
		})
	}()
}

func (a *application) removeRecovery(id string) {
	for i, item := range a.recovery {
		if item.ID == id {
			a.recovery = append(a.recovery[:i], a.recovery[i+1:]...)
			return
		}
	}
}
func (a *application) activityView(c *ui.Context) {
	ui.Column(c).Fill().Padding(30).Gap(20).Children(func() {
		section(c, "操作记录", "", nil)
		if len(a.activity) == 0 {
			muted(c, "暂无操作记录")
			return
		}
		components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				for _, entry := range a.activity {
					ui.Row(c).FillWidth().Padding(12).Gap(12).Background(c.Theme().Surface).Radius(8).Children(func() {
						components.Text(c, entry.Time).FontSize(11).TextColor(c.Theme().TextMuted)
						components.Text(c, entry.Action).Grow(1).MinWidth(0).Wrap().Bold()
						components.Badge(c, entry.Source)
						components.Text(c, fmt.Sprint(entry.Outcome)).FontSize(12)
					})
				}
			})
		})
	})
}
