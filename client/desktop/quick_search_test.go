package desktop

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

type fakeQuickSearchWindow struct {
	shown, hidden, focused, destroyed, frames int
	bounds                                    mygo.Rectangle
}

func (w *fakeQuickSearchWindow) Show()                           { w.shown++ }
func (w *fakeQuickSearchWindow) Hide()                           { w.hidden++ }
func (w *fakeQuickSearchWindow) Focus()                          { w.focused++ }
func (w *fakeQuickSearchWindow) Invalidate()                     { w.frames++ }
func (w *fakeQuickSearchWindow) Destroy()                        { w.destroyed++ }
func (w *fakeQuickSearchWindow) Bounds() mygo.Rectangle          { return w.bounds }
func (w *fakeQuickSearchWindow) SetBounds(bounds mygo.Rectangle) { w.bounds = bounds }

func quickSearchFixture() (*application, *fakeQuickSearchWindow) {
	a := fixture()
	a.state.VaultID = "test-vault"
	window := &fakeQuickSearchWindow{}
	a.quick.window, a.quick.visible, a.quick.focusPending = window, true, true
	return a, window
}

func TestGlobalShortcutTogglesIndependentPanelWithoutChangingManager(t *testing.T) {
	a, window := quickSearchFixture()
	a.quick.visible = false
	a.busy = true // Opening during other work still displays the panel immediately.
	a.page, a.query, a.filter = "settings", "manager search", "folder:开发"
	a.editor = editorState{open: true, contents: "unsaved document"}
	editor := a.editor
	a.globalShortcutActivated()
	if !a.quick.visible || window.shown != 1 || window.focused != 1 {
		t.Fatal("global shortcut did not show the independent panel")
	}
	if a.page != "settings" || a.query != "manager search" || a.filter != "folder:开发" || a.editor != editor {
		t.Fatal("opening global search changed the manager or its editor")
	}
	a.quick.passphrase = "private"
	a.globalShortcutActivated()
	if a.quick.visible || window.hidden != 1 || a.quick.passphrase != "" {
		t.Fatal("second shortcut did not dismiss and clear the panel")
	}
}

func TestQuickSearchIsIndependentAndKeyboardCopiesThroughService(t *testing.T) {
	a, window := quickSearchFixture()
	a.query, a.filter = "main query", "folder:生活"
	callbacks, requests := make(chan func(), 2), make(chan app.Request, 2)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	data := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: data}
	})
	view := ui.NewTester(a.quickSearchView, 660, 460)
	if err := view.Click("快速搜索条目"); err != nil {
		t.Fatal(err)
	}
	view.Type("开发")
	if len(a.quick.results) != 2 || a.quick.results[0].Name != "Cloudflare" {
		t.Fatalf("global search did not search all folders: %#v", a.quick.results)
	}
	view.Key(0, ui.KeyDown)
	if a.quick.selected != 1 {
		t.Fatal("arrow key did not move the selected result while typing")
	}
	view.Key(0, ui.KeyEnter)
	awaitMain(t, callbacks)
	request := <-requests
	if request.Action != "copy" || request.Entry != "开发/GitHub" || request.Field != "password" || request.VaultID != "test-vault" {
		t.Fatalf("wrong credential operation: %#v", request)
	}
	if a.quick.visible || window.hidden != 1 || a.query != "main query" || a.filter != "folder:生活" {
		t.Fatal("successful copy did not dismiss the independent search")
	}
	if a.state.Password != "" || a.state.Contents != "" {
		t.Fatal("panel retained a decrypted document")
	}
}

func TestQuickSearchLockedDoesNotExposeNamesAndUnlocksInPanel(t *testing.T) {
	a, _ := quickSearchFixture()
	a.state.Locked = true
	a.state.Settings.BrowseWhileLocked = false
	callbacks, requests := make(chan func(), 1), make(chan app.Request, 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	data := a.state
	data.Locked = false
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: data}
	})
	view := ui.NewTester(a.quickSearchView, 660, 460)
	if len(a.quick.results) != 0 || view.HasText("GitHub") {
		t.Fatal("locked quick search exposed entry names")
	}
	if err := view.Click("快速搜索私钥口令"); err != nil {
		t.Fatal(err)
	}
	view.Type("test password")
	view.Key(0, ui.KeyEnter)
	awaitMain(t, callbacks)
	request := <-requests
	if request.Action != "unlock" || request.Passphrase != "test password" || request.VaultID != "test-vault" {
		t.Fatalf("wrong unlock request: %#v", request)
	}
	view.Frame()
	if a.state.Locked || a.quick.passphrase != "" || !view.HasText("GitHub") || a.unlockTime.IsZero() {
		t.Fatal("unlock did not return to search or clear the private key passphrase")
	}
}

