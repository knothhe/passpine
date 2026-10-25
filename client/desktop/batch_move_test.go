package desktop

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func moveFixture(t *testing.T, paths []string, failures map[string]string) (*application, *batchBackend, func()) {
	t.Helper()
	a, backend, settle := batchFixture(t, failures)
	backend.paths = paths
	a.state.Entries = nil
	a.state = backend.metadata(a.state)
	a.enterBatch()
	for _, path := range paths {
		a.batch.selected[path] = true
	}
	return a, backend, settle
}

func TestBatchMoveStripKeepsHierarchyAndMatchesFolderBoundaries(t *testing.T) {
	for _, to := range []string{"test0", "archive/test0"} {
		t.Run(to, func(t *testing.T) {
			a, backend, settle := moveFixture(t, []string{"test/a/b", "test/b/c", "test0/a/b", "testing/d"}, nil)
			delete(a.batch.selected, "test0/a/b")
			delete(a.batch.selected, "test/a/b")
			// "test" is not a folder of "testing/d", so nothing is shared.
			if common := batchCommonFolders(a.selectedBatchEntries()); len(common) != 0 {
				t.Fatalf("a partial folder name matched as a shared folder: %v", common)
			}
			delete(a.batch.selected, "testing/d")
			a.batch.selected["test/a/b"] = true
			a.chooseBatchFolder(to)
			plan := a.batchMovePlan()
			wantReady := 2
			if to == "test0" {
				wantReady = 1
				if plan.status["test/a/b"] != batchMoveExists {
					t.Fatalf("an existing target was not skipped: %#v", plan)
				}
			}
			if len(plan.ready) != wantReady {
				t.Fatalf("unexpected plan: %#v", plan)
			}
			a.startBatchMove()
			settle()
			moves := backend.actions("move")
			if len(moves) != wantReady {
				t.Fatalf("unexpected requests: %#v", moves)
			}
			for _, move := range moves {
				if want := batchDestinationPath(to, move.Entry[len("test/"):]); move.Destination != want {
					t.Fatalf("hierarchy lost: got %q want %q", move.Destination, want)
				}
			}
			if to == "test0" && !a.batch.selected["test/a/b"] {
				t.Fatal("a skipped entry did not stay selected")
			}
		})
	}
}

func TestBatchMoveCaseConflictsSendNothing(t *testing.T) {
	a, backend, _ := moveFixture(t, []string{"test/a/b", "test/A/b"}, nil)
	a.chooseBatchFolder("next")
	plan := a.batchMovePlan()
	if len(plan.ready) != 0 || plan.status["test/a/b"] != batchMoveDuplicate || plan.status["test/A/b"] != batchMoveDuplicate {
		t.Fatalf("internal case-insensitive conflicts were not skipped: %#v", plan)
	}
	a.startBatchMove()
	a.batch.chosen = false
	a.startBatchMove()
	if len(backend.actions("move")) != 0 {
		t.Fatal("conflicting or unconfigured moves were sent")
	}
}

func TestBatchMoveRetriesOriginalTargetsAfterPartialFailure(t *testing.T) {
	a, backend, settle := moveFixture(t, []string{"test/a/b", "test/b/c"}, map[string]string{"test/b/c": "vault_busy"})
	a.chooseBatchFolder("test0")
	a.startBatchMove()
	settle()
	if a.batch.result == nil || a.batch.result.completed != 1 || !a.batch.selected["test/b/c"] {
		t.Fatal("partial move result was not preserved")
	}
	delete(backend.failures, "test/b/c")
	// A changed draft must not redirect the entries the result still holds.
	a.batch.destination, a.batch.folderQuery, a.batch.keep = "unrelated", "unrelated", batchKeepName
	a.retryBatch()
	settle()
	moves := backend.actions("move")
	if len(moves) != 3 || moves[2].Destination != "test0/b/c" || a.notice != "已将 2 项移动到“test0”" {
		t.Fatalf("retry lost its original destination: %#v, %q", moves, a.notice)
	}
}

