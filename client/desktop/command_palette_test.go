package desktop

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestCommandPaletteKeyboardSearchAndExecution(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 1120, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	if err := view.Click("搜索条目"); err != nil {
		t.Fatal(err)
	}
	view.Type("Git")
	view.Key(ui.Ctrl|ui.Shift, ui.KeyP)
	view.Advance(200 * time.Millisecond)
	if !a.palette.open || !view.Focused("搜索命令") {
		t.Fatalf("shortcut did not open and focus the command palette: open=%v blocked=%v pending=%v selected=%q bounds=%v", a.palette.open, a.commandPaletteBlocked(), a.palette.focusPending, a.palette.selected, view.HasText("命令面板"))
	}
	if a.palette.selected != "search" {
		t.Fatalf("initial selection = %q", a.palette.selected)
	}
	view.Key(0, ui.KeyDown)
	if a.palette.selected != "new" {
		t.Fatalf("Down did not move selection: %q", a.palette.selected)
	}
	view.Key(0, ui.KeyUp)
	if a.palette.selected != "search" {
		t.Fatal("Up did not move selection")
	}
	view.Type("appearance")
	if len(a.palette.results) != 1 || a.palette.selected != "settings" {
		t.Fatalf("English alias did not filter Chinese commands: %+v", a.palette.results)
	}
	view.Key(0, ui.KeyEnter)
	view.Frame()
	if a.palette.open || a.page != "settings" || a.settingsState.Category != "general" {
		t.Fatal("Enter did not execute selected command")
	}
	if a.query != "Git" {
		t.Fatal("command search changed the entry search")
	}
}

