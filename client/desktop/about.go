package desktop

import (
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/about"
	"github.com/knothhe/passpine/client/buildinfo"
	"github.com/knothhe/passpine/client/i18n"
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
			version := a.t("开发版本")
			if a.buildVersion != "" && a.buildVersion != "development" {
				version = a.t("版本 %s", a.buildVersion)
			}
			muted(c, version)
			muted(c, a.t("兼容 pass 的本地密码管理器。"))
			ui.Link(c, a.t("项目主页"), about.ProjectURL)
		})
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			components.Text(c, a.t("主要依赖")).Bold().FontSize(scaledFontSize(c, 15))
			muted(c, a.t("%d 项主要直接依赖，点击组件查看用途、许可证与上游链接。", len(deps)))
		})
		for _, group := range []struct{ id, title string }{{"desktop", a.t("桌面客户端")}} {
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
								components.Text(c, a.t("包名")).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
								components.Text(c, dep.Module).Wrap().FontSize(scaledFontSize(c, 12))
								ui.Row(c).FillWidth().Gap(12).Wrap().Children(func() {
									muted(c, a.t("许可证"))
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
