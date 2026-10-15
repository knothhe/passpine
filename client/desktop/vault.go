package desktop

import (
	"net/url"
	"strconv"
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
	ui.Column(c).Fill().Padding(16, 12, 12, 12).Gap(14).Children(func() {
		input := components.TextBox(c, &a.query, components.TextBoxOptions{LeadingIcon: lucide.Search()}).Ref(&a.searchRef).FillWidth().Label(a.t("搜索条目")).Placeholder(a.t("搜索名称或文件夹…"))
		if input.Changed() {
			a.lastInput = time.Now()
			a.selectedIndex = -1
			a.filtered = a.entries()
		}
		if input.Submitted() && len(a.filtered) > 0 {
			a.selectedIndex = 0
			a.selectEntry(a.filtered[0].Path)
		}
		if len(a.filtered) == 0 {
			ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(12).Children(func() {
				components.Icon(c, lucide.SearchX).Size(28, 28).TextColor(c.Theme().TextMuted)
				muted(c, a.t("没有找到条目"))
				if a.query == "" {
					if components.Button(c, a.t("新建条目"), components.ButtonOutline).Clicked() {
						a.newEntry()
					}
				}
			})
		} else {
			components.List(c, &a.list, len(a.filtered), func(i int) {
				e := a.filtered[i]
				row := ui.Row(c).FillWidth().MinHeight(56).Padding(7, 7).AlignItems(ui.Center).Gap(3).Radius(c.Theme().Radius).Label(a.t("账号 %s", e.Path))
				showCopy := row.Hovered() || row.FocusWithin() || i == a.selectedIndex && a.list.Focused(c)
				if row.Hovered() && i != a.selectedIndex {
					row.Background(c.Theme().Text.Alpha(.04))
				}
				row.Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						folder := e.Folder
						if folder == "" {
							folder = a.t("密码库")
						}
						components.Text(c, folder).SingleLine().FontSize(10).TextColor(c.Theme().TextMuted)
						components.Text(c, e.Name).SingleLine().FontSize(13)
					})
					copy := iconButton(c, lucide.Copy(), a.t("复制密码 %s", e.Name), components.ButtonGhost).Size(30, 30).Disabled(a.busy)
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
			ui.Column(c).Fill().Padding(24).Gap(6).Children(func() {
				components.Text(c, entryName(a.selected)).FontSize(22).Bold().Wrap()
				components.Text(c, a.selected).FontSize(11).TextColor(c.Theme().TextMuted).Wrap()
				remaining := 180*time.Millisecond - c.Now().Sub(a.detailLoadingStarted)
				if remaining > 0 {
					// A scheduled frame has no state callback and cannot revive a
					// completed or invalidated request. Fast reads finish silently.
					c.After(remaining)
				} else {
					muted(c, a.t("正在读取账号…"))
				}
			})
			return
		}
		ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Padding(24).Gap(12).Children(func() {
			components.Icon(c, lucide.ShieldCheck).Size(36, 36).TextColor(c.Theme().TextMuted)
			components.Text(c, a.t("选择一个条目")).FontSize(18).Bold()
			if components.Button(c, a.t("新建条目"), components.ButtonGhost).Clicked() {
				a.newEntry()
			}
		})
		return
	}
	d := a.detail
	components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
		ui.Column(c).FillWidth().MaxWidth(808).Padding(24).Gap(20).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(4).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
					components.Text(c, entryName(d.Entry)).FontSize(22).Bold().Wrap()
					components.Text(c, d.Entry).FontSize(11).TextColor(c.Theme().TextMuted).Wrap()
				})
				allRevealed := a.allDetailFieldsRevealed()
				revealIcon, revealLabel := lucide.Eye(), a.t("显示全部字段")
				if allRevealed {
					revealIcon, revealLabel = lucide.EyeOff(), a.t("隐藏全部字段")
				}
				if iconButton(c, revealIcon, revealLabel, components.ButtonGhost).Clicked() {
					a.reveal = !allRevealed
					a.revealedFields = nil
				}
				if iconButton(c, lucide.Pencil(), a.t("编辑条目"), components.ButtonGhost).Clicked() {
					a.editEntry()
				}
				if iconButton(c, lucide.Trash(), a.t("移到回收站"), components.ButtonGhost).Clicked() {
					a.deleteOpen = true
				}
			})
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				a.secretField(c, "password", a.t("密码"), d.Password, true)
				if d.Username != "" {
					a.secretField(c, "username", a.t("用户名"), d.Username, false)
				}
				if d.URL != "" {
					a.secretField(c, "url", a.t("网址"), d.URL, false)
				}
				if d.OTP != "" {
					a.secretField(c, "otp", a.t("一次性密码"), d.OTP, true)
				}
				for i, f := range extraFields(d.Contents) {
					a.secretField(c, "custom:"+strconv.Itoa(i), f[0], f[1], true)
				}
			})
			if d.Notes != "" {
				ui.Column(c).FillWidth().Gap(7).Children(func() {
					components.Text(c, a.t("备注")).FontSize(11).TextColor(c.Theme().TextMuted)
					components.Text(c, d.Notes).Wrap().FontSize(13)
				})
			}
			if d.URL != "" {
				ui.Row(c).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
					if components.Button(c, a.t("打开网站"), components.ButtonGhost).FontSize(12).Clicked() {
						if safeWebsite(d.URL) {
							if err := mygo.Shell.OpenExternal(d.URL); err != nil {
								a.errorText = err.Error()
							}
						} else {
							a.errorText = a.t("只允许打开 http 或 https 网址。")
						}
					}
				})
			}
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
	a.call(app.Request{Action: "copy", Entry: path, Field: "password"}, func(app.Data) { a.notice = a.t("已复制密码") })
}

func (a *application) secretField(c *ui.Context, key, label, value string, secret bool) {
	revealed := a.detailFieldRevealed(key)
	ui.Row(c).FillWidth().Padding(10, 0).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
			components.Text(c, label).FontSize(11).TextColor(c.Theme().TextMuted)
			shown := value
			if secret && !revealed {
				shown = "••••••••••••"
			}
			components.Text(c, shown).Wrap().FontSize(13)
		})
		if secret {
			icon := lucide.Eye()
			if revealed {
				icon = lucide.EyeOff()
			}
			if iconButton(c, icon, a.t("显示或隐藏%s", label), components.ButtonGhost).Clicked() {
				if a.revealedFields == nil {
					a.revealedFields = make(map[string]bool)
				}
				a.revealedFields[key] = !revealed
			}
		}
		if iconButton(c, lucide.Copy(), a.t("复制%s", label), components.ButtonGhost).Clicked() {
			copyLabel := label
			switch key {
			case "password":
				copyLabel = "密码"
			case "username":
				copyLabel = "用户名"
			case "otp":
				copyLabel = "一次性密码"
			}
			a.copy(value, copyLabel)
		}
	})
}

func (a *application) detailFieldRevealed(key string) bool {
	if revealed, ok := a.revealedFields[key]; ok {
		return revealed
	}
	return a.reveal
}

func (a *application) allDetailFieldsRevealed() bool {
	if !a.detailFieldRevealed("password") || a.detail.OTP != "" && !a.detailFieldRevealed("otp") {
		return false
	}
	for i := range extraFields(a.detail.Contents) {
		if !a.detailFieldRevealed("custom:" + strconv.Itoa(i)) {
			return false
		}
	}
	return true
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
