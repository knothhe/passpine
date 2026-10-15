package desktop

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine-neo/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

type editorState struct {
	open                                                              bool
	path, originalPath, contents, originalContents, revision, draftID string
	draftSaved                                                        bool
	draftAttempted                                                    bool
	showRaw                                                           bool
	password, username, website, notes                                string
	custom                                                            string
}

func (a *application) newEntry() {
	// The native File menu remains available while a modal editor is open.
	// Reopening it must preserve the current edit, including unsaved changes.
	if a.editor.open {
		a.show()
		a.invalidate()
		return
	}
	if a.busy {
		return
	}
	if !a.state.Configured {
		a.setup = true
		return
	}
	if a.state.Locked {
		a.notice = "请先解锁，然后新建条目。"
		return
	}
	a.editor = editorState{open: true}
	if strings.HasPrefix(a.filter, "folder:") {
		a.editor.path = strings.TrimPrefix(a.filter, "folder:") + "/"
	}
	a.invalidate()
}
func (a *application) editEntry() {
	if a.detail.Entry == "" || a.state.Locked {
		return
	}
	a.editor = editorState{open: true, path: a.detail.Entry, originalPath: a.detail.Entry, contents: a.detail.Contents, originalContents: a.detail.Contents, revision: a.detail.Revision}
	a.editor.readFields()
}
func (e *editorState) readFields() {
	lines := strings.Split(strings.ReplaceAll(e.contents, "\r\n", "\n"), "\n")
	if len(lines) > 0 {
		e.password = lines[0]
	}
	var custom []string
	var notes []string
	for _, line := range lines[1:] {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			if line != "" {
				notes = append(notes, line)
			}
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "username", "user", "login", "email":
			if e.username == "" {
				e.username = strings.TrimSpace(value)
			} else {
				custom = append(custom, line)
			}
		case "url", "website", "site":
			if e.website == "" {
				e.website = strings.TrimSpace(value)
			} else {
				custom = append(custom, line)
			}
		case "notes", "note":
			notes = append(notes, strings.TrimSpace(value))
		default:
			custom = append(custom, line)
		}
	}
	e.notes = strings.Join(notes, "\n")
	e.custom = strings.Join(custom, "\n")
}
func (e *editorState) compose() {
	previous := editorState{contents: e.contents}
	previous.readFields()
	if e.contents == "" {
		e.contents = e.password + "\n"
	} else if previous.password != e.password {
		_, rest, found := strings.Cut(e.contents, "\n")
		lineEnding := "\n"
		if strings.Contains(e.contents, "\r\n") {
			lineEnding = "\r\n"
		}
		e.contents = e.password
		if found {
			e.contents += lineEnding + rest
		}
	}
	if previous.username != e.username {
		e.contents = replaceNamedField(e.contents, []string{"username", "user", "login", "email"}, e.username)
	}
	if previous.website != e.website {
		e.contents = replaceNamedField(e.contents, []string{"url", "website", "site"}, e.website)
	}
	if previous.notes != e.notes {
		e.contents = replaceNotes(e.contents, e.notes)
	}
	e.draftSaved = false
	e.draftAttempted = false
}

