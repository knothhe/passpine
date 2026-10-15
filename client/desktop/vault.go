package desktop

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine-neo/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func (a *application) vaultView(c *ui.Context) {
	a.filtered = a.entries()
	components.Split(c, &a.listWidth, func() { a.listView(c) }, func() { a.detailView(c) }, components.SplitOptions{FirstMin: 250, SecondMin: 320}).Label("调整条目列表宽度").Fill()
}
func (a *application) listView(c *ui.Context) {
	ui.Column(c).Fill().Padding(16, 12, 12, 12).Gap(14).Children(func() {
		input := components.TextBox(c, &a.query, components.TextBoxOptions{LeadingIcon: lucide.Search()}).Ref(&a.searchRef).FillWidth().Label("搜索条目").Placeholder("搜索名称或文件夹…")
		if input.Changed() {
			a.lastInput = time.Now()
			a.selectedIndex = -1
		}
		if input.Submitted() && len(a.filtered) > 0 {
			a.selectedIndex = 0
			a.selectEntry(a.filtered[0].Path)
		}
		ui.Row(c).FillWidth().Padding(0, 4).Children(func() {
			components.Text(c, countLabel(len(a.filtered))).FontSize(11).TextColor(c.Theme().TextMuted)
			ui.Spacer(c)
			components.Text(c, "⌘ F").FontSize(11).TextColor(c.Theme().TextMuted)
		})
		if len(a.filtered) == 0 {
			ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(12).Children(func() {
				components.Icon(c, lucide.SearchX).Size(28, 28).TextColor(c.Theme().TextMuted)
				muted(c, "没有找到条目")
				if a.query == "" {
					if components.Button(c, "新建条目", components.ButtonOutline).Clicked() {
						a.newEntry()
					}
				}
			})
		} else {
			components.List(c, &a.list, len(a.filtered), func(i int) {
				e := a.filtered[i]
				ui.Row(c).FillWidth().Padding(12, 10).AlignItems(ui.Center).Gap(10).Children(func() {
					components.Icon(c, lucide.KeyRound).Size(19, 19).TextColor(c.Theme().Accent)
					ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
						components.Text(c, e.Name).SingleLine().Bold().FontSize(13)
						subtitle := e.Folder
						if subtitle == "" {
							subtitle = "密码库"
						}
						components.Text(c, subtitle).SingleLine().FontSize(11).TextColor(c.Theme().TextMuted)
					})
					if e.Favorite {
						components.Icon(c, lucide.Star).Size(13, 13).TextColor(c.Theme().Accent)
					}
				})
			}).Grow(1).FillWidth()
		}
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			components.Icon(c, lucide.GitBranch).Size(12, 12).TextColor(c.Theme().TextMuted)
			label := "本地密码库"
			if a.state.GitInitialized {
				label = a.state.GitBranch
				if a.state.GitChanges > 0 {
					label += fmt.Sprintf(" · %d 项变更", a.state.GitChanges)
				}
			}
			components.Text(c, label).FontSize(10).TextColor(c.Theme().TextMuted).SingleLine()
		})
	})
}

func (a *application) detailView(c *ui.Context) {
	if a.detail.Entry == "" {
		ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Padding(30).Gap(16).Children(func() {
			components.Icon(c, lucide.ShieldCheck).Size(48, 48).TextColor(c.Theme().Accent)
			components.Text(c, "选择一个条目").FontSize(23).Bold()
			if components.Button(c, "新建条目", components.ButtonPrimary).Clicked() {
				a.newEntry()
			}
		})
		return
	}
	d := a.detail
	components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(30).Gap(24).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(10).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
					components.Text(c, entryName(d.Entry)).FontSize(26).Bold().Wrap()
					components.Text(c, d.Entry).FontSize(11).TextColor(c.Theme().TextMuted).Wrap()
				})
				favorite := false
				for _, e := range a.state.Entries {
					if e.Path == d.Entry {
						favorite = e.Favorite
					}
				}
				star := iconButton(c, lucide.Star(), "切换收藏", components.ButtonOutline)
				if favorite {
					star.TextColor(c.Theme().Accent)
				}
				if star.Clicked() {
					a.call(app.Request{Action: "favorite", Entry: d.Entry}, nil)
				}
				if iconButton(c, lucide.Pencil(), "编辑条目", components.ButtonOutline).Clicked() {
					a.editEntry()
				}
			})
			ui.Column(c).FillWidth().Gap(16).Children(func() {
				a.secretField(c, "密码", d.Password, true)
				if d.Username != "" {
					a.secretField(c, "用户名", d.Username, false)
				}
				if d.URL != "" {
					a.secretField(c, "网址", d.URL, false)
				}
				if d.OTP != "" {
					a.secretField(c, fmt.Sprintf("一次性密码 · %d 秒", d.OTPSeconds), d.OTP, true)
				}
				if d.Notes != "" {
					ui.Column(c).FillWidth().Gap(7).Children(func() {
						components.Text(c, "备注").Bold().FontSize(12)
						components.Text(c, d.Notes).Wrap().FontSize(13)
					})
				}
				custom := extraFields(d.Contents)
				for _, f := range custom {
					a.secretField(c, f[0], f[1], true)
				}
			})
			ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
				if components.Button(c, "复制密码", components.ButtonPrimary).Clicked() {
					a.copy(d.Password, "密码")
				}
				if d.URL != "" {
					if components.Button(c, "打开网站", components.ButtonOutline).Clicked() {
						if safeWebsite(d.URL) {
							if err := mygo.Shell.OpenExternal(d.URL); err != nil {
								a.errorText = err.Error()
							}
						} else {
							a.errorText = "只允许打开 http 或 https 网址。"
						}
					}
				}
				ui.Spacer(c)
				if iconButton(c, lucide.Trash(), "移到回收站", components.ButtonGhost).Clicked() {
					a.deleteOpen = true
				}
			})
		})
	})
}

func (a *application) secretField(c *ui.Context, label, value string, secret bool) {
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		components.Text(c, label).FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Row(c).FillWidth().Padding(12).Gap(8).Background(c.Theme().Surface).Radius(c.Theme().Radius).Border(1, c.Theme().Border).AlignItems(ui.Center).Children(func() {
			shown := value
			if secret && !a.reveal {
				shown = "••••••••••••••••"
			}
			components.Text(c, shown).Grow(1).MinWidth(0).Wrap().FontSize(14)
			if secret {
				icon := lucide.Eye()
				if a.reveal {
					icon = lucide.EyeOff()
				}
				if iconButton(c, icon, "显示或隐藏"+label, components.ButtonGhost).Clicked() {
					a.reveal = !a.reveal
				}
			}
			if iconButton(c, lucide.Copy(), "复制"+label, components.ButtonGhost).Clicked() {
				a.copy(value, label)
			}
		})
	})
}

func extraFields(contents string) [][2]string {
	var fields [][2]string
	lines := strings.Split(strings.ReplaceAll(contents, "\r\n", "\n"), "\n")
	if len(lines) < 2 {
		return fields
	}
	for _, line := range lines[1:] {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		switch strings.ToLower(key) {
		case "username", "user", "login", "email", "url", "website", "site", "notes", "note", "otpauth", "otp":
			continue
		}
		fields = append(fields, [2]string{key, strings.TrimSpace(value)})
	}
	return fields
}
func safeWebsite(value string) bool {
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
}
