package desktop

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

type stableListNode struct {
	id      uint64
	x, y    float64
	opacity float64
}

// Tester.Find reports layout bounds, so it cannot detect an inherited fade or
// replacement of a visually identical subtree. Inspect the search field and its
// ancestors as well to cover the intermediate frames that caused the flicker.
func listPresentation(t *testing.T, view *ui.Tester, label string) []stableListNode {
	t.Helper()
	root := reflect.ValueOf(view).Elem().FieldByName("rt").Elem().FieldByName("c").FieldByName("root")
	var found reflect.Value
	var walk func(reflect.Value)
	walk = func(pointer reflect.Value) {
		if pointer.IsNil() || found.IsValid() {
			return
		}
		n := pointer.Elem()
		if n.FieldByName("label").String() == label {
			found = pointer
			return
		}
		for child := n.FieldByName("first"); !child.IsNil(); child = child.Elem().FieldByName("next") {
			walk(child)
		}
	}
	walk(root)
	if !found.IsValid() {
		t.Fatalf("list search %q is missing", label)
	}
	var path []stableListNode
	for !found.IsNil() {
		n := found.Elem()
		opacity := float64(1)
		if n.FieldByName("opacitySet").Bool() {
			opacity = n.FieldByName("opacity").Float()
		}
		path = append(path, stableListNode{n.FieldByName("id").Uint(), n.FieldByName("x").Float(), n.FieldByName("y").Float(), opacity})
		found = n.FieldByName("parent")
	}
	return path
}

func assertListPresentation(t *testing.T, view *ui.Tester, a *application, before []stableListNode) {
	t.Helper()
	after := listPresentation(t, view, a.t("搜索条目"))
	for _, node := range after {
		if node.opacity != 1 {
			t.Errorf("list ancestor %d faded to %v", node.id, node.opacity)
		}
	}
	if before != nil && !reflect.DeepEqual(after, before) {
		t.Errorf("list subtree changed identity or position: before=%+v after=%+v", before, after)
	}
}

func TestEntryActionsKeepListMountedWithoutPageMotion(t *testing.T) {
	for _, action := range []string{"edit", "select"} {
		t.Run(action, func(t *testing.T) {
			a := fixture()
			state, original := a.state, a.detail
			callbacks := make(chan func(), 1)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.service = backendFunc(func(request app.Request) app.Response {
				data := state
				data.Entry, data.Password, data.Username, data.URL, data.Contents = request.Entry, original.Password, original.Username, original.URL, original.Contents
				return app.Response{Status: "ok", Data: data}
			})
			view := ui.NewTester(testView(a), 1120, 760)
			view.Advance(time.Second)
			before := listPresentation(t, view, a.t("搜索条目"))
			label := a.t("编辑条目")
			if action == "select" {
				label = a.t("账号 %s", "工作/Figma")
			}
			if err := view.Click(label); err != nil {
				t.Fatal(err)
			}
			assertListPresentation(t, view, a, before)
			view.Advance(60 * time.Millisecond)
			assertListPresentation(t, view, a, before)
			if action == "edit" {
				if err := view.Click(a.t("关闭编辑器")); err != nil {
					t.Fatal(err)
				}
			} else {
				awaitMain(t, callbacks)
				view.Frame()
			}
			assertListPresentation(t, view, a, before)
		})
	}
}

