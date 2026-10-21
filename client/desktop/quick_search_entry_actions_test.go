package desktop

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestQuickSearchEntryActionsTargetTheirOwnEntryWithoutCopyingTheRow(t *testing.T) {
	for _, mode := range []string{quickSearchGlobal, quickSearchTray} {
		for _, action := range []struct{ label, action, field string }{
			{"复制用户名 GitHub", "copy", "username"},
			{"复制密码 GitHub", "copy", "password"},
			{"查看条目 GitHub", "fetch", ""},
			{"开发/GitHub", "copy", "password"},
		} {
			t.Run(quickSearchActionModeName(mode)+"/"+action.label, func(t *testing.T) {
				a, window := quickSearchFixture()
				a.quick.mode = mode
				a.detail, a.selected = app.Data{}, ""
				data := a.state
				callbacks, requests := make(chan func(), 3), make(chan app.Request, 3)
				a.runOnMain = func(callback func()) { callbacks <- callback }
				a.service = backendFunc(func(request app.Request) app.Response {
					requests <- request
					response := data
					if request.Action == "fetch" {
						response.Entry, response.Contents = request.Entry, "manager document"
					}
					return app.Response{Status: "ok", Data: response}
				})
				view := quickSearchActionsView(a, mode)
				if a.quick.results[a.quick.selected].Path == "开发/GitHub" {
					t.Fatal("test must activate an entry other than the current selection")
				}
				if err := view.Click(action.label); err != nil {
					t.Fatal(err)
				}
				awaitMain(t, callbacks)
				request := <-requests
				if request.Action != action.action || request.Field != action.field || request.Entry != "开发/GitHub" || request.VaultID != a.state.VaultID {
					t.Fatalf("entry action targeted the selection or invoked a row copy: %+v", request)
				}
				if len(requests) != 0 || len(callbacks) != 0 || a.quick.visible || window.hidden != 1 {
					t.Fatal("entry child action bubbled or failed to dismiss the panel")
				}
				if action.action == "fetch" && (a.detail.Entry != "开发/GitHub" || a.page != "entries" || a.selected != "开发/GitHub") {
					t.Fatal("entry details did not open the selected account in the manager")
				}
			})
		}
	}
}

func TestQuickSearchEntryActionsResumeTheirCapturedIntentAfterUnlock(t *testing.T) {
	for _, mode := range []string{quickSearchGlobal, quickSearchTray} {
		for _, action := range []struct{ label, field, title, request string }{
			{"复制用户名 GitHub", "username", "解锁以复制用户名", "copy"},
			{"复制密码 GitHub", "password", "解锁以复制密码", "copy"},
			{"查看条目 GitHub", "details", "解锁以查看条目", "fetch"},
		} {
			t.Run(quickSearchActionModeName(mode)+"/"+action.field, func(t *testing.T) {
				a, _ := quickSearchFixture()
				a.quick.mode = mode
				a.state.Locked, a.state.Settings.BrowseWhileLocked = true, true
				a.detail, a.selected = app.Data{}, ""
				data := a.state
				data.Locked = false
				callbacks, requests := make(chan func(), 3), make(chan app.Request, 3)
				a.runOnMain = func(callback func()) { callbacks <- callback }
				a.service = backendFunc(func(request app.Request) app.Response {
					requests <- request
					response := data
					if request.Action == "fetch" {
						response.Entry, response.Contents = request.Entry, "manager document"
					}
					return app.Response{Status: "ok", Data: response}
				})
				view := quickSearchActionsView(a, mode)
				if err := view.Click(action.label); err != nil {
					t.Fatal(err)
				}
				if !view.HasText(action.title) || a.quick.pendingCopy.field != action.field || a.quick.pendingCopy.entry != "开发/GitHub" || len(requests) != 0 {
					t.Fatal("locked entry action did not defer its exact intent to authentication")
				}
				a.quick.passphrase = "private"
				a.unlockQuickSearch("")
				a.quick.selected = 0
				awaitMain(t, callbacks)
				awaitMain(t, callbacks)
				unlock, request := <-requests, <-requests
				if unlock.Action != "unlock" || request.Action != action.request || request.Entry != "开发/GitHub" {
					t.Fatalf("unlock lost the captured entry action: %+v, %+v", unlock, request)
				}
				if request.Action == "copy" && request.Field != action.field {
					t.Fatalf("unlock copied the wrong credential field: %+v", request)
				}
				if a.quick.visible || a.quick.pendingCopy.entry != "" || a.quick.passphrase != "" || len(requests) != 0 {
					t.Fatal("unlock repeated the row action or retained the panel continuation")
				}
			})
		}
	}
}

