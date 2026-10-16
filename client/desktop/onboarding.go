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
				components.Tabs(c, &a.setupMode, a.t("创建密码库"), a.t("导入密码库"), a.t("从 Git 下载"))
				if a.setupMode == 1 && a.setupDirectory == a.defaultVaultDirectory && a.defaultVaultDirectory != "" {
					muted(c, a.t("已找到默认密码库 ~/.password-store。输入私钥口令即可继续。"))
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
					}
					input := field(c, a.t("私钥口令"), &a.passphrase, a.t("输入你的私钥口令"), true)
					if a.setupMode == 0 {
						field(c, a.t("确认口令"), &a.confirmPassphrase, a.t("再次输入口令"), true)
						muted(c, a.t("口令至少需要 12 个字符。"))
					}
					label := a.t("创建并解锁")
					if a.setupMode == 1 {
						label = a.t("打开并解锁")
					}
					ui.Row(c).FillWidth().Gap(8).Children(func() {
						if a.state.Configured {
							if components.Button(c, a.t("返回"), components.ButtonGhost).Clicked() {
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
			a.inform(a.t("下载完成。输入系统 GPG 私钥口令即可继续。"))
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
		req.Mode = "system-gpg"
	}
	a.call(req, func(d app.Data) {
		a.history = navigationState{}
		a.setup = false
		a.passphrase = ""
		a.confirmPassphrase = ""
		a.page = "entries"
		a.filter = "all"
		a.selected = ""
		a.settings = d.Settings
		a.applyTheme()
		a.notice = a.t("密码库已打开")
		if d.DeviceUnlockNotice != "" {
			a.warn(d.DeviceUnlockNotice)
		}
	})
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
					if components.Button(c, a.t("使用 Touch ID 或 Mac 登录密码"), components.ButtonPrimary).FillWidth().Disabled(!a.state.DeviceUnlockAvailable).Clicked() {
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
