package desktop

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine-neo/client/app"
)

func TestRequestsNeverShowGlobalProgressOrMoveContent(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 1120, 760)
	before, ok := view.Find("显示或隐藏密码")
	if !ok {
		t.Fatal("password reveal control missing")
	}
	for _, background := range []bool{true, false} {
		a.busy, a.backgroundBusy = true, background
		view.Frame()
		after, ok := view.Find("显示或隐藏密码")
		if !ok || after != before {
			t.Fatalf("busy state moved the detail: before=%v after=%v", before, after)
		}
		if view.HasText("正在处理") {
			t.Fatalf("background=%t showed the removed global progress bar", background)
		}
	}
	a.busy, a.backgroundBusy = false, false
	view.Frame()
	if after, _ := view.Find("显示或隐藏密码"); after != before {
		t.Fatal("completion moved the detail")
	}
}

func TestClientManagerHasOnlyEntrySidebarAndDetail(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 1120, 760)
	row, ok := view.Find("账号 开发/GitHub")
	if !ok || row.X < 0 || row.X+row.W > 310 {
		t.Fatalf("account rows are outside the single entry sidebar: %v", row)
	}
	detail, ok := view.Find("显示或隐藏密码")
	if !ok || detail.X < 310 {
		t.Fatal("detail is not next to the entry sidebar")
	}
	if view.HasText("全部账号") || view.HasText("恢复中心") || view.HasText("操作记录") {
		t.Fatal("the removed navigation column is still visible")
	}
	for _, label := range []string{"更多操作", "筛选文件夹", "所有文件夹"} {
		if _, found := view.Find(label); found {
			t.Fatalf("removed sidebar control remains: %s", label)
		}
	}
	count, ok := view.Find("条目数量")
	if !ok || count.X+count.W > 310 || count.Y < 700 {
		t.Fatalf("entry count is not in the sidebar footer: %v", count)
	}
}

func TestBackgroundRefreshKeepsLatestAccountClick(t *testing.T) {
	a := fixture()
	a.state.VaultID = "synthetic-vault"
	callbacks := make(chan func(), 2)
	requests := make(chan app.Request, 2)
	state := a.state
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(req app.Request) app.Response {
		requests <- req
		data := state
		if req.Action == "fetch" {
			data.Entry = req.Entry
		}
		return app.Response{Status: "ok", Data: data}
	})
	a.queueMetadataRefresh(false)
	a.selectEntry("开发/Cloudflare")
	a.selectEntry("工作/Figma")
	if a.pendingSelection != "工作/Figma" || a.detail.Password != "" {
		t.Fatal("background refresh dropped the latest click or retained another account's secret")
	}
	awaitMain(t, callbacks)
	awaitMain(t, callbacks)
	first, second := <-requests, <-requests
	if first.Action != "configure" || second.Action != "fetch" || second.Entry != "工作/Figma" || a.detail.Entry != "工作/Figma" || a.pendingSelection != "" {
		t.Fatalf("latest click was not fetched after background refresh: %#v %#v", first, second)
	}
}

func TestMetadataRefreshDoesNotPaintIntermediateBusyState(t *testing.T) {
	a, window := quickSearchFixture()
	callbacks := make(chan func(), 1)
	state := a.state
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(app.Request) app.Response { return app.Response{Status: "ok", Data: state} })
	a.queueMetadataRefresh(false)
	if window.frames != 0 {
		t.Fatal("background status refresh scheduled an intermediate disabled frame")
	}
	awaitMain(t, callbacks)
	if a.busy || window.frames != 1 || a.detail.Entry != "开发/GitHub" {
		t.Fatal("background refresh did not finish with one stable result frame")
	}
}

func TestRapidAccountClicksKeepHighlightAndDetailTogether(t *testing.T) {
	a := fixture()
	a.state.VaultID = "synthetic-vault"
	callbacks := make(chan func(), 2)
	requests := make(chan app.Request, 2)
	state := a.state
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(req app.Request) app.Response {
		requests <- req
		data := state
		data.Entry, data.Password = req.Entry, "synthetic-password"
		return app.Response{Status: "ok", Data: data}
	})
	view := ui.NewTester(testView(a), 1120, 760)
	for _, path := range []string{"开发/Cloudflare", "工作/Figma"} {
		if err := view.Click("账号 " + path); err != nil {
			t.Fatal(err)
		}
	}
	if a.pendingSelection != "工作/Figma" || a.filtered[a.selectedIndex].Path != "工作/Figma" {
		t.Fatal("second click did not keep its list highlight and pending selection")
	}
	awaitMain(t, callbacks) // The first fetch must not revive its secret.
	view.Frame()
	if a.detail.Entry != "" || a.selected != "工作/Figma" || a.detail.Password != "" || view.HasText("正在处理") {
		t.Fatal("first fetch flashed stale detail while the latest click was pending")
	}
	awaitMain(t, callbacks)
	view.Frame()
	first, second := <-requests, <-requests
	if first.Entry != "开发/Cloudflare" || second.Entry != "工作/Figma" || a.detail.Entry != "工作/Figma" || a.filtered[a.selectedIndex].Path != "工作/Figma" {
		t.Fatal("final detail and highlighted account disagree after rapid clicks")
	}
}

