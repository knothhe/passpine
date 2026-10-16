package desktop

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestQuickSearchHeaderActionsReloadEntriesWithoutChangingManagerOrDraft(t *testing.T) {
	for _, mode := range []string{quickSearchGlobal, quickSearchTray} {
		t.Run(quickSearchActionModeName(mode), func(t *testing.T) {
			a, _ := quickSearchFixture()
			a.quick.mode, a.quick.query = mode, "GitHub"
			a.page, a.query, a.filter = "activity", "manager query", "folder:工作"
			a.editor = editorState{open: true, contents: "unsaved secret", originalContents: "previous secret", path: "work/draft"}
			editor, revision, epoch := a.editor, a.history.revision, a.metadataEpoch
			data := a.state
			data.EntriesVersion = a.entriesVersion + 1
			data.Entries = []app.Entry{{Path: "开发/GitHub-Updated", Name: "GitHub Updated", Folder: "开发"}}
			callbacks, requests := make(chan func(), 1), make(chan app.Request, 2)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.service = backendFunc(func(request app.Request) app.Response {
				requests <- request
				return app.Response{Status: "ok", Data: data}
			})
			view := quickSearchActionsView(a, mode)
			for _, label := range []string{"刷新条目", "打开主窗口", "打开设置"} {
				if _, ok := view.Find(label); !ok {
					t.Fatalf("shared quick-window header lacks %q", label)
				}
			}
			if err := view.Click("刷新条目"); err != nil {
				t.Fatal(err)
			}
			awaitMain(t, callbacks)
			request := <-requests
			if request.Action != "configure" || request.VaultID != a.state.VaultID || request.Mode != "" || request.Passphrase != "" || request.Entry != "" || request.Field != "" {
				t.Fatalf("entry refresh requested authentication or a credential operation: %+v", request)
			}
			view.Frame()
			if !view.HasText("GitHub Updated") || !a.quick.visible || a.quick.mode != mode || a.quick.query != "GitHub" || a.quick.operation != "" || a.busy {
				t.Fatal("entry refresh did not replace results in the same open quick window")
			}
			if a.page != "activity" || a.query != "manager query" || a.filter != "folder:工作" || a.editor != editor || a.history.revision != revision {
				t.Fatal("quick-window refresh changed manager navigation, query or its unsaved editor")
			}
			if a.metadataEpoch != epoch+1 || a.detail.Contents != "" || a.cacheReady {
				t.Fatal("changed entry metadata did not invalidate stale manager detail and caches")
			}
			if len(requests) != 0 {
				t.Fatal("refresh chained a credential request")
			}
		})
	}
}

func TestQuickSearchHeaderRefreshFailureRemainsInPanelAndCanRetry(t *testing.T) {
	a, _ := quickSearchFixture()
	a.quick.mode, a.quick.query = quickSearchTray, "GitHub"
	data := a.state
	callbacks, requests := make(chan func(), 2), make(chan app.Request, 2)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	first := true
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		if first {
			first = false
			return app.Response{Status: "error", Data: data, Message: "refresh failed"}
		}
		data.EntriesVersion++
		data.Entries = []app.Entry{{Path: "GitHub-Retried", Name: "GitHub Retried"}}
		return app.Response{Status: "ok", Data: data}
	})
	view := quickSearchActionsView(a, quickSearchTray)
	if err := view.Click("刷新条目"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	view.Frame()
	if !view.HasText("refresh failed") || !a.quick.visible || a.busy || a.quick.operation != "" || a.errorText != "" {
		t.Fatal("refresh failure did not produce retryable panel feedback")
	}
	if err := view.Click("刷新条目"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	view.Frame()
	if !view.HasText("GitHub Retried") || view.HasText("refresh failed") || !a.quick.visible || a.quick.query != "GitHub" {
		t.Fatal("retry did not refresh results or clear the previous panel failure")
	}
	for range 2 {
		if request := <-requests; request.Action != "configure" {
			t.Fatalf("refresh retry requested %q", request.Action)
		}
	}
}

func TestQuickSearchHeaderRefreshDoesNotAuthenticateLockedVault(t *testing.T) {
	a, _ := quickSearchFixture()
	a.state.Locked, a.state.Settings.BrowseWhileLocked = true, false
	data := a.state
	data.Entries = nil
	callbacks, requests := make(chan func(), 1), make(chan app.Request, 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: data}
	})
	a.refreshQuickSearchEntries()
	awaitMain(t, callbacks)
	if request := <-requests; request.Action != "configure" || request.Passphrase != "" || request.Mode != "" {
		t.Fatalf("locked refresh attempted authentication: %+v", request)
	}
	if !a.state.Locked || a.quick.pendingCopy.entry != "" || a.quick.operation != "" {
		t.Fatal("locked refresh changed authentication or copy state")
	}
}

