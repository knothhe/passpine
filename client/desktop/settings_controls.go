package desktop

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func (a *application) pageNavigation(c *ui.Context) {
	target, label := "settings", "返回设置"
	if a.page == "settings" {
		target, label = "entries", "返回密码库"
	}
	ui.Row(c).FillWidth().Shrink(0).Padding(8, 16).AlignItems(ui.Center).Children(func() {
		button := components.Button(c, "", components.ButtonGhost).Label(label)
		button.Children(func() {
			components.Icon(c, lucide.ChevronLeft).Size(16, 16)
			components.Text(c, label)
		})
		if button.Clicked() {
			a.setup = false
			a.navigate(target, "")
		}
	})
}

// Preserve a previously saved custom duration as a visible option. Rendering
// never changes a setting; only an explicit segment selection writes it.
func durationSegments(c *ui.Context, value *int, label string, presets []int, unit string) {
	values := append([]int(nil), presets...)
	found := false
	for _, preset := range values {
		if preset == *value {
			found = true
			break
		}
	}
	if !found {
		values = append(values, *value)
		sort.Ints(values)
	}
	labels := make([]string, len(values))
	selected := 0
	for i, duration := range values {
		if duration == *value {
			selected = i
		}
		labels[i] = fmt.Sprintf("%d %s", duration, unit)
		if duration == 0 {
			labels[i] = "关闭"
		} else if unit == "分钟" && duration >= 60 && duration%60 == 0 {
			labels[i] = fmt.Sprintf("%d 小时", duration/60)
		}
	}
	if ui.Segmented(c, &selected, labels...).Label(label).AlignSelf(ui.Center).Changed() {
		*value = values[selected]
	}
}

func settingSwitch(c *ui.Context, value *bool, label string) ui.Element {
	return components.Switch(c, value, label, components.SwitchOptions{HideLabel: true}).AlignSelf(ui.Center).Shrink(0)
}

func gitTime(value string) string {
	stamp, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return ""
	}
	return stamp.Local().Format("01-02 15:04")
}

func (a *application) deviceUnlockDescription() string {
	if !a.state.DeviceUnlockAvailable {
		return "此设备的系统解锁不可用"
	}
	if a.settings.DeviceUnlock && !a.state.DeviceUnlockEnrolled {
		return "使用私钥口令解锁后完成启用"
	}
	return ""
}

func (a *application) gitStatusLabel() string {
	switch {
	case a.state.GitSyncing:
		return "正在同步"
	case a.state.GitRefreshing && a.state.GitStatusAt == "":
		return "正在检查"
	case a.state.GitConflict:
		return "存在冲突"
	case a.state.GitError != "":
		return "同步失败"
	case !a.state.GitAvailable && a.state.GitStatusAt != "":
		return "Git 不可用"
	case !a.state.GitInitialized:
		return "未初始化 Git"
	case a.state.GitRemote == "":
		return "未配置远端"
	case a.state.GitUpstream == "":
		return "尚未建立远端跟踪"
	case a.state.GitChanges > 0:
		return fmt.Sprintf("%d 项变更", a.state.GitChanges)
	case a.state.GitAhead > 0 || a.state.GitBehind > 0:
		return "有待同步的提交"
	default:
		return "已同步"
	}
}

func (a *application) gitStatusDescription() string {
	lines := []string{a.gitStatusLabel()}
	if a.state.GitBranch != "" {
		lines = append(lines, "分支："+a.state.GitBranch)
	}
	if a.state.GitUpstream != "" {
		lines = append(lines, "跟踪分支："+a.state.GitUpstream)
	}
	if a.state.GitInitialized {
		lines = append(lines, fmt.Sprintf("%d 项变更 · %d 个提交待推送 · %d 个提交待拉取", a.state.GitChanges, a.state.GitAhead, a.state.GitBehind))
	}
	if a.state.GitError != "" {
		lines = append(lines, a.state.GitError)
	} else if a.state.GitLastResult != "" {
		lines = append(lines, a.state.GitLastResult)
	}
	if stamp := gitTime(a.state.GitLastSync); stamp != "" {
		lines = append(lines, "上次同步："+stamp)
	}
	if stamp := gitTime(a.state.GitStatusAt); stamp != "" {
		lines = append(lines, "状态更新："+stamp)
	}
	return strings.Join(lines, "\n")
}

func (a *application) gitHeaderStatus(c *ui.Context) {
	ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(6).Children(func() {
		if a.state.GitSyncing {
			components.ProgressRing(c, -1).Size(14, 14).Label("正在同步密码库")
		} else {
			icon, color := lucide.GitBranch(), c.Theme().TextMuted
			if a.state.GitError != "" || a.state.GitConflict {
				icon, color = lucide.CircleAlert(), c.Theme().Danger
			}
			components.Icon(c, icon).Size(14, 14).TextColor(color)
		}
		label := components.Text(c, a.gitStatusLabel()).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
		components.Tooltip(c, label, a.gitStatusDescription())
	})
}