func TestQuickSearchLockedAllowsNameSearchAndResumesSelectedCopyAfterPassphrase(t *testing.T) {
	a, window := quickSearchFixture()
	a.state.Locked, a.state.Settings.BrowseWhileLocked = true, true
	a.detail = app.Data{}
	callbacks, requests := make(chan func(), 2), make(chan app.Request, 2)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	data := a.state
	data.Locked = false
	data.Password, data.Contents = "must-not-be-retained", "must-not-be-retained"
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: data}
	})
	view := ui.NewTester(a.quickSearchView, 660, 460)
	if !view.HasText("GitHub") || !view.HasText("快速搜索条目") {
		t.Fatal("allowing locked browsing did not expose searchable entry names")
	}
	if err := view.Click("快速搜索条目"); err != nil {
		t.Fatal(err)
	}
	view.Type("开发")
	view.Key(ui.Ctrl, ui.KeyN)
	view.Key(0, ui.KeyEnter)
	if !view.HasText("解锁以复制密码") || a.quick.pendingCopy.entry != "开发/GitHub" || len(requests) != 0 {
		t.Fatal("copying a locked result did not enter authentication for the selected path")
	}
	if err := view.Click("快速搜索私钥口令"); err != nil {
		t.Fatal(err)
	}
	view.Type("test password")
	view.Key(0, ui.KeyEnter)
	awaitMain(t, callbacks)
	a.quick.selected = 0 // Completion must use the captured path, not a later index.
	awaitMain(t, callbacks)
	unlock, copy := <-requests, <-requests
	if unlock.Action != "unlock" || unlock.Passphrase != "test password" || unlock.VaultID != "test-vault" {
		t.Fatalf("wrong unlock request: %+v", unlock)
	}
	if copy.Action != "copy" || copy.Entry != "开发/GitHub" || copy.Field != "password" || copy.VaultID != "test-vault" {
		t.Fatalf("authentication did not resume the captured copy operation: %+v", copy)
	}
	if a.quick.visible || window.hidden != 1 || a.quick.passphrase != "" || a.quick.pendingCopy.entry != "" || a.state.Password != "" || a.state.Contents != "" || a.detail.Contents != "" {
		t.Fatal("successful quick copy retained secrets or failed to dismiss the panel")
	}
}

func TestQuickSearchLockedCopyStartsDeviceUnlockAndCanReturnToSearch(t *testing.T) {
	t.Run("device unlock", func(t *testing.T) {
		a, _ := quickSearchFixture()
		a.state.Locked, a.state.Settings.BrowseWhileLocked = true, true
		a.state.Settings.DeviceUnlock, a.state.DeviceUnlockAvailable, a.state.DeviceUnlockEnrolled = true, true, true
		callbacks, requests := make(chan func(), 2), make(chan app.Request, 2)
		a.runOnMain = func(callback func()) { callbacks <- callback }
		data := a.state
		data.Locked = false
		a.service = backendFunc(func(request app.Request) app.Response {
			requests <- request
			return app.Response{Status: "ok", Data: data}
		})
		a.quick.query = "开发"
		a.prepareQuickSearchList()
		a.copyQuickSearchResult()
		if a.quick.operation != "device-unlock" {
			t.Fatal("locked quick copy did not start available system authentication")
		}
		awaitMain(t, callbacks)
		awaitMain(t, callbacks)
		unlock, copy := <-requests, <-requests
		if unlock.Action != "unlock" || unlock.Mode != "device" || unlock.Passphrase != "" || copy.Action != "copy" || copy.Entry != "开发/Cloudflare" {
			t.Fatalf("device authentication did not continue the selected copy: %+v, %+v", unlock, copy)
		}
	})
	t.Run("return to names", func(t *testing.T) {
		a, _ := quickSearchFixture()
		a.state.Locked, a.state.Settings.BrowseWhileLocked = true, true
		view := ui.NewTester(a.quickSearchView, 660, 460)
		view.Key(0, ui.KeyEnter)
		a.quick.passphrase = "private"
		if err := view.Click("返回搜索"); err != nil {
			t.Fatal(err)
		}
		if !a.state.Locked || a.quick.pendingCopy.entry != "" || a.quick.passphrase != "" || !view.HasText("快速搜索条目") {
			t.Fatal("canceling authentication did not return to locked name browsing")
		}
	})
}