func TestBatchMoveLockedMovePreservesUnsentEntriesForRetry(t *testing.T) {
	a, backend, settle := moveFixture(t, []string{"test/a/b", "test/b/c"}, map[string]string{"test/a/b": "locked"})
	a.chooseBatchFolder("test0")
	a.startBatchMove()
	settle()
	if len(backend.actions("move")) != 1 || a.batch.result == nil || len(a.batch.result.failures) != 2 || len(a.batch.selected) != 2 {
		t.Fatal("session stop dropped unsent entries")
	}
	delete(backend.failures, "test/a/b")
	a.retryBatch()
	settle()
	if len(backend.actions("move")) != 3 || len(a.batch.selected) != 0 {
		t.Fatal("retry did not finish both interrupted entries")
	}
}

type interruptedBatchBackend struct {
	*batchBackend
	started, release chan struct{}
	hideNames        bool
}

func (b *interruptedBatchBackend) Execute(request app.Request) app.Response {
	response := b.batchBackend.Execute(request)
	if b.hideNames {
		response.Data.Settings.BrowseWhileLocked = false
	}
	if b.started != nil && request.Action == "move" && request.Entry == "test/a/b" {
		close(b.started)
		<-b.release
	}
	return response
}

func TestBatchMoveLockAfterSnapshotDoesNotRestoreHiddenNames(t *testing.T) {
	a, backend, _ := moveFixture(t, []string{"test/a/b", "test/b/c"}, map[string]string{"test/b/c": "vault_busy"})
	a.service = &interruptedBatchBackend{batchBackend: backend, hideNames: true}
	a.state.Settings.BrowseWhileLocked = false
	a.chooseBatchFolder("test0")
	callbacks := make(chan func(), 4)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.startBatchMove()
	// Drain the posted callbacks without executing them. This puts the unlocked
	// list snapshot before the lock and its UI application after the lock.
	posted := make([]func(), 0, 3)
	for range 3 {
		select {
		case callback := <-callbacks:
			posted = append(posted, callback)
		case <-time.After(time.Second):
			t.Fatal("batch did not post its completed snapshot")
		}
	}
	backend.mu.Lock()
	backend.locked = true
	backend.mu.Unlock()
	a.handleSessionLock()
	if !a.busy || len(a.state.Entries) != 0 {
		t.Fatal("lock did not retain the lease and clear names")
	}
	for _, callback := range posted {
		callback()
	}
	a.entries()
	if a.busy || !a.state.Locked || len(a.state.Entries) != 0 || len(a.state.Folders) != 0 || len(a.sortedEntries) != 0 || len(a.folders) != 0 || a.batch.progress != nil {
		t.Fatal("an unlocked snapshot restored names after locking")
	}
	if a.batch.result == nil || a.batch.result.completed != 1 || len(a.batch.result.failures) != 1 || !a.batch.selected["test/b/c"] {
		t.Fatal("hiding names discarded the completed/failed result")
	}
}

func TestBatchMoveLockAfterSnapshotDoesNotRevealDestinationInNotification(t *testing.T) {
	a, backend, _ := moveFixture(t, []string{"test/a/b", "test/b/c"}, nil)
	a.service = &interruptedBatchBackend{batchBackend: backend, hideNames: true}
	a.state.Settings.BrowseWhileLocked = false
	a.chooseBatchFolder("private/destination")
	callbacks := make(chan func(), 4)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.startBatchMove()
	// Both moves and the unlocked list snapshot finish before the lock; their
	// queued UI callbacks must not reveal the destination after names are hidden.
	posted := make([]func(), 0, 3)
	for range 3 {
		select {
		case callback := <-callbacks:
			posted = append(posted, callback)
		case <-time.After(time.Second):
			t.Fatal("batch did not post its completed snapshot")
		}
	}
	backend.mu.Lock()
	backend.locked = true
	backend.mu.Unlock()
	a.handleSessionLock()
	for _, callback := range posted {
		callback()
	}
	if a.busy || !a.state.Locked || a.batch.active || a.batch.progress != nil || len(backend.actions("move")) != 2 {
		t.Fatal("hiding the completion notification discarded the completed operation")
	}
	if a.notice != "" || a.notifications.typedMessage != "" || a.notifications.action != "" || a.notifications.onAction != nil {
		t.Fatalf("locked completion restored a notification with folder metadata: %q", a.notice)
	}
}

