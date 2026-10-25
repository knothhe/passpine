package desktop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func folderCleanupFixture(t *testing.T, language string, width int) (*application, *app.Service, *ui.Tester, func()) {
	t.Helper()
	service, err := app.New(app.Options{DataDir: t.TempDir(), Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	settings := service.Execute(app.Request{Action: "configure"}).Data.Settings
	settings.Language = language
	if r := service.Execute(app.Request{Action: "settings", Settings: &settings}); r.Status != "ok" {
		t.Fatal(r.Message)
	}
	a := newApplication(service)
	a.state = service.Execute(app.Request{Action: "configure"}).Data
	callbacks := make(chan func(), 16)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	view := ui.NewTester(testView(a), width, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	settle := func() {
		t.Helper()
		for a.busy {
			awaitMain(t, callbacks)
		}
		view.Frame()
		view.Advance(time.Second)
	}
	return a, service, view, settle
}

func TestFolderCleanupUIWithRealVault(t *testing.T) {
	for _, test := range []struct {
		language string
		width    int
	}{{"zh-CN", 1180}, {"en", 960}} {
		t.Run(test.language, func(t *testing.T) {
			a, service, view, settle := folderCleanupFixture(t, test.language, test.width)
			if r := service.Execute(app.Request{Action: "save", Entry: "test/a/b/c/d", Contents: "keep-this-secret", Mode: "create"}); r.Status != "ok" {
				t.Fatal(r.Message)
			}
			if r := service.Execute(app.Request{Action: "move", Entry: "test/a/b/c/d", Destination: "test/a/b/d"}); r.Status != "ok" {
				t.Fatal(r.Message)
			}
			a.state = service.Execute(app.Request{Action: "list"}).Data
			a.entriesVersion = a.state.EntriesVersion
			a.filter = "folder:test/a/b/c"
			view.Frame()
			if _, found := view.Find(a.t("清理空文件夹")); !found {
				if err := view.Click(a.t("切换侧栏")); err != nil {
					t.Fatal(err)
				}
				view.Advance(time.Second)
			}
			managerResizeScreenshot(t, view, "folder-cleanup-button-"+test.language)
			if err := view.Click(a.t("清理空文件夹")); err != nil {
				t.Fatal(err)
			}
			a.cleanupEmptyFolders() // A second request while busy must be ignored.
			settle()
			if a.filter != "all" || !view.HasText(a.t("已清理 %d 个空文件夹", 1)) {
				t.Fatal("cleanup did not refresh navigation and show its count")
			}
			if _, err := os.Stat(filepath.Join(a.state.Directory, "test/a/b/c")); !os.IsNotExist(err) {
				t.Fatal("cleanup left the empty source folder behind", err)
			}
			moved := service.Execute(app.Request{Action: "fetch", Entry: "test/a/b/d"})
			if moved.Status != "ok" || moved.Data.Contents != "keep-this-secret" {
				t.Fatal("cleanup changed the moved entry")
			}
			activity := service.Execute(app.Request{Action: "activity"})
			cleanups := 0
			for _, record := range activity.Data.Activity {
				if record.Action == "cleanup-folders" {
					cleanups++
				}
			}
			if cleanups != 1 {
				t.Fatalf("duplicate click sent %d cleanup requests", cleanups)
			}
			managerResizeScreenshot(t, view, "folder-cleanup-complete-"+test.language)
			a.cleanupEmptyFolders()
			settle()
			if !view.HasText(a.t("没有需要清理的空文件夹")) {
				t.Fatal("no-op cleanup did not report its outcome")
			}
		})
	}
}

func TestFolderCleanupWhileLockedDoesNotAuthenticate(t *testing.T) {
	a, service, view, settle := folderCleanupFixture(t, "zh-CN", 1180)
	if r := service.Execute(app.Request{Action: "create-folder", Destination: "empty/a/b"}); r.Status != "ok" {
		t.Fatal(r.Message)
	}
	a.state = service.Execute(app.Request{Action: "lock"}).Data
	a.entriesVersion = a.state.EntriesVersion
	// Advertise system authentication as available so any accidental unlock
	// continuation would issue a device request through this rejecting backend.
	a.state.Settings.DeviceUnlock = true
	a.state.DeviceUnlockEnrolled = true
	a.state.DeviceUnlockAvailable = true
	lockedState := a.state
	requests := make(chan app.Request, 4)
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		if request.Action != "cleanup-folders" {
			return app.Response{Status: "error", Code: "unexpected_authentication", Data: lockedState}
		}
		return service.Execute(request)
	})
	view.Frame()
	if err := view.Click("清理空文件夹"); err != nil {
		t.Fatal(err)
	}
	settle()
	if !a.state.Locked || a.pendingUnlock.action != "" || !view.HasText("已清理 3 个空文件夹") {
		t.Fatal("cleanup did not complete while retaining the locked session")
	}
	if request := <-requests; request.Action != "cleanup-folders" || request.Mode != "" || request.Passphrase != "" {
		t.Fatalf("cleanup requested authentication: %#v", request)
	}
	if len(requests) != 0 {
		t.Fatal("cleanup issued extra backend requests")
	}
	if _, err := os.Stat(filepath.Join(a.state.Directory, "empty")); !os.IsNotExist(err) {
		t.Fatal("locked cleanup left empty directories", err)
	}
}

func TestFolderCleanupDisabledDuringEditingAndSelection(t *testing.T) {
	for _, mode := range []string{"edit", "batch"} {
		a := fixture()
		requests := 0
		a.service = backendFunc(func(app.Request) app.Response {
			requests++
			return app.Response{Status: "ok", Data: a.state}
		})
		if mode == "edit" {
			a.editor.open = true
		} else {
			a.enterBatch()
		}
		view := ui.NewTester(testView(a), 1180, 760)
		if err := view.Click("清理空文件夹"); err != nil {
			t.Fatal(err)
		}
		a.cleanupEmptyFolders()
		if a.busy || requests != 0 {
			t.Fatal("cleanup could remove a folder being edited or selected")
		}
	}
}

func TestFolderCleanupPreservesRecoveryNavigation(t *testing.T) {
	for _, status := range []string{"ok", "error"} {
		t.Run(status, func(t *testing.T) {
			a := fixture()
			a.state.VaultID = "current-vault"
			a.state.Folders = []string{"removed"}
			a.filter = "folder:removed"
			data := a.state
			data.Folders = nil
			data.EntriesVersion++
			callbacks := make(chan func(), 2)
			requests := make(chan app.Request, 2)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.service = backendFunc(func(request app.Request) app.Response {
				requests <- request
				response := app.Response{Status: "ok", Data: data}
				if request.Action == "cleanup-folders" {
					response.Status = status
					response.Data.RemovedFolders = 1
					if status == "error" {
						response.Code = "operation_failed"
					}
				} else if request.Action == "recovery" {
					response.Data.Recovery = []app.RecoveryItem{{ID: "trash-item", Kind: "trash", Path: "deleted/account"}}
				}
				return response
			})
			view := ui.NewTester(testView(a), 1180, 760)
			if err := view.Click("清理空文件夹"); err != nil {
				t.Fatal(err)
			}
			if err := view.Click("回收站"); err != nil {
				t.Fatal(err)
			}
			if a.page != "recovery" || a.preferences.pendingRead == nil {
				t.Fatal("recovery navigation did not queue its list behind cleanup")
			}
			awaitMain(t, callbacks)
			if a.page != "recovery" || a.recoveryKind != 0 {
				t.Fatal("cleanup replaced the newer recovery destination")
			}
			if a.filter != "all" || containsFolder(a.folders, "removed") {
				t.Fatal("cleanup did not refresh the deleted folder while preserving recovery")
			}
			awaitMain(t, callbacks)
			first, second := <-requests, <-requests
			if first.Action != "cleanup-folders" || second.Action != "recovery" || second.Kind != "trash" || second.VaultID != "current-vault" {
				t.Fatalf("unexpected request sequence: %#v then %#v", first, second)
			}
			view.Frame()
			if a.page != "recovery" || a.busy || len(a.recovery) != 1 || a.recovery[0].ID != "trash-item" || !view.HasText("deleted/account") {
				t.Fatal("recovery did not receive and display its queued list after cleanup")
			}
		})
	}
}

func TestFolderCleanupFailureShowsFeedbackAfterNavigationRefresh(t *testing.T) {
	for _, removed := range []int{0, 2} {
		a := fixture()
		a.filter = "folder:removed"
		data := a.state
		data.RemovedFolders = removed
		a.service = backendFunc(func(app.Request) app.Response {
			return app.Response{Status: "error", Code: "operation_failed", Data: data}
		})
		callbacks := make(chan func(), 1)
		a.runOnMain = func(callback func()) { callbacks <- callback }
		a.cleanupEmptyFolders()
		awaitMain(t, callbacks)
		view := ui.NewTester(testView(a), 1180, 760)
		want := a.t("无法清理空文件夹，请检查目录权限后重试。")
		if removed > 0 {
			want = a.t("已清理 %d 个空文件夹，清理未完成，请重试。", removed)
		}
		if a.filter != "all" || !view.HasText(want) {
			t.Fatal("cleanup failure was hidden by navigation refresh")
		}
	}
}
