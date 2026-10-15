package desktop

import (
	"fmt"
	"strings"

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
func (a *application) generatorView(c *ui.Context) {
	components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(32).Gap(26).Children(func() {
			section(c, "密码生成器", "", nil)
			ui.Column(c).FillWidth().Padding(24).Gap(20).Background(c.Theme().Surface).Radius(12).Border(1, c.Theme().Border).Children(func() {
				password := a.generator.password
				if password == "" {
					password = "点击下方按钮，生成新密码"
				}
				components.Text(c, password).Wrap().FontSize(22).TextColor(c.Theme().Accent)
				ui.Row(c).Gap(8).Children(func() {
					if components.Button(c, "生成密码", components.ButtonPrimary).Disabled(a.busy).Clicked() {
						a.generate(nil)
					}
					if iconButton(c, lucide.Copy(), "复制生成的密码", components.ButtonOutline).Disabled(a.generator.password == "").Clicked() {
						a.copy(a.generator.password, "密码")
					}
				})
			})
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				components.Text(c, "密码长度").Bold()
				ui.Spacer(c)
				components.NumberInput(c, &a.generator.length, 8, 128, 1).Label("密码长度")
			})
			ui.Column(c).FillWidth().Gap(16).Children(func() {
				components.Switch(c, &a.generator.lower, "小写字母  a–z")
				components.Switch(c, &a.generator.upper, "大写字母  A–Z")
				components.Switch(c, &a.generator.numbers, "数字  0–9")
				components.Switch(c, &a.generator.symbols, "特殊符号  !@#$")
				components.Switch(c, &a.generator.exclude, "排除易混淆字符  I l 1 O 0")
			})
			muted(c, "生成的密码不会自动保存。复制后会按你的安全设置清理剪贴板。")
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
	if a.state.Locked {
		return
	}
	a.call(app.Request{Action: "recovery", Kind: a.recoveryType()}, func(d app.Data) { a.recovery = d.Recovery })
}
func (a *application) recoveryView(c *ui.Context) {
	ui.Column(c).Fill().Padding(30).Gap(20).Children(func() {
		section(c, "恢复中心", "恢复副本仅保存在本机，不随 Git 同步。", nil)
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
						ui.Column(c).Grow(1).Gap(5).Children(func() {
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
	if !a.settingsLoaded {
		a.settings = a.state.Settings
		a.settingsLoaded = true
	}
	width, _ := c.Size()
	if a.shell != nil && a.shell.LeftExpanded() {
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
		selectBox.Trigger.Label(label).MinWidth(155).Children(func() { components.Text(c, display) })
		selectBox.Popup(func(ui.Element) {
			for i, v := range values {
				v, label := v, labels[i]
				selectBox.Item(v).Label(label).Children(func() { components.Text(c, label) })
			}
		})
	}
	number := func(c *ui.Context, value *int, label string, lo, hi float64) {
		v := float64(*value)
		input := components.NumberInput(c, &v, lo, hi, 1).Label(label)
		if input.Changed() {
			*value = int(v)
		}
	}
	button := func(c *ui.Context, label, action string) {
		if components.Button(c, label, components.ButtonOutline).Disabled(a.busy).Clicked() {
			a.call(app.Request{Action: action}, func(app.Data) { a.notice = label + "完成" })
		}
	}
	general := []components.SettingsItem{
		{ID: "theme", Title: "外观", Build: func(c *ui.Context) {
			selectValue(c, &a.settings.Theme, "外观", []string{"system", "light", "dark"}, []string{"跟随系统", "浅色", "深色"})
		}},
		{ID: "close", Title: "关闭窗口后继续运行", Description: "保留菜单栏图标和浏览器连接。使用菜单中的退出可完全关闭。", Build: func(c *ui.Context) {
			components.Switch(c, &a.settings.CloseToTray, "关闭窗口后继续运行", components.SwitchOptions{HideLabel: true})
		}},
		{ID: "shortcut", Title: "全局搜索快捷键", Description: "Control + Option + Space 唤出密码搜索。演示模式不注册全局快捷键。"},
	}
	security := []components.SettingsItem{
		{ID: "idle", Title: "空闲自动锁定", Description: "1–240 分钟。锁屏和休眠时始终锁定。", Build: func(c *ui.Context) { number(c, &a.settings.IdleMinutes, "空闲自动锁定", 1, 240) }},
		{ID: "max", Title: "最长解锁时间", Description: "1–1440 分钟。到期后重新验证私钥口令。", Build: func(c *ui.Context) { number(c, &a.settings.MaxSessionMinutes, "最长解锁时间", 1, 1440) }},
		{ID: "clipboard", Title: "清理剪贴板", Description: "5–300 秒；只清除仍由 Passpine 持有的内容。", Build: func(c *ui.Context) { number(c, &a.settings.ClipboardSeconds, "清理剪贴板", 5, 300) }},
		{ID: "locked", Title: "锁定时允许列出名称", Description: "浏览器只能看到条目名称，仍无法读取密码。", Build: func(c *ui.Context) {
			components.Switch(c, &a.settings.BrowseWhileLocked, "锁定时允许列出名称", components.SwitchOptions{HideLabel: true})
		}},
	}
	vaultItems := []components.SettingsItem{
		{ID: "directory", Title: a.state.VaultName, Description: a.state.Directory, Build: func(c *ui.Context) {
			if components.Button(c, "切换密码库", components.ButtonOutline).Clicked() {
				a.setup = true
			}
		}},
		{ID: "export", Title: "导出恢复私钥", Description: "恢复需要密码库、受口令保护的私钥和口令，请分别备份。", Build: func(c *ui.Context) {
			if components.Button(c, "导出…", components.ButtonOutline).Clicked() {
				a.exportKey()
			}
		}},
		{ID: "verify", Title: "验证恢复备份", Build: func(c *ui.Context) {
			if components.Button(c, "选择并验证…", components.ButtonOutline).Clicked() {
				a.verifyBackup()
			}
		}},
	}
	syncItems := []components.SettingsItem{
		{ID: "remote", Title: "Git 远端地址", Description: "设置 origin；使用本机 SSH 或 HTTPS 凭据认证。", Build: func(c *ui.Context) {
			ui.Column(c).Width(220).Gap(8).Children(func() {
				components.TextBox(c, &a.gitRemote, components.TextBoxOptions{}).FillWidth().Label("Git 远端地址").Placeholder("git@github.com:you/store.git")
				if components.Button(c, "保存远端", components.ButtonOutline).Disabled(a.busy).Clicked() {
					a.call(app.Request{Action: "git-remote", URL: a.gitRemote}, func(d app.Data) { a.gitRemote = d.GitRemote; a.notice = "Git 远端地址已保存" })
				}
			})
		}},
		{ID: "auto", Title: "变更后自动同步", Description: "只同步加密条目与收件人文件。Git 远端认证由系统 Git 处理。", Build: func(c *ui.Context) {
			components.Switch(c, &a.settings.AutoSync, "自动同步", components.SwitchOptions{HideLabel: true})
		}},
		{ID: "interval", Title: "定时同步间隔", Description: "分钟；0 表示不定时同步。", Build: func(c *ui.Context) { number(c, &a.settings.SyncMinutes, "定时同步间隔", 0, 1440) }},
		{ID: "git", Title: "Git 同步状态", Description: strings.TrimSpace(a.state.GitSummary + " " + a.state.GitError), Build: func(c *ui.Context) {
			if a.state.GitInitialized {
				button(c, "立即同步", "git-sync")
			} else {
				button(c, "初始化 Git", "git-init")
			}
		}},
	}
	browserItems := []components.SettingsItem{
		{ID: "browser", Title: "连接浏览器扩展", Description: "安装或更新应用后重新注册。", Build: func(c *ui.Context) { button(c, "注册浏览器连接", "browser-install") }},
		{ID: "activity", Title: "本机操作记录", Description: "记录操作类型与结果，不记录密码、网址和条目名称。", Build: func(c *ui.Context) {
			components.Switch(c, &a.settings.RecordActivity, "本机操作记录", components.SwitchOptions{HideLabel: true})
		}},
	}
	ui.Column(c).Fill().Children(func() {
		components.Settings(c, &a.settingsState, []components.SettingsCategory{
			{ID: "general", Title: "通用", Groups: []components.SettingsGroup{{ID: "appearance", Items: general}}},
			{ID: "security", Title: "安全", Groups: []components.SettingsGroup{{ID: "session", Items: security}}},
			{ID: "vault", Title: "密码库", Groups: []components.SettingsGroup{{ID: "backup", Items: vaultItems}, {ID: "sync", Title: "Git 同步", Items: syncItems}}},
			{ID: "browser", Title: "浏览器", Groups: []components.SettingsGroup{{ID: "integration", Items: browserItems}}},
		}, components.SettingsOptions{Narrow: width < 700, Title: "设置", SearchLabel: "搜索设置", SearchPlaceholder: "搜索设置…", CategoryLabel: "设置分类", ResultsTitle: "搜索结果", EmptyText: "没有找到相关设置"})
		ui.Row(c).FillWidth().Padding(16, 24).Gap(10).AlignItems(ui.Center).Children(func() {
			muted(c, "修改后点击保存生效")
			ui.Spacer(c)
			if components.Button(c, "保存设置", components.ButtonPrimary).Disabled(a.busy).Clicked() {
				settings := a.settings
				a.call(app.Request{Action: "settings", Settings: &settings}, func(d app.Data) { a.settings = d.Settings; a.applyTheme(); a.notice = "设置已保存" })
			}
		})
	})
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
						components.Text(c, entry.Action).Grow(1).Bold()
						components.Badge(c, entry.Source)
						components.Text(c, fmt.Sprint(entry.Outcome)).FontSize(12)
					})
				}
			})
		})
	})
}
