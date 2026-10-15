package desktop

import (
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
	if a.state.Locked {
		a.unlockView(c)
	} else {
		a.detailView(c)
	}
}
func (a *application) listView(c *ui.Context) {
	a.filtered = a.entries()
	if a.folderSelectionFilter != a.filter {
		a.folderSelection = ""
		if strings.HasPrefix(a.filter, "folder:") {
			a.folderSelection = strings.TrimPrefix(a.filter, "folder:")
		}
		a.folderSelectionFilter = a.filter
	}
	ui.Column(c).Fill().Padding(16, 12, 12, 12).Gap(14).Children(func() {
		input := components.TextBox(c, &a.query, components.TextBoxOptions{LeadingIcon: lucide.Search()}).Ref(&a.searchRef).FillWidth().Label("搜索条目").Placeholder("搜索名称或文件夹…")
		if input.Changed() {
			a.lastInput = time.Now()
			a.selectedIndex = -1
			a.filtered = a.entries()
		}
		if input.Submitted() && len(a.filtered) > 0 {
			a.selectedIndex = 0
			a.selectEntry(a.filtered[0].Path)
		}
		ui.Row(c).FillWidth().Padding(0, 4).Gap(8).Children(func() {
			folder := a.folderSelect(c, &a.folderSelection, &a.folderQuery, "筛选文件夹", "所有文件夹", false).Grow(1).MinWidth(0)
			if folder.Changed() {
				a.filter = "all"
				if a.folderSelection != "" {
					a.filter = "folder:" + a.folderSelection
				}
				a.folderSelectionFilter = a.filter
				a.selectedIndex = -1
				a.filtered = a.entries()
			}

			components.Text(c, countLabel(len(a.filtered))).FontSize(11).TextColor(c.Theme().TextMuted)
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
				row := ui.Row(c).FillWidth().MinHeight(56).Padding(7, 7).AlignItems(ui.Center).Gap(3).Radius(c.Theme().Radius).Label("账号 " + e.Path)
				showCopy := row.Hovered() || row.FocusWithin() || i == a.selectedIndex && a.list.Focused(c)
				if row.Hovered() && i != a.selectedIndex {
					row.Background(c.Theme().Text.Alpha(.04))
				}
				row.Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						folder := e.Folder
						if folder == "" {
							folder = "密码库"
						}
						components.Text(c, folder).SingleLine().FontSize(10).TextColor(c.Theme().TextMuted)
						components.Text(c, e.Name).SingleLine().FontSize(13)
					})
					copy := iconButton(c, lucide.Copy(), "复制密码 "+e.Name, components.ButtonGhost).Size(30, 30).Disabled(a.busy)
					if !showCopy {
						copy.Opacity(0)
					}
					if copy.Clicked() {
						a.copyEntry(e.Path)
					}
				})
			}).Grow(1).FillWidth()
		}
	})
}

func (a *application) detailView(c *ui.Context) {
	if a.detail.Entry == "" {
		if a.selected != "" && a.busy {
			ui.Column(c).Fill().Padding(30).Gap(12).Children(func() {
				components.Text(c, entryName(a.selected)).FontSize(26).Bold().Wrap()
				components.Text(c, a.selected).FontSize(11).TextColor(c.Theme().TextMuted).Wrap()
				remaining := 180*time.Millisecond - c.Now().Sub(a.detailLoadingStarted)
				if remaining > 0 {
					// A scheduled frame has no state callback and cannot revive a
					// completed or invalidated request. Fast reads finish silently.
					c.After(remaining)
				} else {
					muted(c, "正在读取账号…")
				}
			})
			return
		}
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
					a.secretField(c, "一次性密码", d.OTP, true)
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

func (a *application) copyEntry(path string) {
	if a.busy {
		return
	}
	if a.state.Locked {
		a.selectEntry(path)
		a.pendingCopyEntry, a.pendingCopyVaultID = path, a.state.VaultID
		a.invalidate()
		return
	}
	a.call(app.Request{Action: "copy", Entry: path, Field: "password"}, func(app.Data) { a.notice = "已复制密码" })
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
