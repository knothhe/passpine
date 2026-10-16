package desktop

import (
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestNavigationBackExitsEntirePreferencesArea(t *testing.T) {
	a := fixture()
	a.query = "github"
	a.settingsState.Category = "general"
	before := a.currentLocation()
	a.navigate("settings", "")
	a.navigateSettingsCategory("security")
	a.navigate("recovery", "")
	a.recoveryKind = 1
	a.navigate("activity", "")
	a.navigateSettingsCategory("browser")
	a.settingsState.Query = "Firefox"
	preferences := a.currentLocation()
	if len(a.history.back) != 1 {
		t.Fatalf("internal preferences navigation added history entries: %#v", a.history)
	}
	a.moveHistory(-1)
	if a.currentLocation() != before || a.selectedIndex != 0 || len(a.filtered) != 1 {
		t.Fatal("back did not restore the manager search and selected row")
	}
	a.moveHistory(1)
	if a.currentLocation() != preferences {
		t.Fatal("forward did not restore the last settings category, search and recovery tab")
	}
	a.moveHistory(-1)
	if a.currentLocation() != before || len(a.history.back) != 0 {
		t.Fatal("returning from the restored preferences destination took more than one step")
	}
}

func TestPreferencesInternalNavigationPreservesForwardHistory(t *testing.T) {
	a := fixture()
	a.navigate("settings", "")
	a.navigate("entries", "favorites")
	manager := a.currentLocation()
	a.moveHistory(-1)
	a.navigateSettingsCategory("security")
	a.navigate("activity", "")
	if len(a.history.back) != 1 || len(a.history.forward) != 1 {
		t.Fatal("changing preferences contents altered manager history")
	}
	a.moveHistory(1)
	if a.currentLocation() != manager {
		t.Fatal("changing preferences contents lost the next manager destination")
	}
	a.moveHistory(-1)
	if a.page != "activity" || a.settingsState.Category != "security" {
		t.Fatal("back did not restore the latest preferences contents")
	}
	a.navigate("entries", "all")
	if len(a.history.forward) != 0 {
		t.Fatal("leaving preferences through a new manager destination retained stale forward history")
	}
}

func TestNavigationRetainsNewAndExistingInlineDrafts(t *testing.T) {
	for _, newEntry := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing", true: "new"}[newEntry], func(t *testing.T) {
			a := fixture()
			if newEntry {
				a.newEntry()
			} else {
				a.editEntry()
			}
			a.editor.password = "retained-draft-secret"
			a.editor.compose()
			before := a.editor
			a.navigate("settings", "")
			a.navigateSettingsCategory("security")
			a.navigate("recovery", "")
			a.navigate("activity", "")
			a.navigateSettingsCategory("browser")
			a.moveHistory(-1)
			view := ui.NewTester(testView(a), 1120, 760)
			if a.page != "entries" || a.discardOpen || a.editor != before {
				t.Fatal("returning to the retained draft required discarding or changed it")
			}
			if _, ok := view.Find("账号名称"); !ok {
				t.Fatal("the retained inline editor is not visible")
			}
			a.moveHistory(1)
			if a.page != "settings" || a.settingsState.Category != "browser" || a.editor != before {
				t.Fatal("forward navigation discarded the retained draft")
			}
		})
	}
}

func TestNavigationToAnotherEntryWaitsForDraftDiscard(t *testing.T) {
	a := fixture()
	a.history.back = []navigationLocation{{page: "entries", selected: "工作/Figma", vaultID: a.state.VaultID}}
	a.editEntry()
	a.editor.password = "unsaved-draft"
	a.editor.compose()
	a.moveHistory(-1)
	if !a.discardOpen || a.selected != "开发/GitHub" || len(a.history.back) != 1 {
		t.Fatal("history moved before the user decided what to do with the draft")
	}
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("继续编辑"); err != nil {
		t.Fatal(err)
	}
	if a.selected != "开发/GitHub" || !a.editor.open || a.editor.exit != nil {
		t.Fatal("cancelling discard changed selection or retained the pending navigation")
	}
	a.moveHistory(-1)
	view.Frame()
	if err := view.Click("放弃修改"); err != nil {
		t.Fatal(err)
	}
	if a.editor.open || a.selected != "工作/Figma" || len(a.history.back) != 0 || len(a.history.forward) != 1 {
		t.Fatal("confirmed discard did not navigate once to the previous entry")
	}
}