func TestAccountClickDoesNotInterruptOtherForegroundOperations(t *testing.T) {
	a := fixture()
	a.page, a.busy, a.activeRequestAction = "settings", true, "settings"
	a.selectEntry("工作/Figma")
	if a.page != "settings" || a.pendingSelection != "" || a.selected != "开发/GitHub" {
		t.Fatal("account click interrupted another foreground operation")
	}
}

func TestExplicitNavigationCancelsQueuedAccountRead(t *testing.T) {
	for _, action := range []string{"configure", "fetch"} {
		for _, target := range []string{"settings", "setup"} {
			t.Run(action+"_to_"+target, func(t *testing.T) {
				a := fixture()
				a.state.VaultID = "synthetic-vault"
				callbacks := make(chan func(), 3)
				requests := make(chan app.Request, 3)
				state := a.state
				a.runOnMain = func(callback func()) { callbacks <- callback }
				a.service = backendFunc(func(req app.Request) app.Response {
					requests <- req
					data := state
					if req.Action == "fetch" {
						data.Entry = req.Entry
					}
					return app.Response{Status: "ok", Data: data}
				})
				if action == "configure" {
					a.callBackground(app.Request{Action: "configure"}, nil)
				} else {
					a.selectEntry("开发/Cloudflare")
				}
				a.selectEntry("工作/Figma")
				if a.pendingSelection != "工作/Figma" {
					t.Fatal("test did not queue an account behind the in-flight response")
				}
				expectedResponses := 1
				if target == "settings" {
					a.navigate("settings", "")
					expectedResponses++ // Settings also checks Git status.
				} else {
					a.openSetup()
				}
				if a.pendingSelection != "" {
					t.Fatal("explicit navigation retained the pending account read")
				}
				for range expectedResponses {
					awaitMain(t, callbacks)
				}
				if target == "settings" && a.preferencesPage() != "settings" || target == "setup" && !a.setup || a.busy || a.pendingSelection != "" {
					t.Fatal("old response navigated away from the user's new destination")
				}
				if len(requests) != expectedResponses {
					t.Fatal("old response dispatched the cancelled account read")
				}
				for range expectedResponses {
					if req := <-requests; req.Entry == "工作/Figma" {
						t.Fatal("cancelled account read reached the backend")
					}
				}
			})
		}
	}
}

func TestPendingAccountClickCannotCrossVaultOrLockBoundary(t *testing.T) {
	for _, boundary := range []string{"lock", "metadata", "vault", "generation", "page", "setup"} {
		t.Run(boundary, func(t *testing.T) {
			a := fixture()
			a.state.VaultID = "synthetic-vault"
			a.busy, a.backgroundBusy = true, true
			a.selectEntry("工作/Figma")
			switch boundary {
			case "lock":
				a.clearSecrets()
				a.state.Locked = true
			case "metadata":
				a.dropDetail()
			case "vault":
				a.state.VaultID = "another-vault"
			case "generation":
				a.generation++
			case "page":
				a.page = "settings"
			case "setup":
				a.setup = true
			}
			a.busy = false
			a.flushPendingSelection() // No backend, so unsafe continuation fails.
			if a.pendingSelection != "" || a.busy {
				t.Fatal("pending account click survived its request boundary")
			}
		})
	}
}

func TestVaultSelectorTracksRepeatedAndExternalSwitches(t *testing.T) {
	a := fixture()
	a.state.VaultID = "personal"
	a.state.Vaults = []app.VaultInfo{{ID: "personal", Name: "个人密码库"}, {ID: "work", Name: "工作密码库"}}
	callbacks := make(chan func(), 1)
	state := a.state
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(req app.Request) app.Response {
		data := state
		data.VaultID = req.ID
		for _, vault := range state.Vaults {
			if vault.ID == req.ID {
				data.VaultName = vault.Name
			}
		}
		return app.Response{Status: "ok", Data: data}
	})
	view := ui.NewTester(testView(a), 1120, 760)
	for _, v := range []app.VaultInfo{{ID: "work", Name: "工作密码库"}, {ID: "personal", Name: "个人密码库"}} {
		if err := view.Click("切换密码库"); err != nil {
			t.Fatal(err)
		}
		if err := view.Click("切换到密码库 " + v.Name); err != nil {
			t.Fatal(err)
		}
		awaitMain(t, callbacks)
		view.Frame()
		if a.state.VaultID != v.ID || a.vaultSelection != v.ID || a.state.VaultName != v.Name {
			t.Fatalf("vault selector lost its current selected item: state=%s selected=%s", a.state.VaultID, a.vaultSelection)
		}
	}
	external := a.state
	external.VaultID, external.VaultName = "external", "外部密码库"
	a.receiveResponse(a.generation, app.Response{Status: "ok", Data: external}, nil)
	view.Frame()
	if a.vaultSelection != "external" || !view.HasText("外部密码库") {
		t.Fatal("external vault change left the old selected indicator")
	}
}

