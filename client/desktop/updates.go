package desktop

import (
	"context"
	"regexp"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/about"
	"github.com/knothhe/passpine/client/buildinfo"
	"github.com/knothhe/passpine/client/update"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

type updateController interface {
	Start(context.Context)
	Snapshot() update.Snapshot
	Check(context.Context) error
	Download(context.Context) error
	Install(context.Context) error
	SetAutoDownload(bool)
	SetProtectedPaths([]string)
}

type updateUI struct {
	manager                          updateController
	snapshot                         update.Snapshot
	ctx                              context.Context
	cancel                           context.CancelFunc
	confirmOpen, preparing, starting bool
	quit                             func()
}

func (a *application) startUpdates(cfg update.Config) {
	if a.updates.manager != nil || a.updates.starting {
		return
	}
	a.updates.snapshot = update.Snapshot{State: update.StateDisabled, CurrentVersion: a.buildVersion}
	if a.state.Preview || !buildinfo.IsRelease() {
		a.invalidate()
		return
	}
	enabled := a.settings.AutoDownloadUpdates
	cfg.AutoDownload = &enabled
	for _, vault := range a.state.Vaults {
		cfg.ProtectedPaths = append(cfg.ProtectedPaths, vault.Directory)
	}
	cfg.OnChange = func(snapshot update.Snapshot) {
		a.runOnMain(func() { a.receiveUpdate(snapshot) })
	}
	a.updates.ctx, a.updates.cancel = context.WithCancel(context.Background())
	a.updates.quit = mygo.App.Quit
	a.updates.starting = true
	go func() {
		manager, err := update.New(cfg)
		a.runOnMain(func() {
			a.updates.starting = false
			if a.closing {
				return
			}
			if err != nil {
				a.updates.snapshot.ErrorCode = update.ErrorConfiguration
				a.invalidate()
				return
			}
			a.updates.manager = manager
			manager.SetAutoDownload(a.settingsSaved.AutoDownloadUpdates)
			a.updates.snapshot = manager.Snapshot()
			manager.Start(a.updates.ctx)
			a.invalidate()
		})
	}()
	a.invalidate()
}

// Update callbacks only repaint their own state. They never touch credential
// requests, navigation, editor inputs, or the vault's activity/idle timer.
func (a *application) receiveUpdate(snapshot update.Snapshot) {
	if a.closing || snapshot.Revision < a.updates.snapshot.Revision {
		return
	}
	a.updates.snapshot = snapshot
	a.invalidate()
}

func (a *application) updateContext() context.Context {
	if a.updates.ctx != nil {
		return a.updates.ctx
	}
	return context.Background()
}

func (a *application) checkUpdates() {
	if a.updates.manager == nil || a.updates.preparing {
		return
	}
	go func() { _ = a.updates.manager.Check(a.updateContext()) }()
}

func (a *application) downloadUpdate() {
	if a.updates.manager == nil || a.updates.preparing {
		return
	}
	go func() { _ = a.updates.manager.Download(a.updateContext()) }()
}

func (a *application) retryUpdate() {
	s := a.updates.snapshot
	switch s.ErrorCode {
	case update.ErrorDownload, update.ErrorVerification, update.ErrorStorage, update.ErrorInstall:
		if s.Version != "" {
			a.downloadUpdate()
			return
		}
	}
	a.checkUpdates()
}

func updateWorking(state update.State) bool {
	return state == update.StateChecking || state == update.StateDownloading || state == update.StateVerifying || state == update.StateInstalling
}

func (a *application) updateStatus() (string, string) {
	s := a.updates.snapshot
	switch s.State {
	case update.StateChecking:
		return a.t("正在检查更新"), a.t("正在获取可用版本。你可以继续使用 Passpine。")
	case update.StateLatest:
		return a.t("已是最新版本"), a.t("Passpine %s 已是最新版本。", a.buildVersion)
	case update.StateAvailable:
		return a.t("发现新版本 %s", s.Version), a.t("准备好后可手动下载更新。")
	case update.StateDownloading:
		return a.t("正在下载完整更新包"), a.t("正在下载完整更新包。完成后等待你确认安装。")
	case update.StateVerifying:
		return a.t("正在校验更新包"), a.t("正在检查更新包的完整性与签名。校验完成后才能安装。")
	case update.StateReady:
		return a.t("更新已准备好"), a.t("Passpine %s 已下载并通过校验。重启安装会短暂退出应用。", s.Version)
	case update.StateInstalling:
		return a.t("正在准备安装"), a.t("正在重新校验更新包，随后重启安装。")
	case update.StateFailed:
		return a.t("更新未完成"), a.updateError(s.ErrorCode)
	case update.StateDisabled:
		if a.state.Preview {
			return a.t("预览模式"), a.t("预览模式不会检查、下载或安装更新。")
		}
		if !buildinfo.IsRelease() {
			return a.t("开发版本"), a.t("开发版本不会检查、下载或安装正式更新。请从 GitHub Release 获取正式版本。")
		}
		return a.t("此版本暂不支持自动更新"), a.t("请从项目主页获取支持签名更新的正式版本。")
	default:
		return a.t("自动检查更新"), a.t("启动时和运行期间会自动检查更新。")
	}
}

func (a *application) updateError(code string) string {
	switch code {
	case update.ErrorNetwork:
		return a.t("无法检查更新。请检查网络连接后重试。")
	case update.ErrorSignature, update.ErrorManifest:
		return a.t("更新信息未通过安全校验。请稍后重试。")
	case update.ErrorVerification:
		return a.t("更新包未通过安全校验，未安装。请重新下载后重试。")
	case update.ErrorStorage:
		return a.t("无法保存更新包。请检查可用空间和目录权限后重试。")
	case update.ErrorInstall:
		return a.t("无法准备安装更新。请检查安装目录权限后重试。")
	case update.ErrorConfiguration:
		return a.t("请从项目主页获取支持签名更新的正式版本。")
	default:
		return a.t("更新下载失败。请检查网络连接后重试。")
	}
}

func (a *application) updateInstallBlocked() bool {
	return a.closing || a.busy || a.cli.operation != "" || a.configSync.operation != "" || a.gitActionPending || a.state.GitSyncing || a.settingsSaving || a.settingsDirty || a.gitRemoteSaving || !a.gitRemoteSaveAt.IsZero() || a.editor.open && a.editor.dirty() || a.discardOpen || a.generator.open || a.backupOpen || a.purgeOpen || a.deleteOpen
}

func (a *application) requestUpdateInstall() {
	if a.updates.snapshot.State != update.StateReady || a.updateInstallBlocked() {
		return
	}
	a.updates.confirmOpen = true
	a.invalidate()
}

func (a *application) installUpdate() {
	if a.updates.manager == nil || !a.updates.confirmOpen || a.updates.preparing || a.updateInstallBlocked() {
		return
	}
	a.updates.preparing = true
	protected := []string{}
	for _, vault := range a.state.Vaults {
		protected = append(protected, vault.Directory)
	}
	a.updates.manager.SetProtectedPaths(protected)
	ctx := a.updateContext()
	go func() {
		resume := func() {}
		if service, ok := a.service.(interface{ PauseForUpdate() func() }); ok {
			resume = service.PauseForUpdate()
		}
		err := a.updates.manager.Install(ctx)
		if err != nil {
			resume()
		}
		a.runOnMain(func() {
			if a.closing {
				return
			}
			if err != nil {
				a.updates.preparing, a.updates.confirmOpen = false, false
				a.updates.snapshot = a.updates.manager.Snapshot()
				if a.updates.snapshot.State != update.StateFailed {
					a.updates.snapshot.State, a.updates.snapshot.ErrorCode = update.StateFailed, update.ErrorInstall
				}
				a.invalidate()
				return
			}
			if a.updates.quit != nil {
				a.updates.quit()
			}
		})
	}()
	a.invalidate()
}

func (a *application) updatesView(c *ui.Context) {
	s := a.updates.snapshot
	title, detail := a.updateStatus()
	ui.Column(c).FillWidth().MinWidth(0).Gap(20).Children(func() {
		ui.Row(c).FillWidth().MinWidth(0).Wrap().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
				components.Text(c, a.t("当前版本")).Bold()
				version := a.t("开发版本")
				if a.buildVersion != "" && a.buildVersion != "development" {
					version = a.t("版本 %s", a.buildVersion)
				}
				muted(c, version)
			})
			label := a.t("检查更新")
			if s.State == update.StateChecking {
				label = a.t("检查中…")
			}
			if components.Button(c, label, components.ButtonOutline).Disabled(!s.Configured || updateWorking(s.State) || s.State == update.StateReady || a.updates.preparing).Clicked() {
				a.checkUpdates()
			}
		})
		ui.Column(c).FillWidth().MinWidth(0).Padding(18).Gap(12).Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(8).Children(func() {
			ui.Row(c).FillWidth().MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
				if updateWorking(s.State) {
					components.ProgressRing(c, -1).Size(18, 18).Label(title)
				} else {
					icon, color := lucide.Download(), c.Theme().TextMuted
					if s.State == update.StateReady || s.State == update.StateLatest {
						icon, color = lucide.CircleCheck(), c.Theme().Success
					}
					if s.State == update.StateFailed {
						icon, color = lucide.CircleAlert(), c.Theme().Danger
					}
					components.Icon(c, icon).Size(18, 18).TextColor(color)
				}
				components.Text(c, title).Bold().Grow(1).MinWidth(0).Wrap()
			})
			muted(c, detail)
			if s.State == update.StateDownloading {
				progress := s.Progress
				if s.Total <= 0 {
					progress = -1
				}
				components.Progress(c, progress).Label(a.t("更新下载进度"))
				if s.Total > 0 {
					muted(c, a.t("已下载 %.1f / %.1f MB · %.0f%%", float64(s.Downloaded)/1e6, float64(s.Total)/1e6, s.Progress*100))
				} else {
					muted(c, a.t("已下载 %.1f MB", float64(s.Downloaded)/1e6))
				}
			}
			if s.State == update.StateFailed {
				muted(c, a.t("本次更新尚未安装，现有版本与用户数据保持完整。"))
			}
			if s.State == update.StateDisabled && !a.state.Preview {
				ui.Link(c, a.t("项目主页"), about.ProjectURL)
			}
			if s.State == update.StateAvailable {
				if components.Button(c, a.t("下载更新"), components.ButtonPrimary).AlignSelf(ui.Start).Clicked() {
					a.downloadUpdate()
				}
			}
			if s.State == update.StateReady {
				if components.Button(c, a.t("重启并安装"), components.ButtonPrimary).AlignSelf(ui.Start).Disabled(a.updateInstallBlocked()).Clicked() {
					a.requestUpdateInstall()
				}
				if a.updateInstallBlocked() {
					muted(c, a.t("请先保存当前编辑，并等待正在进行的操作完成，再重启安装。"))
				}
			}
			if s.State == update.StateFailed {
				if components.Button(c, a.t("重试"), components.ButtonOutline).AlignSelf(ui.Start).Disabled(!s.Configured).Clicked() {
					a.retryUpdate()
				}
			}
		})
		a.updateReleaseNotesRow(c)
		muted(c, a.t("安装前会再次校验更新包。"))
	})
}

var updateReleaseVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// Only stable numeric versions produce a release tag. Snapshot.Version comes
// from the verified feed; the compiled version provides a useful fallback
// before a check succeeds. Development versions never invent release URLs.
func updateReleaseLink(available, current string) (version, target string) {
	for _, candidate := range []string{available, current} {
		if updateReleaseVersion.MatchString(candidate) {
			return candidate, "https://github.com/knothhe/passpine/releases/tag/v" + candidate
		}
	}
	return "", ""
}

func (a *application) updateReleaseNotesRow(c *ui.Context) {
	version, target := updateReleaseLink(a.updates.snapshot.Version, a.buildVersion)
	label, detail := a.t("更新说明"), a.t("暂无对应的更新说明")
	if target != "" {
		label = a.t("查看版本 %s 的更新说明", version)
		detail = a.t("v%s · GitHub Release", version)
	}
	row := ui.ButtonBase(c).Key("update-release-notes").Role(ui.RoleLink).Label(label).Row().
		FillWidth().MinWidth(0).Padding(15, 19).Gap(16).AlignItems(ui.Center).
		Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(8).
		TextColor(c.Theme().Text).Disabled(target == "")
	if row.Hovered() && target != "" {
		row.Background(c.Theme().SurfaceHover)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
			components.Text(c, a.t("更新说明")).Bold()
			muted(c, detail)
		})
		if target != "" {
			components.Icon(c, lucide.ExternalLink()).Size(17, 17).Shrink(0).TextColor(c.Theme().TextMuted)
		}
	})
	if row.Clicked() && target != "" {
		c.OpenURL(target)
	}
}