func TestQuickSearchDisablingLockedBrowsingClearsCachedNames(t *testing.T) {
	a, _ := quickSearchFixture()
	a.state.Locked, a.state.Settings.BrowseWhileLocked = true, true
	view := ui.NewTester(a.quickSearchView, 660, 460)
	if !view.HasText("GitHub") || len(a.quick.cachedResults) == 0 {
		t.Fatal("fixture did not show locked entry names")
	}
	a.state.Settings.BrowseWhileLocked = false
	view.Frame()
	if view.HasText("GitHub") || len(a.quick.results) != 0 || len(a.quick.cachedResults) != 0 || !view.HasText("解锁以快速搜索") {
		t.Fatal("disabling locked browsing left entry names in the search panel")
	}
}

func TestQuickSearchUnlockContinuationIsDiscardedAfterCloseLockOrVaultChange(t *testing.T) {
	for _, scenario := range []string{"close", "reopen", "lock", "vault change", "response vault change"} {
		t.Run(scenario, func(t *testing.T) {
			a, window := quickSearchFixture()
			a.state.Locked, a.state.Settings.BrowseWhileLocked = true, true
			callbacks, requests := make(chan func(), 2), make(chan app.Request, 2)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			data := a.state
			data.Locked = false
			if scenario == "response vault change" {
				data.VaultID = "other-vault"
			}
			a.service = backendFunc(func(request app.Request) app.Response {
				requests <- request
				return app.Response{Status: "ok", Data: data}
			})
			a.prepareQuickSearchList()
			a.copyQuickSearchResult()
			a.quick.passphrase = "private"
			a.unlockQuickSearch("")
			switch scenario {
			case "close", "reopen":
				a.hideQuickSearch()
				if scenario == "reopen" {
					a.quick.visible = true
					a.quick.passphrase = "new private input"
				}
			case "lock":
				a.clearSecrets()
				a.generation++
				a.busy = false
			case "vault change":
				a.clearSecrets()
				a.state.VaultID = "other-vault"
			}
			awaitMain(t, callbacks)
			if (<-requests).Action != "unlock" || a.quick.pendingCopy.entry != "" {
				t.Fatal("late unlock continued a canceled quick copy")
			}
			select {
			case request := <-requests:
				t.Fatalf("canceled unlock issued another request: %+v", request)
			case <-time.After(20 * time.Millisecond):
			}
			if scenario == "reopen" && (!a.quick.visible || window.hidden != 1 || a.quick.passphrase != "new private input") {
				t.Fatal("late unlock changed the reopened panel")
			}
			if (scenario == "vault change" || scenario == "response vault change") && a.state.VaultID != "other-vault" {
				t.Fatal("late unlock restored a previous vault snapshot")
			}
		})
	}
}

func TestQuickSearchOpenManagerDuringUnlockContinuesSelectedRead(t *testing.T) {
	a, _ := quickSearchFixture()
	a.state.Locked, a.state.Settings.BrowseWhileLocked = true, true
	a.detail = app.Data{}
	callbacks, requests := make(chan func(), 3), make(chan app.Request, 3)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	data := a.state
	data.Locked = false
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		result := data
		if request.Action == "fetch" {
			result.Entry, result.Password, result.Contents = request.Entry, "manager-secret", "manager-secret"
		}
		return app.Response{Status: "ok", Data: result}
	})
	a.quick.query = "开发"
	a.prepareQuickSearchList()
	a.quick.selected = 1
	a.copyQuickSearchResult()
	a.quick.passphrase = "private"
	a.unlockQuickSearch("")
	a.openQuickSearchInManager()
	if a.quick.visible || a.quick.pendingCopy.entry != "" || a.pendingSelection != "开发/GitHub" {
		t.Fatal("opening the manager did not replace quick copy with the selected read")
	}
	awaitMain(t, callbacks)
	awaitMain(t, callbacks)
	unlock, read := <-requests, <-requests
	if unlock.Action != "unlock" || read.Action != "fetch" || read.Entry != "开发/GitHub" || a.detail.Entry != "开发/GitHub" || a.quick.visible {
		t.Fatalf("opening the manager lost its selected read or copied instead: %+v, %+v", unlock, read)
	}
}

