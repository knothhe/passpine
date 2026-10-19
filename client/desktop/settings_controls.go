package desktop

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// Settings uses the application's existing sidebar. The shared Onevra catalog
// still owns the searchable metadata, without creating a second navigation pane.
func (a *application) settingsSidebar(c *ui.Context) {
	categories := a.settingsCategories()
	a.selectedSettingsCategory(categories)
	ui.Column(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(14, 16, 18, 16).Gap(12).Children(func() {
			components.Text(c, a.t("设置")).Bold().FontSize(scaledFontSize(c, 18))
			search := textBox(c, &a.settingsState.Query, components.TextBoxOptions{LeadingIcon: lucide.Search()}).Label(a.t("搜索设置")).Placeholder(a.t("搜索设置…")).FillWidth()
			if search.Changed() && a.preferencesPage() != "settings" {
				a.navigate("settings", "")
			}
		})
		components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
			components.Sidebar(c).Children(func() {
				for _, category := range categories {
					selected := category.ID == a.settingsState.Category
					if a.preferencesPage() == "recovery" {
						selected = category.ID == "vault"
					} else if a.preferencesPage() == "activity" {
						selected = category.ID == "general"
					}
					if components.SidebarItem(c, category.Title, category.Icon, selected).Key(category.ID).Clicked() {
						a.navigateSettingsCategory(category.ID)
					}
				}
			})
		})
	})
}

func (a *application) selectedSettingsCategory(categories []components.SettingsCategory) int {
	for i, category := range categories {
		if category.ID == a.settingsState.Category {
			return i
		}
	}
	a.settingsState.Category = categories[0].ID
	return 0
}

func (a *application) settingsContent(c *ui.Context, categories []components.SettingsCategory) {
	selected := a.selectedSettingsCategory(categories)
	searching := strings.TrimSpace(a.settingsState.Query) != ""
	width, _ := c.Size()
	if a.shell == nil {
		width -= 310
	} else if a.shell.LeftExpanded() {
		width -= a.shell.LeftWidth()
	}
	narrow := width < 660
	title, description := categories[selected].Title, categories[selected].Description
	if searching {
		title, description = a.t("搜索结果"), ""
	}
	ui.Column(c).Fill().MinWidth(0).Children(func() {
		ui.Row(c).FillWidth().Shrink(0).Padding(24, 28, 20, 28).AlignItems(ui.Center).Gap(16).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
				components.Text(c, title).FontSize(scaledFontSize(c, 21)).Bold()
				if description != "" {
					muted(c, description)
				}
			})
			if a.settingsSaving || a.gitRemoteSaving {
				muted(c, a.t("正在保存…"))
			}
		})
		components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
			ui.Column(c).Key(a.settingsState.Category).FillWidth().MinWidth(0).Padding(0, 28, 28, 28).Gap(24).Children(func() {
				count, lastCategory, lastGroup := 0, -1, -1
				for _, result := range components.SearchSettings(categories, a.settingsState.Query) {
					if !searching && result.Category != selected {
						continue
					}
					category := categories[result.Category]
					group := category.Groups[result.Group]
					item := group.Items[result.Item]
					if lastCategory != result.Category || lastGroup != result.Group {
						heading := group.Title
						if searching {
							heading = category.Title
							if group.Title != "" {
								heading += " · " + group.Title
							}
						}
						if heading != "" {
							ui.Column(c).FillWidth().Gap(12).Children(func() {
								components.Text(c, heading).Bold().TextColor(c.Theme().TextMuted)
								ui.Box(c).FillWidth().Height(1).Background(c.Theme().Border)
							})
						}
					}
					lastCategory, lastGroup = result.Category, result.Group
					count++
					if item.ID == "git" || item.ID == "browser-status" || item.ID == "about-content" {
						if item.ID == "browser-status" {
							components.Text(c, item.Title).FontSize(scaledFontSize(c, 14)).Bold()
							muted(c, item.Description)
						}
						item.Build(c)
						continue
					}
					var row ui.Element
					if narrow {
						row = ui.Column(c)
					} else {
						row = ui.Row(c)
					}
					row.Key(struct{ Category, Group, Item string }{category.ID, group.ID, item.ID}).FillWidth().MinWidth(0).Gap(16).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
							components.Text(c, item.Title).Wrap()
							if item.Description != "" {
								components.Text(c, item.Description).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted).Wrap()
							}
						})
						if item.Build != nil {
							if narrow {
								ui.Row(c).FillWidth().MinWidth(0).Children(func() { item.Build(c) })
							} else {
								item.Build(c)
							}
						}
					})
				}
				if count == 0 {
					muted(c, a.t("没有找到相关设置"))
				}
			})
		})
	})
}

// Keep all idle presets selectable while preserving a saved custom duration.
// The extra option disappears after the user chooses one of the presets.
func idleDurationPresets(value int) []int {
	values := []int{1, 10, 30}
	for _, preset := range values {
		if preset == value {
			return values
		}
	}
	values = append(values, value)
	sort.Ints(values)
	return values
}

