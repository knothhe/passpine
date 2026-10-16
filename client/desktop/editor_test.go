package desktop

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/vault"
)

func TestEditorCustomFieldsPreserveUnknownLinesAndSupportChanges(t *testing.T) {
	raw := "secret\r\nEmail: user@example.test\r\n  api-key : original  \r\napi-key: duplicate\r\nnotes: retain notes\r\nfree text\r\n\r\n"
	e := editorState{contents: raw, originalContents: raw}
	e.readFields()
	if len(e.fields.rows) != 2 {
		t.Fatalf("custom rows: %#v", e.fields.rows)
	}
	first, second := e.fields.rows[0], e.fields.rows[1]
	e.password = "new-secret"
	e.compose()
	if want := strings.Replace(raw, "secret", "new-secret", 1); e.contents != want {
		t.Fatalf("editing password changed other bytes: %q", e.contents)
	}
	first.key, first.value = "token", "updated: value"
	second.deleted = true
	e.fields.add("account-id", "42", -1)
	e.compose()
	want := "new-secret\r\nEmail: user@example.test\r\ntoken: updated: value\r\nnotes: retain notes\r\nfree text\r\n\r\naccount-id: 42\r\n"
	if e.contents != want {
		t.Fatalf("custom fields did not save in place:\n got %q\nwant %q", e.contents, want)
	}
	e.username = "renamed@example.test"
	e.compose()
	e.compose()
	if want = strings.Replace(want, "user@example.test", "renamed@example.test", 1); e.contents != want {
		t.Fatalf("repeated compose lost custom field positions: %q", e.contents)
	}
	if e.fields.rows[0] != first || e.fields.rows[1] != second {
		t.Fatal("compose replaced custom row identity")
	}
}

func TestEditorCustomFieldsRemainEditableAfterRemovingKnownField(t *testing.T) {
	e := editorState{contents: "secret\nusername: user\nfirst: one\nsecond: two\n"}
	e.readFields()
	e.username = ""
	e.fields.rows[0].deleted = true
	e.compose()
	e.fields.rows[1].value = "updated"
	e.compose()
	if e.contents != "secret\nsecond: updated\n" {
		t.Fatalf("shifted custom field overwrote another line: %q", e.contents)
	}
}

func TestEditorEmptyKnownFieldsAndNotesSurvivePasswordEdit(t *testing.T) {
	raw := "secret\nusername:\nurl: \nnotes: keep\n"
	e := editorState{contents: raw}
	e.readFields()
	e.password = "updated"
	e.compose()
	if e.contents != strings.Replace(raw, "secret", "updated", 1) {
		t.Fatalf("empty fields were removed by an unrelated edit: %q", e.contents)
	}
}

func TestEditorRejectsIncompleteCustomFields(t *testing.T) {
	e := editorState{}
	e.readFields()
	e.fields.add("", "orphan value", -1)
	if e.customFieldsValid() || !e.dirty() {
		t.Fatal("an unnamed value can be silently discarded")
	}
	row := e.fields.rows[0]
	row.key = "invalid:key"
	if e.customFieldsValid() {
		t.Fatal("a key containing a field delimiter was accepted")
	}
	row.key = "token"
	row.value = "value: with colon"
	if !e.customFieldsValid() {
		t.Fatal("a colon in a custom value was rejected")
	}
}

func TestEditorExistingCustomRowCannotBeSilentlyCleared(t *testing.T) {
	a := fixture()
	a.detail.Contents = "secret\napi-key: old-value\n"
	a.editEntry()
	row := a.editor.fields.rows[0]
	row.key, row.value = "", ""
	a.editor.compose()
	if a.editor.customFieldsValid() || !a.editor.dirty() {
		t.Fatal("clearing a persisted row was accepted without using its delete action")
	}
	a.saveEntry()
	if a.errorText == "" || a.busy {
		t.Fatal("save did not reject the empty existing row")
	}
	row.deleted = true
	a.editor.compose()
	if !a.editor.customFieldsValid() || strings.Contains(a.editor.contents, "old-value") {
		t.Fatal("the explicit delete action did not remove the persisted row")
	}
}

