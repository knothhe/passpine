package desktop

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestInlineEditorUsesDetailPaneAndKeepsSearchAvailable(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("编辑条目"); err != nil {
		t.Fatal(err)
	}
	input, ok := view.Find("账号名称")
	if !ok || input.X < defaultSidebarWidth || input.X+input.W > 1120 {
		t.Fatalf("entry form is outside the right detail pane: %v", input)
	}
	contents := a.editor.contents
	if err := view.Click("搜索条目"); err != nil {
		t.Fatal(err)
	}
	view.Type("cloud")
	if a.query != "cloud" || !a.editor.open || a.editor.contents != contents {
		t.Fatal("editing blocked sidebar search or changed the current draft")
	}
}

func TestInlineEditorCloseReturnsToDetailWithoutClosingApplication(t *testing.T) {
	a := fixture()
	a.editEntry()
	view := ui.NewTester(testView(a), 1120, 760)
	before := a.detail
	if err := view.Click("关闭编辑器"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		view.Frame()
	}
	if a.editor.open || a.closing || a.generator.open || a.selected != before.Entry || a.detail.Contents != before.Contents {
		t.Fatal("closing an inline form altered the window or selected detail")
	}
	if _, ok := view.Find("显示或隐藏密码"); !ok {
		t.Fatal("closing the form did not return to the password detail")
	}
}

func TestInlineEditorDirtyCloseRequiresExplicitDiscard(t *testing.T) {
	a := fixture()
	a.editEntry()
	a.editor.password = "unsaved-inline-password"
	a.editor.compose()
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("关闭编辑器"); err != nil {
		t.Fatal(err)
	}
	if !a.editor.open || !a.discardOpen || a.closing {
		t.Fatal("closing a dirty form lost its contents or closed the application")
	}
	if err := view.Click("继续编辑"); err != nil {
		t.Fatal(err)
	}
	if !a.editor.open || a.discardOpen || a.editor.password != "unsaved-inline-password" {
		t.Fatal("continuing an inline edit discarded its draft")
	}
	if err := view.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("放弃修改"); err != nil {
		t.Fatal(err)
	}
	if a.editor.open || a.discardOpen || a.closing || a.detail.Password != "fake-Secret-42!" {
		t.Fatal("discard did not return to the original detail")
	}
}

func TestInlineEditorCloseIntentRunsOnlyAfterConfirmedDiscard(t *testing.T) {
	a := fixture()
	a.editEntry()
	a.editor.password = "unsaved"
	a.editor.compose()
	called := ""
	a.requestEditorClose(func() { called = "cancelled-navigation" })
	view := ui.NewTester(testView(a), 1120, 760)
	if called != "" {
		t.Fatal("navigation ran before discard was confirmed")
	}
	if err := view.Click("继续编辑"); err != nil {
		t.Fatal(err)
	}
	if a.editor.exit != nil || called != "" {
		t.Fatal("cancelled navigation remained queued")
	}
	a.cancelEditor()
	view.Frame()
	if err := view.Click("放弃修改"); err != nil {
		t.Fatal(err)
	}
	if called != "" || a.editor.open {
		t.Fatal("closing later replayed cancelled navigation")
	}
	a.editEntry()
	a.editor.password = "another-unsaved-password"
	a.editor.compose()
	a.requestEditorClose(func() { called = "first-navigation" })
	a.requestEditorClose(func() { called = "latest-navigation" })
	view.Frame()
	if err := view.Click("放弃修改"); err != nil {
		t.Fatal(err)
	}
	if called != "latest-navigation" || a.editor.open {
		t.Fatal("confirmed discard did not execute the latest navigation once")
	}
}

func TestInlineEditorSaveReturnsUpdatedDetail(t *testing.T) {
	a := fixture()
	a.editEntry()
	callbacks := make(chan func(), 1)
	requests := make(chan app.Request, 1)
	a.runOnMain = func(f func()) { callbacks <- f }
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		data := state
		data.Entry, data.Contents, data.Password = request.Destination, request.Contents, "updated-inline-password"
		return app.Response{Status: "ok", Data: data}
	})
	a.editor.setPath("开发", "renamed")
	a.editor.password = "updated-inline-password"
	a.editor.compose()
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("保存"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	view.Frame()
	request := <-requests
	if request.Action != "save" || request.Mode != "edit" || request.Entry != "开发/GitHub" || request.Destination != "开发/renamed" {
		t.Fatalf("inline editing saved the wrong entry: %#v", request)
	}
	if !strings.HasPrefix(request.Contents, "updated-inline-password\n") || !strings.Contains(request.Contents, "notes: ") {
		t.Fatal("inline editing changed unrelated entry content")
	}
	if a.editor.open || a.generator.open || a.detail.Entry != "开发/renamed" || a.selected != a.detail.Entry || a.reveal {
		t.Fatal("saving did not return to the updated masked detail")
	}
}

