package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// Discovery suggests an existing pass store; opening it still requires a passphrase.
func (a *application) useDefaultVault(home string) {
	if a.state.Configured || a.state.Preview || a.setupDirectory != "" || home == "" {
		return
	}
	directory := filepath.Join(home, ".password-store")
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() {
		return
	}
	info, err = os.Lstat(filepath.Join(directory, ".gpg-id"))
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	a.defaultVaultDirectory = directory
	a.setupDirectory = directory
	a.setupMode = 1
}

func (a *application) setupView(c *ui.Context) {
	components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(38, 32).Children(func() {
			ui.Column(c).Width(440).MaxWidth(580).Gap(22).Children(func() {
				components.Icon(c, lucide.FolderKey).Size(42, 42).TextColor(c.Theme().Accent)
				section(c, a.t("打开密码库"), "", nil)
				previousMode := a.setupMode
				ui.Column(c).FillWidth().Disabled(a.busy).Children(func() {
					components.Tabs(c, &a.setupMode, components.TabsUnderline, a.t("创建密码库"), a.t("导入密码库"), a.t("从 Git 下载"))
				})
				if a.setupMode == 1 && a.setupDirectory == a.defaultVaultDirectory && a.defaultVaultDirectory != "" {
					muted(c, a.t("已找到默认密码库 ~/.password-store。选择密钥来源后即可导入。"))
				}
				if a.setupMode == 0 {
					muted(c, a.t("首次使用？创建密码库时会在本机生成受口令保护的 OpenPGP（GPG）私钥，无需安装 GnuPG 或准备已有密钥。"))
				}
				if previousMode != a.setupMode && a.defaultVaultDirectory != "" {
					if a.setupMode == 1 && a.setupDirectory == "" {
						a.setupDirectory = a.defaultVaultDirectory
					} else if previousMode == 1 && a.setupDirectory == a.defaultVaultDirectory {
						a.setupDirectory = ""
					}
				}
				if a.setupMode == 2 {
					ui.Column(c).FillWidth().Gap(14).Disabled(a.busy).Children(func() {
						field(c, a.t("Git 仓库地址"), &a.setupRemote, "git@github.com:you/password-store.git", false)
						ui.Row(c).FillWidth().Gap(8).AlignItems(ui.End).Children(func() {
							ui.Column(c).Grow(1).Children(func() {
								field(c, a.t("下载到新目录"), &a.setupDirectory, a.t("例如：/Users/you/password-store"), false)
							})
							if iconButton(c, lucide.FolderOpen(), a.t("选择下载位置"), components.ButtonOutline).Clicked() {
								a.chooseDirectory()
							}
						})
						muted(c, a.t("目标目录必须不存在。"))
						if components.Button(c, a.t("下载密码库"), components.ButtonPrimary).Clicked() {
							a.submitSetup()
						}
					})
					if a.busy {
						if components.Button(c, a.t("取消下载"), components.ButtonOutline).Clicked() {
							a.lock()
							a.inform(a.t("下载已取消，可以修改后重试。"))
						}
					}
					if a.state.Configured && !a.busy {
						if components.Button(c, a.t("返回密码库"), components.ButtonGhost).Clicked() {
							a.setup = false
						}
					}
					return
				}
				ui.Column(c).FillWidth().Gap(14).Disabled(a.busy).Children(func() {
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.End).Children(func() {
						ui.Column(c).Grow(1).Children(func() { field(c, a.t("密码库目录"), &a.setupDirectory, a.t("选择一个本机目录"), false) })
						if iconButton(c, lucide.FolderOpen(), a.t("选择密码库目录"), components.ButtonOutline).Clicked() {
							a.chooseDirectory()
						}
					})
					field(c, a.t("密码库名称"), &a.setupName, a.vaultNamePlaceholder(), false)
					if a.setupMode == 0 {
						field(c, a.t("密钥邮箱（可选）"), &a.setupEmail, "you@example.com", false)
					} else {
						ui.Column(c).FillWidth().Gap(6).Children(func() {
							components.Text(c, a.t("密钥来源")).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
							components.Tabs(c, &a.setupKeySource, components.TabsUnderline, a.t("系统 GPG"), a.t("密钥文件"))
						})
						if a.setupKeySource == 0 {
							muted(c, a.t("从本机 GPG 密钥环读取匹配的密钥。需要安装 GnuPG。"))
						} else {
							a.setupKeyFileField(c, false)
							a.setupKeyFileField(c, true)
							muted(c, a.t("私钥文件通常已包含公钥。多收件人密码库可补充公钥文件，无需安装 GnuPG。"))
						}
					}
					passphraseLabel, passphrasePlaceholder := a.t("私钥口令"), a.t("输入你的私钥口令")
					if a.setupMode == 0 {
						passphraseLabel, passphrasePlaceholder = a.t("设置私钥口令"), a.t("设置至少 12 个字符的口令")
					}
					input := field(c, passphraseLabel, &a.passphrase, passphrasePlaceholder, true)
					if a.setupMode == 0 {
						field(c, a.t("确认口令"), &a.confirmPassphrase, a.t("再次输入口令"), true)
						muted(c, a.t("口令至少需要 12 个字符。"))
					}
					label := a.t("创建密钥与密码库")
					if a.setupMode == 1 {
						label = a.t("打开并解锁")
					}
					ui.Row(c).FillWidth().Gap(8).Children(func() {
						if a.state.Configured {
							if components.Button(c, a.t("返回"), components.ButtonGhost).Clicked() {
								a.setup = false
								a.passphrase = ""
								a.confirmPassphrase = ""
								a.setupPrivateKey, a.setupPublicKey = "", ""
							}
						}
						ui.Spacer(c)
						if components.Button(c, label, components.ButtonPrimary).Clicked() || input.Submitted() {
							a.submitSetup()
						}
					})
				})
				if len(a.state.Vaults) > 0 {
					components.Text(c, a.t("最近打开")).Bold().FontSize(scaledFontSize(c, 12))
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
	if a.busy {
		return
	}
	if strings.TrimSpace(a.setupDirectory) == "" {
		a.errorText = a.t("请先选择密码库目录。")
		return
	}
	if a.setupMode == 2 {
		if strings.TrimSpace(a.setupRemote) == "" {
			a.errorText = a.t("请输入 Git 仓库地址。")
			return
		}
		a.call(app.Request{Action: "clone", URL: a.setupRemote, Directory: a.setupDirectory}, func(d app.Data) {
			if d.Destination != "" {
				a.setupDirectory = d.Destination
			}
			a.setupMode = 1
			a.inform(a.t("下载完成。选择密钥来源并输入私钥口令即可继续。"))
		})
		return
	}
	if a.setupMode == 0 && (len([]rune(a.passphrase)) < 12 || a.passphrase != a.confirmPassphrase) {
		a.errorText = a.t("口令至少需要 12 个字符，且两次输入必须一致。")
		return
	}
	req := app.Request{Directory: a.setupDirectory, Name: a.setupName, Email: a.setupEmail, Passphrase: a.passphrase}
	if a.setupMode == 0 {
		req.Action = "create"
	} else {
		req.Action = "open"
		if a.setupKeySource == 0 {
			req.Mode = "system-gpg"
		} else {
			if strings.TrimSpace(a.setupPrivateKey) == "" {
				a.errorText = a.t("请先选择私钥文件。")
				return
			}
			req.Mode = "key-file"
			req.KeyFile = a.setupPrivateKey
			req.PublicKeyFile = a.setupPublicKey
		}
	}
	a.call(req, func(d app.Data) {
		a.history = navigationState{}
		a.setup = false
		a.passphrase = ""
		a.confirmPassphrase = ""
		a.setupPrivateKey, a.setupPublicKey = "", ""
		a.page = "entries"
		a.filter = "all"
		a.selected = ""
		a.settings = d.Settings
		a.applyTheme()
		a.notice = a.t("密码库已打开")
		if req.Action == "create" {
			a.notice = a.t("密码库与私钥已创建。请在设置的“密码库”中导出恢复私钥并验证备份。")
		}
		if d.DeviceUnlockNotice != "" {
			if req.Action == "create" {
				a.warn(a.t("密码库与私钥已创建。%s。请在设置的“密码库”中导出恢复私钥并验证备份。", a.t(d.DeviceUnlockNotice)))
			} else {
				a.warn(d.DeviceUnlockNotice)
			}
		}
	})
}

func (a *application) setupKeyFileField(c *ui.Context, public bool) {
	label, placeholder, title, value := a.t("OpenPGP 私钥文件"), a.t("选择私钥文件"), a.t("选择私钥文件"), &a.setupPrivateKey
	if public {
		label, placeholder, title, value = a.t("OpenPGP 公钥文件（可选）"), a.t("选择补充公钥文件"), a.t("选择公钥文件"), &a.setupPublicKey
	}
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.End).Children(func() {
		ui.Column(c).Grow(1).Children(func() { field(c, label, value, placeholder, false) })
		if iconButton(c, lucide.FolderOpen(), title, components.ButtonOutline).Clicked() {
			a.chooseSetupKeyFile(public)
		}
	})
}

func (a *application) chooseSetupKeyFile(public bool) {
	title := a.t("选择私钥文件")
	if public {
		title = a.t("选择公钥文件")
	}
	options := mygo.OpenDialogOptions{Parent: a.dialogParent(), Title: title, ShowHiddenFiles: true,
		Filters: []mygo.FileFilter{{Name: a.t("OpenPGP 密钥文件"), Extensions: []string{"asc", "pgp", "gpg", "key"}}}}
	current, setup, directory := a.preferencesLease(), a.setup, a.setupDirectory
	go func() {
		paths, err := mygo.Dialog.Open(options)
		mygo.RunOnMain(func() {
			if !current() || setup != a.setup || a.setupMode != 1 || a.setupKeySource != 1 || directory != a.setupDirectory || a.busy {
				return
			}
			if err != nil {
				a.errorText = a.t("无法打开文件选择器，请重试或直接输入文件路径。")
			} else if len(paths) > 0 {
				if public {
					a.setupPublicKey = paths[0]
				} else {
					a.setupPrivateKey = paths[0]
				}
			}
			a.invalidate()
		})
	}()
}

func (a *application) unlockView(c *ui.Context) {
	ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Padding(40).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(360).MinWidth(0).Gap(20).Children(func() {
			components.Icon(c, lucide.LockKeyhole).Size(40, 40).TextColor(c.Theme().Accent)
			section(c, a.t("解锁密码库"), a.state.VaultName, nil)
			if a.selected != "" && a.state.Settings.BrowseWhileLocked {
				components.Text(c, entryName(a.selected)).Bold()
				muted(c, a.selected)
			}
			ui.Column(c).FillWidth().Gap(16).Disabled(a.busy).Children(func() {
				if a.state.DeviceUnlockEnrolled && a.state.Settings.DeviceUnlock {
					if components.Button(c, a.t("使用系统验证"), components.ButtonPrimary).FillWidth().Disabled(!a.state.DeviceUnlockAvailable).Clicked() {
						a.unlock("device")
					}
					if !a.state.DeviceUnlockAvailable {
						muted(c, a.t("系统解锁不可用，请在安全设置中关闭后使用私钥口令。"))
						if components.Button(c, a.t("打开安全设置"), components.ButtonOutline).FillWidth().Clicked() {
							a.settingsState.Category = "security"
							a.navigate("settings", "")
						}
					}
				} else {
					input := field(c, a.t("私钥口令"), &a.passphrase, a.t("输入口令解锁"), true)
					input.AutoFocus()
					if components.Button(c, a.t("解锁密码库"), components.ButtonPrimary).FillWidth().Clicked() || input.Submitted() {
						a.unlock("")
					}
				}
			})
			if components.Button(c, a.t("打开其他密码库"), components.ButtonGhost).Clicked() {
				a.openSetup()
			}
		})
	})
}

func (a *application) unlock(mode string) {
	req := app.Request{Action: "unlock", Mode: mode}
	if mode != "device" {
		req.Passphrase = a.passphrase
	}
	a.call(req, func(d app.Data) {
		a.passphrase = ""
		a.lastInput = time.Now()
		a.warn(d.DeviceUnlockNotice)
		if a.resumeUnlockAction() {
			return
		}
		if a.pendingCopyEntry != "" {
			entry, vaultID := a.pendingCopyEntry, a.pendingCopyVaultID
			a.pendingCopyEntry, a.pendingCopyVaultID = "", ""
			if !a.state.Locked && a.state.VaultID == vaultID && a.selected == entry && a.page == "entries" && !a.setup {
				a.beginDetailTransition(entry)
				a.detailTransition.unlocking = true
				a.call(app.Request{Action: "copy", Entry: entry, Field: "password"}, func(app.Data) {
					a.notice = a.t("已复制密码")
					a.selectEntry(entry)
				})
				return
			}
		}
		if a.preferencesPage() == "recovery" {
			a.loadRecovery()
		} else if a.selected != "" {
			a.selectEntry(a.selected)
			if a.detailTransition.entry == a.selected {
				a.detailTransition.unlocking = true
			}
		}
	})
}

func (a *application) switchVault(id string) {
	if a.editor.open {
		a.requestEditorClose(func() { a.switchVault(id) })
		return
	}
	if a.busy {
		return
	}
	a.clearSecrets()
	a.selected = ""
	a.query = ""
	a.call(app.Request{Action: "switch", ID: id}, func(app.Data) {
		a.setup = false
		a.page = "entries"
		a.filter = "all"
		a.history = navigationState{}
	})
}

func (a *application) chooseDirectory() {
	options := mygo.OpenDialogOptions{Parent: a.dialogParent(), Title: a.t("选择密码库目录"), Directory: true, ShowHiddenFiles: true, CreateDirectories: true, DefaultPath: a.setupDirectory}
	current, setup := a.preferencesLease(), a.setup
	go func() {
		paths, err := mygo.Dialog.Open(options)
		mygo.RunOnMain(func() {
			if !current() || setup != a.setup {
				return
			}
			if err != nil {
				a.errorText = err.Error()
			} else if len(paths) > 0 {
				a.setupDirectory = paths[0]
			}
			a.invalidate()
		})
	}()
}
