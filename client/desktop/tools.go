package desktop

import (
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func (a *application) generatorDialog(c *ui.Context) {
	if a.generator.open {
		c.Key("password-generator")
	}
	components.Dialog(c, &a.generator.open, components.DialogOptions{Label: a.t("密码生成器"), DisableBackdropClose: true, DisableEscapeClose: true}, func(panel ui.Element) {
		panel.Width(420).Padding(18).Gap(12)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			components.Text(c, a.t("密码生成器")).FontSize(16).Bold()
			ui.Spacer(c)
			if iconButton(c, lucide.X(), a.t("关闭密码生成器"), components.ButtonGhost).Size(26, 26).Padding(5).Clicked() {
				a.closeGenerator()
			}
		})
		ui.Row(c).Key("generator-preview").FillWidth().MinWidth(0).Height(42).Padding(0, 6).Gap(4).AlignItems(ui.Center).
			Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(c.Theme().Radius).Children(func() {
			components.TextBox(c, &a.generator.password, components.TextBoxOptions{ReadOnly: true}).
				Width(0).Grow(1).MinWidth(0).Height(40).Padding(10, 6).Clip().Border(0, c.Theme().Border).
				Font("monospace").FontSize(13).Background(ui.Color{}).TextColor(c.Theme().Text).
				Label(a.t("生成的密码")).Placeholder(a.t("点击重新生成密码"))
			if iconButton(c, lucide.RefreshCw(), a.t("重新生成密码"), components.ButtonGhost).
				Size(28, 28).Shrink(0).Padding(6).Tooltip(a.t("重新生成密码")).Clicked() {
				a.generate(nil)
			}
		})
		changed := false
		ui.Row(c).Key("generator-length").FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			components.Text(c, a.t("长度")).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
			slider := components.Slider(c, &a.generator.length, 8, 128, components.SliderOptions{Step: 1})
			slider.Label(a.t("密码长度")).Grow(1).MinWidth(60).Height(24)
			if slider.Changed() {
				changed = true
			}
			ui.Box(c).Key("length-number").Width(76).Children(func() {
				number := components.NumberInput(c, &a.generator.length, 8, 128, 1)
				number.Label(a.t("密码长度数值")).Width(76).Height(30)
				if number.Changed() {
					a.generator.length = math.Round(a.generator.length)
					changed = true
				}
			})
		})
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			count := 0
			for _, on := range []bool{a.generator.lower, a.generator.upper, a.generator.numbers, a.generator.symbols} {
				if on {
					count++
				}
			}
			for _, option := range []struct {
				key, label, sample string
				on                 *bool
			}{
				{"lower", "小写字母", "a–z", &a.generator.lower},
				{"upper", "大写字母", "A–Z", &a.generator.upper},
				{"numbers", "数字", "0–9", &a.generator.numbers},
				{"symbols", "符号", "!@#", &a.generator.symbols},
			} {
				ui.Box(c).Key(option.key).Width(0).Grow(1).MinWidth(0).Disabled(*option.on && count == 1).Children(func() {
					button := components.ToggleButton(c, option.on, option.sample).Label(a.t(option.label)).Tooltip(a.t(option.label)).
						FillWidth().Height(30).Padding(5, 8).FontSize(12)
					if *option.on {
						button.Background(c.Theme().Accent.Alpha(.14)).TextColor(c.Theme().Accent).Border(1, c.Theme().Accent.Alpha(.4))
					}
					if button.Changed() {
						changed = true
					}
				})
			}
		})
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			if components.Checkbox(c, &a.generator.exclude, a.t("排除易混淆字符")).FontSize(11).Changed() {
				changed = true
			}
			ui.Spacer(c)
			components.Text(c, "I l 1 O 0").Font("monospace").FontSize(10).TextColor(c.Theme().TextMuted).SingleLine()
		})
		if changed {
			a.requestGeneratorUpdate()
		}
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			ui.Spacer(c)
			if components.Button(c, a.t("取消"), components.ButtonGhost).FontSize(12).Height(30).Padding(5, 12).Clicked() {
				a.closeGenerator()
			}
			if components.Button(c, a.t("使用此密码"), components.ButtonPrimary).FontSize(12).Height(30).Padding(5, 14).Disabled(!a.generatorReady()).Clicked() {
				a.applyGeneratedPassword()
			}
		})
	})
}
func (a *application) recoveryType() string {
	if a.recoveryKind == 1 {
		return "draft"
	}
	return "trash"
}
func (a *application) loadRecovery() {
	a.requestPreferencesRead("recovery")
}
func (a *application) recoveryView(c *ui.Context) {
	ui.Column(c).Fill().Padding(30).Gap(20).Children(func() {
		section(c, a.t("恢复中心"), "", nil)
		old := a.recoveryKind
		ui.Column(c).FillWidth().Disabled(a.busy).Children(func() {
			components.Tabs(c, &a.recoveryKind, a.t("回收站"), a.t("加密草稿"))
		})
		if old != a.recoveryKind {
			a.loadRecovery()
		}
		if len(a.recovery) == 0 {
			ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(14).Children(func() {
				components.Icon(c, lucide.ArchiveRestore).Size(36, 36).TextColor(c.Theme().TextMuted)
				muted(c, a.t("目前没有可恢复的内容"))
			})
			return
		}
		field(c, a.t("恢复到其他位置（可选）"), &a.restoreDestination, a.t("留空则恢复到原位置"), false)
		if a.recoveryKind == 1 && a.editor.open {
			muted(c, a.t("请先保存或关闭当前编辑的条目。"))
		}
		components.ScrollView(c, components.ScrollViewOptions{}).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Gap(10).Children(func() {
				for _, item := range a.recovery {
					item := item
					ui.Row(c).FillWidth().Padding(14).Gap(10).Background(c.Theme().Surface).Radius(8).AlignItems(ui.Center).Children(func() {
						components.Icon(c, lucide.FileKey).Size(22, 22).TextColor(c.Theme().Accent)
						ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
							components.Text(c, item.Path).Bold().Wrap()
							components.Text(c, item.CreatedAt).FontSize(11).TextColor(c.Theme().TextMuted)
						})
						label := a.t("恢复")
						if a.recoveryKind == 1 {
							label = a.t("继续编辑")
						}
						if components.Button(c, label, components.ButtonGhost).Disabled(a.busy || a.recoveryKind == 1 && a.editor.open).Clicked() {
							if a.recoveryKind == 1 {
								current := a.preferencesLease()
								a.call(app.Request{Action: "fetch", Kind: "draft", ID: item.ID}, func(d app.Data) {
									if !current() || a.recoveryKind != 1 {
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
							} else {
								a.call(app.Request{Action: "restore", ID: item.ID, Kind: "trash", Destination: a.restoreDestination}, func(d app.Data) { a.removeRecovery(item.ID); a.notice = a.t("已恢复条目") })
							}
						}
						if iconButton(c, lucide.Trash(), a.t("永久删除恢复副本"), components.ButtonGhost).Clicked() {
							a.purgeID = item.ID
							a.purgeOpen = true
						}
					})
				}
			})
		})
	})
}

