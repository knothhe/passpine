package desktop

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func batchUndoFixture(t *testing.T, action string, partial bool) (*application, *batchBackend, func(), []string) {
	t.Helper()
	paths := []string{"开发/Cloudflare", "工作/Figma"}
	if action == "strip" {
		paths = []string{"test/a/b", "test/b/c"}
	}
	failures := map[string]string{}
	if partial {
		failures[paths[1]] = "vault_busy"
	}
	var a *application
	var backend *batchBackend
	var settle func()
	if action == "strip" {
		a, backend, settle = moveFixture(t, append([]string(nil), paths...), failures)
		a.chooseBatchFolder("next")
	} else {
		a, backend, settle = batchFixture(t, failures)
		a.enterBatch()
		for _, path := range paths {
			a.batch.selected[path] = true
		}
		a.chooseBatchFolder("归档")
	}
	return a, backend, settle, paths
}

func startUndoFixtureAction(a *application, action string) {
	if action == "trash" {
		a.startBatchTrash()
	} else {
		a.startBatchMove()
	}
}

func assertUndoFixtureOriginals(t *testing.T, a *application, backend *batchBackend, paths []string) {
	t.Helper()
	for _, path := range paths {
		if !slices.Contains(backend.paths, path) || !a.batch.selected[path] {
			t.Fatalf("undo did not restore and select %q", path)
		}
	}
}

func TestBatchUndoRestoresMoveStripAndTrashExactlyOnce(t *testing.T) {
	for _, action := range []string{"move", "strip", "trash"} {
		t.Run(action, func(t *testing.T) {
			a, backend, settle, paths := batchUndoFixture(t, action, false)
			startUndoFixtureAction(a, action)
			settle()
			undo := a.batchUndo
			callback := a.notifications.onAction
			if undo == nil || len(undo.requests) != 2 || callback == nil || a.notifications.action != a.t("撤销") || a.notifications.timeout != 8*time.Second {
				t.Fatal("completed organization omitted its undo action or inverse requests")
			}
			for _, request := range undo.requests {
				if request.VaultID != "vault" || !slices.Contains(paths, request.Destination) || request.Action == "move" && request.Revision == "" {
					t.Fatal("undo omitted its original vault, path, or exact output revision")
				}
			}
			callback()
			callback() // The second click while the inverse requests run is ignored.
			settle()
			callback() // A retained toast callback cannot replay a completed undo.
			settle()
			assertUndoFixtureOriginals(t, a, backend, paths)
			if a.batchUndo != nil || a.notice != a.t("已撤销") {
				t.Fatal("completed undo retained a usable receipt or omitted feedback")
			}
			if action == "trash" {
				if len(backend.actions("restore")) != 2 {
					t.Fatal("trash undo restored entries more than once")
				}
			} else if len(backend.actions("move")) != 4 {
				t.Fatal("move undo did not send exactly one inverse per completed move")
			}
		})
	}
}

func TestBatchUndoOffersPartialSuccessesAndPreservesFailedSelection(t *testing.T) {
	for _, action := range []string{"move", "strip", "trash"} {
		t.Run(action, func(t *testing.T) {
			a, backend, settle, paths := batchUndoFixture(t, action, true)
			startUndoFixtureAction(a, action)
			settle()
			result := a.batch.result
			if result == nil || result.undo != a.batchUndo || result.completed != 1 || len(result.undo.requests) != 1 || !a.batch.selected[paths[1]] || a.notifications.onAction == nil {
				t.Fatal("partial success did not retain both its failure and completed-item undo")
			}
			if a.notifications.description != a.t("%d 项未完成，可重试或撤销已完成的操作。", 1) {
				t.Fatal("partial feedback did not explain its retry and undo choices")
			}
			completedUndo := result.undo
			a.notifications.onAction()
			settle()
			assertUndoFixtureOriginals(t, a, backend, paths)
			if a.batch.result == nil || len(a.batch.result.failures) != 1 || a.batch.result.failures[0].entry != paths[1] {
				t.Fatal("undo discarded the original operation's failed entry and retry result")
			}
			if action == "trash" {
				if len(backend.actions("restore")) != 1 {
					t.Fatal("partial trash undo attempted to restore a failed trash")
				}
			} else if len(backend.actions("move")) != 3 {
				t.Fatal("partial move undo attempted to reverse a failed move")
			}
			if action != "trash" {
				delete(backend.failures, paths[1])
				a.retryBatch()
				settle()
				if a.batchUndo == nil || a.batchUndo == completedUndo || len(a.batchUndo.requests) != 1 || a.notifications.onAction == nil {
					t.Fatal("retrying the original failure after undo did not create a fresh undo receipt")
				}
				a.notifications.onAction()
				settle()
				assertUndoFixtureOriginals(t, a, backend, paths)
			}
		})
	}
}