func TestEditorInvalidCustomDraftCannotSwitchToRawMode(t *testing.T) {
	for _, viewport := range []struct {
		name, language string
		width, height  int
	}{
		{"default-zh", "zh-CN", 1120, 760},
		{"narrow-zh", "zh-CN", 860, 620},
		{"narrow-en", "en", 860, 620},
	} {
		for _, invalid := range []string{"key", "value", "cleared"} {
			t.Run(viewport.name+"/"+invalid, func(t *testing.T) {
				a := fixture()
				a.state.Settings.Language = viewport.language
				a.detail.Contents = "secret\napi-key: old-value\n"
				a.editEntry()
				row := a.editor.fields.rows[0]
				switch invalid {
				case "key":
					row.key, row.value = "api key", "new-value"
				case "value":
					row.value = "new\nvalue"
				case "cleared":
					row.key, row.value = "", ""
				}
				a.editor.compose()
				key, value, source := row.key, row.value, a.editor.contents
				view := ui.NewTester(testView(a), viewport.width, viewport.height)
				keyLabel, valueLabel := a.t("自定义字段名称 %d", row.id), a.t("自定义字段值 %d", row.id)
				for _, label := range []string{keyLabel, valueLabel, a.t("原文模式"), a.t("保存草稿"), a.t("取消"), a.t("保存")} {
					box, ok := view.Find(label)
					if !ok || box.X < defaultSidebarWidth || box.X+box.W > float32(viewport.width) || box.Y+box.H > float32(viewport.height) {
						t.Fatalf("editor control %q is outside the detail pane: %v", label, box)
					}
				}
				if !view.HasText(a.editorFieldValidationMessage()) {
					t.Fatal("invalid custom fields have no inline validation reason")
				}
				if err := view.Click(a.t("原文模式")); err != nil {
					t.Fatal(err)
				}
				if a.editor.showRaw || a.editor.customFieldsValid() || row.key != key || row.value != value || a.editor.contents != source {
					t.Fatal("switching modes discarded or bypassed the invalid custom draft")
				}
				if view.HasText(a.t("操作未完成")) {
					t.Fatal("switching an invalid form displayed a toast over the editor actions")
				}
				for _, repair := range []struct{ label, value string }{
					{keyLabel, "api-key"},
					{valueLabel, "corrected-value"},
				} {
					if err := view.Click(repair.label); err != nil {
						t.Fatal(err)
					}
					view.Key(ui.Cmd, ui.KeyA)
					view.Type(repair.value)
				}
				if a.editor.contents != "secret\napi-key: corrected-value\n" {
					t.Fatal("repairing an invalid draft left stale source fragments")
				}
				view.Frame()
				if err := view.Click(a.t("原文模式")); err != nil {
					t.Fatal(err)
				}
				if !a.editor.showRaw {
					t.Fatal("a repaired draft could not enter raw mode")
				}
			})
		}
	}
}

func TestEditorReservedAliasesCannotShadowDedicatedFields(t *testing.T) {
	for _, key := range []string{"username", "USER", "login", "email", "url", "uri", "website", "site", "link"} {
		for _, existing := range []bool{false, true} {
			e := editorState{contents: "secret\napi-key: old-value\nusername: actual-user\nurl: https://actual.example\n"}
			e.readFields()
			row := e.fields.rows[0]
			if !existing {
				e.fields.add("", "", -1)
				row = e.fields.rows[1]
			}
			row.key, row.value = key, "shadow-value"
			e.compose()
			if e.customFieldsValid() || strings.Contains(e.contents, "shadow-value") {
				t.Fatalf("new or renamed alias %q could shadow a dedicated field (existing=%v)", key, existing)
			}
		}
	}
	for _, key := range []string{"username", "user", "login", "email", "url", "uri", "website", "site", "link"} {
		raw := "secret\nusername: actual-user\nurl: https://actual.example\n" + key + ": old-secondary\n"
		e := editorState{contents: raw}
		e.readFields()
		row := e.fields.rows[0]
		row.value = "updated-secondary"
		e.compose()
		if !e.customFieldsValid() || e.contents != strings.Replace(raw, "old-secondary", "updated-secondary", 1) {
			t.Fatalf("an existing secondary alias %q could not retain its key and edit its value", key)
		}
	}
}

func TestEditorCustomFieldKeysFollowDocumentGrammar(t *testing.T) {
	for _, key := range []string{"api token", "foo.bar", "note", "NOTES", "token/key"} {
		e := editorState{}
		e.readFields()
		e.fields.add(key, "sensitive-value", -1)
		e.compose()
		if e.customFieldsValid() || strings.Contains(e.contents, "sensitive-value") {
			t.Fatalf("invalid field %q could be saved as visible notes", key)
		}
	}
	for _, key := range []string{"api-token", "account_id", "字段1"} {
		e := editorState{}
		e.readFields()
		e.fields.add(key, "sensitive-value", -1)
		e.compose()
		d := vault.ParseDocument("test", e.contents, "")
		if !e.customFieldsValid() || len(d.Fields) != 1 || d.Fields[0].Key != key || d.Notes != "" {
			t.Fatalf("valid field %q was not parsed as a field: %#v", key, d)
		}
	}
	raw := "secret\napi token: existing value\n"
	e := editorState{contents: raw}
	e.readFields()
	e.password = "updated"
	e.compose()
	if !e.customFieldsValid() || e.contents != strings.Replace(raw, "secret", "updated", 1) {
		t.Fatal("an unrelated edit changed an existing nonstandard line")
	}
	e.fields.rows[0].value = "new value"
	e.compose()
	if e.customFieldsValid() || strings.Contains(e.contents, "new value") {
		t.Fatal("editing an existing nonstandard line did not require repairing its key")
	}
}

func TestEditorCustomKeyRenamePreservesValueWhitespace(t *testing.T) {
	raw := "secret\r\napi-key:   sensitive value  \r\n"
	e := editorState{contents: raw}
	e.readFields()
	e.fields.rows[0].key = "token"
	e.compose()
	if want := strings.Replace(raw, "api-key", "token", 1); e.contents != want {
		t.Fatalf("key rename changed value whitespace: got %q want %q", e.contents, want)
	}
}