func TestInlineEditorLateSavePreservesNewNavigation(t *testing.T) {
	for _, page := range []string{"settings", "activity", "recovery"} {
		t.Run(page, func(t *testing.T) {
			a := fixture()
			a.editEntry()
			a.editor.setPath("work", "renamed")
			a.editor.password = "saved-after-navigation"
			a.editor.compose()
			callbacks := make(chan func(), 4)
			a.runOnMain = func(f func()) { callbacks <- f }
			state := a.state
			a.service = backendFunc(func(request app.Request) app.Response {
				data := state
				if request.Action == "save" {
					data.Entry, data.Contents, data.Password = request.Destination, request.Contents, "saved-after-navigation"
				}
				return app.Response{Status: "ok", Data: data}
			})
			a.saveEntry()
			completeSave := nextGeneratorCallback(t, callbacks)
			a.navigate(page, "folder:work")
			revision := a.history.revision
			completeSave()
			if a.page != page || a.filter != "folder:work" || a.history.revision != revision {
				t.Fatal("a late save replaced the user's newer destination or filter")
			}
			if a.editor.open || a.detail.Entry != "work/renamed" || a.detail.Password != "saved-after-navigation" || a.selected != a.detail.Entry {
				t.Fatal("a late save did not finish its editor and update the saved detail")
			}
		})
	}
}

func TestInlineEditorLateSaveCannotCloseReplacementEditor(t *testing.T) {
	a := fixture()
	a.editEntry()
	a.editor.password = "first-editor-save"
	a.editor.compose()
	callbacks := make(chan func(), 1)
	a.runOnMain = func(f func()) { callbacks <- f }
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		data := state
		data.Entry, data.Contents = request.Destination, request.Contents
		return app.Response{Status: "ok", Data: data}
	})
	a.saveEntry()
	completeSave := nextGeneratorCallback(t, callbacks)
	a.editor = editorState{open: true, path: "other-entry", contents: "replacement-secret"}
	a.editor.readFields()
	identity := a.editor.identity
	completeSave()
	if !a.editor.open || a.editor.identity != identity || a.editor.contents != "replacement-secret" {
		t.Fatal("the first editor's save closed or changed its replacement")
	}
}

func TestInlineEditorLateSaveHistoryReturnsToSavedEntry(t *testing.T) {
	for _, newEntry := range []bool{false, true} {
		t.Run(map[bool]string{false: "rename", true: "new"}[newEntry], func(t *testing.T) {
			a := fixture()
			a.state.VaultID = "saved-vault"
			if newEntry {
				a.newEntry()
			} else {
				a.editEntry()
			}
			a.editor.setPath("work", "saved-entry")
			a.editor.password = "saved-before-history-back"
			a.editor.compose()
			callbacks := make(chan func(), 1)
			a.runOnMain = func(f func()) { callbacks <- f }
			state := a.state
			a.service = backendFunc(func(request app.Request) app.Response {
				data := state
				data.Entry = request.Destination
				if data.Entry == "" {
					data.Entry = request.Entry
				}
				data.Contents, data.Password = request.Contents, "saved-before-history-back"
				data.Entries = []app.Entry{{Path: data.Entry, Name: "saved-entry", Folder: "work"}}
				return app.Response{Status: "ok", Data: data}
			})
			a.saveEntry()
			completeSave := nextGeneratorCallback(t, callbacks)
			// The save has returned but its main-thread callback is still pending.
			// Keep this test's navigation free of an unrelated Git status request.
			a.service = nil
			a.navigate("settings", "")
			completeSave()
			if a.page != "settings" || a.editor.open || a.detail.Entry != "work/saved-entry" {
				t.Fatal("the pending save did not settle while keeping settings open")
			}
			a.moveHistory(-1)
			if a.page != "entries" || a.selected != "work/saved-entry" || a.detail.Entry != a.selected || a.detail.Password != "saved-before-history-back" || a.selectedIndex != 0 {
				t.Fatal("back navigation restored the old path or discarded the saved detail")
			}
			a.moveHistory(1)
			a.moveHistory(-1)
			if a.selected != "work/saved-entry" || a.detail.Entry != a.selected {
				t.Fatal("forward/back navigation revived the pre-save selection")
			}
		})
	}
}

