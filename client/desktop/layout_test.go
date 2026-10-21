package desktop

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
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

func TestClientManagerHasNavigationListAndDetail(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 1120, 760)
	navigation, ok := view.Find("文件夹 开发")
	if !ok || navigation.X < 0 || navigation.X+navigation.W > a.shell.LeftWidth() {
		t.Fatalf("folder navigation is outside the left sidebar: %v", navigation)
	}
	row, ok := view.Find("账号 开发/GitHub")
	if !ok || row.X < a.shell.LeftWidth() {
		t.Fatalf("account rows are not in the middle column: %v", row)
	}
	detail, ok := view.Find("显示或隐藏密码")
	if !ok || detail.X < row.X+row.W {
		t.Fatal("detail is not beside the middle entry column")
	}
	for _, label := range []string{"加密草稿", "回收站", "设置"} {
		control, found := view.Find(label)
		if !found || control.X+control.W > a.shell.LeftWidth() || control.Y < 600 {
			t.Fatalf("sidebar footer control is missing or not fixed at the bottom: %s %v", label, control)
		}
	}
	count, ok := view.Find("条目数量")
	if !ok || count.X < a.shell.LeftWidth() || count.X+count.W > row.X+row.W+20 || count.Y < 700 {
		t.Fatalf("entry count is not in the middle column footer: %v", count)
	}
}

