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
	ui.Column(c).Fill().MinHeight(0).Padding(24, 16, 12, 16).Gap(16).Children(func() {
		ui.Row(c).FillWidth().MinHeight(43).AlignItems(ui.Start).Gap(8).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				components.Text(c, a.entriesTitle()).FillWidth().SingleLine().Ellipsis("…").FontSize(scaledFontSize(c, 18)).Bold()
				ui.Row(c).FillWidth().Label(a.t("条目数量")).Children(func() {
					components.Text(c, a.entriesCountLabel()).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
				})
			})
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
				if strings.TrimSpace(a.query) == "" {
					if components.Button(c, a.t("新建条目"), components.ButtonOutline).Clicked() {
						a.newEntry()
					}
				} else {
					components.Text(c, a.t("试试其他关键词，或清除搜索以查看当前文件夹的条目。")).Wrap().TextAlign(ui.Center).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
					if components.Button(c, a.t("清除搜索"), components.ButtonOutline).Clicked() {
						a.query = ""
						a.filtered = a.entries()
						a.syncSelectedIndex()
						a.list.ScrollTo(0, ui.Start)
						a.searchRef.Focus()
						a.lastInput = time.Now()
					}
				}
			})
		} else {
			components.List(c, &a.list, len(a.filtered), func(i int) {
				e := a.filtered[i]
				row := ui.Row(c).FillWidth().MinHeight(60).Padding(8, 9).AlignItems(ui.Center).Gap(8).Radius(c.Theme().Radius).Label(a.t("账号 %s", e.Path))
				selected := e.Path == a.selected
				showCopy := row.Hovered() || row.FocusWithin() || i == a.selectedIndex && a.list.Focused(c)
				if selected {
					if isMonochromePalette(c.Theme()) {
						row.Background(c.Theme().Selection)
					} else {
						row.Background(c.Theme().Accent.Alpha(.10))
					}
				} else if row.Hovered() {
					row.Background(c.Theme().Text.Alpha(.04))
				}
				row.Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						components.Text(c, e.Name).SingleLine().Ellipsis("…").FontSize(scaledFontSize(c, 13)).Bold()
						components.Text(c, a.entrySubtitle(e)).SingleLine().Ellipsis("…").FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
					})
					ui.Box(c).Size(28, 28).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
						if selected && !showCopy {
							components.Icon(c, lucide.ChevronRight()).Size(14, 14).TextColor(c.Theme().TextMuted)
						}
						copy := iconButton(c, lucide.Copy(), a.t("复制密码 %s", e.Name), components.ButtonGhost).Key("copy-entry").Absolute().Left(0).Top(0).Size(28, 28).Disabled(a.busy)
						if !showCopy {
							copy.Opacity(0)
						}
						if copy.Clicked() {
							a.copyEntry(e.Path)
						}
					})
				})
			}).Grow(1).FillWidth()
		}
	})
}