func TestInlineEditorSavedHistoryMappingStaysWithinVaultAndIdentity(t *testing.T) {
	a := fixture()
	identity, otherIdentity := &editorIdentity{}, &editorIdentity{}
	a.history.revision = 17
	a.history.back = []navigationLocation{
		{page: "entries", vaultID: "current", selected: "old-path"},
		{page: "entries", vaultID: "current", selected: "previous-selection", editorID: identity},
		{page: "entries", vaultID: "other-vault", selected: "old-path", editorID: identity},
		{page: "settings", vaultID: "current", selected: "unrelated"},
	}
	a.history.forward = []navigationLocation{
		{page: "entries", vaultID: "current", selected: "old-path"},
		{page: "entries", vaultID: "current", selected: "other-entry", editorID: otherIdentity},
	}
	a.rebaseSavedEntryHistory(identity, "current", "old-path", "new-path")
	if a.history.back[0].selected != "new-path" || a.history.back[1].selected != "new-path" || a.history.back[1].editorID != nil || a.history.forward[0].selected != "new-path" {
		t.Fatal("saved editor positions or old path references were not mapped")
	}
	if a.history.back[2].selected != "old-path" || a.history.back[2].editorID != identity || a.history.back[3].selected != "unrelated" || a.history.forward[1].selected != "other-entry" || a.history.forward[1].editorID != otherIdentity || a.history.revision != 17 {
		t.Fatal("saved history mapping changed unrelated navigation metadata")
	}
}

func TestInlineNewEntryUsesDetailPaneAndCleanCancelKeepsSelection(t *testing.T) {
	a := fixture()
	a.newEntry()
	view := ui.NewTester(testView(a), 1120, 760)
	input, ok := view.Find("账号名称")
	if !ok || input.X < defaultSidebarWidth {
		t.Fatal("new entries do not use the inline form")
	}
	if err := view.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.editor.open || a.discardOpen || a.closing || a.detail.Entry != "开发/GitHub" {
		t.Fatal("cancelling an empty new entry closed or replaced the selected account")
	}
	a.newEntry()
	view.Frame()
	if err := view.Click("账号名称"); err != nil {
		t.Fatal(err)
	}
	view.Type("path/to/entry")
	if a.editor.path != "path/to/entry" {
		t.Fatal("the inline name field did not accept nested entry paths")
	}
}

func TestInlineEditorLockClearsFormAndGenerator(t *testing.T) {
	a := fixture()
	a.editEntry()
	a.editor.password = "private-inline-value"
	a.editor.compose()
	a.generator.open, a.generator.target, a.generator.password = true, a.editor.identity, "private-generator-value"
	a.state.Locked = true
	a.clearSecrets()
	view := ui.NewTester(testView(a), 1120, 760)
	if a.editor.open || a.editor.fields != nil || a.editor.contents != "" || a.generator.open || a.generator.password != "" {
		t.Fatal("locking retained decrypted inline editor state")
	}
	if view.HasText("private-inline-value") || view.HasText("private-generator-value") {
		t.Fatal("locking left a secret visible in the detail pane")
	}
}

func TestInlineCustomFieldsScrollWhileActionsStayVisible(t *testing.T) {
	a := fixture()
	a.editEntry()
	for _, key := range []string{"first", "second", "third"} {
		a.editor.fields.add(key, "original", -1)
	}
	a.editor.compose()
	view := ui.NewTester(testView(a), 860, 620)
	before, ok := view.Find("保存")
	if !ok || before.Y+before.H > 620 {
		t.Fatal("inline form actions are outside the narrow viewport")
	}
	view.Scroll(600, 460, 0, 400)
	if err := view.Click("自定义字段值 3"); err != nil {
		t.Fatal(err)
	}
	view.Key(ui.Cmd, ui.KeyA)
	view.Type("updated")
	if a.editor.fields.rows[2].value != "updated" || !strings.Contains(a.editor.contents, "third: updated") {
		t.Fatal("a custom field below the narrow fold could not be edited")
	}
	if after, _ := view.Find("保存"); after != before {
		t.Fatal("scrolling the form moved the save action")
	}
}

func TestInlineEditorRender(t *testing.T) {
	directory := os.Getenv("PASSPINE_INLINE_EDITOR_SCREENSHOTS")
	if directory == "" {
		t.Skip("set PASSPINE_INLINE_EDITOR_SCREENSHOTS to export inline editor previews")
	}
	for _, sample := range []struct {
		name, language      string
		width, height       int
		dark, newEntry, raw bool
		custom              bool
	}{
		{"edit-en-dark", "en", 1120, 760, true, false, false, false},
		{"edit-zh", "zh-CN", 1120, 760, false, false, false, false},
		{"new-en", "en", 1120, 760, false, true, false, false},
		{"custom-en-narrow", "en", 860, 620, false, false, false, true},
		{"raw-en", "en", 1120, 760, false, false, true, false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			a := fixture()
			a.state.Settings.Language = sample.language
			if sample.newEntry {
				a.newEntry()
			} else {
				a.editEntry()
			}
			a.editor.showRaw = sample.raw
			if sample.custom {
				a.editor.fields.add("account-id", "pine-account-42", -1)
				a.editor.fields.add("api-token", "fake-token-value", -1)
				a.editor.compose()
			}
			view := ui.NewTester(testView(a), sample.width, sample.height)
			view.SetDark(sample.dark)
			view.Frame()
			if err := os.MkdirAll(directory, 0755); err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(filepath.Join(directory, sample.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := png.Encode(file, view.Image()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