func TestQuickSearchHeaderRefreshAndCopyAreDisabledWhileBusy(t *testing.T) {
	a, _ := quickSearchFixture()
	a.quick.mode = quickSearchTray
	data := a.state
	callbacks, requests := make(chan func(), 1), make(chan app.Request, 1)
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		<-release
		return app.Response{Status: "ok", Data: data}
	})
	view := quickSearchActionsView(a, quickSearchTray)
	if err := view.Click("刷新条目"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-requests:
	case <-time.After(3 * time.Second):
		t.Fatal("refresh request did not start")
	}
	if !a.busy || a.quick.operation != "configure" {
		t.Fatal("refresh did not acquire the panel operation lane")
	}
	requestID := a.quick.request
	for _, label := range []string{"刷新条目", "复制密码"} {
		if err := view.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if a.quick.request != requestID || len(requests) != 0 {
		t.Fatal("busy refresh/copy controls started another operation")
	}
	close(release)
	awaitMain(t, callbacks)
}

func TestQuickSearchHeaderRefreshPreservesSelectedPathAfterSortedInsert(t *testing.T) {
	a, _ := quickSearchFixture()
	a.prepareQuickSearchList()
	selectedPath := "工作/Figma"
	for index, entry := range a.quick.results {
		if entry.Path == selectedPath {
			a.quick.selected = index
			break
		}
	}
	beforeIndex := a.quick.selected
	data := a.state
	data.EntriesVersion = a.entriesVersion + 1
	data.Entries = append([]app.Entry{{Path: "000/Added", Name: "Added", Folder: "000"}}, data.Entries...)
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		if request.Action != "configure" {
			t.Errorf("refresh requested %q", request.Action)
		}
		return app.Response{Status: "ok", Data: data}
	})
	a.refreshQuickSearchEntries()
	awaitMain(t, callbacks)
	if a.quick.selected == beforeIndex || a.quick.selected >= len(a.quick.results) || a.quick.results[a.quick.selected].Path != selectedPath {
		t.Fatal("refresh kept a numeric index instead of the selected entry path")
	}
	if !a.quick.refreshedUntil.After(time.Now()) {
		t.Fatal("active refresh did not produce short-lived completion feedback")
	}
}

func TestQuickSearchHeaderLateRefreshCannotReviveDismissedOrLockedPanel(t *testing.T) {
	for _, action := range []string{"hide", "lock"} {
		t.Run(action, func(t *testing.T) {
			a, window := quickSearchFixture()
			a.state.Locked = false
			data := a.state
			data.EntriesVersion = a.entriesVersion + 1
			data.Password, data.Contents = "late secret", "late decrypted document"
			callbacks, requests := make(chan func(), 1), make(chan app.Request, 1)
			release := make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.service = lifecycleLockBackend{execute: func(request app.Request) app.Response {
				requests <- request
				<-release
				return app.Response{Status: "ok", Data: data}
			}}
			a.refreshQuickSearchEntries()
			select {
			case <-requests:
			case <-time.After(3 * time.Second):
				t.Fatal("held refresh never started")
			}
			a.quick.passphrase = "private"
			a.quick.pendingCopy = quickSearchCopy{entry: "pending", vaultID: a.state.VaultID, generation: a.generation}
			if action == "hide" {
				a.hideQuickSearch()
			} else {
				a.handleSessionLock()
			}
			visible := a.quick.visible
			close(release)
			awaitMain(t, callbacks)
			if a.quick.visible != visible || window.shown != 0 || window.focused != 0 || !a.quick.refreshedUntil.IsZero() {
				t.Fatal("late refresh reopened, focused or produced success feedback for an invalid invocation")
			}
			if a.quick.passphrase != "" || a.quick.pendingCopy.entry != "" || a.state.Password != "" || a.state.Contents != "" || a.detail.Password != "" || a.detail.Contents != "" {
				t.Fatal("late refresh retained panel credentials or decrypted response data")
			}
			if action == "lock" && !a.state.Locked {
				t.Fatal("late refresh restored the unlocked snapshot after session lock")
			}
		})
	}
}