func TestFolderNavigationIncludesDescendantsAndSearchFiltersScope(t *testing.T) {
	a := fixture()
	a.state.Entries = append(a.state.Entries, app.Entry{Path: "开发/服务/API", Name: "API", Folder: "开发/服务"})
	a.state.Entries = append(a.state.Entries, app.Entry{Path: "家庭/设备/NAS", Name: "NAS", Folder: "家庭/设备"})
	view := ui.NewTester(testView(a), 1120, 760)
	for _, folder := range []string{"开发/服务", "家庭/设备"} {
		if _, found := view.Find("文件夹 " + folder); found {
			t.Fatalf("nested folder %q should not appear in the single-level sidebar", folder)
		}
	}
	if _, found := view.Find("文件夹 家庭"); !found {
		t.Fatal("a folder containing only nested entries is missing from the sidebar")
	}
	if err := view.Click("文件夹 开发"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if a.filter != "folder:开发" || len(a.filtered) != 3 {
		t.Fatalf("parent folder did not include its descendant accounts: filter=%q entries=%+v", a.filter, a.filtered)
	}
	if _, found := view.Find("账号 开发/服务/API"); !found {
		t.Fatal("the selected folder omitted its nested account")
	}
	if err := view.Click("搜索条目"); err != nil {
		t.Fatal(err)
	}
	view.Type("cloud")
	if len(a.filtered) != 1 || a.filtered[0].Name != "Cloudflare" {
		t.Fatal("search did not filter the current folder")
	}
	if err := view.Click("全部条目"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if a.filter != "all" {
		t.Fatal("the all-entries navigation kept the previous folder scope")
	}
}

func TestNarrowManagerKeepsListAndDetailWithSidebarOverlay(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 860, 620)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	if !a.shell.LeftOverlayActive() || a.shell.LeftExpanded() {
		t.Fatal("the narrow window did not collapse its folder navigation")
	}
	row, ok := view.Find("账号 开发/GitHub")
	detail, detailFound := view.Find("显示或隐藏密码")
	if !ok || !detailFound || row.X < 0 || detail.X < row.X+row.W || detail.X+detail.W > 860 {
		t.Fatalf("narrow layout did not retain visible list and detail columns: row=%v detail=%v", row, detail)
	}
	if err := view.Click("切换侧栏"); err != nil {
		t.Fatal(err)
	}
	if !a.shell.LeftExpanded() {
		t.Fatal("the sidebar toggle did not open the narrow navigation overlay")
	}
	if after, found := view.Find("显示或隐藏密码"); !found || after != detail {
		t.Fatal("opening the navigation overlay moved the detail column")
	}
	view.Key(0, ui.KeyEscape)
	if a.shell.LeftExpanded() {
		t.Fatal("Escape did not dismiss the narrow navigation overlay")
	}
}

func TestSidebarCollapseAndResizeKeepEditorSearchAndSelection(t *testing.T) {
	a := fixture()
	a.filter, a.query = "folder:开发", "git"
	a.editEntry()
	view := ui.NewTester(testView(a), 1120, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	if err := view.Click("账号名称"); err != nil {
		t.Fatal(err)
	}
	view.Key(ui.Cmd, ui.KeyA)
	view.Type("GitHub edited")
	identity, path, contents := a.editor.identity, a.editor.path, a.editor.contents
	assertRetained := func() {
		t.Helper()
		if !a.editor.open || a.editor.identity != identity || a.editor.path != path || a.editor.contents != contents || a.query != "git" || a.filter != "folder:开发" || a.selected != "开发/GitHub" {
			t.Fatal("sidebar presentation changed the current scope, query, selection or editor input")
		}
		if _, found := view.Find("账号名称"); !found {
			t.Fatal("the retained editor disappeared from the detail column")
		}
	}
	for range 2 {
		if err := view.Click("切换侧栏"); err != nil {
			t.Fatal(err)
		}
		assertRetained()
	}
	view.SetSize(860, 620)
	assertRetained()
	view.SetSize(1120, 760)
	assertRetained()
}

func TestThreeColumnSettingsAndDetailIconsRemainInteractive(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 1120, 760)
	for _, label := range []string{"显示全部字段", "隐藏全部字段"} {
		if err := view.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if a.reveal {
		t.Fatal("the detail visibility icons stopped updating the reveal state")
	}
	if err := view.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if a.page != "settings" || !view.HasText("通用") {
		t.Fatal("the settings icon did not open the existing settings pages")
	}
	if err := view.Click("后退"); err != nil {
		t.Fatal(err)
	}
	view.Advance(time.Second)
	if a.page != "entries" {
		t.Fatal("the retained history icon did not return to the manager")
	}
	if err := view.Click("编辑条目"); err != nil {
		t.Fatal(err)
	}
	if !a.editor.open {
		t.Fatal("the detail edit icon did not open its inline editor")
	}
}

func TestThreeColumnScrollAreasKeepFootersAndOtherColumnsStill(t *testing.T) {
	a := fixture()
	for i := range 60 {
		folder := fmt.Sprintf("更多文件夹%02d", i)
		a.state.Entries = append(a.state.Entries, app.Entry{Path: folder + "/Account", Name: "Account", Folder: folder})
	}
	a.detail.Notes = strings.Repeat("较长备注用于验证三栏独立滚动。", 200)
	view := ui.NewTester(testView(a), 1120, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	footer, footerFound := view.Find("回收站")
	count, countFound := view.Find("条目数量")
	if !footerFound || !countFound {
		t.Fatal("navigation or list footer is missing")
	}
	folder, folderFound := view.Find("文件夹 更多文件夹00")
	if !folderFound {
		t.Fatal("the long folder navigation fixture is missing")
	}
	first, last := a.list.Visible()
	view.Scroll(100, 350, 0, 1000)
	if after, found := view.Find("文件夹 更多文件夹00"); !found || after.Y >= folder.Y {
		t.Fatal("the long folder navigation could not scroll")
	}
	if nextFirst, nextLast := a.list.Visible(); nextFirst != first || nextLast != last || a.detailScroll.Y != 0 {
		t.Fatal("scrolling folders moved the account list or detail")
	}
	if after, found := view.Find("回收站"); !found || after != footer {
		t.Fatal("scrolling folders moved their fixed recovery footer")
	}
	view.Scroll(350, 350, 0, 800)
	if nextFirst, _ := a.list.Visible(); nextFirst <= first {
		t.Fatal("the middle account list could not scroll")
	}
	first, last = a.list.Visible()
	if a.detailScroll.Y != 0 {
		t.Fatal("scrolling the account list moved detail content")
	}
	view.Scroll(900, 400, 0, 800)
	if a.detailScroll.Y <= 0 {
		t.Fatal("long detail content could not scroll independently")
	}
	if nextFirst, nextLast := a.list.Visible(); nextFirst != first || nextLast != last {
		t.Fatal("scrolling detail moved the account list")
	}
	if after, found := view.Find("条目数量"); !found || after != count {
		t.Fatal("scrolling the account list or detail moved the fixed list footer")
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
	a.detailLoadingStarted = view.Now().Add(-250 * time.Millisecond)
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
			label = "使用系统验证"
		}
		button, ok := view.Find(label)
		if !ok || button.X < defaultSidebarWidth || button.X+button.W > 860 {
			t.Fatalf("narrow unlock control overflowed its detail pane: %q %v", label, button)
		}
	}
}
