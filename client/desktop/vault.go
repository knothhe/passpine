package desktop

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
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
	ui.Column(c).Fill().Padding(18, 16, 12, 16).Gap(14).Children(func() {
		ui.Row(c).FillWidth().MinHeight(32).AlignItems(ui.Center).Gap(8).Children(func() {
			components.Text(c, a.t("全部条目")).FontSize(scaledFontSize(c, 18)).Bold()
			ui.Spacer(c)
			if iconButton(c, lucide.Plus(), a.t("新建条目"), components.ButtonGhost).Tooltip(a.t("新建条目")).Clicked() {
				a.newEntry()
			}
		})
		input := textBox(c, &a.query, components.TextBoxOptions{LeadingIcon: lucide.Search()}).Ref(&a.searchRef).FillWidth().Label(a.t("搜索条目")).Placeholder(a.t("搜索名称或文件夹…"))
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
				row := ui.Row(c).FillWidth().MinHeight(60).Padding(10, 9).AlignItems(ui.Center).Gap(10).Radius(c.Theme().Radius).Label(a.t("账号 %s", e.Path))
				selected := e.Path == a.selected
				showCopy := row.Hovered() || row.FocusWithin() || i == a.selectedIndex && a.list.Focused(c)
				if selected {
					if c.Theme().Accent == c.Theme().Text {
						row.Background(c.Theme().Selection)
					} else {
						row.Background(c.Theme().Accent.Alpha(.10))
					}
				} else if row.Hovered() {
					row.Background(c.Theme().Text.Alpha(.04))
				}
				row.Children(func() {
					entryAvatar(c, e.Path, 34, selected)
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						components.Text(c, e.Name).SingleLine().FontSize(scaledFontSize(c, 13)).Bold()
						if e.Folder != "" {
							components.Text(c, e.Path).SingleLine().FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
						}
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

func entryAvatar(c *ui.Context, path string, size float32, selected bool) {
	color, background := c.Theme().TextMuted, c.Theme().Text.Alpha(.06)
	if selected {
		color, background = c.Theme().Accent, c.Theme().Accent.Alpha(.12)
	}
	initials := []rune(strings.ToUpper(entryName(path)))
	if len(initials) > 2 {
		initials = initials[:2]
	}
	ui.Box(c).Size(size, size).Shrink(0).Radius(size / 4).Background(background).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		components.Text(c, string(initials)).FontSize(scaledFontSize(c, size/3)).Bold().TextColor(color).SingleLine()
	})
}

func (a *application) detailView(c *ui.Context) {
	pending := a.detailTransitionPresentation(c.Now())
	if a.detail.Entry == "" && !pending.Pending {
		ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Padding(24).Gap(12).Children(func() {
			components.Icon(c, lucide.ShieldCheck).Size(36, 36).TextColor(c.Theme().TextMuted)
			components.Text(c, a.t("选择一个条目")).FontSize(scaledFontSize(c, 18)).Bold()
			if components.Button(c, a.t("新建条目"), components.ButtonGhost).Clicked() {
				a.newEntry()
			}
		})
		return
	}
	d := a.detail
	path := d.Entry
	if pending.Pending {
		path = pending.Entry
		if pending.Remaining > 0 {
			c.After(pending.Remaining)
		}
	}
	inset := float32(32)
	width, _ := c.Size()
	if width <= 900 {
		inset = 24
	}
	components.ScrollView(c, components.ScrollViewOptions{}).Key("detail-scroll").Label(a.t("条目详情")).TrackScroll(&a.detailScroll).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(24, inset).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Key("entry-detail").FillWidth().MaxWidth(560).Children(func() {
				ui.Row(c).Key("account-heading").FillWidth().Margin(0, 0, 26, 0).AlignItems(ui.Start).Gap(12).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
						components.Text(c, entryName(path)).FontSize(scaledFontSize(c, 22)).FontWeight(500).Wrap()
						ui.Row(c).Key("detail-path").FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
							components.Text(c, path).Grow(1).MinWidth(0).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted).Wrap()
						})
					})
					ui.Row(c).Key("detail-actions").Margin(-4, 0, 0, 0).Shrink(0).AlignItems(ui.Center).Children(func() {
						allRevealed := a.allDetailFieldsRevealed()
						revealIcon, revealLabel := lucide.Eye(), a.t("显示全部字段")
						if allRevealed {
							revealIcon, revealLabel = lucide.EyeOff(), a.t("隐藏全部字段")
						}
						if iconButton(c, revealIcon, revealLabel, components.ButtonGhost).Disabled(pending.Pending).Tooltip(revealLabel).Clicked() {
							a.reveal = !allRevealed
							a.revealedFields = nil
						}
						if iconButton(c, lucide.Pencil(), a.t("编辑条目"), components.ButtonGhost).Disabled(pending.Pending).Tooltip(a.t("编辑条目")).Clicked() {
							a.editEntry()
						}
						if iconButton(c, lucide.Trash(), a.t("移到回收站"), components.ButtonGhost).Disabled(pending.Pending).Tooltip(a.t("移到回收站")).Clicked() {
							a.deleteOpen = true
						}
					})
					if pending.Delayed {
						components.Text(c, a.t("正在读取账号…")).Absolute().Left(0).Bottom(-20).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted).SingleLine()
					}
				})
				ui.Column(c).Key("credential-card").Label(a.t("账号字段")).FillWidth().Children(func() {
					if pending.Pending {
						for _, f := range pending.Fields {
							label := f.Label
							if !strings.HasPrefix(f.Key, "custom:") {
								label = a.t(label)
							}
							a.detailField(c, f.Key, label, "", f.Secret, true)
						}
					} else {
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
					}
				})
				if d.Notes != "" || pending.HasNotes {
					notes := ui.Column(c).Key("detail-notes").FillWidth().Margin(24, 4, 0, 4).Gap(8)
					if pending.Pending {
						notes.MinHeight(notes.LayoutBounds().H)
					}
					notes.Children(func() {
						components.Text(c, a.t("备注")).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
						if pending.Pending {
							detailPlaceholder(c, 240)
						} else {
							components.Text(c, d.Notes).Wrap().FontSize(scaledFontSize(c, 13))
						}
					})
				}
				if d.URL != "" || pending.HasWebsite {
					ui.Row(c).Key("website-action").FillWidth().Margin(20, 0, 0, 0).Children(func() {
						open := components.Button(c, "", components.ButtonOutline).Label(a.t("打开网站")).Height(32).Padding(6, 10).Gap(6).Disabled(pending.Pending)
						open.Children(func() {
							components.Icon(c, lucide.ArrowUpRight()).Size(16, 16)
							components.Text(c, a.t("打开网站")).FontSize(scaledFontSize(c, 12)).SingleLine()
						})
						if open.Clicked() {
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
	})
}

func detailPlaceholder(c *ui.Context, width float32) {
	ui.Box(c).Width(width).MaxWidthPercent(100).Children(func() {
		components.Text(c, " ").FontSize(scaledFontSize(c, 13))
		ui.Box(c).Absolute().Left(0).Right(0).Top(4).Height(8).Radius(4).Background(c.Theme().Text.Alpha(.08))
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
	a.detailField(c, key, label, value, secret, false)
}

func (a *application) detailField(c *ui.Context, key, label, value string, secret, pending bool) {
	revealed := a.detailFieldRevealed(key)
	minimumHeight := float32(64)
	row := ui.Row(c).Key(key).FillWidth().Padding(12, 4).Gap(12).AlignItems(ui.Center)
	if key == "password" {
		minimumHeight = 80
		row.Padding(16).Margin(0, 0, 5, 0).Radius(8).Background(c.Theme().Background.Mix(c.Theme().Surface, .55))
	} else {
		row.BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border.Alpha(.6))
	}
	if pending {
		// Preserve wrapped field geometry while discarding its previous value.
		minimumHeight = max(minimumHeight, row.LayoutBounds().H)
	}
	row.MinHeight(minimumHeight)
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
			components.Text(c, label).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
			if pending {
				detailPlaceholder(c, 164)
			} else {
				shown := value
				if secret && !revealed {
					shown = "••••••••••••"
				}
				text := components.Text(c, shown).Wrap().FontSize(scaledFontSize(c, 13))
				if secret {
					text.Font("monospace")
				}
			}
		})
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(2).Children(func() {
			if secret {
				icon := lucide.Eye()
				if revealed {
					icon = lucide.EyeOff()
				}
				if iconButton(c, icon, a.t("显示或隐藏%s", label), components.ButtonGhost).Disabled(pending).Clicked() {
					if a.revealedFields == nil {
						a.revealedFields = make(map[string]bool)
					}
					a.revealedFields[key] = !revealed
				}
			}
			copyText := a.t("复制")
			if key == "password" {
				copyText = a.t("复制密码")
			}
			copy := components.Button(c, "", components.ButtonGhost).Label(a.t("复制%s", label)).Height(32).Shrink(0).Padding(0, 9).Gap(6).Border(0, c.Theme().Border).TextColor(c.Theme().TextMuted).Disabled(pending)
			copy.Children(func() {
				components.Icon(c, lucide.Copy()).Size(16, 16)
				components.Text(c, copyText).FontSize(scaledFontSize(c, 12)).SingleLine()
			})
			if copy.Clicked() {
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
