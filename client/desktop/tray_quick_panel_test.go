package desktop

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestTrayQuickPanelPlacementBelowIconClampsAndFlips(t *testing.T) {
	for _, test := range []struct {
		name         string
		anchor, area mygo.Rectangle
		want         mygo.Rectangle
	}{
		{"below menu bar", mygo.Rectangle{X: 900, Y: 0, Width: 24, Height: 25}, mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}, mygo.Rectangle{X: 702, Y: 31, Width: 420, Height: 440}},
		{"right edge", mygo.Rectangle{X: 1410, Y: 0, Width: 24, Height: 25}, mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}, mygo.Rectangle{X: 1020, Y: 31, Width: 420, Height: 440}},
		{"secondary display", mygo.Rectangle{X: -1910, Y: 0, Width: 24, Height: 25}, mygo.Rectangle{X: -1920, Y: 25, Width: 1920, Height: 1055}, mygo.Rectangle{X: -1920, Y: 31, Width: 420, Height: 440}},
		{"bottom taskbar", mygo.Rectangle{X: 900, Y: 860, Width: 24, Height: 40}, mygo.Rectangle{X: 0, Y: 0, Width: 1440, Height: 860}, mygo.Rectangle{X: 702, Y: 414, Width: 420, Height: 440}},
		{"small display", mygo.Rectangle{X: 290, Y: 0, Width: 24, Height: 25}, mygo.Rectangle{X: 0, Y: 25, Width: 320, Height: 300}, mygo.Rectangle{X: 0, Y: 25, Width: 320, Height: 300}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := trayQuickPanelBounds(test.anchor, test.area); got != test.want {
				t.Fatalf("tray placement = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestTrayQuickPanelNeverOverwritesGlobalSearchPosition(t *testing.T) {
	a, window := quickSearchFixture()
	a.quick.positionPath = filepath.Join(t.TempDir(), quickSearchPositionFile)
	window.bounds = mygo.Rectangle{X: 240, Y: 190, Width: 660, Height: 460}
	a.quick.query = "global query"
	a.setQuickSearchMode(quickSearchTray)
	if a.quick.query != "" || a.quick.globalQuery != "global query" {
		t.Fatal("tray mode did not keep a separate search query")
	}
	window.bounds = mygo.Rectangle{X: 980, Y: 31, Width: 420, Height: 440}
	a.quick.query = "tray query"
	a.queueQuickSearchPositionSave()
	a.saveQuickSearchPosition()
	a.hideQuickSearch()
	if a.quick.positionTimer != nil {
		t.Fatal("tray mode scheduled global position persistence")
	}
	position, found, err := loadQuickSearchPosition(a.quick.positionPath)
	if err != nil || !found || position != (quickSearchPosition{X: 240, Y: 190}) {
		t.Fatalf("tray popup changed the dragged global position: %+v, %v", position, err)
	}
	a.setQuickSearchMode(quickSearchGlobal)
	if a.quick.query != "global query" || a.quick.trayQuery != "tray query" {
		t.Fatal("returning to global search did not restore its independent query")
	}
	area := mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
	options := quickSearchOptions(area)
	a.restoreQuickSearchPosition(&options, []mygo.Rectangle{area}, area)
	if options.X != 240 || options.Y != 190 || options.Width != 660 || options.Height != 460 {
		t.Fatal("returning to global search lost its original size or position")
	}
}

func TestGlobalShortcutSwitchesTrayPresentationToGlobalSearch(t *testing.T) {
	a, window := quickSearchFixture()
	a.quick.mode = quickSearchTray
	a.quick.query = "tray query"
	a.quick.globalQuery = "global query"
	a.busy = true
	a.toggleQuickSearch()
	if !a.quick.visible || a.quick.mode != quickSearchGlobal || a.quick.query != "global query" || window.shown != 1 {
		t.Fatal("global shortcut dismissed the tray panel instead of opening global search")
	}
	a.toggleQuickSearch()
	if a.quick.visible {
		t.Fatal("second global shortcut did not dismiss global search")
	}
	a.trayQuickPanel()
	if !a.quick.visible || a.quick.mode != quickSearchTray || a.quick.query != "tray query" {
		t.Fatal("tray action did not restore its compact presentation and query")
	}
}

func TestTrayQuickPanelOnlyQueriesAndCopiesPasswords(t *testing.T) {
	a, window := quickSearchFixture()
	a.quick.mode = quickSearchTray
	callbacks, requests := make(chan func(), 1), make(chan app.Request, 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	data := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: data}
	})
	view := ui.NewTester(a.quickSearchView, 420, 440)
	if view.HasText("新建条目") || view.HasText("编辑条目") {
		t.Fatal("tray panel exposes entry creation or editing")
	}
	if err := view.Click("快速搜索条目"); err != nil {
		t.Fatal(err)
	}
	view.Type("GitHub")
	view.Key(ui.Cmd, ui.KeyO)
	if !a.quick.visible {
		t.Fatal("tray panel retained the global search open-manager shortcut")
	}
	view.Key(0, ui.KeyEnter)
	awaitMain(t, callbacks)
	request := <-requests
	if request.Action != "copy" || request.Entry != "开发/GitHub" || request.Field != "password" || a.quick.visible || window.hidden != 1 {
		t.Fatalf("tray panel did not copy the selected password and dismiss: %+v", request)
	}
}