// Keep a saved custom duration visible without adding a fourth segment. The
// nearest nonzero preset yields its place; a disabled timer remains available.
// Rendering never changes a setting.
func durationOptions(value int, presets []int) []int {
	values := append([]int(nil), presets...)
	for _, preset := range values {
		if preset == value {
			return values
		}
	}
	nearest, distance := -1, int(^uint(0)>>1)
	for i, preset := range values {
		if preset == 0 {
			continue
		}
		delta := preset - value
		if delta < 0 {
			delta = -delta
		}
		if delta < distance {
			nearest, distance = i, delta
		}
	}
	if nearest < 0 {
		return append(values, value)
	}
	values[nearest] = value
	sort.Ints(values)
	return values
}

func (a *application) durationSegments(c *ui.Context, value *int, label string, presets []int, unit string) {
	values := durationOptions(*value, presets)
	labels := make([]string, len(values))
	selected := 0
	for i, duration := range values {
		if duration == *value {
			selected = i
		}
		labels[i] = fmt.Sprintf("%d %s", duration, unit)
		if duration == 0 {
			labels[i] = a.t("关闭")
		} else if unit == a.t("分钟") && duration >= 60 && duration%60 == 0 {
			labels[i] = a.t("%d 小时", duration/60)
			if duration == 60 {
				labels[i] = a.t("1 小时")
			}
		} else if duration == 1 {
			switch unit {
			case a.t("天"):
				labels[i] = a.t("1 天")
			case a.t("分钟"):
				labels[i] = a.t("1 分钟")
			case a.t("秒"):
				labels[i] = a.t("1 秒")
			case a.t("条"):
				labels[i] = a.t("1 条")
			}
		}
	}
	if ui.Segmented(c, &selected, labels...).Label(label).AlignSelf(ui.Center).Changed() {
		*value = values[selected]
	}
}

func (a *application) booleanSettingSegments(c *ui.Context, value *bool, label string) ui.Element {
	selected := 1
	if *value {
		selected = 0
	}
	control := ui.Segmented(c, &selected, a.t("开启"), a.t("关闭")).Label(label).AlignSelf(ui.Center).Shrink(0)
	if control.Changed() {
		*value = selected == 0
	}
	return control
}

func choiceSettingSegments(c *ui.Context, value *string, label string, values, labels []string) ui.Element {
	selected := 0
	for i, option := range values {
		if option == *value {
			selected = i
			break
		}
	}
	control := ui.Segmented(c, &selected, labels...).Label(label).AlignSelf(ui.Center).Shrink(0)
	if control.Changed() {
		*value = values[selected]
	}
	return control
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
		return a.t("此设备的系统解锁不可用")
	}
	if a.settings.DeviceUnlock && !a.state.DeviceUnlockEnrolled {
		return a.t("使用私钥口令解锁后完成启用")
	}
	return ""
}

func (a *application) gitStatusLabel() string {
	switch {
	case a.state.GitSyncing:
		return a.t("正在同步")
	case a.state.GitRefreshing && a.state.GitStatusAt == "":
		return a.t("正在检查")
	case a.state.GitConflict:
		return a.t("存在冲突")
	case !a.state.GitAvailable && a.state.GitStatusAt != "":
		return a.t("Git 不可用")
	case a.state.GitError != "":
		return a.t("同步失败")
	case !a.state.GitInitialized:
		return a.t("未初始化 Git")
	case a.state.GitRemote == "":
		return a.t("未配置远端")
	case a.state.GitUpstream == "":
		return a.t("尚未建立远端跟踪")
	case a.state.GitChanges > 0:
		return fmt.Sprintf(a.t("%d 项变更"), a.state.GitChanges)
	case a.state.GitAhead > 0 || a.state.GitBehind > 0:
		return a.t("有待同步的提交")
	default:
		return a.t("已同步")
	}
}

func (a *application) gitStatusDescription() string {
	lines := []string{a.gitStatusLabel(), a.gitStatusHint()}
	if a.state.GitBranch != "" {
		lines = append(lines, a.t("分支：")+a.state.GitBranch)
	}
	if a.state.GitUpstream != "" {
		lines = append(lines, a.t("跟踪分支：")+a.state.GitUpstream)
	}
	if summary := a.gitPendingSummary(); summary != "" {
		lines = append(lines, summary)
	}
	if a.state.GitError != "" {
		lines = append(lines, a.state.GitError)
	}
	if stamp := gitTime(a.state.GitLastSync); stamp != "" {
		lines = append(lines, a.t("上次同步：")+stamp)
	}
	return strings.Join(lines, "\n")
}

