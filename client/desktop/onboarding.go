package desktop

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine-neo/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func (a *application) setupView(c *ui.Context) {
	components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(38, 32).Children(func() {
			ui.Column(c).Width(440).MaxWidth(580).Gap(22).Children(func() {
				components.Icon(c, lucide.FolderKey).Size(42, 42).TextColor(c.Theme().Accent)
				section(c, "打开密码库", "", nil)
				components.Tabs(c, &a.setupMode, "创建密码库", "导入密码库", "从 Git 下载")
				if a.setupMode == 2 {
					ui.Column(c).FillWidth().Gap(14).Disabled(a.busy).Children(func() {
						field(c, "Git 仓库地址", &a.setupRemote, "git@github.com:you/password-store.git", false)
						ui.Row(c).FillWidth().Gap(8).AlignItems(ui.End).Children(func() {
							ui.Column(c).Grow(1).Children(func() { field(c, "下载到新目录", &a.setupDirectory, "例如：/Users/you/password-store", false) })
							if iconButton(c, lucide.FolderOpen(), "选择下载位置", components.ButtonOutline).Clicked() {
								a.chooseDirectory()
							}
						})
						muted(c, "使用本机 Git 的 SSH 或 HTTPS 认证。下载完成后再导入私钥，不需要重新下载。目标目录必须不存在。")
						if components.Button(c, "下载密码库", components.ButtonPrimary).Clicked() {
							a.submitSetup()
						}
					})
					if a.busy {
						if components.Button(c, "取消下载", components.ButtonOutline).Clicked() {
							a.lock()
							a.notice = "下载已取消，可以修改后重试。"
						}
					}
					if a.state.Configured && !a.busy {
						if components.Button(c, "返回密码库", components.ButtonGhost).Clicked() {
							a.setup = false
						}
					}
					return
				}
				ui.Column(c).FillWidth().Gap(14).Disabled(a.busy).Children(func() {
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.End).Children(func() {
						ui.Column(c).Grow(1).Children(func() { field(c, "密码库目录", &a.setupDirectory, "选择一个本机目录", false) })
						if iconButton(c, lucide.FolderOpen(), "选择密码库目录", components.ButtonOutline).Clicked() {
							a.chooseDirectory()
						}
					})
					if a.setupMode == 0 {
						field(c, "密码库名称", &a.setupName, "我的密码库", false)
						field(c, "密钥邮箱（可选）", &a.setupEmail, "you@example.com", false)
					} else {
						ui.Row(c).FillWidth().Gap(8).AlignItems(ui.End).Children(func() {
							ui.Column(c).Grow(1).Children(func() { field(c, "OpenPGP 私钥文件", &a.setupKey, "选择 .asc / .pgp 私钥", false) })
							if iconButton(c, lucide.FileKey(), "选择私钥文件", components.ButtonOutline).Clicked() {
								a.chooseKey()
							}
						})
						muted(c, "请选择从原客户端导出的恢复私钥。导入后，原密码库中的 .gpg 文件保持兼容。")
					}
					input := field(c, "私钥口令", &a.passphrase, "输入你的私钥口令", true)
					if a.setupMode == 0 {
						field(c, "确认口令", &a.confirmPassphrase, "再次输入口令", true)
						muted(c, "至少 12 个字符。请妥善保存口令，并在创建后导出恢复私钥。")
					}
					label := "创建并解锁"
					if a.setupMode == 1 {
						label = "导入并解锁"
					}
					ui.Row(c).FillWidth().Gap(8).Children(func() {
						if a.state.Configured {
							if components.Button(c, "返回", components.ButtonGhost).Clicked() {
								a.setup = false
								a.passphrase = ""
								a.confirmPassphrase = ""
							}
						}
						ui.Spacer(c)
						if components.Button(c, label, components.ButtonPrimary).Clicked() || input.Submitted() {
							a.submitSetup()
						}
					})
				})
				if len(a.state.Vaults) > 0 {
					components.Text(c, "最近打开").Bold().FontSize(12)
					for _, v := range a.state.Vaults {
						v := v
						if components.Button(c, v.Name, components.ButtonOutline).FillWidth().Clicked() {
							a.switchVault(v.ID)
						}
					}
				}
			})
		})
	})
}

