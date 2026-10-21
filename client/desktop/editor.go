package desktop

import (
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

type editorState struct {
	open                                                              bool
	path, originalPath, contents, originalContents, revision, draftID string
	draftSaved                                                        bool
	draftAttempted                                                    bool
	showRaw                                                           bool
	revealPassword                                                    bool
	password, username, website                                       string
	fields                                                            *editorFields
	identity                                                          *editorIdentity
	exit                                                              *editorExitIntent
	folderQuery                                                       string
	folder, name, pathFieldsPath                                      string
}

type editorIdentity struct{ marker byte }

type editorExitIntent struct{ done func() }

type editorField struct {
	id, line      int
	key, value    string
	originalKey   string
	originalValue string
	deleted       bool
}

// Rows keep their identity while a user edits or removes adjacent fields.
// Source line positions let unrelated form changes preserve the original bytes.
type editorFields struct {
	rows                      []*editorField
	nextID                    int
	usernameLine, websiteLine int
	username, website         string
}

func (e *editorState) ensureIdentity() *editorIdentity {
	if e.identity == nil {
		e.identity = &editorIdentity{}
	}
	return e.identity
}

func (a *application) newEntry() {
	// The native File menu remains available while an inline editor is open.
	// Reopening it must preserve the current edit, including unsaved changes.
	if a.editor.open {
		a.navigate("entries", "")
		a.show()
		a.invalidate()
		return
	}
	if a.busy {
		if a.backgroundBusy {
			a.pendingNewEntry = unlockAction{action: "new-entry", vaultID: a.state.VaultID, filter: a.filter, generation: a.generation}
		}
		return
	}
	if !a.state.Configured {
		a.openSetup()
		return
	}
	if a.state.Locked {
		a.navigate("entries", "")
		a.selected = ""
		a.beginUnlock("new-entry")
		return
	}
	a.editor = editorState{open: true}
	a.navigate("entries", "")
	a.closeGenerator()
	if strings.HasPrefix(a.filter, "folder:") {
		a.editor.path = strings.TrimPrefix(a.filter, "folder:") + "/"
	}
	a.editor.readFields()
	a.invalidate()
}
func (a *application) editEntry() {
	if a.detail.Entry == "" || a.state.Locked {
		return
	}
	if a.editor.open {
		return
	}
	a.closeGenerator()
	a.editor = editorState{open: true, path: a.detail.Entry, originalPath: a.detail.Entry, contents: a.detail.Contents, originalContents: a.detail.Contents, revision: a.detail.Revision}
	a.editor.readFields()
	a.invalidate()
}
func (e *editorState) readFields() {
	e.ensureIdentity()
	e.password, e.username, e.website = "", "", ""
	e.fields = &editorFields{usernameLine: -1, websiteLine: -1}
	lines := strings.Split(strings.ReplaceAll(e.contents, "\r\n", "\n"), "\n")
	if len(lines) > 0 {
		e.password = lines[0]
	}
	for i, line := range lines[1:] {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "username", "user", "login", "email":
			if e.fields.usernameLine < 0 {
				e.username = strings.TrimSpace(value)
				e.fields.usernameLine = i + 1
			} else {
				e.fields.add(key, strings.TrimPrefix(value, " "), i+1)
			}
		case "url", "website", "site":
			if e.fields.websiteLine < 0 {
				e.website = strings.TrimSpace(value)
				e.fields.websiteLine = i + 1
			} else {
				e.fields.add(key, strings.TrimPrefix(value, " "), i+1)
			}
		case "notes", "note":
			// Notes are retained in the source without a separate form field.
		default:
			e.fields.add(key, strings.TrimPrefix(value, " "), i+1)
		}
	}
	e.fields.username, e.fields.website = e.username, e.website
	e.syncPathFields()
}

func (f *editorFields) add(key, value string, line int) {
	f.nextID++
	key = strings.TrimSpace(key)
	f.rows = append(f.rows, &editorField{id: f.nextID, line: line, key: key, value: value, originalKey: key, originalValue: value})
}