func (a *application) updateDialog(c *ui.Context) {
	components.Dialog(c, &a.updates.confirmOpen, components.DialogOptions{Label: a.t("重启并安装更新？"), DisableBackdropClose: true, DisableEscapeClose: a.updates.preparing}, func(panel ui.Element) {
		panel.Width(450).MaxWidth(500).Padding(24).Gap(18)
		components.Text(c, a.t("重启并安装更新？")).Bold().FontSize(scaledFontSize(c, 18))
		muted(c, a.t("将关闭 Passpine，安装版本 %s 后重新打开。密码库和设置会保留。", a.updates.snapshot.Version))
		if a.updates.preparing {
			components.ProgressRing(c, -1).Size(20, 20).Label(a.t("正在准备安装"))
			muted(c, a.t("正在重新校验更新包，随后重启安装。"))
		}
		if a.updateInstallBlocked() {
			muted(c, a.t("请先保存当前编辑，并等待正在进行的操作完成，再重启安装。"))
		}
		ui.Row(c).FillWidth().Wrap().Gap(8).Justify(ui.End).Children(func() {
			if components.Button(c, a.t("稍后"), components.ButtonOutline).Disabled(a.updates.preparing).Clicked() {
				a.updates.confirmOpen = false
			}
			if components.Button(c, a.t("重启并安装"), components.ButtonPrimary).Disabled(a.updates.preparing || a.updateInstallBlocked()).Clicked() {
				a.installUpdate()
			}
		})
	})
}