func TestEditorPasswordControlsAndDraftStatusKeepGeometry(t *testing.T) {
	a := fixture()
	a.editEntry()
	view := ui.NewTester(testView(a), 1120, 760)
	if view.HasText("备注") {
		t.Fatal("the form still exposes a notes editor")
	}
	before, ok := view.Find("账号名称")
	if !ok {
		t.Fatal("entry name input missing")
	}
	if err := view.Click("显示或隐藏编辑密码"); err != nil {
		t.Fatal(err)
	}
	if !a.editor.revealPassword {
		t.Fatal("password reveal did not toggle")
	}
	if err := view.Click("账号名称"); err != nil {
		t.Fatal(err)
	}
	for _, saved := range []bool{true, false, true} {
		a.editor.draftSaved = saved
		a.busy, a.backgroundBusy, a.activeRequestAction = !saved, !saved, "draft"
		view.Frame()
		if after, _ := view.Find("账号名称"); after != before || !view.Focused("账号名称") {
			t.Fatalf("draft state moved the editor or dropped input focus: before=%v after=%v", before, after)
		}
	}
	view.Type("-edited")
	if !strings.Contains(a.editor.name, "-edited") {
		t.Fatal("draft save disabled typing")
	}
}

func TestEditorDraftResponseDoesNotMarkLaterInputSaved(t *testing.T) {
	a := fixture()
	a.editEntry()
	callbacks := make(chan func(), 1)
	a.runOnMain = func(f func()) { callbacks <- f }
	state := a.state
	a.service = backendFunc(func(app.Request) app.Response {
		state.ID = "encrypted-draft"
		return app.Response{Status: "ok", Data: state}
	})
	a.saveDraft()
	if a.editorBusy() || !a.backgroundBusy {
		t.Fatal("draft save disabled the editor")
	}
	a.editor.password = "typed-during-save"
	a.editor.compose()
	awaitMain(t, callbacks)
	if a.editor.draftSaved || a.editor.draftAttempted || a.editor.password != "typed-during-save" || a.editor.draftID != "encrypted-draft" {
		t.Fatal("a delayed draft save replaced or marked newer input saved")
	}
}

func TestEditorDraftResponseCannotAlterReplacementEditor(t *testing.T) {
	a := fixture()
	a.editEntry()
	callbacks := make(chan func(), 1)
	a.runOnMain = func(f func()) { callbacks <- f }
	state := a.state
	a.service = backendFunc(func(app.Request) app.Response {
		state.ID = "old-draft"
		return app.Response{Status: "ok", Data: state}
	})
	a.saveDraft()
	a.editor = editorState{open: true, contents: "replacement"}
	a.editor.readFields()
	awaitMain(t, callbacks)
	if a.editor.draftID != "" || a.editor.password != "replacement" {
		t.Fatal("the old draft response modified a replacement editor")
	}
}

func TestEditorOpensGeneratorAndAppliesConfiguredPassword(t *testing.T) {
	a := fixture()
	a.editEntry()
	before := a.editor.password
	callbacks := make(chan func(), 2)
	requests := make(chan app.Request, 2)
	a.runOnMain = func(f func()) { callbacks <- f }
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		state.Password = strings.Repeat("a", request.Length)
		return app.Response{Status: "ok", Data: state}
	})
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("生成并填入密码"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	view.Frame()
	<-requests
	if !a.generator.open || !view.HasText("密码生成器") || a.editor.password != before {
		t.Fatal("the generate button did not open a separate generator")
	}
	a.generator.length = 32
	a.generator.lower, a.generator.upper, a.generator.numbers, a.generator.symbols = true, false, false, false
	view.Frame()
	if err := view.Click("重新生成密码"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	view.Frame()
	request := <-requests
	if request.Length != 32 || !request.Lowercase || request.Uppercase || request.Numbers || request.Symbols {
		t.Fatalf("generator ignored options: %#v", request)
	}
	if err := view.Click("使用此密码"); err != nil {
		t.Fatal(err)
	}
	if a.generator.open || a.generator.password != "" || a.editor.password != strings.Repeat("a", 32) || !strings.HasPrefix(a.editor.contents, a.editor.password+"\n") {
		t.Fatal("applying the generator did not update the edited password")
	}
}

func TestCancelledGeneratorCannotApplyLatePassword(t *testing.T) {
	a := fixture()
	a.editEntry()
	before := a.editor.password
	callbacks := make(chan func(), 1)
	a.runOnMain = func(f func()) { callbacks <- f }
	state := a.state
	a.service = backendFunc(func(app.Request) app.Response {
		state.Password = "late-password"
		return app.Response{Status: "ok", Data: state}
	})
	a.openGenerator()
	a.closeGenerator()
	awaitMain(t, callbacks)
	if a.generator.password != "" || a.editor.password != before {
		t.Fatal("a cancelled generator revived its password")
	}
}
