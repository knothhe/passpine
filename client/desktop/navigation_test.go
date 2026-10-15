package desktop

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine-neo/client/app"
)

func TestNavigationBackForwardRestoresManagerAndSettings(t *testing.T) {
	a := fixture()
	a.query = "github"
	a.settingsState.Category = "general"
	before := a.currentLocation()
	a.navigate("settings", "")
	a.navigateSettingsCategory("security")
	a.navigate("recovery", "")
	if len(a.history.back) != 3 {
		t.Fatalf("navigation destinations were not recorded: %#v", a.history)
	}
	a.moveHistory(-1)
	if a.page != "settings" || a.settingsState.Category != "security" {
		t.Fatal("back did not return from recovery to the selected settings category")
	}
	a.moveHistory(-1)
	if a.page != "settings" || a.settingsState.Category != "general" {
		t.Fatal("back did not restore the previous category")
	}
	a.moveHistory(-1)
	if a.currentLocation() != before || a.selectedIndex != 0 || len(a.filtered) != 1 {
		t.Fatal("back did not restore the manager search and selected row")
	}
	a.moveHistory(1)
	if a.page != "settings" || a.settingsState.Category != "general" {
		t.Fatal("forward did not restore the settings page")
	}
	a.navigateSettingsCategory("browser")
	if len(a.history.forward) != 0 {
		t.Fatal("a new navigation branch retained stale forward destinations")
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
			a.moveHistory(-1)
			view := ui.NewTester(testView(a), 1120, 760)
			if a.page != "entries" || a.discardOpen || a.editor != before {
				t.Fatal("returning to the retained draft required discarding or changed it")
			}
			if _, ok := view.Find("账号名称"); !ok {
				t.Fatal("the retained inline editor is not visible")
			}
			a.moveHistory(1)
			if a.page != "settings" || a.editor != before {
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