func TestUnlockShowsOpaqueStationaryListAndLockRemovesSecrets(t *testing.T) {
	for _, browse := range []bool{false, true} {
		t.Run(fmt.Sprintf("browse=%t", browse), func(t *testing.T) {
			a := fixture()
			a.state.Settings.BrowseWhileLocked = browse
			state, original := a.state, a.detail
			a.clearSecrets()
			a.state.Locked = true
			callbacks := make(chan func(), 2)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.service = backendFunc(func(request app.Request) app.Response {
				data := state
				if request.Action == "fetch" {
					data.Entry, data.Password, data.Username, data.URL, data.Contents = request.Entry, original.Password, original.Username, original.URL, original.Contents
				}
				return app.Response{Status: "ok", Data: data}
			})
			view := ui.NewTester(testView(a), 1120, 760)
			view.Advance(time.Second)
			var before []stableListNode
			if browse {
				before = listPresentation(t, view, a.t("搜索条目"))
			}
			a.unlock("")
			awaitMain(t, callbacks)
			view.Frame()
			if browse {
				assertListPresentation(t, view, a, before)
			} else if _, found := view.Find(a.t("搜索条目")); found {
				t.Error("unlock mounted the list before its selected detail was ready")
			}
			awaitMain(t, callbacks)
			view.Frame()
			assertListPresentation(t, view, a, before)
			if before == nil {
				before = listPresentation(t, view, a.t("搜索条目"))
			}
			view.Advance(220 * time.Millisecond)
			assertListPresentation(t, view, a, before)
			a.reveal = true
			view.Frame()
			if !view.HasText(original.Password) {
				t.Fatal("fixture did not reveal its password")
			}
			a.lock()
			view.Frame()
			for _, value := range []string{original.Password, original.Username, original.URL} {
				if view.HasText(value) {
					t.Errorf("locking retained visible credential %q", value)
				}
			}
		})
	}
}

func TestEntryActionsRetainSearchFocusAndListScroll(t *testing.T) {
	for _, action := range []string{"edit", "select"} {
		t.Run(action, func(t *testing.T) {
			a := fixture()
			for i := range 50 {
				a.state.Entries = append(a.state.Entries, app.Entry{Path: fmt.Sprintf("更多/Account%02d", i), Name: fmt.Sprintf("Account%02d", i), Folder: "更多"})
			}
			view := ui.NewTester(testView(a), 1120, 760)
			view.Advance(time.Second)
			view.Scroll(350, 350, 0, 800)
			first, last := a.list.Visible()
			if first == 0 {
				t.Fatal("fixture did not scroll the account list")
			}
			if err := view.Click(a.t("搜索条目")); err != nil {
				t.Fatal(err)
			}
			if action == "edit" {
				a.editEntry()
			} else {
				// Queue a selection already in view, as an asynchronous caller
				// would, without moving focus or asking the list to scroll.
				a.busy, a.backgroundBusy = true, true
				a.selectEntry(a.filtered[first+2].Path)
			}
			view.Frame()
			if !view.Focused(a.t("搜索条目")) {
				t.Error("updating the inspector removed search focus")
			}
			if gotFirst, gotLast := a.list.Visible(); gotFirst != first || gotLast != last {
				t.Errorf("updating the inspector changed list scroll: before=%d:%d after=%d:%d", first, last, gotFirst, gotLast)
			}
		})
	}
}

func TestNewEditorSessionResetsInputStateWithoutReplacingList(t *testing.T) {
	a := fixture()
	a.editEntry()
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click(a.t("账号名称")); err != nil {
		t.Fatal(err)
	}
	view.Key(ui.Cmd, ui.KeyA)
	previous := a.editor.identity
	before := listPresentation(t, view, a.t("搜索条目"))
	// A new session can be opened before a closing frame is drawn, for example
	// by a pending navigation action. It must not inherit the old text selection.
	a.closeEditor()
	a.selected = "工作/Figma"
	a.detail = app.Data{Entry: a.selected, Password: "other-password", Username: "other-user", Contents: "other-password\nusername: other-user\n"}
	a.editEntry()
	view.Frame()
	if a.editor.identity == previous || a.editor.name != "Figma" || a.editor.username != "other-user" {
		t.Fatal("new editor session retained the previous account")
	}
	if view.Focused(a.t("账号名称")) {
		t.Error("new editor session retained the previous input focus and selection")
	}
	assertListPresentation(t, view, a, before)
}