func (a *application) settingsCategories() []components.SettingsCategory {
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
			a.call(app.Request{Action: action}, func(app.Data) { a.notice = a.t("操作已完成") })
		}
	}
	general := []components.SettingsItem{
		{ID: "theme-style", Title: a.t("主题风格"), Description: a.t("主题配色和应用图标一起切换。"), Build: func(c *ui.Context) {
			selectValue(c, &a.settings.ThemeStyle, a.t("主题风格"), []string{app.ThemeStyleOnevra, app.ThemeStyleMonochrome}, []string{a.t("陶土橙"), a.t("黑白")})
		}},
		{ID: "theme", Title: a.t("外观"), Build: func(c *ui.Context) {
			selectValue(c, &a.settings.Theme, a.t("外观"), []string{"system", "light", "dark"}, []string{a.t("跟随系统"), a.t("浅色"), a.t("深色")})
		}},
		{ID: "language", Title: a.t("语言"), Build: func(c *ui.Context) {
			selectValue(c, &a.settings.Language, a.t("语言"), []string{"system", "en", "zh-CN"}, []string{a.t("跟随系统"), "English", a.t("简体中文")})
		}},
		{ID: "close", Title: a.t("关闭窗口后继续运行"), Build: func(c *ui.Context) {
			settingSwitch(c, &a.settings.CloseToTray, a.t("关闭后继续运行"))
		}},
		{ID: "shortcut", Title: a.t("全局搜索快捷键"), Description: a.t("点击录制后按下组合键，Esc 取消。保存后立即生效。"), Build: a.shortcutControls},
	}
	systemItems := []components.SettingsItem{
		{ID: "dock", Title: a.t("Dock 图标"), Description: a.t("跟随窗口时，仅在主窗口打开时展示。"), Build: func(c *ui.Context) {
			choiceSettingSegments(c, &a.settings.DockIcon, a.t("Dock 图标"), []string{app.DockIconShow, app.DockIconHide, app.DockIconFollowWindow}, []string{a.t("展示"), a.t("隐藏"), a.t("跟随窗口")})
		}},
		{ID: "tray-click", Title: a.t("点击托盘图标"), Build: func(c *ui.Context) {
			choiceSettingSegments(c, &a.settings.TrayClickAction, a.t("点击托盘图标"), []string{app.TrayClickQuickPanel, app.TrayClickMainWindow}, []string{a.t("打开快捷面板"), a.t("打开主窗口")})
		}},
		{ID: "lightweight", Title: a.t("轻量模式"), Description: a.t("窗口和面板全部关闭一分钟后释放界面内存，再次打开时重新加载。"), Build: func(c *ui.Context) {
			a.booleanSettingSegments(c, &a.settings.LightweightMode, a.t("启用轻量模式"))
		}},
		{ID: "launch-at-login", Title: a.t("开机自启"), Description: a.t("登录系统后在后台启动 Passpine。"), Build: func(c *ui.Context) {
			ui.Column(c).AlignSelf(ui.Center).Disabled(a.state.Preview).Children(func() {
				a.booleanSettingSegments(c, &a.settings.LaunchAtLogin, a.t("启用开机自启"))
			})
		}},
	}
	security := []components.SettingsItem{
		{ID: "device", Title: a.t("使用 Touch ID 或 Mac 登录密码解锁"), Description: a.deviceUnlockDescription(), Build: func(c *ui.Context) {
			ui.Column(c).AlignSelf(ui.Center).Disabled(!a.state.DeviceUnlockAvailable && !a.state.DeviceUnlockEnrolled).Children(func() {
				settingSwitch(c, &a.settings.DeviceUnlock, a.t("使用 Touch ID 或 Mac 登录密码解锁"))
			})
		}},
		{ID: "idle", Title: a.t("空闲自动锁定"), Build: func(c *ui.Context) {
			a.durationSegments(c, &a.settings.IdleMinutes, a.t("空闲自动锁定"), idleDurationPresets(a.settings.IdleMinutes), a.t("分钟"))
		}},
		{ID: "max", Title: a.t("最长解锁时间"), Build: func(c *ui.Context) {
			a.durationSegments(c, &a.settings.MaxSessionMinutes, a.t("最长解锁时间"), []int{30, 60, 480}, a.t("分钟"))
		}},
		{ID: "clipboard", Title: a.t("清理剪贴板"), Build: func(c *ui.Context) {
			a.durationSegments(c, &a.settings.ClipboardSeconds, a.t("清理剪贴板"), []int{15, 30, 60}, a.t("秒"))
		}},
		{ID: "locked", Title: a.t("锁定时允许列出名称"), Build: func(c *ui.Context) {
			settingSwitch(c, &a.settings.BrowseWhileLocked, a.t("锁定时允许列出名称"))
		}},
	}
	vaultItems := []components.SettingsItem{
		{ID: "name", Title: a.t("密码库名称"), Description: a.t("留空则使用密码库目录作为名称。"), Build: a.vaultNameControls},
		{ID: "recovery", Title: a.t("恢复中心"), Description: a.t("找回已删除的条目，或继续编辑加密草稿。"), Build: func(c *ui.Context) {
			if components.Button(c, a.t("打开恢复中心"), components.ButtonOutline).AlignSelf(ui.Center).Clicked() {
				a.navigate("recovery", "")
			}
		}},
		{ID: "directory", Title: a.state.VaultName, Description: a.state.Directory, Build: func(c *ui.Context) {
			if components.Button(c, a.t("切换密码库"), components.ButtonOutline).AlignSelf(ui.Center).Clicked() {
				a.openSetup()
			}
		}},
		{ID: "export", Title: a.t("导出恢复私钥"), Build: func(c *ui.Context) {
			if components.Button(c, a.t("导出…"), components.ButtonOutline).AlignSelf(ui.Center).Clicked() {
				a.exportKey()
			}
		}},
		{ID: "verify", Title: a.t("验证恢复备份"), Build: func(c *ui.Context) {
			if components.Button(c, a.t("选择并验证…"), components.ButtonOutline).AlignSelf(ui.Center).Clicked() {
				a.verifyBackup()
			}
		}},
	}
	syncItems := []components.SettingsItem{
		{ID: "remote", Title: a.t("Git 远端地址"), Build: func(c *ui.Context) {
			ui.Column(c).Width(320).MaxWidth(420).AlignSelf(ui.Center).Gap(8).Children(func() {
				components.TextBox(c, &a.gitRemote, components.TextBoxOptions{}).FillWidth().Disabled(!a.state.GitInitialized || a.state.GitSyncing).Label(a.t("Git 远端地址")).Placeholder("git@github.com:you/store.git")
			})
		}},
		{ID: "auto", Title: a.t("自动同步"), Build: func(c *ui.Context) {
			settingSwitch(c, &a.settings.AutoSync, a.t("自动同步"))
		}},
		{ID: "interval", Title: a.t("定时同步间隔"), Build: func(c *ui.Context) {
			a.durationSegments(c, &a.settings.SyncMinutes, a.t("定时同步间隔"), []int{0, 15, 30}, a.t("分钟"))
		}},
		{ID: "git", Title: a.t("Git 同步状态"), Description: a.gitStatusDescription(), Build: a.gitSettingsStatus},
	}
	browserItems := []components.SettingsItem{
		{ID: "browser-status", Title: a.t("浏览器注册"), Description: a.t("注册状态表示本机连接和扩展身份是否匹配，不代表浏览器已安装扩展。"), Build: a.browserRegistrationView},
		{ID: "browser", Title: a.t("连接浏览器扩展"), Description: a.t("仅为此设备上已安装的浏览器配置本机连接。"), Build: func(c *ui.Context) {
			if a.state.Preview {
				muted(c, a.t("预览模式不会注册个人浏览器"))
				return
			}
			button(c, a.t("重新检测并修复"), "browser-install")
		}},
	}
	activityItems := []components.SettingsItem{
		{ID: "activity", Title: a.t("本机操作记录"), Build: func(c *ui.Context) {
			settingSwitch(c, &a.settings.RecordActivity, a.t("本机操作记录"))
		}},
		{ID: "activity-count", Title: a.t("最多保留记录"), Build: func(c *ui.Context) {
			a.durationSegments(c, &a.settings.ActivityMaxEntries, a.t("最多保留记录"), []int{500, 1000, 5000}, a.t("条"))
		}},
		{ID: "activity-days", Title: a.t("保留天数"), Description: a.t("超出条数或时间的记录会自动清理。"), Build: func(c *ui.Context) {
			a.durationSegments(c, &a.settings.ActivityRetentionDays, a.t("保留天数"), []int{1, 7, 30}, a.t("天"))
		}},
		{ID: "history", Title: a.t("查看操作记录"), Description: a.t("查看此设备上的密码库操作。"), Build: func(c *ui.Context) {
			if components.Button(c, a.t("打开操作记录"), components.ButtonOutline).AlignSelf(ui.Center).Clicked() {
				a.navigate("activity", "")
			}
		}},
	}
	return []components.SettingsCategory{
		{ID: "general", Title: a.t("通用"), Icon: lucide.Settings(), Description: a.t("调整外观、快捷键与本机操作记录。"), Groups: []components.SettingsGroup{{ID: "appearance", Items: general}, {ID: "activity", Title: a.t("操作记录"), Items: activityItems}}},
		{ID: "system", Title: a.t("窗口与启动"), Icon: lucide.AppWindow(), Description: a.t("管理 Dock、托盘、内存释放与开机自启。"), Groups: []components.SettingsGroup{{ID: "system", Items: systemItems}}},
		{ID: "security", Title: a.t("安全"), Icon: lucide.LockKeyhole(), Description: a.t("管理解锁方式和密码的保留时间。"), Groups: []components.SettingsGroup{{ID: "session", Items: security}}},
		{ID: "vault", Title: a.t("密码库"), Icon: lucide.FolderKey(), Description: a.t("管理恢复备份与 Git 同步。"), Groups: []components.SettingsGroup{{ID: "backup", Items: vaultItems}, {ID: "sync", Title: a.t("Git 同步"), Items: syncItems}}},
		{ID: "browser", Title: a.t("浏览器"), Icon: lucide.Globe(), Description: a.t("让浏览器扩展安全地访问当前密码库。"), Groups: []components.SettingsGroup{{ID: "integration", Items: browserItems}}},
	}
}