func (a *application) detailView(c *ui.Context) {
	pending := a.detailTransitionPresentation(c.Now())
	if pending.Pending && pending.Remaining > 0 {
		c.After(pending.Remaining)
	}
	if pending.Pending && pending.Unlocking {
		// Keep authentication's last surface until the first complete read. A
		// password-only skeleton would add an unnecessary intermediate layout.
		a.unlockView(c)
		if pending.Delayed {
			a.detailLoadingFeedback(c)
		}
		return
	}
	if a.detail.Entry == "" && (!pending.Pending || pending.DisplayEntry == "") {
		ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Padding(24).Gap(12).Children(func() {
			components.Icon(c, lucide.ShieldCheck).Size(36, 36).TextColor(c.Theme().TextMuted)
			components.Text(c, a.t("选择一个条目")).FontSize(scaledFontSize(c, 18)).Bold()
			muted(c, a.t("从列表选择条目，在右侧查看详情。"))
			if components.Button(c, a.t("新建条目"), components.ButtonGhost).Disabled(pending.Pending).Clicked() {
				a.newEntry()
			}
		})
		if pending.Delayed {
			a.detailLoadingFeedback(c)
		}
		return
	}
	d := a.detail
	path := d.Entry
	if pending.Pending {
		path = pending.DisplayEntry
	}
	components.ScrollView(c, components.ScrollViewOptions{}).Key("detail-scroll").Label(a.t("条目详情")).TrackScroll(&a.detailScroll).Fill().Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Column(c).Key("entry-detail").FillWidth().MaxWidth(808).Padding(24).Children(func() {
				ui.Row(c).Key("detail-toolbar").FillWidth().MinHeight(32).AlignItems(ui.Center).Gap(12).Children(func() {
					a.detailBreadcrumb(c, path, pending.Pending)
					ui.Row(c).Key("detail-actions").Height(32).Shrink(0).AlignItems(ui.Center).Children(func() {
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
				})
				website := d.URL
				if pending.Pending {
					for _, field := range pending.Fields {
						if field.Key == "url" {
							website = field.Value
						}
					}
				}
				a.detailHero(c, path, website)
				ui.Column(c).Key("credential-card").Label(a.t("账号字段")).FillWidth().Gap(1).Dividers(1, c.Theme().Border).Children(func() {
					if pending.Pending {
						for _, f := range orderedDetailFields(pending.Fields) {
							label := f.Label
							if !strings.HasPrefix(f.Key, "custom:") {
								label = a.t(label)
							}
							a.detailField(c, f.Key, label, f.Value, f.Secret, true)
						}
					} else {
						if d.Username != "" {
							a.secretField(c, "username", a.t("用户名"), d.Username, false)
						}
						a.secretField(c, "password", a.t("密码"), d.Password, true)
						if d.OTP != "" {
							a.secretField(c, "otp", a.t("一次性密码"), d.OTP, true)
						}
						if d.URL != "" {
							a.secretField(c, "url", a.t("网址"), d.URL, false)
						}
						for i, f := range extraFields(d.Contents) {
							a.secretField(c, "custom:"+strconv.Itoa(i), f[0], f[1], true)
						}
					}
				})
				if d.Notes != "" || pending.HasNotes {
					notes := ui.Column(c).Key("detail-notes").FillWidth().Margin(20, 0, 0, 0).Gap(8)
					if pending.Pending {
						notes.MinHeight(notes.LayoutBounds().H)
					}
					notes.Children(func() {
						components.Text(c, a.t("备注")).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
						if pending.Pending {
							components.Text(c, pending.Notes).Wrap().FontSize(scaledFontSize(c, 13))
						} else {
							components.Text(c, d.Notes).Wrap().FontSize(scaledFontSize(c, 13))
						}
					})
				}
				a.detailMetadata(c, path)
			})
		})
	})
	if pending.Delayed {
		a.detailLoadingFeedback(c)
	}
}

// The feedback stays in the viewport and takes no space from the retained
// surface, including when its heading has scrolled out of view.
func (a *application) detailLoadingFeedback(c *ui.Context) {
	ui.Row(c).Key("detail-loading-feedback").Absolute().Right(24).Bottom(24).Padding(6, 10).
		Radius(c.Theme().Radius).Background(c.Theme().Surface).Children(func() {
		components.Text(c, a.t("正在读取账号…")).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted).SingleLine()
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
	minimumHeight := float32(76)
	row := ui.Column(c).Key(key).FillWidth().Padding(12, 0).Gap(6)
	if pending {
		// Keep long fields in place while the read-only surface waits.
		minimumHeight = max(minimumHeight, row.LayoutBounds().H)
	}
	row.MinHeight(minimumHeight)
	row.Children(func() {
		components.Text(c, label).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
		valueRow := ui.Row(c).Key("field-value").FillWidth().MinHeight(32).AlignItems(ui.Center).Gap(12)
		if pending {
			valueRow.MinHeight(max(32, valueRow.LayoutBounds().H))
		}
		valueRow.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				shown := value
				if secret && (!revealed || pending) {
					shown = "••••••••••••"
				}
				text := components.Text(c, shown).Wrap().FontSize(scaledFontSize(c, 13))
				if secret {
					text.Font("monospace")
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
				if key == "url" {
					if iconButton(c, lucide.ArrowUpRight(), a.t("打开网站"), components.ButtonGhost).Disabled(pending).Tooltip(a.t("打开网站")).Clicked() {
						if safeWebsite(value) {
							if err := mygo.Shell.OpenExternal(value); err != nil {
								a.errorText = err.Error()
							}
						} else {
							a.errorText = a.t("只允许打开 http 或 https 网址。")
						}
					}
				}
				copy := components.Button(c, "", components.ButtonGhost).Label(a.t("复制%s", label)).Tooltip(a.t("复制%s", label)).Height(32).MinWidth(c.Theme().Rem(5.6)).Shrink(0).Padding(0, 9).Gap(6).AlignItems(ui.Center).Border(0, c.Theme().Border).TextColor(c.Theme().TextMuted).Disabled(pending)
				copy.Children(func() {
					components.Icon(c, lucide.Copy()).Size(managerGlyphSize(c), managerGlyphSize(c))
					components.Text(c, a.t("复制")).FontSize(scaledFontSize(c, 12)).SingleLine()
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
					a.copy(key, value, copyLabel)
				}
			})
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