func TestQuickSearchControlNPSelectsFromSearchAndResults(t *testing.T) {
	a, _ := quickSearchFixture()
	view := ui.NewTester(a.quickSearchView, 660, 460)
	if err := view.Click("快速搜索条目"); err != nil {
		t.Fatal(err)
	}
	view.Type("开发")
	view.Key(ui.Ctrl, ui.KeyN)
	if a.quick.selected != 1 || a.quick.query != "开发" {
		t.Fatalf("Control N did not select the next result without editing the query: selected=%d query=%q results=%d", a.quick.selected, a.quick.query, len(a.quick.results))
	}
	view.Key(ui.Ctrl, ui.KeyN)
	if a.quick.selected != 1 {
		t.Fatal("Control N moved beyond the last result")
	}
	view.Key(ui.Ctrl, ui.KeyP)
	if a.quick.selected != 0 {
		t.Fatal("Control P did not select the previous result")
	}
	view.Key(0, ui.KeyTab) // Continue using the same shortcuts with list focus.
	view.Key(ui.Ctrl, ui.KeyN)
	if a.quick.selected != 1 || !view.HasText(quickSearchOpenShortcut()) {
		t.Fatal("result navigation or main window shortcut hint is missing")
	}
}

func TestQuickSearchLateCopyDoesNotDismissReopenedPanel(t *testing.T) {
	a, window := quickSearchFixture()
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	data := a.state
	a.service = backendFunc(func(app.Request) app.Response { return app.Response{Status: "ok", Data: data} })
	a.prepareQuickSearchList()
	a.copyQuickSearchResult()
	a.hideQuickSearch()
	a.quick.visible = true // A later invocation owns the newly shown panel.
	awaitMain(t, callbacks)
	if !a.quick.visible || window.hidden != 1 || a.busy {
		t.Fatal("late completion dismissed the reopened panel or left the application busy")
	}
}

func TestQuickSearchErrorsStayInPanelAndLocksClearCredentials(t *testing.T) {
	a, _ := quickSearchFixture()
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	data := a.state
	a.service = backendFunc(func(app.Request) app.Response {
		return app.Response{Status: "error", Message: "无法复制此条目", Data: data}
	})
	a.prepareQuickSearchList()
	a.copyQuickSearchResult()
	awaitMain(t, callbacks)
	if a.quick.errorText != "无法复制此条目" || a.errorText != "" || !a.quick.visible {
		t.Fatal("copy failure was lost or leaked to the manager")
	}
	view := ui.NewTester(a.quickSearchView, 660, 460)
	if !view.HasText("操作未完成") || !view.HasText("无法复制此条目") {
		t.Fatal("failure was not presented as a client toast")
	}
	a.quick.passphrase = "secret"
	a.clearSecrets()
	if a.quick.passphrase != "" || len(a.quick.results) != 0 {
		t.Fatal("lock retained quick search credentials")
	}
}

func TestQuickSearchStartsAboveCenterAndRemembersIndependentPosition(t *testing.T) {
	for _, area := range []mygo.Rectangle{{X: 0, Y: 25, Width: 1440, Height: 875}, {X: -1920, Y: 40, Width: 1920, Height: 1040}, {X: 0, Y: 0, Width: 600, Height: 400}} {
		options := quickSearchOptions(area)
		if !options.Panel || !options.Frameless || options.StateKey != "" || options.DisableMove || options.Transparent {
			t.Fatal("quick search lacks native panel behavior, remembered dragging, or an opaque background")
		}
		bounds := quickSearchBounds(area)
		if bounds.X < area.X || bounds.Y < area.Y || bounds.X+bounds.Width > area.X+area.Width || bounds.Y+bounds.Height > area.Y+area.Height {
			t.Fatalf("panel is outside the display work area: %#v", bounds)
		}
		if bounds.X != area.X+(area.Width-bounds.Width)/2 || bounds.Y > area.Y+area.Height/4 {
			t.Fatal("panel did not start at the horizontal center near the top of the display")
		}
	}
	a, window := quickSearchFixture()
	window.bounds = mygo.Rectangle{X: 320, Y: 310, Width: 660, Height: 460}
	a.busy = true
	a.hideQuickSearch()
	a.quickSearch()
	if window.bounds.X != 320 || window.bounds.Y != 310 {
		t.Fatal("reopening reset the user's dragged position")
	}
	a.closeQuickSearch()
	if window.destroyed != 1 || a.quick.window != nil {
		t.Fatal("manager shutdown left a hidden panel alive")
	}
}