func TestBatchUndoAccumulatesForwardRetrySuccesses(t *testing.T) {
	for _, action := range []string{"move", "strip"} {
		t.Run(action, func(t *testing.T) {
			a, backend, settle, paths := batchUndoFixture(t, action, true)
			startUndoFixtureAction(a, action)
			settle()
			undo := a.batchUndo
			delete(backend.failures, paths[1])
			a.retryBatch()
			settle()
			if a.batchUndo != undo || len(undo.requests) != 2 {
				t.Fatal("forward retry lost the earlier success or duplicated its undo")
			}
			a.notifications.onAction()
			settle()
			assertUndoFixtureOriginals(t, a, backend, paths)
			if len(backend.actions("move")) != 5 {
				t.Fatal("undo did not reverse both initial and retried successes once")
			}
		})
	}
}

func TestBatchUndoRetriesOnlyRemainingFailures(t *testing.T) {
	for _, action := range []string{"move", "trash"} {
		t.Run(action, func(t *testing.T) {
			a, backend, settle, paths := batchUndoFixture(t, action, false)
			startUndoFixtureAction(a, action)
			settle()
			undo := a.batchUndo
			blocked := undo.requests[1]
			fail := true
			attempts := []app.Request{}
			a.service = backendFunc(func(request app.Request) app.Response {
				if request.Action == blocked.Action {
					attempts = append(attempts, request)
					if fail && request.Entry == blocked.Entry && request.ID == blocked.ID {
						return app.Response{Status: "error", Code: "vault_busy", Data: app.Data{VaultID: "vault"}}
					}
				}
				return backend.Execute(request)
			})
			a.startBatchUndo(undo)
			settle()
			if undo.completed != 1 || len(undo.requests) != 1 || len(undo.failures) != 1 || a.notifications.action != a.t("重试撤销") || a.notifications.onAction == nil {
				t.Fatal("partial undo did not retain its remaining inverse and retry action")
			}
			fail = false
			a.notifications.onAction()
			settle()
			if len(attempts) != 3 || attempts[2] != blocked {
				t.Fatal("undo retry repeated an already restored entry or changed its inverse request")
			}
			assertUndoFixtureOriginals(t, a, backend, paths)
			if a.batchUndo != nil || undo.completed != 2 {
				t.Fatal("retried undo did not finish the retained receipt")
			}
		})
	}
}

func TestBatchUndoRejectsUnavailableAndStaleCallbacks(t *testing.T) {
	for _, blocked := range []string{"vault", "generation", "replaced", "busy", "editor", "closing", "update", "setup"} {
		t.Run(blocked, func(t *testing.T) {
			a, backend, settle, _ := batchUndoFixture(t, "move", false)
			a.startBatchMove()
			settle()
			callback := a.notifications.onAction
			if callback == nil {
				t.Fatal("missing undo callback")
			}
			switch blocked {
			case "vault":
				a.state.VaultID = "other-vault"
			case "generation":
				a.generation++
			case "replaced":
				a.batchUndo = &batchUndoState{}
			case "busy":
				a.busy = true
			case "editor":
				a.editor.open = true
			case "closing":
				a.closing = true
			case "update":
				a.updates.preparing = true
			case "setup":
				a.setup = true
			}
			callback()
			if len(backend.actions("move")) != 2 || a.batch.progress != nil || a.batch.active {
				t.Fatal("an unavailable or stale callback sent inverse requests or replaced the current UI")
			}
		})
	}
}

