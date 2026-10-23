package desktop

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

type gitRecoveryPresentation struct {
	Title, Hint, Error  string
	Action, ActionLabel string
	Progress, Warning   bool
}

// Recovery depends only on reported Git state. A missing first status must not
// turn the zero values into a claim that Git, a repository, or a remote is absent.
func presentGitRecovery(state app.Data, setup, pending bool) gitRecoveryPresentation {
	if !state.Configured || setup {
		return gitRecoveryPresentation{}
	}
	if state.GitSyncing || pending {
		title, hint := "正在处理 Git 操作…", "Git 操作正在后台进行，本地密码库仍可使用。"
		if state.GitSyncing {
			title, hint = "正在同步", "正在同步加密条目，可以继续使用密码库。"
		}
		return gitRecoveryPresentation{Title: title, Hint: hint, Progress: true}
	}
	p := gitRecoveryPresentation{Error: strings.TrimSpace(state.GitError)}
	switch {
	case state.GitConflict:
		p.Title, p.Hint, p.Warning = "存在同步冲突", "已保存的本地内容仍可使用。请先解决仓库冲突，再重新同步。", true
	case state.GitStatusAt != "" && !state.GitAvailable:
		p.Title, p.Hint = "Git 不可用", "已保存的本地内容仍可使用。安装 Git 后可启用密码库同步。"
	case state.GitStatusAt != "" && !state.GitInitialized:
		p.Title, p.Hint = "未初始化 Git", "已保存的本地内容仍可使用。初始化 Git 后可备份与同步加密条目。"
		p.Action, p.ActionLabel = "git-init", "初始化 Git"
	case state.GitStatusAt != "" && strings.TrimSpace(state.GitRemote) == "":
		p.Title, p.Hint = "未配置远端", "已保存的本地内容仍可使用。添加远端地址后可跨设备同步。"
	case p.Error != "":
		p.Title, p.Hint, p.Warning = "远端同步未完成", "已保存的本地内容仍可使用。查看错误详情，确认同步配置后重试。", true
		if state.GitAvailable && state.GitInitialized && strings.TrimSpace(state.GitRemote) != "" {
			p.Action, p.ActionLabel = "git-sync", "重试同步"
		}
	default:
		return gitRecoveryPresentation{}
	}
	return p
}

func (a *application) gitRecoveryView(c *ui.Context) {
	p := presentGitRecovery(a.state, a.setup, a.gitActionPending)
	if p.Title == "" {
		return
	}
	t := c.Theme()
	ui.Column(c).Key("git-recovery").FillWidth().MinWidth(0).Shrink(0).Margin(0, 0, 12, 0).Gap(5).Label(a.t("Git 同步状态")).Children(func() {
		ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(8).Children(func() {
			color, icon := t.TextMuted, lucide.GitBranch()
			if p.Warning {
				color, icon = t.Danger, lucide.CircleAlert()
			}
			if p.Progress {
				components.ProgressRing(c, -1).Size(16, 16).Shrink(0).Label(a.t(p.Title))
			} else {
				components.Icon(c, icon).Size(16, 16).Shrink(0).TextColor(color)
			}
			components.Text(c, a.t(p.Title)).Grow(1).MinWidth(0).FontSize(scaledFontSize(c, 13)).Bold().Wrap().TextColor(color)
			if p.Action != "" {
				if components.Button(c, a.t(p.ActionLabel), components.ButtonOutline).Shrink(0).Disabled(a.updates.preparing).Clicked() {
					a.runGitRecoveryAction(p.Action)
				}
			}
			if components.Button(c, a.t("查看同步设置"), components.ButtonGhost).Shrink(0).Clicked() {
				a.navigateSettingsCategory("vault")
			}
		})
		components.Text(c, a.t(p.Hint)).FillWidth().MinWidth(0).FontSize(scaledFontSize(c, 12)).TextColor(t.TextMuted).Wrap()
		if p.Error != "" {
			components.Text(c, p.Error).FillWidth().MinWidth(0).FontSize(scaledFontSize(c, 11)).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Tooltip(p.Error)
		}
	})
}

// Recheck the presentation so a click from an older frame cannot retry while
// Git is already running, conflicted, or missing a synchronization prerequisite.
func (a *application) runGitRecoveryAction(action string) {
	p := presentGitRecovery(a.state, a.setup, a.gitActionPending)
	if p.Action == "" || action != p.Action {
		return
	}
	a.call(app.Request{Action: action}, func(app.Data) {
		a.notice = a.t("同步完成")
		if action == "git-init" {
			a.notice = a.t("Git 已初始化")
		}
	})
}