func TestBatchStaleLeaseReleasesBusyAndRefreshesWithoutOldResult(t *testing.T) {
	a, _, _ := moveFixture(t, []string{"test/a/b", "test/b/c"}, nil)
	a.chooseBatchFolder("test0")
	callbacks := make(chan func(), 8)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.startBatchMove()
	posted := make([]func(), 0, 3)
	for range 3 {
		select {
		case callback := <-callbacks:
			posted = append(posted, callback)
		case <-time.After(time.Second):
			t.Fatal("batch did not post its completed snapshot")
		}
	}
	a.generation++
	a.state.VaultID = "replacement-vault"
	for _, callback := range posted {
		callback()
	}
	if a.batch.progress != nil || a.batch.result != nil || a.activeRequestAction != "configure" || !a.backgroundBusy || a.notice != "" {
		t.Fatal("stale batch retained its lease or applied its old result")
	}
	awaitMain(t, callbacks)
	if a.busy || a.backgroundBusy || a.batch.progress != nil {
		t.Fatal("metadata refresh did not release the stale operation")
	}
}

func (b *interruptedBatchBackend) IsLocked() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.locked
}

func TestBatchMoveSessionLockStopsWorkerAndPreservesResult(t *testing.T) {
	a, backend, settle := moveFixture(t, []string{"test/a/b", "test/b/c"}, nil)
	interrupted := &interruptedBatchBackend{batchBackend: backend, started: make(chan struct{}), release: make(chan struct{})}
	a.service = interrupted
	a.chooseBatchFolder("test0")
	a.startBatchMove()
	select {
	case <-interrupted.started:
	case <-time.After(time.Second):
		t.Fatal("first move did not start")
	}
	backend.mu.Lock()
	backend.locked = true
	backend.mu.Unlock()
	a.handleSessionLock()
	if !a.busy || a.batch.progress == nil || !a.batch.progress.cancelled.Load() {
		t.Fatal("lock released the operation lease before the worker stopped")
	}
	a.startBatchMove()
	close(interrupted.release)
	settle()
	if !a.state.Locked || a.batch.progress != nil || len(backend.actions("move")) != 1 || a.batch.result == nil || a.batch.result.completed != 1 || len(a.batch.result.failures) != 1 || !a.batch.selected["test/b/c"] || a.batch.selected["test/a/b"] {
		t.Fatalf("lock lost completed/unsent paths or left progress stuck: %+v", a.batch)
	}
	backend.mu.Lock()
	backend.locked = false
	backend.mu.Unlock()
	a.state.Locked = false
	a.retryBatch()
	settle()
	moves := backend.actions("move")
	if len(moves) != 2 || moves[1].Destination != "test0/b/c" {
		t.Fatalf("interrupted retry lost its original target: %#v", moves)
	}
}

func TestBatchFolderInputAndSelectionStayInSync(t *testing.T) {
	a, _, _ := batchFixture(t, nil)
	a.enterBatch()
	a.batch.selected["开发/GitHub"] = true
	view := ui.NewTester(testView(a), 1180, 800)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	if err := view.Click("选择文件夹 家庭"); err != nil {
		t.Fatal(err)
	}
	if a.batch.folderQuery != "家庭" || !a.batch.chosen {
		t.Fatal("tree selection did not update the field")
	}
	if err := view.Click("搜索目标文件夹"); err != nil {
		t.Fatal(err)
	}
	view.Key(ui.Cmd, ui.KeyA)
	view.Type("工作/客户项目")
	if a.batch.destination != "工作/客户项目" || !a.batch.chosen {
		t.Fatal("exact existing path did not select its destination")
	}
	view.Key(ui.Cmd, ui.KeyA)
	view.Type("new/path")
	if a.batch.chosen || a.batch.destination != "" {
		t.Fatal("new input retained an unrelated destination")
	}
	a.chooseBatchFolder("")
	if !a.batch.chosen || a.batch.folderQuery != "" || a.batch.destination != "" {
		t.Fatal("vault root lost its empty-path selection")
	}
}