func TestCommandPaletteDismissalRestoresFocusAndDraft(t *testing.T) {
	for _, dismissal := range []string{"escape", "backdrop", "shortcut", "button"} {
		t.Run(dismissal, func(t *testing.T) {
			a := fixture()
			a.editEntry()
			a.editor.username = "unsaved@example.test"
			a.editor.compose()
			before := a.editor
			view := ui.NewTester(testView(a), 1120, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			if err := view.Click("搜索条目"); err != nil {
				t.Fatal(err)
			}
			view.Key(ui.Ctrl|ui.Shift, ui.KeyP)
			view.Advance(200 * time.Millisecond)
			view.Type("no-such-command")
			if !view.HasText("没有匹配的命令") {
				t.Fatal("missing empty state")
			}
			view.Key(0, ui.KeyEnter)
			if !a.palette.open {
				t.Fatal("empty results executed a command")
			}
			switch dismissal {
			case "escape":
				view.Key(0, ui.KeyEscape)
			case "backdrop":
				view.ClickAt(20, 400)
			case "shortcut":
				view.Key(ui.Ctrl|ui.Shift, ui.KeyP)
			case "button":
				if err := view.Click("关闭命令面板"); err != nil {
					t.Fatal(err)
				}
			}
			view.Advance(250 * time.Millisecond)
			if a.palette.open || a.palette.query != "" || a.editor != before {
				t.Fatal("dismissal lost the draft or retained palette state")
			}
			if !view.Focused("搜索条目") {
				t.Fatal("dismissal did not restore the previous focus")
			}
		})
	}
}

func TestCommandPaletteSkipsUnavailableAndRechecksExecution(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 1120, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Key(ui.Ctrl|ui.Shift, ui.KeyP)
	view.Advance(200 * time.Millisecond)
	view.Type("copy password")
	if a.palette.selected != "copy-password" {
		t.Fatal("copy command not selected")
	}
	// Lock arrives after rendering but before activation; the stale command
	// must neither execute nor dismiss the user's search.
	a.state.Locked = true
	a.submitPaletteCommand("copy-password")
	view.Frame()
	if !a.palette.open || a.palette.selected != "" || !view.HasText("请先解锁密码库") {
		t.Fatal("lock failed to invalidate copy")
	}
	view.Key(0, ui.KeyEnter)
	if !a.palette.open {
		t.Fatal("disabled command executed")
	}
	a.palette.query = ""
	view.Frame()
	view.Key(0, ui.KeyDown)
	if a.palette.selected != "new" {
		t.Fatalf("navigation did not select new: %q", a.palette.selected)
	}
	view.Key(0, ui.KeyDown)
	if a.palette.selected != "all-entries" {
		t.Fatalf("navigation did not skip disabled commands: %q", a.palette.selected)
	}
}

func TestCommandPaletteModalIsolationAndLanguageChange(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 1120, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	a.deleteOpen = true
	view.Frame()
	a.toggleCommandPalette()
	if a.palette.open {
		t.Fatal("palette opened over confirmation")
	}
	a.deleteOpen = false
	view.Frame()
	view.Key(ui.Ctrl|ui.Shift, ui.KeyP)
	view.Advance(200 * time.Millisecond)
	view.Key(ui.Cmd, ui.KeyN)
	if a.editor.open {
		t.Fatal("background shortcut escaped the modal scope")
	}
	view.Type("settings")
	query := a.palette.query
	a.settings.Language = "en"
	view.Frame()
	if !view.HasText("Command palette") || a.palette.query != query || !view.Focused("Search commands") {
		t.Fatal("language change lost search input or focus")
	}
	a.toggleCommandPalette()
	view.Frame()
	a.toggleCommandPalette()
	view.Advance(200 * time.Millisecond)
	if a.palette.query != "" || !view.Focused("Search commands") {
		t.Fatal("reopening retained query or failed to focus")
	}
}

func TestCommandPaletteSearchActionFocus(t *testing.T) {
	for _, page := range []string{"entries", "settings"} {
		t.Run(page, func(t *testing.T) {
			a := fixture()
			a.page = page
			view := ui.NewTester(testView(a), 1120, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			view.Key(ui.Ctrl|ui.Shift, ui.KeyP)
			view.Advance(200 * time.Millisecond)
			view.Key(0, ui.KeyEnter)
			view.Advance(250 * time.Millisecond)
			if a.palette.open || !view.Focused("搜索条目") {
				t.Fatal("search command did not focus entry search after closing")
			}
		})
	}
}

func TestCommandPaletteWindowClosePreservesEditor(t *testing.T) {
	a := fixture()
	a.editEntry()
	a.editor.username = "unsaved@example.test"
	a.editor.compose()
	before := a.editor
	a.toggleCommandPalette()
	a.handleMainWindowClose(&mygo.CloseEvent{})
	if a.palette.open || a.discardOpen || a.editor != before || a.quit.requested {
		t.Fatal("closing the palette affected the editor or window lifecycle")
	}
}

func TestCommandPalettePreservesTypedEditorValue(t *testing.T) {
	a := fixture()
	a.editEntry()
	view := ui.NewTester(testView(a), 1120, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	label, found := view.Find("用户名")
	if !found {
		t.Fatal("missing username field")
	}
	view.ClickAt(label.X+16, label.Y+label.H+18)
	if !view.Focused("用户名") {
		t.Fatal("username input not focused")
	}
	view.Key(ui.Cmd, ui.KeyA)
	view.Type("palette-smoke@example.test")
	view.Key(ui.Ctrl|ui.Shift, ui.KeyP)
	view.Advance(200 * time.Millisecond)
	if !a.palette.open {
		t.Fatal("text input shortcut failed")
	}
	view.Key(0, ui.KeyEscape)
	view.Advance(200 * time.Millisecond)
	if a.editor.username != "palette-smoke@example.test" || !view.Focused("用户名") {
		t.Fatal("opening the palette lost typed input or its focus")
	}
}

func TestCommandPaletteScrollAndPointerActivation(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 860, 620)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	if err := view.Click("打开命令面板"); err != nil {
		t.Fatal(err)
	}
	view.Advance(200 * time.Millisecond)
	view.Key(0, ui.KeyUp)
	view.Advance(200 * time.Millisecond)
	if a.palette.selected != "about" {
		t.Fatal("Up from first did not wrap to last command")
	}
	assertTypographyBounds(t, view, 860, 620, "关于 Passpine")
	if err := view.Click("关于 Passpine"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if a.palette.open || a.page != "settings" || a.settingsState.Category != "about" {
		t.Fatal("pointer did not execute scrolled command")
	}
}

func TestCommandPaletteLayouts(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			for _, scale := range []int{100, 120} {
				t.Run(fmt.Sprintf("%s-dark%v-scale%d", locale, dark, scale), func(t *testing.T) {
					a := fixture()
					a.state.Settings.Language, a.state.Settings.FontScale = locale, scale
					view := ui.NewTester(testView(a), 860, 620)
					view.SetDark(dark)
					view.SetPreferences(ui.Preferences{ReduceMotion: true})
					view.Key(ui.Ctrl|ui.Shift, ui.KeyP)
					view.Advance(time.Second)
					assertTypographyBounds(t, view, 860, 620, a.t("搜索命令"), a.t("关闭命令面板"), a.t("%d 个命令", 18))
					if !view.Focused(a.t("搜索命令")) {
						t.Fatal("input not focused")
					}
					managerResizeScreenshot(t, view, fmt.Sprintf("palette-%s-%v-%d", locale, dark, scale))
					view.Type("no-such-command")
					assertTypographyBounds(t, view, 860, 620, a.t("没有匹配的命令"), a.t("%d 个命令", 0))
					managerResizeScreenshot(t, view, fmt.Sprintf("palette-empty-%s-%v-%d", locale, dark, scale))
				})
			}
		}
	}
}