func TestNavigationNeverRestoresSelectionFromAnotherVault(t *testing.T) {
	a := fixture()
	a.state.VaultID = "current-vault"
	a.service = backendFunc(func(app.Request) app.Response {
		t.Error("history fetched a path from another vault")
		return app.Response{}
	})
	a.restoreLocation(navigationLocation{page: "entries", selected: "private/other", vaultID: "old-vault"})
	if a.selected != "" || a.detail.Entry != "" || a.detail.Contents != "" {
		t.Fatal("history retained credentials or selection from another vault")
	}
}

func TestNavigationDetailReadPreservesStructureAndHistory(t *testing.T) {
	a := fixture()
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		data := state
		data.Entry, data.Password = request.Entry, "loaded-password"
		return app.Response{Status: "ok", Data: data}
	})
	a.history.back = []navigationLocation{{page: "entries", selected: "工作/Figma", vaultID: state.VaultID}}
	a.moveHistory(-1)
	transition := a.detailTransitionPresentation(time.Now())
	if !transition.Pending || len(transition.Fields) != 3 || !transition.HasNotes || !transition.HasWebsite {
		t.Fatal("history navigation discarded the detail structure before the next read")
	}
	if a.detail.Entry != "" || a.detail.Password != "" || a.detail.Contents != "" {
		t.Fatal("history navigation retained credentials from the previous entry")
	}
	awaitMain(t, callbacks)
	if a.detail.Entry != "工作/Figma" || len(a.history.back) != 0 || len(a.history.forward) != 1 {
		t.Fatal("history detail read did not finish at its destination without recording another step")
	}
}

func TestNavigationRejectsDetailFromAnEarlierVisitToTheSameEntry(t *testing.T) {
	a := fixture()
	callbacks := make(chan func(), 2)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	state, reads := a.state, 0
	a.service = backendFunc(func(request app.Request) app.Response {
		reads++
		data := state
		data.Entry, data.Password = request.Entry, "stale-history-password"
		if reads == 2 {
			data.Password = "latest-history-password"
		}
		return app.Response{Status: "ok", Data: data}
	})
	a.history.back = []navigationLocation{{page: "entries", selected: "工作/Figma", vaultID: state.VaultID}}
	a.moveHistory(-1)
	a.moveHistory(1)
	a.moveHistory(-1)
	awaitMain(t, callbacks)
	if a.detail.Entry != "" || !a.busy || a.selected != "工作/Figma" {
		t.Fatal("an earlier visit repopulated details before the current history read completed")
	}
	awaitMain(t, callbacks)
	if a.detail.Entry != "工作/Figma" || a.detail.Password != "latest-history-password" || len(a.history.back) != 0 || len(a.history.forward) != 1 {
		t.Fatal("the latest history read did not replace the stale visit exactly once")
	}
}

func TestNativeWindowCloseDismissesEditorAndGeneratorFirst(t *testing.T) {
	a := fixture()
	a.editEntry()
	a.generator.open = true
	event := &mygo.CloseEvent{}
	a.handleMainWindowClose(event)
	if !event.DefaultPrevented() || a.generator.open || !a.editor.open || a.closing {
		t.Fatal("native close did not dismiss the generator before the editor")
	}
	event = &mygo.CloseEvent{}
	a.handleMainWindowClose(event)
	if !event.DefaultPrevented() || a.editor.open || a.closing || a.detail.Entry != "开发/GitHub" {
		t.Fatal("native close dismissed the application instead of the clean editor")
	}
	event = &mygo.CloseEvent{}
	a.handleMainWindowClose(event)
	if event.DefaultPrevented() {
		t.Fatal("normal window close remained blocked after the editor was dismissed")
	}
	a.editEntry()
	a.editor.password = "changed"
	a.editor.compose()
	event = &mygo.CloseEvent{}
	a.handleMainWindowClose(event)
	if !event.DefaultPrevented() || !a.discardOpen || !a.editor.open || a.closing {
		t.Fatal("native close lost a dirty draft or closed the application")
	}
}