func (a *application) submitSetup() {
	if strings.TrimSpace(a.setupDirectory) == "" {
		a.errorText = "请先选择密码库目录。"
		return
	}
	if a.setupMode == 2 {
		if strings.TrimSpace(a.setupRemote) == "" {
			a.errorText = "请输入 Git 仓库地址。"
			return
		}
		a.call(app.Request{Action: "clone", URL: a.setupRemote, Directory: a.setupDirectory}, func(d app.Data) {
			if d.Destination != "" {
				a.setupDirectory = d.Destination
			}
			a.setupMode = 1
			a.notice = "下载完成。选择私钥并输入口令即可继续。"
		})
		return
	}
	if a.setupMode == 0 && (len([]rune(a.passphrase)) < 12 || a.passphrase != a.confirmPassphrase) {
		a.errorText = "口令至少需要 12 个字符，且两次输入必须一致。"
		return
	}
	if a.setupMode == 1 && strings.TrimSpace(a.setupKey) == "" {
		a.errorText = "请选择受口令保护的 OpenPGP 私钥文件。"
		return
	}
	req := app.Request{Directory: a.setupDirectory, KeyFile: a.setupKey, Name: a.setupName, Email: a.setupEmail, Passphrase: a.passphrase}
	if a.setupMode == 0 {
		req.Action = "create"
	} else {
		req.Action = "open"
	}
	a.call(req, func(d app.Data) {
		a.setup = false
		a.passphrase = ""
		a.confirmPassphrase = ""
		a.page = "entries"
		a.filter = "all"
		a.selected = ""
		a.unlockTime = time.Now()
		a.settings = d.Settings
		a.applyTheme()
		a.notice = "密码库已就绪。建议现在导出并验证恢复私钥。"
	})
}

func (a *application) unlockView(c *ui.Context) {
	ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Padding(40).Children(func() {
		ui.Column(c).Width(360).Gap(20).Children(func() {
			components.Icon(c, lucide.LockKeyhole).Size(40, 40).TextColor(c.Theme().Accent)
			section(c, "解锁密码库", a.state.VaultName, nil)
			ui.Column(c).FillWidth().Gap(16).Disabled(a.busy).Children(func() {
				input := field(c, "私钥口令", &a.passphrase, "输入口令解锁", true)
				input.AutoFocus()
				if components.Button(c, "解锁密码库", components.ButtonPrimary).FillWidth().Clicked() || input.Submitted() {
					a.call(app.Request{Action: "unlock", Passphrase: a.passphrase}, func(app.Data) {
						a.passphrase = ""
						a.unlockTime = time.Now()
						a.lastInput = time.Now()
						if a.page == "recovery" {
							a.page = "entries"
						}
						a.notice = ""
					})
				}
			})
			if components.Button(c, "打开其他密码库", components.ButtonGhost).Clicked() {
				a.setup = true
			}
		})
	})
}

func (a *application) switchVault(id string) {
	if a.busy {
		return
	}
	a.clearSecrets()
	a.selected = ""
	a.query = ""
	a.call(app.Request{Action: "switch", ID: id}, func(app.Data) { a.setup = false; a.page = "entries"; a.filter = "all" })
}

func (a *application) chooseDirectory() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.window, Title: "选择密码库目录", Directory: true, ShowHiddenFiles: true, CreateDirectories: true, DefaultPath: a.setupDirectory})
		mygo.RunOnMain(func() {
			if err != nil {
				a.errorText = err.Error()
			} else if len(paths) > 0 {
				a.setupDirectory = paths[0]
			}
			a.invalidate()
		})
	}()
}

func (a *application) chooseKey() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.window, Title: "选择 OpenPGP 私钥", ShowHiddenFiles: true, Filters: []mygo.FileFilter{{Name: "OpenPGP 私钥", Extensions: []string{"asc", "pgp", "gpg", "key"}}}})
		mygo.RunOnMain(func() {
			if err != nil {
				a.errorText = err.Error()
			} else if len(paths) > 0 {
				a.setupKey = paths[0]
			}
			a.invalidate()
		})
	}()
}