func (a *application) settingsView(c *ui.Context) {
	a.shortcut.visible = false
	if !a.settingsLoaded {
		a.settings = a.state.Settings
		a.settingsSaved = a.state.Settings
		a.settingsObserved = a.state.Settings
		a.settingsLoaded = true
	}
	a.settingsContent(c, a.settingsCategories())

	if a.shortcut.recording && !a.shortcut.visible {
		a.setShortcutRecording(false)
	}
	if a.settings != a.settingsObserved {
		if a.settings.Theme != a.settingsObserved.Theme || a.settings.ThemeStyle != a.settingsObserved.ThemeStyle {
			a.applyTheme()
		}
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
	if a.state.Locked {
		a.beginUnlock("export")
		return
	}
	options := mygo.SaveDialogOptions{Parent: a.dialogParent(), Title: a.t("导出受口令保护的恢复私钥"), DefaultPath: "passpine-recovery.asc", Filters: []mygo.FileFilter{{Name: a.t("OpenPGP 私钥"), Extensions: []string{"asc"}}}, CreateDirectories: true}
	current := a.preferencesLease()
	go func() {
		path, err := mygo.Dialog.Save(options)
		mygo.RunOnMain(func() {
			if !current() {
				return
			}
			if err != nil {
				a.errorText = err.Error()
			} else if path != "" {
				a.call(app.Request{Action: "export", Destination: path}, func(app.Data) { a.notice = a.t("恢复私钥已导出。请使用验证备份确认可恢复。") })
			}
			a.invalidate()
		})
	}()
}
func (a *application) verifyBackup() {
	options := mygo.OpenDialogOptions{Parent: a.dialogParent(), Title: a.t("选择要验证的恢复私钥"), Filters: []mygo.FileFilter{{Name: a.t("OpenPGP 私钥"), Extensions: []string{"asc", "pgp", "gpg", "key"}}}}
	current := a.preferencesLease()
	go func() {
		paths, err := mygo.Dialog.Open(options)
		mygo.RunOnMain(func() {
			if !current() {
				return
			}
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
		section(c, a.t("操作记录"), "", nil)
		if len(a.activity) == 0 {
			muted(c, a.t("暂无操作记录"))
			return
		}
		components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				for _, entry := range a.activity {
					ui.Row(c).FillWidth().Padding(12).Gap(12).Background(c.Theme().Surface).Radius(8).Children(func() {
						components.Text(c, entry.Time).FontSize(11).TextColor(c.Theme().TextMuted)
						components.Text(c, a.activityLabel("action", entry.Action)).Grow(1).MinWidth(0).Wrap().Bold()
						components.Badge(c, a.activityLabel("source", entry.Source))
						components.Text(c, a.activityLabel("outcome", entry.Outcome)).FontSize(12)
					})
				}
			})
		})
	})
}