// Only edited lines are rewritten. Duplicate and unknown lines, spacing,
// ordering, notes, and original line endings survive unrelated form changes.
func (e *editorState) compose() {
	if e.fields == nil {
		e.fields = &editorFields{usernameLine: -1, websiteLine: -1}
	}
	ending := "\n"
	if strings.Contains(e.contents, "\r\n") {
		ending = "\r\n"
	}
	lines := strings.SplitAfter(e.contents, "\n")
	rows := make(map[int]*editorField)
	for _, row := range e.fields.rows {
		if row.line >= 0 {
			rows[row.line] = row
		}
	}
	var out []string
	usernameLine, websiteLine := -1, -1
	for i, line := range lines {
		body, suffix := splitEditorLine(line)
		if i == 0 {
			if body != e.password || e.contents == "" {
				if e.contents == "" {
					suffix = ending
				}
				line = e.password + suffix
			}
		} else if row := rows[i]; row != nil {
			if row.deleted {
				row.line = -1
				continue
			}
			if row.writable() && (row.key != row.originalKey || row.value != row.originalValue) {
				line = strings.TrimSpace(row.key) + ": " + row.value + suffix
				row.originalKey, row.originalValue = row.key, row.value
			}
			row.line = len(out)
		} else if i == e.fields.usernameLine {
			if e.username == "" && e.username != e.fields.username {
				continue
			}
			if e.username != e.fields.username {
				key, _, _ := strings.Cut(body, ":")
				line = key + ": " + e.username + suffix
			}
			usernameLine = len(out)
		} else if i == e.fields.websiteLine {
			if e.website == "" && e.website != e.fields.website {
				continue
			}
			if e.website != e.fields.website {
				key, _, _ := strings.Cut(body, ":")
				line = key + ": " + e.website + suffix
			}
			websiteLine = len(out)
		}
		out = append(out, line)
	}
	appendLine := func(line string) int {
		if len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
		if len(out) > 0 && !strings.HasSuffix(out[len(out)-1], "\n") {
			out[len(out)-1] += ending
		}
		index := len(out)
		out = append(out, line+ending)
		return index
	}
	if usernameLine < 0 && e.username != "" {
		usernameLine = appendLine("username: " + e.username)
	}
	if websiteLine < 0 && e.website != "" {
		websiteLine = appendLine("url: " + e.website)
	}
	for _, row := range e.fields.rows {
		if row.line < 0 && !row.deleted && row.writable() {
			row.line = appendLine(strings.TrimSpace(row.key) + ": " + row.value)
			row.originalKey, row.originalValue = row.key, row.value
		}
	}
	e.fields.usernameLine, e.fields.websiteLine = usernameLine, websiteLine
	e.fields.username, e.fields.website = e.username, e.website
	e.contents = strings.Join(out, "")
	e.draftSaved = false
	e.draftAttempted = false
}

func splitEditorLine(line string) (body, ending string) {
	if strings.HasSuffix(line, "\r\n") {
		return strings.TrimSuffix(line, "\r\n"), "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return strings.TrimSuffix(line, "\n"), "\n"
	}
	return line, ""
}

func (e *editorState) customFieldsValid() bool {
	if e.showRaw || e.fields == nil {
		return true
	}
	for _, row := range e.fields.rows {
		if row.deleted || row.line < 0 && row.key == "" && row.value == "" {
			continue
		}
		if strings.ContainsAny(row.value, "\r\n") {
			return false
		}
		if !row.validKey() && !(row.line >= 0 && row.key == row.originalKey && row.value == row.originalValue) {
			return false
		}
	}
	return true
}

func validEditorFieldKey(key string) bool {
	key = strings.TrimSpace(key)
	return key != "" && !strings.EqualFold(key, "note") && !strings.EqualFold(key, "notes") && !reservedEditorFieldKey(key) && strings.IndexFunc(key, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-'
	}) < 0
}

func reservedEditorFieldKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "username", "user", "login", "email", "url", "uri", "website", "site", "link":
		return true
	}
	return false
}

func (row *editorField) validKey() bool {
	// Existing duplicate aliases retain their position and remain editable. A
	// new alias or rename could otherwise shadow the form's dedicated fields.
	return validEditorFieldKey(row.key) || row.line >= 0 && row.key == row.originalKey && reservedEditorFieldKey(row.key)
}

func (row *editorField) writable() bool {
	return row.validKey() && !strings.ContainsAny(row.value, "\r\n")
}

func (a *application) editorFieldValidationMessage() string {
	return a.t("字段名不能为空，仅支持文字、数字、下划线和连字符，不能使用 note/notes；用户名和网址字段请使用专用输入框，字段值不能包含换行。")
}