// Edit only the first recognized field. Duplicate and unknown lines, spacing,
// ordering and the original line endings survive unrelated form changes.
func replaceNamedField(raw string, aliases []string, value string) string {
	lines := strings.SplitAfter(raw, "\n")
	for i := 1; i < len(lines); i++ {
		body := strings.TrimSuffix(strings.TrimSuffix(lines[i], "\n"), "\r")
		key, _, ok := strings.Cut(body, ":")
		if !ok {
			continue
		}
		match := false
		for _, alias := range aliases {
			if strings.EqualFold(strings.TrimSpace(key), alias) {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		ending := ""
		if strings.HasSuffix(lines[i], "\r\n") {
			ending = "\r\n"
		} else if strings.HasSuffix(lines[i], "\n") {
			ending = "\n"
		}
		if value == "" {
			lines[i] = ""
		} else {
			lines[i] = key + ": " + value + ending
		}
		return strings.Join(lines, "")
	}
	if value == "" {
		return raw
	}
	ending := "\n"
	if strings.Contains(raw, "\r\n") {
		ending = "\r\n"
	}
	if !strings.HasSuffix(raw, "\n") {
		raw += ending
	}
	return raw + aliases[0] + ": " + value + ending
}

func replaceNotes(raw, value string) string {
	ending := "\n"
	if strings.Contains(raw, "\r\n") {
		ending = "\r\n"
	}
	lines := strings.SplitAfter(raw, "\n")
	kept := make([]string, 0, len(lines))
	for i, line := range lines {
		if i == 0 {
			kept = append(kept, line)
			continue
		}
		body := strings.TrimSpace(line)
		key, _, hasKey := strings.Cut(body, ":")
		if body == "" || hasKey && !strings.EqualFold(strings.TrimSpace(key), "notes") && !strings.EqualFold(strings.TrimSpace(key), "note") {
			kept = append(kept, line)
		}
	}
	raw = strings.Join(kept, "")
	if value == "" {
		return raw
	}
	if !strings.HasSuffix(raw, "\n") {
		raw += ending
	}
	return raw + "notes: " + strings.ReplaceAll(value, "\n", ending) + ending
}
func (e *editorState) dirty() bool {
	return e.contents != e.originalContents || e.path != e.originalPath
}

func (a *application) dialogs(c *ui.Context) {
	components.Dialog(c, &a.editor.open, components.DialogOptions{Label: "编辑密码条目", DisableBackdropClose: true, DisableEscapeClose: true}, func(panel ui.Element) {
		panel.Width(600).Padding(24).Gap(18)
		title := "新建条目"
		if a.editor.originalPath != "" {
			title = "编辑条目"
		}
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			components.Text(c, title).FontSize(21).Bold()
			ui.Spacer(c)
			if iconButton(c, lucide.X(), "关闭编辑器", components.ButtonGhost).Clicked() {
				a.cancelEditor()
			}
		})
		if a.errorText != "" {
			components.Text(c, a.errorText).Wrap().TextColor(c.Theme().Danger).FontSize(12)
		}
		components.ScrollView(c, components.ScrollViewOptions{}).FillWidth().MaxHeight(420).Children(func() {
			ui.Column(c).FillWidth().Gap(14).Disabled(a.busy).Children(func() {
				if field(c, "名称与文件夹", &a.editor.path, "例如：工作/GitHub", false).Changed() {
					a.editor.draftSaved = false
					a.editor.draftAttempted = false
					a.lastInput = time.Now()
				}
				if a.editor.showRaw {
					components.Text(c, "完整内容").FontSize(12).Bold()
					if components.TextBox(c, &a.editor.contents, components.TextBoxOptions{Multiline: true}).FillWidth().Height(230).Label("完整条目内容").Changed() {
						a.editor.draftSaved = false
						a.editor.draftAttempted = false
						a.lastInput = time.Now()
					}
					muted(c, "第一行是密码，其余行按原样保存。原文模式保留重复字段、未知字段与空行。")
				} else {
					changed := false
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.End).Children(func() {
						ui.Column(c).Grow(1).Children(func() {
							if field(c, "密码", &a.editor.password, "输入或生成密码", true).Changed() {
								changed = true
							}
						})
						if iconButton(c, lucide.Sparkles(), "生成并填入密码", components.ButtonOutline).Clicked() {
							a.generate(func(password string) { a.editor.password = password; a.editor.compose() })
						}
					})
					if field(c, "用户名", &a.editor.username, "可选", false).Changed() {
						changed = true
					}
					if field(c, "网址", &a.editor.website, "https://example.com", false).Changed() {
						changed = true
					}
					components.Text(c, "备注").FontSize(12).Bold()
					if components.TextBox(c, &a.editor.notes, components.TextBoxOptions{Multiline: true}).FillWidth().Height(90).Label("备注").Changed() {
						changed = true
					}
					if a.editor.custom != "" {
						muted(c, "其他原有字段会保留，可切换原文模式编辑。")
					}
					if changed {
						a.editor.compose()
						a.lastInput = time.Now()
					}
				}
			})
		})
		ui.Row(c).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
			label := "原文模式"
			if a.editor.showRaw {
				label = "表单模式"
			}
			if components.Button(c, label, components.ButtonGhost).Clicked() {
				if a.editor.showRaw {
					a.editor.password = ""
					a.editor.username = ""
					a.editor.website = ""
					a.editor.notes = ""
					a.editor.custom = ""
					a.editor.readFields()
				}
				a.editor.showRaw = !a.editor.showRaw
			}
			if a.editor.draftSaved {
				components.Text(c, "已保存加密草稿").FontSize(11).TextColor(c.Theme().Success)
			} else if components.Button(c, "保存草稿", components.ButtonGhost).Disabled(a.busy || a.editor.path == "").Clicked() {
				a.saveDraft()
			}
			ui.Spacer(c)
			if components.Button(c, "取消", components.ButtonOutline).Clicked() {
				a.cancelEditor()
			}
			if components.Button(c, "保存", components.ButtonPrimary).Disabled(a.busy).Clicked() {
				a.saveEntry()
			}
		})
	})
	components.Dialog(c, &a.backupOpen, components.DialogOptions{Label: "验证恢复私钥", DisableBackdropClose: true}, func(panel ui.Element) {
		panel.Width(460).Padding(24).Gap(18)
		section(c, "验证恢复私钥", "输入这份备份私钥的口令。应用会实际尝试解密当前密码库。", nil)
		muted(c, a.backupPath)
		input := field(c, "备份私钥口令", &a.backupPassphrase, "输入口令", true)
		if a.errorText != "" {
			components.Text(c, a.errorText).Wrap().TextColor(c.Theme().Danger)
		}
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			if components.Button(c, "取消", components.ButtonOutline).Clicked() {
				a.backupOpen = false
				a.backupPassphrase = ""
			}
			if components.Button(c, "验证备份", components.ButtonPrimary).Disabled(a.busy).Clicked() || input.Submitted() {
				a.call(app.Request{Action: "verify-backup", KeyFile: a.backupPath, Passphrase: a.backupPassphrase}, func(app.Data) {
					a.backupOpen = false
					a.backupPassphrase = ""
					a.notice = "恢复备份已通过解密验证"
				})
			}
		})
	})
	if components.AlertDialog(c, &a.discardOpen, components.AlertDialogOptions{Title: "放弃本次修改？", Message: "尚未保存的修改会丢失。你也可以先保存为加密草稿。", ConfirmLabel: "放弃修改", CancelLabel: "继续编辑"}) == components.DialogConfirmed {
		a.editor = editorState{}
	}
	if components.AlertDialog(c, &a.deleteOpen, components.AlertDialogOptions{Title: "将条目移到回收站？", Message: "将在本机保存加密恢复副本，之后可从恢复中心还原。", ConfirmLabel: "移到回收站", CancelLabel: "取消"}) == components.DialogConfirmed {
		entry := a.detail.Entry
		a.call(app.Request{Action: "delete", Entry: entry, Revision: a.detail.Revision}, func(app.Data) { a.detail = app.Data{}; a.selected = ""; a.notice = "已移到回收站" })
	}
	if components.AlertDialog(c, &a.purgeOpen, components.AlertDialogOptions{Title: "永久删除这份恢复副本？", Message: "此操作无法撤销。密码库中的现有条目不会受影响。", ConfirmLabel: "永久删除", CancelLabel: "取消"}) == components.DialogConfirmed {
		id := a.purgeID
		a.call(app.Request{Action: "purge", ID: id, Kind: a.recoveryType()}, func(d app.Data) { a.removeRecovery(id); a.notice = "恢复副本已永久删除" })
	}
}
func (a *application) cancelEditor() {
	if a.editor.dirty() {
		a.discardOpen = true
	} else {
		a.editor = editorState{}
	}
}
func (a *application) saveDraft() {
	if a.busy {
		return
	}
	a.editor.draftAttempted = true
	e := a.editor
	a.call(app.Request{Action: "draft", Entry: e.originalPath, Destination: e.path, Contents: e.contents, Revision: e.revision, ID: e.draftID}, func(d app.Data) {
		a.editor.draftID = d.ID
		a.editor.draftSaved = a.editor.contents == e.contents && a.editor.path == e.path
		a.editor.draftAttempted = a.editor.draftSaved
	})
}
func (a *application) saveEntry() {
	e := a.editor
	if strings.TrimSpace(e.path) == "" || strings.TrimSpace(e.contents) == "" {
		a.errorText = "请填写条目名称与密码。"
		return
	}
	req := app.Request{Action: "save", Entry: e.path, Contents: e.contents, Mode: "create", ID: e.draftID}
	if e.originalPath != "" {
		req.Mode = "edit"
		req.Entry = e.originalPath
		req.Destination = e.path
		req.Revision = e.revision
	}
	a.call(req, func(d app.Data) {
		a.editor = editorState{}
		a.page = "entries"
		a.filter = "all"
		a.selected = e.path
		a.notice = "条目已加密保存"
		a.detail = d
		if d.Entry == "" {
			a.detail = app.Data{}
		}
	})
}