func TestBatchUndoWhileLockedUnlocksOnlyToRestore(t *testing.T) {
	for _, action := range []string{"move", "trash"} {
		t.Run(action, func(t *testing.T) {
			a, backend, settle, paths := batchUndoFixture(t, action, false)
			startUndoFixtureAction(a, action)
			settle()
			undo := a.batchUndo
			backend.locked, a.state.Locked = true, true
			a.startBatchUndo(undo)
			if action == "move" {
				// Reverse moves only rename files, so they run while locked.
				if a.batch.pending != "" || a.pendingUnlock.action != "" {
					t.Fatal("undoing a move asked to unlock")
				}
				settle()
				assertUndoFixtureOriginals(t, a, backend, paths)
				if a.batchUndo != nil || !a.state.Locked {
					t.Fatal("locked move undo did not finish directly")
				}
				return
			}
			if a.batch.pending != "undo" || a.pendingUnlock.action != "batch" || a.busy || undo.running || len(backend.actions("restore")) != 0 {
				t.Fatal("locked undo wrote before entering the existing unlock continuation")
			}
			if a.batchUnlockDescription() != a.t("解锁后将撤销 %d 项。", 2) {
				t.Fatal("unlock continuation omitted the inverse operation count")
			}
			backend.locked, a.state.Locked = false, false
			if !a.resumeBatchAction() {
				t.Fatal("unlock did not resume the requested undo")
			}
			settle()
			assertUndoFixtureOriginals(t, a, backend, paths)
			if a.batchUndo != nil {
				t.Fatal("unlocked undo did not consume its receipt")
			}
		})
	}
}

func TestBatchUndoUIWithRealVaultPreservesEditsAndConflictingOriginals(t *testing.T) {
	service, err := app.New(app.Options{DataDir: t.TempDir(), Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	execute := func(request app.Request) app.Data {
		t.Helper()
		response := service.Execute(request)
		if response.Status != "ok" {
			t.Fatalf("fixture %s failed: %s", request.Action, response.Code)
		}
		return response.Data
	}
	settings := execute(app.Request{Action: "configure"}).Settings
	settings.Language = "zh-CN"
	execute(app.Request{Action: "settings", Settings: &settings})
	paths := []string{"undo/changed", "undo/conflict", "undo/good"}
	for _, path := range paths {
		execute(app.Request{Action: "save", Entry: path, Contents: "fixture-content:" + path, Mode: "create"})
	}
	a := newApplication(service)
	a.state = execute(app.Request{Action: "configure"})
	callbacks := make(chan func(), 32)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	settle := func() {
		t.Helper()
		for a.busy {
			awaitMain(t, callbacks)
		}
	}
	view := ui.NewTester(testView(a), 1180, 800)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	if err := view.Click(a.t("选择")); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if err := view.Click(a.t("选择条目 %s", path)); err != nil {
			t.Fatal(err)
		}
	}
	a.chooseBatchFolder("moved")
	view.Frame()
	if err := view.Click(a.t("完整路径")); err != nil {
		t.Fatal(err)
	}
	if err := view.Click(a.t("移动 %d 项", 3)); err != nil {
		t.Fatal(err)
	}
	settle()
	changedPath := "moved/undo/changed"
	changed := execute(app.Request{Action: "fetch", Entry: changedPath})
	execute(app.Request{Action: "save", Entry: changedPath, Contents: "later-edited-content", Revision: changed.Revision, Mode: "edit"})
	execute(app.Request{Action: "save", Entry: "undo/conflict", Contents: "later-original-entry", Mode: "create"})
	view.Frame()
	if err := view.Click(a.t("撤销")); err != nil {
		t.Fatal(err)
	}
	settle()
	view.Frame()
	if a.batchUndo == nil || a.batchUndo.completed != 1 || len(a.batchUndo.requests) != 2 || !view.HasText(a.t("目标条目已更改，未还原。")) || !view.HasText(a.t("原路径已有条目，未覆盖。")) {
		t.Fatal("real undo did not retain and explain both unsafe inverses while restoring the safe one")
	}
	for path, want := range map[string]string{
		changedPath:           "later-edited-content",
		"undo/conflict":       "later-original-entry",
		"moved/undo/conflict": "fixture-content:undo/conflict",
		"undo/good":           "fixture-content:undo/good",
	} {
		if got := execute(app.Request{Action: "fetch", Entry: path}); got.Contents != want {
			t.Fatalf("real undo altered preserved contents at %q", path)
		}
	}
	for _, path := range []string{"undo/changed", "moved/undo/good"} {
		if got := service.Execute(app.Request{Action: "fetch", Entry: path}); got.Status == "ok" {
			t.Fatalf("real undo unexpectedly created or kept %q", path)
		}
	}
	managerResizeScreenshot(t, view, "batch-undo-partial-real-vault")
}