func TestInitialConfigurationFinishesBeforeOpeningWindow(t *testing.T) {
	for _, status := range []string{"ok", "error"} {
		t.Run(status, func(t *testing.T) {
			a := fixture()
			a.initializing = true
			a.receiveResponse(a.generation+1, app.Response{Status: status, Data: a.state}, nil)
			if !a.initializing {
				t.Fatal("a stale response released the first-paint gate")
			}
			called := false
			a.receiveResponse(a.generation, app.Response{Status: status, Message: "synthetic configuration error", Data: a.state}, func(app.Data) {
				called = true
				if !a.initializing {
					t.Fatal("window was released before configuration and theme callback finished")
				}
			})
			if a.initializing || called != (status == "ok") {
				t.Fatal("initial response did not release the first-paint gate after handling its result")
			}
			if status == "error" && a.errorText != "synthetic configuration error" {
				t.Fatal("initial failure was hidden instead of becoming a visible error")
			}
		})
	}
}

func TestSelectingCurrentEntryKeepsDetailWithoutFetching(t *testing.T) {
	a := fixture() // No backend: a redundant fetch would fail this test.
	before := a.detail
	a.selectEntry(a.selected)
	if a.busy || !reflect.DeepEqual(a.detail, before) {
		t.Fatal("selecting the current account cleared or reloaded its detail")
	}
}

func TestSelectingAnotherEntryShowsItsLoadingState(t *testing.T) {
	a := fixture()
	callbacks := make(chan func(), 1)
	state := a.state
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(req app.Request) app.Response {
		data := state
		data.Entry, data.Password = req.Entry, "synthetic-password"
		return app.Response{Status: "ok", Data: data}
	})
	a.selectEntry("工作/Figma")
	view := ui.NewTester(testView(a), 1120, 760)
	if view.HasText("正在读取账号…") || view.HasText("正在处理") || view.HasText("选择一个条目") || a.detail.Password != "" {
		t.Fatal("selection flashed the empty state or retained the previous secret")
	}
	a.detailLoadingStarted = time.Now().Add(-250 * time.Millisecond)
	view.Frame()
	if !view.HasText("正在读取账号…") || view.HasText("正在处理") {
		t.Fatal("slow selection did not show its delayed local loading feedback")
	}
	awaitMain(t, callbacks)
	view.Frame()
	if view.HasText("正在读取账号…") || a.detail.Entry != "工作/Figma" {
		t.Fatal("selection loading state did not finish")
	}
}

func TestSidebarSearchStillMatchesFoldersWithoutFilter(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("搜索条目"); err != nil {
		t.Fatal(err)
	}
	view.Type("开发")
	view.Frame()
	if len(a.entries()) != 2 || !view.HasText("2 / 4 个条目") || view.HasText("Figma") {
		t.Fatalf("folder text search or result count is incorrect: %v", view.Texts())
	}
}