func TestQuickSearchRender(t *testing.T) {
	directory := os.Getenv("PASSPINE_QUICK_SCREENSHOTS")
	if directory == "" {
		t.Skip("set PASSPINE_QUICK_SCREENSHOTS to render the native panel")
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for _, screen := range []struct {
		name, language string
		width          int
		dark, locked   bool
		hideNames      bool
		copyUnlock     bool
	}{
		{name: "quick-search", language: "zh-CN", width: 660},
		{name: "quick-search-en", language: "en", width: 660},
		{name: "quick-search-dark", language: "zh-CN", width: 660, dark: true},
		{name: "quick-search-en-dark", language: "en", width: 660, dark: true},
		{name: "quick-search-en-narrow", language: "en", width: 600},
		{name: "quick-search-locked", language: "zh-CN", width: 660, locked: true},
		{name: "quick-search-locked-hidden", language: "zh-CN", width: 660, locked: true, hideNames: true},
		{name: "quick-search-copy-unlock", language: "zh-CN", width: 660, locked: true, copyUnlock: true},
		{name: "quick-search-copy-unlock-en", language: "en", width: 600, locked: true, copyUnlock: true},
	} {
		a, _ := quickSearchFixture()
		a.state.Locked, a.state.Settings.Language = screen.locked, screen.language
		a.state.Settings.BrowseWhileLocked = !screen.hideNames
		if screen.copyUnlock {
			a.prepareQuickSearchList()
			a.copyQuickSearchResult()
		}
		view := ui.NewTester(a.quickSearchView, screen.width, 460)
		if screen.dark {
			view.SetDark(true)
		}
		file, err := os.Create(filepath.Join(directory, screen.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, view.Image()); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestQuickSearchReturnsFromDisconnectedDisplayWithoutMovingSavedOnscreenPosition(t *testing.T) {
	primary := mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
	secondary := mygo.Rectangle{X: -1920, Y: 0, Width: 1920, Height: 1080}
	saved := mygo.Rectangle{X: -1500, Y: 280, Width: 660, Height: 460}
	if got := quickSearchPlacement(saved, []mygo.Rectangle{primary, secondary}, primary); got != saved {
		t.Fatalf("connected display did not retain the user's saved location: %#v", got)
	}
	if got := quickSearchPlacement(saved, []mygo.Rectangle{primary}, primary); got != quickSearchBounds(primary) {
		t.Fatalf("disconnected display did not fall back to Spotlight placement: %#v", got)
	}
	partial := mygo.Rectangle{X: 1340, Y: 810, Width: 660, Height: 460}
	want := mygo.Rectangle{X: 780, Y: 440, Width: 660, Height: 460}
	if got := quickSearchPlacement(partial, []mygo.Rectangle{primary}, primary); got != want {
		t.Fatalf("partly offscreen panel did not keep its controls accessible: %#v", got)
	}
}

func TestQuickSearchDismissClearsOldFailureToast(t *testing.T) {
	a, _ := quickSearchFixture()
	a.quick.errorText = "previous operation failed"
	view := ui.NewTester(a.quickSearchView, 660, 460)
	if !view.HasText("previous operation failed") {
		t.Fatal("fixture did not show a failure")
	}
	a.hideQuickSearch()
	a.busy = true
	a.quickSearch()
	view.Frame()
	if view.HasText("previous operation failed") {
		t.Fatal("reopening the panel retained a stale failure toast")
	}
}

func TestQuickSearchLockDiscardsLateUnlockResponse(t *testing.T) {
	a, _ := quickSearchFixture()
	a.state.Locked = true
	a.quick.passphrase = "private"
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	data := a.state
	data.Locked = false
	a.service = backendFunc(func(app.Request) app.Response { return app.Response{Status: "ok", Data: data} })
	a.unlockQuickSearch("")
	a.clearSecrets()
	a.generation++
	a.busy = false
	awaitMain(t, callbacks)
	if !a.state.Locked || a.quick.passphrase != "" || !a.unlockTime.IsZero() {
		t.Fatal("a stale unlock response revived a locked session")
	}
}