func TestQuickSearchEntryDetailsKeepsUnsavedManagerEditorUntilDiscarded(t *testing.T) {
	a, _ := quickSearchFixture()
	a.editor = editorState{open: true, path: "work/draft", contents: "unsaved document"}
	a.detail = app.Data{}
	data := a.state
	callbacks, requests := make(chan func(), 2), make(chan app.Request, 2)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		response := data
		response.Entry = request.Entry
		return app.Response{Status: "ok", Data: response}
	})
	view := quickSearchActionsView(a, quickSearchGlobal)
	if err := view.Click("查看条目 GitHub"); err != nil {
		t.Fatal(err)
	}
	if a.quick.visible || !a.discardOpen || !a.editor.open || a.editor.contents != "unsaved document" || len(requests) != 0 {
		t.Fatal("entry details discarded a manager draft or read before its close decision")
	}
	manager := ui.NewTester(testView(a), 1120, 760)
	if err := manager.Click("继续编辑"); err != nil {
		t.Fatal(err)
	}
	if !a.editor.open || a.editor.contents != "unsaved document" || a.discardOpen || len(requests) != 0 {
		t.Fatal("canceling the detail navigation did not preserve the draft")
	}
}

func TestQuickSearchEntryActionsAreLocalizedAndStayInsideCompactPanels(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		for _, scale := range []int{80, 120} {
			t.Run(fmt.Sprintf("%s/%d", language, scale), func(t *testing.T) {
				a, _ := quickSearchFixture()
				a.quick.mode = quickSearchTray
				a.state.Settings.Language, a.state.Settings.FontScale = language, scale
				a.state.Entries = []app.Entry{{Path: "Long account with a long name", Name: "Long account with a long name"}}
				view := quickSearchActionsView(a, quickSearchTray)
				assertTypographyBounds(t, view, 420, 440, a.state.VaultName,
					a.t("复制用户名 %s", a.state.Entries[0].Name), a.t("复制密码 %s", a.state.Entries[0].Name), a.t("查看条目 %s", a.state.Entries[0].Name))
				if view.HasText("更多") || view.HasText("More actions") {
					t.Fatal("compact entry actions expose a more menu")
				}
			})
		}
	}
}

func TestQuickSearchEntryKeyboardActivationHonorsFocusedBodyAndButtons(t *testing.T) {
	for _, mode := range []string{quickSearchGlobal, quickSearchTray} {
		for _, key := range []ui.Key{ui.KeyEnter, ui.KeySpace} {
			for _, action := range []struct{ label, field string }{
				{"开发/GitHub", "password"},
				{"复制用户名 GitHub", "username"},
				{"复制密码 GitHub", "password"},
				{"查看条目 GitHub", "details"},
			} {
				t.Run(fmt.Sprintf("%s/%v/%s", quickSearchActionModeName(mode), key, action.label), func(t *testing.T) {
					a, _ := quickSearchFixture()
					a.quick.mode = mode
					a.detail, a.selected = app.Data{}, ""
					data := a.state
					callbacks, requests := make(chan func(), 3), make(chan app.Request, 3)
					a.runOnMain = func(callback func()) { callbacks <- callback }
					a.service = backendFunc(func(request app.Request) app.Response {
						requests <- request
						response := data
						response.Entry = request.Entry
						return app.Response{Status: "ok", Data: response}
					})
					view := quickSearchActionsView(a, mode)
					focused := func() bool {
						// Tester.Focused also accepts a focused ancestor. Exclude
						// collection focus, which would contain every entry.
						return view.Focused(action.label) && !view.Focused("工作/Figma")
					}
					for range 40 {
						if focused() {
							break
						}
						view.Key(0, ui.KeyTab)
					}
					if !focused() {
						t.Fatal("entry body or action is not reachable with Tab")
					}
					view.Key(0, key)
					awaitMain(t, callbacks)
					request := <-requests
					wantAction, wantField := "copy", action.field
					if action.field == "details" {
						wantAction, wantField = "fetch", ""
					}
					if request.Action != wantAction || request.Entry != "开发/GitHub" || request.Field != wantField || len(requests) != 0 {
						t.Fatalf("focused keyboard action fell through to list submission: %+v", request)
					}
				})
			}
		}
	}
}