func (e *editorState) dirty() bool {
	if e.contents != e.originalContents || e.path != e.originalPath {
		return true
	}
	if e.fields != nil && !e.showRaw {
		for _, row := range e.fields.rows {
			if !row.deleted && (row.key != row.originalKey || row.value != row.originalValue || row.line < 0 && (row.key != "" || row.value != "")) {
				return true
			}
		}
	}
	return false
}

func (a *application) editorView(c *ui.Context) {
	if !a.editor.open || a.state.Locked {
		return
	}
	a.editor.ensureIdentity()
	if a.editor.fields == nil {
		a.editor.fields = &editorFields{usernameLine: -1, websiteLine: -1}
	}
	ui.Column(c).Key(struct{ identity *editorIdentity }{a.editor.identity}).Fill().MinWidth(0).MaxWidth(808).AlignSelf(ui.Center).Padding(24).Gap(16).Children(func() {
		title := a.t("新建条目")
		if a.editor.originalPath != "" {
			title = a.t("编辑条目")
		}
		ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(12).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Row(c).FillWidth().Height(32).AlignItems(ui.Center).Children(func() {
					components.Text(c, title).FontSize(scaledFontSize(c, 22)).Bold()
				})
				if a.editor.originalPath != "" {
					components.Text(c, a.editor.originalPath).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted).Wrap()
				}
			})
			if iconButton(c, lucide.X(), a.t("关闭编辑器"), components.ButtonGhost).Disabled(a.editorBusy()).Clicked() {
				a.cancelEditor()
			}
		})
		// Closing clears the form immediately. Stop this build before creating
		// inputs backed by that now-cleared state (including custom field rows).
		if !a.editor.open {
			return
		}
		components.ScrollView(c, components.ScrollViewOptions{}).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Gap(12).Disabled(a.editorBusy()).Children(func() {
				a.editor.syncPathFields()
				if field(c, a.t("名称"), &a.editor.name, a.t("例如：path/to/entry"), false).Label(a.t("账号名称")).Changed() {
					a.editor.setPath(a.editor.folder, a.editor.name)
					a.lastInput = time.Now()
				}
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					components.Text(c, a.t("文件夹")).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
					if a.folderSelect(c, &a.editor.folder, &a.editor.folderQuery, a.t("选择文件夹"), a.t("密码库根目录"), true).Changed() {
						a.editor.setPath(a.editor.folder, a.editor.name)
					}
				})
				if a.editor.showRaw {
					components.Text(c, a.t("完整内容")).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
					if textBox(c, &a.editor.contents, components.TextBoxOptions{Multiline: true}).FillWidth().Height(230).Label(a.t("完整条目内容")).Changed() {
						a.editor.draftSaved = false
						a.editor.draftAttempted = false
						a.lastInput = time.Now()
					}
					muted(c, a.t("第一行是密码，其余行按原样保存。"))
				} else {
					changed := false
					ui.Column(c).Key("editor-password").FillWidth().Gap(6).Children(func() {
						components.Text(c, a.t("密码")).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
						ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
							input := textBox(c, &a.editor.password, components.TextBoxOptions{}).Grow(1).MinWidth(0).
								Label(a.t("密码")).Placeholder(a.t("输入或生成密码"))
							if !a.editor.revealPassword {
								input.Password()
							}
							if input.Changed() {
								changed = true
							}
							eye := lucide.Eye()
							if a.editor.revealPassword {
								eye = lucide.EyeOff()
							}
							if iconButton(c, eye, a.t("显示或隐藏编辑密码"), components.ButtonGhost).Shrink(0).Clicked() {
								a.editor.revealPassword = !a.editor.revealPassword
							}
							if iconButton(c, lucide.Sparkles(), a.t("生成并填入密码"), components.ButtonGhost).Shrink(0).Clicked() {
								a.openGenerator()
							}
						})
					})
					if field(c, a.t("用户名"), &a.editor.username, a.t("可选"), false).Changed() {
						changed = true
					}
					if field(c, a.t("网址"), &a.editor.website, "https://example.com", false).Changed() {
						changed = true
					}
					ui.Column(c).Key("editor-custom-fields").FillWidth().Gap(8).Children(func() {
						components.Text(c, a.t("自定义字段")).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
						for _, row := range a.editor.fields.rows {
							if row.deleted {
								continue
							}
							ui.Row(c).Key(row.id).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
								if textBox(c, &row.key, components.TextBoxOptions{}).Width(150).MinWidth(88).Shrink(1).Label(a.t("自定义字段名称 %d", row.id)).Placeholder(a.t("字段名")).Changed() {
									changed = true
								}
								if textBox(c, &row.value, components.TextBoxOptions{}).Grow(1).MinWidth(0).Label(a.t("自定义字段值 %d", row.id)).Placeholder(a.t("字段值")).Changed() {
									changed = true
								}
								if iconButton(c, lucide.Trash(), a.t("删除自定义字段 %d", row.id), components.ButtonGhost).Shrink(0).Clicked() {
									row.deleted, changed = true, true
								}
							})
						}
						if components.Button(c, a.t("添加字段"), components.ButtonGhost).Clicked() {
							a.editor.fields.add("", "", -1)
						}
						if !a.editor.customFieldsValid() {
							components.Text(c, a.editorFieldValidationMessage()).Wrap().FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().Danger)
						}
					})
					if changed {
						a.editor.compose()
						a.lastInput = time.Now()
					}
				}
			})
		})
		draftStatus := ""
		if a.editor.draftSaved {
			draftStatus = a.t("已保存加密草稿")
		}
		components.Text(c, draftStatus).Height(16).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().Success)
		ui.Row(c).Key("editor-actions").FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
			label := a.t("原文模式")
			if a.editor.showRaw {
				label = a.t("表单模式")
			}
			if components.Button(c, label, components.ButtonGhost).Disabled(a.editorBusy() || !a.editor.customFieldsValid()).Clicked() {
				if a.editor.showRaw {
					a.editor.readFields()
					a.editor.showRaw = false
				} else {
					a.editor.showRaw = true
				}
			}
			if components.Button(c, a.t("保存草稿"), components.ButtonGhost).Disabled(a.busy || a.editor.path == "" || a.editor.draftSaved || !a.editor.customFieldsValid()).Clicked() {
				a.saveDraft()
			}
			ui.Spacer(c)
			if components.Button(c, a.t("取消"), components.ButtonGhost).Disabled(a.editorBusy()).Clicked() {
				a.cancelEditor()
			}
			if components.Button(c, a.t("保存"), components.ButtonPrimary).Disabled(a.busy).Clicked() {
				a.saveEntry()
			}
		})
	})
}

