package desktop

import (
	"errors"
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/buildinfo"
	"github.com/knothhe/passpine/client/internal/commandlink"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

const cliProgressDelay = 180 * time.Millisecond

type cliSettingsState struct {
	status       commandlink.Status
	directory    string
	loaded       bool
	includeAlias bool
	operation    string
	started      time.Time
	helpExpanded bool
	err          error
	manager      func() (commandlink.Manager, error)
}

// This worker is independent of vault reads and settings writes. Repainting or
// changing language never starts another filesystem operation.
func (a *application) manageCLI(operation string) {
	if a.closing || a.quit.requested || a.updates.preparing || a.cli.operation != "" || a.state.Preview || runtime.GOOS == "windows" {
		return
	}
	if operation != "status" && operation != "install" && operation != "uninstall" {
		return
	}
	manager := a.cli.manager
	if manager == nil {
		manager = func() (commandlink.Manager, error) { return commandlink.Current("") }
	}
	includeAlias := a.cli.includeAlias
	// Retain the previous result until its replacement is ready. In particular,
	// retrying a failure must not collapse and reinsert the error region.
	a.cli.operation = operation
	a.cli.started = time.Now()
	go func() {
		m, err := manager()
		var status commandlink.Status
		var statusErr error
		if err == nil {
			switch operation {
			case "install":
				err = m.Install(includeAlias)
			case "uninstall":
				err = m.Uninstall()
			}
			// Always inspect after a mutation, including partial failures.
			status, statusErr = m.Status()
		} else {
			statusErr = err
			err = nil
		}
		a.runOnMain(func() {
			a.cli.operation = ""
			a.cli.started = time.Time{}
			if a.closing {
				return
			}
			a.cli.err = errors.Join(err, statusErr)
			a.cli.loaded = statusErr == nil
			if a.cli.loaded {
				a.cli.status, a.cli.directory = status, m.Directory
			}
			a.invalidate()
			a.continueQuit()
		})
	}()
	a.invalidate()
}

func (a *application) cliEntryStatus(entry commandlink.Entry) string {
	switch {
	case !a.cli.loaded:
		return a.t("尚未检查")
	case entry.Owned:
		return a.t("已安装")
	case entry.Exists:
		return a.t("已占用")
	default:
		return a.t("未安装")
	}
}

func (a *application) cliStatusBadge(c *ui.Context, entry commandlink.Entry) {
	t := c.Theme()
	background, foreground, border := t.Surface, t.Text, t.Border
	hint := ""
	if a.cli.loaded {
		if entry.Owned {
			background, foreground, border = t.Background.Mix(t.Success, .12), t.Success.Mix(t.Text, .3), t.Background.Mix(t.Success, .3)
			if !entry.Removable {
				hint = a.t("由应用安装器提供")
			}
		} else if entry.Exists {
			background, foreground, border = t.Background.Mix(t.Warning, .12), t.Warning.Mix(t.Text, .3), t.Background.Mix(t.Warning, .3)
			hint = a.t("被其他程序占用")
		}
	}
	// Use the shared badge's appearance without its keyed text exit/enter and
	// size transitions: a command result should paint in one stable frame.
	ui.Row(c).AlignSelf(ui.Start).Shrink(0).FontSize(t.Rem(.85)).FontWeight(500).
		Padding(t.Space(.5), t.Space(2)).Radius(t.Space(3)).
		Background(background).TextColor(foreground).Border(1, border).Tooltip(hint).Children(func() {
		components.Text(c, a.cliEntryStatus(entry)).SingleLine().Unselectable().Shrink(0)
	})
}

func (a *application) cliOperationLabel() string {
	switch a.cli.operation {
	case "install":
		return a.t("正在安装命令…")
	case "uninstall":
		return a.t("正在卸载命令…")
	default:
		return a.t("正在检查命令状态…")
	}
}

func (a *application) cliSettingsView(c *ui.Context) {
	working := a.cli.operation != ""
	// A short filesystem operation must not toggle every control's disabled
	// style. The worker rejects repeated actions; alias edits are a draft for
	// the next install and cannot change the in-flight request's captured choice.
	disabled := a.quit.requested || a.updates.preparing
	showProgress := false
	if working {
		if remaining := cliProgressDelay - c.Now().Sub(a.cli.started); remaining > 0 {
			c.After(remaining)
		} else {
			showProgress = true
		}
	}
	ui.Column(c).Key("cli-settings").FillWidth().MaxWidth(664).MinWidth(0).Gap(12).Children(func() {
		ui.Column(c).Key("installation-card").FillWidth().MinWidth(0).Padding(20).Gap(20).Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(10).Children(func() {
			if runtime.GOOS == "windows" {
				muted(c, a.t("Windows 版暂不支持命令行，请在 macOS 或 Linux 上使用 %s。", a.t("命令行工具")))
				return
			}
			if a.state.Preview {
				muted(c, a.t("预览模式不会安装或移除命令链接。"))
				return
			}
			ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Box(c).Size(40, 40).Shrink(0).Radius(8).Background(c.Theme().TextMuted.Alpha(.08)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					components.Icon(c, lucide.Terminal()).Size(22, 22).TextColor(c.Theme().TextMuted)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
					name := a.cli.status.Main.Name
					if name == "" {
						name = "passpine-dev"
						if buildinfo.IsRelease() {
							name = "passpine"
						}
					}
					components.Text(c, name).Bold().FontSize(scaledFontSize(c, 16))
					a.cliStatusBadge(c.Key("main-status"), a.cli.status.Main)
				})
				ui.Box(c).Key("refresh-control").Size(32, 32).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					if showProgress {
						components.ProgressRing(c, -1).Size(20, 20).Label(a.cliOperationLabel())
					} else if iconButton(c, lucide.RefreshCw(), a.t("刷新状态"), components.ButtonGhost).Tooltip(a.t("刷新状态")).Disabled(disabled).Clicked() && !working {
						a.manageCLI("status")
					}
				})
			})
			ui.Box(c).FillWidth().Height(1).Background(c.Theme().Border)
			ui.Row(c).FillWidth().MinWidth(0).Wrap().AlignItems(ui.Center).Gap(12).Children(func() {
				components.Checkbox(c, &a.cli.includeAlias, a.t("同时安装 pass 别名")).Grow(1).Shrink(0).Disabled(disabled).Tooltip(a.t("点击安装时生效。"))
				a.cliStatusBadge(c.Key("alias-status"), a.cli.status.Alias)
			})
			ui.Row(c).FillWidth().MinWidth(0).Wrap().Gap(8).Children(func() {
				if components.Button(c, a.t("安装命令"), components.ButtonPrimary).Disabled(disabled || !a.cli.loaded).Clicked() && !working {
					a.manageCLI("install")
				}
				removable := a.cli.status.Main.Removable || a.cli.status.Alias.Removable
				if components.Button(c, a.t("卸载"), components.ButtonGhost).Disabled(disabled || !a.cli.loaded || !removable).Tooltip(a.t("只移除当前应用的命令链接，不影响其他程序。")).Clicked() && !working {
					a.manageCLI("uninstall")
				}
			})
			if a.cli.err != nil {
				var conflict *commandlink.ConflictError
				message := a.t("命令行操作失败：%s", a.cli.err)
				if errors.As(a.cli.err, &conflict) {
					message = a.t("%s 已存在且不是当前 Passpine，未覆盖。", conflict.Path)
				}
				components.Text(c, message).Key("operation-error").TextColor(c.Theme().Danger).Wrap()
			}
		})
		if runtime.GOOS == "windows" || a.state.Preview {
			return
		}
		if a.cli.directory != "" {
			ui.Row(c).Key("installation-location").FillWidth().MinWidth(0).Wrap().Gap(10).Children(func() {
				muted(c, a.t("安装位置"))
				components.Text(c, a.cli.directory).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted).Wrap()
			})
		}
		parts := components.Collapsible(c.Key("usage-tips"), a.t("使用说明"), &a.cli.helpExpanded, components.CollapsibleOptions{ArrowPosition: components.ArrowLeft}, func() {
			if a.cli.directory != "" {
				components.Text(c, a.t("若终端找不到命令，将 %s 加入 PATH。", a.cli.directory)).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted).Wrap()
			}
			components.Text(c, a.t("只移除当前应用的命令链接，不影响其他程序。")).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted).Wrap()
		})
		parts.Root.FillWidth().MinWidth(0)
	})
}