func TestQuickSearchHeaderRefreshDiscoveringLockDoesNotReportSuccess(t *testing.T) {
	a, window := quickSearchFixture()
	a.state.Locked, a.state.Settings.BrowseWhileLocked = false, false
	a.quick.passphrase = "private"
	a.quick.pendingCopy = quickSearchCopy{entry: "pending", vaultID: a.state.VaultID, generation: a.generation}
	data := a.state
	data.Locked, data.Entries = true, nil
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(app.Request) app.Response { return app.Response{Status: "ok", Data: data} })
	a.refreshQuickSearchEntries()
	awaitMain(t, callbacks)
	if !a.state.Locked || !a.quick.refreshedUntil.IsZero() || a.quick.passphrase != "" || a.quick.pendingCopy.entry != "" || len(a.quick.results) != 0 {
		t.Fatal("refresh that discovered a lock retained old invocation state or reported success")
	}
	if window.shown != 0 || window.focused != 0 {
		t.Fatal("refresh that discovered a lock unexpectedly reopened or focused the panel")
	}
}

func TestQuickSearchHeaderActionsFitNarrowWindowsAtTextSizeExtremes(t *testing.T) {
	for _, width := range []int{320, 420, 660} {
		for _, scale := range []int{80, 120} {
			for _, locale := range []string{"zh-CN", "en"} {
				t.Run(fmt.Sprintf("%d/%d/%s", width, scale, locale), func(t *testing.T) {
					a, _ := quickSearchFixture()
					a.state.Settings.FontScale, a.state.Settings.Language = scale, locale
					a.state.VaultName = strings.Repeat("Long Password Vault 密码库 ", 8)
					if width < 660 {
						a.quick.mode = quickSearchTray
					}
					view := ui.NewTester(a.quickSearchView, width, 440)
					var previous ui.Rect
					for index, source := range []string{"关闭快速搜索", "刷新条目", "打开主窗口", "打开设置"} {
						label := a.t(source)
						bounds, ok := view.Find(label)
						if !ok || bounds.W <= 0 || bounds.H <= 0 || bounds.X < 0 || bounds.Y < 0 || bounds.X+bounds.W > float32(width)+.5 || bounds.Y+bounds.H > 38.5 {
							t.Fatalf("header action %q is outside %dpx viewport: %+v", label, width, bounds)
						}
						if index > 0 && bounds.X < previous.X+previous.W-.5 {
							t.Fatalf("header actions overlap at %d%%: previous=%+v, current=%+v", scale, previous, bounds)
						}
						previous = bounds
					}
				})
			}
		}
	}
}