func (a *application) dialogs(c *ui.Context) {
	a.generatorDialog(c)
	if a.discardOpen {
		c.Key("discard-editor")
	}
	switch components.AlertDialog(c, &a.discardOpen, components.AlertDialogOptions{Title: a.t("放弃本次修改？"), Message: a.t("尚未保存的修改会丢失。你也可以先保存为加密草稿。"), ConfirmLabel: a.t("放弃修改"), CancelLabel: a.t("继续编辑")}) {
	case components.DialogConfirmed:
		intent := a.editor.exit
		a.closeEditor()
		if intent != nil && intent.done != nil {
			intent.done()
		}
	case components.DialogCancelled:
		a.editor.exit = nil
	}
	if a.deleteOpen {
		c.Key("delete-entry")
	}
	if components.AlertDialog(c, &a.deleteOpen, components.AlertDialogOptions{Title: a.t("将条目移到回收站？"), Message: a.t("将在本机保存加密恢复副本，之后可从恢复中心还原。"), ConfirmLabel: a.t("移到回收站"), CancelLabel: a.t("取消")}) == components.DialogConfirmed {
		entry := a.detail.Entry
		a.call(app.Request{Action: "delete", Entry: entry, Revision: a.detail.Revision}, func(app.Data) {
			a.detail, a.reveal, a.revealedFields = app.Data{}, false, nil
			a.selected = ""
			a.notice = a.t("已移到回收站")
		})
	}
}