func TestEditorFolderSelectionAndCreationPreserveDocument(t *testing.T) {
	a := fixture()
	a.editEntry()
	before := a.editor.contents
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("选择文件夹"); err != nil {
		t.Fatal(err)
	}
	view.Type("生活")
	if err := view.Click("选择文件夹 生活"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if a.editor.path != "生活/GitHub" || a.editor.contents != before || a.editor.originalPath != "开发/GitHub" {
		t.Fatalf("folder selection lost the account name, revision origin or document: path=%q folder=%q name=%q texts=%v", a.editor.path, a.editor.folder, a.editor.name, view.Texts())
	}
	if err := view.Click("选择文件夹"); err != nil {
		t.Fatal(err)
	}
	view.Type("新建/嵌套")
	if err := view.Click("新建文件夹 新建/嵌套"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if a.editor.path != "新建/嵌套/GitHub" || a.editor.contents != before {
		t.Fatalf("new folder did not preserve the entry: %q", a.editor.path)
	}
	if err := view.Click("账号名称"); err != nil {
		t.Fatal(err)
	}
	view.Key(ui.Cmd, ui.KeyA)
	view.Type("Personal")
	view.Frame()
	if a.editor.path != "新建/嵌套/Personal" || a.editor.contents != before {
		t.Fatal("editing the account name did not preserve its selected folder")
	}
}

func TestLockedListCopyContinuesOnlyAfterUnlock(t *testing.T) {
	a := fixture()
	a.state.VaultID, a.state.Locked = "synthetic-vault", true
	a.detail = app.Data{}
	callbacks := make(chan func(), 3)
	requests := make(chan app.Request, 3)
	state := a.state
	state.Locked = false
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(req app.Request) app.Response {
		requests <- req
		data := state
		if req.Action == "fetch" {
			data.Entry = req.Entry
		}
		return app.Response{Status: "ok", Data: data}
	})
	a.copyEntry("开发/GitHub")
	if len(requests) != 0 || a.pendingCopyEntry != "开发/GitHub" || a.selected != "开发/GitHub" {
		t.Fatal("locked copy did not wait at the normal unlock screen")
	}
	a.unlock("")
	for range 3 {
		awaitMain(t, callbacks)
	}
	var actions []string
	for range 3 {
		req := <-requests
		actions = append(actions, req.Action)
		if req.Action == "copy" && (req.Entry != "开发/GitHub" || req.Field != "password") {
			t.Fatal("unlock copied another account or field")
		}
	}
	if strings.Join(actions, ",") != "unlock,copy,fetch" || a.pendingCopyEntry != "" {
		t.Fatalf("unexpected unlocked copy sequence: %v", actions)
	}
	view := ui.NewTester(testView(a), 1120, 760)
	if !view.HasText("已复制密码") {
		t.Fatal("follow-up fetch swallowed the copy confirmation")
	}
}

func TestPendingListCopyIsClearedWithSecrets(t *testing.T) {
	a := fixture()
	a.pendingCopyEntry, a.pendingCopyVaultID = "开发/GitHub", "synthetic-vault"
	a.clearSecrets()
	if a.pendingCopyEntry != "" || a.pendingCopyVaultID != "" {
		t.Fatal("pending copy survived a lock or vault switch")
	}
}

func TestSelectingAnotherLockedEntryCancelsPendingCopy(t *testing.T) {
	a := fixture()
	a.state.VaultID, a.state.Locked = "synthetic-vault", true
	a.detail = app.Data{}
	state := a.state
	state.Locked = false
	callbacks := make(chan func(), 3)
	requests := make(chan app.Request, 3)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(req app.Request) app.Response {
		requests <- req
		data := state
		if req.Action == "fetch" {
			data.Entry = req.Entry
		}
		return app.Response{Status: "ok", Data: data}
	})
	a.copyEntry("开发/GitHub")
	a.selectEntry("工作/Figma")
	a.unlock("")
	awaitMain(t, callbacks)
	awaitMain(t, callbacks)
	first, second := <-requests, <-requests
	if first.Action != "unlock" || second.Action != "fetch" || second.Entry != "工作/Figma" {
		t.Fatalf("superseded copy was resumed instead of the current selection: %s, %s %s", first.Action, second.Action, second.Entry)
	}
	if a.pendingCopyEntry != "" || a.detail.Entry != "工作/Figma" {
		t.Fatal("unlock did not keep the newly selected account")
	}
}

func TestNavigationCancelsPendingLockedCopy(t *testing.T) {
	for _, action := range []string{"navigate", "setup", "metadata"} {
		t.Run(action, func(t *testing.T) {
			a := fixture()
			a.state.VaultID, a.state.Locked = "synthetic-vault", true
			a.copyEntry("开发/GitHub")
			switch action {
			case "navigate":
				a.navigate("entries", "all")
			case "setup":
				a.openSetup()
			case "metadata":
				a.dropDetail()
			}
			if a.pendingCopyEntry != "" || a.pendingCopyVaultID != "" {
				t.Fatal("superseded navigation retained a pending credential copy")
			}
		})
	}
}

func TestLockedNarrowUnlockControlsFitDetailPane(t *testing.T) {
	for _, device := range []bool{false, true} {
		a := fixture()
		a.state.Locked = true
		a.state.DeviceUnlockAvailable, a.state.DeviceUnlockEnrolled, a.state.Settings.DeviceUnlock = device, device, device
		view := ui.NewTester(testView(a), 860, 620)
		label := "打开其他密码库"
		if device {
			label = "使用 Touch ID 或 Mac 登录密码"
		}
		button, ok := view.Find(label)
		if !ok || button.X < 310 || button.X+button.W > 860 {
			t.Fatalf("narrow unlock control overflowed its detail pane: %q %v", label, button)
		}
	}
}