func TestQuickSearchHeaderOpenManagerKeepsContextAndClearsPanelSecrets(t *testing.T) {
	for _, mode := range []string{quickSearchGlobal, quickSearchTray} {
		t.Run(quickSearchActionModeName(mode), func(t *testing.T) {
			a, window := quickSearchFixture()
			a.quick.mode, a.quick.query = mode, "panel query"
			a.quick.passphrase = "private"
			a.quick.pendingCopy = quickSearchCopy{entry: "pending", vaultID: a.state.VaultID, generation: a.generation}
			a.page, a.query, a.filter = "activity", "manager query", "folder:工作"
			a.editor = editorState{open: true, contents: "unsaved secret"}
			editor, revision := a.editor, a.history.revision
			a.initializing, a.lifecycle.resumeTarget = true, "tray"
			view := quickSearchActionsView(a, mode)
			if err := view.Click("打开主窗口"); err != nil {
				t.Fatal(err)
			}
			if a.quick.visible || window.hidden != 1 || a.quick.passphrase != "" || a.quick.pendingCopy.entry != "" || a.lifecycle.resumeTarget != "main" {
				t.Fatal("header manager action did not dismiss and clear the panel then request the manager")
			}
			if a.page != "activity" || a.query != "manager query" || a.filter != "folder:工作" || a.editor != editor || a.history.revision != revision {
				t.Fatal("header manager action imported the panel query or changed the manager's context")
			}
		})
	}
}

func TestQuickSearchHeaderOpenSettingsKeepsDraftAndClearsPanelSecrets(t *testing.T) {
	for _, mode := range []string{quickSearchGlobal, quickSearchTray} {
		t.Run(quickSearchActionModeName(mode), func(t *testing.T) {
			a, window := quickSearchFixture()
			a.quick.mode, a.quick.passphrase = mode, "private"
			a.quick.pendingCopy = quickSearchCopy{entry: "pending", vaultID: a.state.VaultID, generation: a.generation}
			a.editor = editorState{open: true, contents: "unsaved secret"}
			editor := a.editor
			a.initializing, a.lifecycle.resumeTarget = true, "tray"
			view := quickSearchActionsView(a, mode)
			if err := view.Click("打开设置"); err != nil {
				t.Fatal(err)
			}
			if a.page != "settings" || a.editor != editor || a.lifecycle.resumeTarget != "main" {
				t.Fatal("header settings action did not navigate and request the manager while preserving its draft")
			}
			if a.quick.visible || window.hidden != 1 || a.quick.passphrase != "" || a.quick.pendingCopy.entry != "" {
				t.Fatal("header settings action retained panel secrets or visibility")
			}
		})
	}
}

func TestQuickSearchHeaderRefreshGuardsUnavailableStates(t *testing.T) {
	for _, state := range []string{"busy", "closing", "no service", "hidden", "unconfigured"} {
		t.Run(state, func(t *testing.T) {
			a, _ := quickSearchFixture()
			a.service = backendFunc(func(app.Request) app.Response {
				t.Error("unavailable panel started a refresh")
				return app.Response{}
			})
			switch state {
			case "busy":
				a.busy = true
			case "closing":
				a.closing = true
			case "no service":
				a.service = nil
			case "hidden":
				a.quick.visible = false
			case "unconfigured":
				a.state.Configured = false
			}
			beforeBusy, requestID := a.busy, a.quick.request
			a.refreshQuickSearchEntries()
			if a.quick.request != requestID || a.busy != beforeBusy || a.quick.operation != "" {
				t.Fatal("refresh guard changed the panel operation state")
			}
		})
	}
}

func TestQuickSearchHeaderNavigationDoesNothingDuringQuit(t *testing.T) {
	for _, action := range []string{"manager", "settings"} {
		t.Run(action, func(t *testing.T) {
			a, window := quickSearchFixture()
			a.closing = true
			page, requestID := a.page, a.quick.request
			if action == "manager" {
				a.showQuickSearchManager()
			} else {
				a.openQuickSearchSettings()
			}
			if a.page != page || a.quick.request != requestID || !a.quick.visible || window.hidden != 0 {
				t.Fatal("header navigation changed UI state while quitting")
			}
		})
	}
}

func quickSearchActionsView(a *application, mode string) *ui.Tester {
	width, height := 660, 460
	if mode == quickSearchTray {
		width, height = 420, 440
	}
	return ui.NewTester(a.quickSearchView, width, height)
}

func quickSearchActionModeName(mode string) string {
	if mode == quickSearchTray {
		return "tray"
	}
	return "global"
}
