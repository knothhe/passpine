package desktop

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/about"
	"github.com/knothhe/passpine/client/buildinfo"
	"github.com/knothhe/passpine/client/i18n"
	"github.com/knothhe/passpine/client/internal/secureclipboard"
	"github.com/oneo-me/onevra/source/components"
)

func (a *application) aboutView(c *ui.Context) {
	deps := about.Dependencies()
	if a.aboutExpanded == nil {
		a.aboutExpanded = make(map[string]bool)
	}
	ui.Column(c).FillWidth().MinWidth(0).Gap(20).Children(func() {
		ui.Column(c).FillWidth().MinWidth(0).Padding(20).Gap(8).Background(c.Theme().Surface).Radius(10).Border(1, c.Theme().Border).Children(func() {
			components.Text(c, a.t(buildinfo.Name())).FontSize(scaledFontSize(c, 22)).Bold()
			muted(c, a.t("兼容 pass 的本地密码管理器。"))
			version := a.t("开发版本")
			if a.buildVersion != "" && a.buildVersion != "development" {
				version = a.t("版本 %s", a.buildVersion)
			}
			muted(c, version)
			info := buildinfo.Current(a.buildVersion)
			channel := a.t("开发通道")
			if info.Channel == "release" {
				channel = a.t("正式通道")
			}
			muted(c, a.t("构建通道：%s", channel))
			if info.Commit != "" && info.Commit != "unknown" {
				components.Text(c, a.t("源码提交：%s", info.Commit)).Wrap().FontSize(scaledFontSize(c, 11))
			}
			modified := a.t("无法确认")
			if info.Modified == "true" {
				modified = a.t("包含未提交修改")
			} else if info.Modified == "false" {
				modified = a.t("无未提交修改")
			}
			muted(c, a.t("源码状态：%s", modified))
			executor := a.t("本机构建")
			if info.Executor == "github-actions" {
				executor = a.t("GitHub Actions")
			}
			muted(c, a.t("构建环境：%s · %s", executor, info.OS+"/"+info.Arch))
			ui.Row(c).FillWidth().Margin(8, 0, 0, 0).Gap(16).AlignItems(ui.Center).Children(func() {
				if components.Button(c, a.t("复制构建信息"), components.ButtonOutline).Clicked() {
					data, _ := json.MarshalIndent(info, "", "  ")
					// Diagnostic metadata contains no vault paths or credentials.
					write := a.clipboardWriter
					if write == nil {
						write = secureclipboard.WriteText
					}
					if err := write(string(data), false); err != nil {
						a.errorText = a.t(i18n.ClipboardWriteFailed)
					} else {
						a.notice = a.t("构建信息已复制")
					}
				}
				ui.Link(c, a.t("项目主页"), about.ProjectURL)
			})
		})
		for _, group := range []struct{ id, title string }{{"desktop", a.t("主要依赖")}} {
			ui.Column(c).FillWidth().MinWidth(0).Gap(8).Children(func() {
				components.Text(c, group.title).Bold()
				ui.Column(c).FillWidth().MinWidth(0).Padding(12).Gap(4).Background(c.Theme().Surface).Radius(8).Border(1, c.Theme().Border).Children(func() {
					for _, dep := range deps {
						if dep.Group != group.id {
							continue
						}
						ui.Column(c).Key(dep.ID).FillWidth().MinWidth(0).Children(func() {
							open := a.aboutExpanded[dep.ID]
							parts := components.Collapsible(c, dep.Name, &open, components.CollapsibleOptions{Trailing: func() { muted(c, dep.Version) }}, func() {
								muted(c, a.t(i18n.DependencyDescriptionKey(dep.ID)))
								// Label/value pairs share one label column so the values line up.
								ui.Row(c).FillWidth().MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
									components.Text(c, a.t("包名")).Width(56).Shrink(0).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
									components.Text(c, dep.Module).Grow(1).MinWidth(0).Wrap().FontSize(scaledFontSize(c, 12))
								})
								ui.Row(c).FillWidth().MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
									components.Text(c, a.t("许可证")).Width(56).Shrink(0).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
									ui.Link(c, dep.License, "https://spdx.org/licenses/"+dep.License+".html")
								})
								ui.Row(c).FillWidth().Gap(16).Wrap().Children(func() {
									ui.Link(c, a.t("文档与主页"), dep.URL)
									ui.Link(c, a.t("源代码"), dep.Source)
								})
							})
							parts.Root.FillWidth().MinWidth(0)
							a.aboutExpanded[dep.ID] = open
						})
					}
				})
			})
		}
	})
}