func (a *application) gitStatusHint() string {
	switch {
	case a.state.GitSyncing:
		return a.t("正在同步加密条目，可以继续使用密码库。")
	case a.state.GitRefreshing && a.state.GitStatusAt == "":
		return a.t("正在读取本机仓库状态。")
	case a.state.GitConflict:
		return a.t("请先解决仓库中的冲突，再重新同步。")
	case !a.state.GitAvailable && a.state.GitStatusAt != "":
		return a.t("安装 Git 后可启用密码库同步。")
	case a.state.GitError != "":
		return a.t("检查网络连接与远端配置后重试。")
	case !a.state.GitInitialized:
		return a.t("初始化后可通过 Git 备份与同步加密条目。")
	case a.state.GitRemote == "":
		return a.t("当前仅保存在本机。添加远端地址后可跨设备同步。")
	case a.state.GitUpstream == "":
		return a.t("首次同步将建立远端跟踪关系。")
	case a.state.GitChanges > 0 || a.state.GitAhead > 0 || a.state.GitBehind > 0:
		return a.t("本机与远端存在更新，可立即同步。")
	default:
		return a.t("本机与远端内容一致。")
	}
}

func (a *application) gitPendingSummary() string {
	parts := []string{}
	for _, metric := range []struct {
		count int
		label string
	}{{a.state.GitChanges, a.t("项变更")}, {a.state.GitAhead, a.t("个提交待推送")}, {a.state.GitBehind, a.t("个提交待拉取")}} {
		if metric.count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", metric.count, metric.label))
		}
	}
	return strings.Join(parts, " · ")
}

func (a *application) gitSettingsStatus(c *ui.Context) {
	t := c.Theme()
	color, icon := t.TextMuted, lucide.GitBranch()
	if a.state.GitConflict || a.state.GitError != "" {
		color, icon = t.Danger, lucide.CircleAlert()
	} else if a.gitStatusLabel() == a.t("已同步") {
		color, icon = t.Success, lucide.CircleCheck()
	}
	ui.Column(c).FillWidth().MinWidth(0).Gap(12).Padding(16).Background(t.Surface).Radius(8).Label(a.t("Git 同步状态")).Children(func() {
		ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(12).Children(func() {
			if a.state.GitSyncing || a.state.GitRefreshing && a.state.GitStatusAt == "" {
				components.ProgressRing(c, -1).Size(18, 18).Label(a.gitStatusLabel())
			} else {
				components.Icon(c, icon).Size(18, 18).TextColor(color)
			}
			components.Text(c, a.gitStatusLabel()).Bold().TextColor(color).Grow(1).MinWidth(0).Wrap()
			label, action := a.t("立即同步"), "git-sync"
			if !a.state.GitInitialized {
				label, action = a.t("初始化 Git"), "git-init"
			} else if a.state.GitSyncing {
				label = a.t("同步中…")
			}
			disabled := a.gitActionPending || a.state.GitSyncing || !a.state.GitAvailable && a.state.GitStatusAt != ""
			if components.Button(c, label, components.ButtonOutline).Disabled(disabled).Clicked() {
				a.call(app.Request{Action: action}, func(app.Data) {
					a.notice = a.t("同步完成")
					if action == "git-init" {
						a.notice = a.t("Git 已初始化")
					}
				})
			}
		})
		components.Text(c, a.gitStatusHint()).FontSize(scaledFontSize(c, 12)).TextColor(t.TextMuted).Wrap()
		if summary := a.gitPendingSummary(); summary != "" {
			components.Text(c, summary).FontSize(scaledFontSize(c, 12)).Wrap()
		}
		if a.state.GitError != "" {
			ui.Column(c).FillWidth().MinWidth(0).Padding(10, 12).Background(t.Danger.Alpha(.06)).Radius(6).Children(func() {
				components.Text(c, a.state.GitError).FontSize(scaledFontSize(c, 12)).TextColor(t.Danger).Wrap()
			})
		}
		metadata := []string{}
		if a.state.GitBranch != "" {
			branch := a.state.GitBranch
			if a.state.GitUpstream != "" {
				branch += " → " + a.state.GitUpstream
			}
			metadata = append(metadata, branch)
		}
		if stamp := gitTime(a.state.GitLastSync); stamp != "" {
			metadata = append(metadata, a.t("上次成功同步 ")+stamp)
		}
		if len(metadata) > 0 {
			ui.Box(c).FillWidth().Height(1).Background(t.Border)
			components.Text(c, strings.Join(metadata, " · ")).FontSize(scaledFontSize(c, 11)).TextColor(t.TextMuted).Wrap()
		}
	})
}

func (a *application) gitHeaderStatus(c *ui.Context) {
	status := ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(6).Tooltip(a.gitStatusDescription())
	status.Children(func() {
		if a.state.GitSyncing {
			components.ProgressRing(c, -1).Size(14, 14).Label(a.t("正在同步密码库"))
		} else {
			icon, color := lucide.GitBranch(), c.Theme().TextMuted
			if a.state.GitError != "" || a.state.GitConflict {
				icon, color = lucide.CircleAlert(), c.Theme().Danger
			}
			components.Icon(c, icon).Size(14, 14).TextColor(color)
		}
		if a.mainContentWidth(c) >= 540 {
			components.Text(c, a.gitStatusLabel()).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted).SingleLine()
		}
	})
}