func (a *application) preferencesDialogs(c *ui.Context) {
	components.Dialog(c, &a.backupOpen, components.DialogOptions{Label: a.t("验证恢复私钥"), DisableBackdropClose: true}, func(panel ui.Element) {
		panel.Width(460).Padding(24).Gap(18)
		section(c, a.t("验证恢复私钥"), "", nil)
		muted(c, a.backupPath)
		input := field(c, a.t("备份私钥口令"), &a.backupPassphrase, a.t("输入口令"), true)
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			if components.Button(c, a.t("取消"), components.ButtonOutline).Clicked() {
				a.backupOpen = false
				a.backupPassphrase = ""
			}
			if components.Button(c, a.t("验证备份"), components.ButtonPrimary).Disabled(a.busy).Clicked() || input.Submitted() {
				a.call(app.Request{Action: "verify-backup", KeyFile: a.backupPath, Passphrase: a.backupPassphrase}, func(app.Data) {
					a.backupOpen = false
					a.backupPassphrase = ""
					a.notice = a.t("恢复备份已通过解密验证")
				})
			}
		})
	})
	if components.AlertDialog(c, &a.purgeOpen, components.AlertDialogOptions{Title: a.t("永久删除这份恢复副本？"), Message: a.t("此操作无法撤销。密码库中的现有条目不会受影响。"), ConfirmLabel: a.t("永久删除"), CancelLabel: a.t("取消")}) == components.DialogConfirmed {
		id := a.purgeID
		a.call(app.Request{Action: "purge", ID: id, Kind: a.recoveryType()}, func(d app.Data) { a.removeRecovery(id); a.notice = a.t("恢复副本已永久删除") })
	}
}
func (a *application) cancelEditor() {
	a.requestEditorClose(nil)
}

// Navigation can defer its next action until the user explicitly discards a
// dirty form. Rendering and the editor's X never close a native window.
func (a *application) requestEditorClose(done func()) {
	if a.editorBusy() {
		return
	}
	if a.editor.open && a.editor.dirty() {
		a.editor.exit = &editorExitIntent{done: done}
		a.discardOpen = true
	} else {
		a.closeEditor()
		if done != nil {
			done()
		}
	}
	a.invalidate()
}

func (a *application) closeEditor() {
	a.closeGenerator()
	a.discardOpen = false
	a.editor = editorState{}
	a.invalidate()
}
func (a *application) saveDraft() {
	if a.busy || !a.editor.customFieldsValid() {
		return
	}
	identity := a.editor.ensureIdentity()
	a.editor.draftAttempted = true
	e := a.editor
	a.callBackground(app.Request{Action: "draft", Entry: e.originalPath, Destination: e.path, Contents: e.contents, Revision: e.revision, ID: e.draftID}, func(d app.Data) {
		if !a.editor.open || a.editor.identity != identity {
			return
		}
		a.editor.draftID = d.ID
		a.editor.draftSaved = a.editor.contents == e.contents && a.editor.path == e.path && a.editor.customFieldsValid()
		a.editor.draftAttempted = a.editor.draftSaved
	})
}
func (a *application) saveEntry() {
	e := a.editor
	if !e.customFieldsValid() {
		a.errorText = a.editorFieldValidationMessage()
		return
	}
	if strings.TrimSpace(e.path) == "" || strings.TrimSpace(e.contents) == "" {
		a.errorText = a.t("请填写条目名称与密码。")
		return
	}
	identity, navigationRevision, vaultID := a.editor.ensureIdentity(), a.history.revision, a.state.VaultID
	req := app.Request{Action: "save", Entry: e.path, Contents: e.contents, Mode: "create", ID: e.draftID}
	if e.originalPath != "" {
		req.Mode = "edit"
		req.Entry = e.originalPath
		req.Destination = e.path
		req.Revision = e.revision
	}
	a.call(req, func(d app.Data) {
		a.rebaseSavedEntryHistory(identity, vaultID, e.originalPath, e.path)
		if a.editor.identity == identity {
			a.closeEditor()
		}
		// Saving may finish after the user opens settings or another page.
		// Update the saved account without replacing that newer navigation.
		if a.page == "entries" && a.history.revision == navigationRevision {
			a.filter = "all"
		}
		a.selected = e.path
		a.notice = a.t("条目已加密保存")
		a.reveal, a.revealedFields = false, nil
		a.detail = d
		if d.Entry == "" {
			a.detail = app.Data{}
		}
	})
}

// History retains metadata rather than form contents. Once the form is saved,
// its destinations refer to the resulting entry; renamed entries also update
// older references in the same vault without changing the navigation order.
func (a *application) rebaseSavedEntryHistory(identity *editorIdentity, vaultID, originalPath, path string) {
	update := func(location *navigationLocation) {
		if location.vaultID != vaultID {
			return
		}
		owned := identity != nil && location.editorID == identity
		if !owned && (originalPath == "" || location.selected != originalPath) {
			return
		}
		location.selected = path
		if owned {
			location.editorID = nil
		}
	}
	for i := range a.history.back {
		update(&a.history.back[i])
	}
	for i := range a.history.forward {
		update(&a.history.forward[i])
	}
}

func (a *application) editorBusy() bool {
	return a.busy && !a.backgroundBusy && a.activeRequestAction != "draft"
}
