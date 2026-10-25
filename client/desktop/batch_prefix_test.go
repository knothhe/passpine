package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func prefixFixture(t *testing.T, paths []string, failures map[string]string) (*application, *batchBackend, func()) {
	t.Helper()
	a, backend, settle := batchFixture(t, failures)
	backend.paths = paths
	a.state.Entries = nil
	a.state = backend.metadata(a.state)
	a.enterBatch()
	for _, path := range paths {
		a.batch.selected[path] = true
	}
	a.setBatchMoveMode(true)
	return a, backend, settle
}

func TestBatchPrefixPreservesHierarchyAndMatchesFolderBoundaries(t *testing.T) {
	for _, to := range []string{"test0", "", "archive/test0"} {
		t.Run(to, func(t *testing.T) {
			a, backend, settle := prefixFixture(t, []string{"test/a/b", "test/b/c", "test0/a/b", "testing/d"}, nil)
			a.batch.prefixFrom, a.batch.prefixTo = "test", to
			ready, conflicts, unchanged := a.batchMovePlan()
			wantReady, wantConflicts := 2, 0
			if to == "test0" {
				wantReady, wantConflicts = 1, 1
			}
			if len(ready) != wantReady || len(conflicts) != wantConflicts || len(unchanged) != 2 {
				t.Fatalf("unexpected boundary/conflict plan: ready=%v conflicts=%v unchanged=%v", ready, conflicts, unchanged)
			}
			a.startBatchMove()
			settle()
			moves := backend.actions("move")
			if len(moves) != wantReady {
				t.Fatalf("unexpected requests: %#v", moves)
			}
			for _, move := range moves {
				if move.Entry != "test/a/b" && move.Entry != "test/b/c" {
					t.Fatalf("partial folder prefix matched: %#v", move)
				}
				want := batchDestinationPath(to, move.Entry[len("test/"):])
				if move.Destination != want {
					t.Fatalf("hierarchy lost: got %q want %q", move.Destination, want)
				}
			}
			if !a.batch.selected["test0/a/b"] || !a.batch.selected["testing/d"] {
				t.Fatal("unmatched entries did not stay selected")
			}
		})
	}
}

func TestBatchPrefixValidationAndCaseConflicts(t *testing.T) {
	a, backend, _ := prefixFixture(t, []string{"test/a/b", "test/A/b"}, nil)
	a.batch.prefixFrom, a.batch.prefixTo = "test", "next"
	ready, conflicts, _ := a.batchMovePlan()
	if len(ready) != 0 || len(conflicts) != 2 {
		t.Fatal("internal case-insensitive conflicts were not skipped")
	}
	for _, input := range []struct{ from, to string }{{"", "next"}, {"test/..", "next"}, {"test", "../escape"}} {
		a.batch.prefixFrom, a.batch.prefixTo = input.from, input.to
		a.startBatchMove()
		if a.batchMoveConfigured() || len(backend.actions("move")) != 0 {
			t.Fatalf("invalid prefix sent a move: %+v", input)
		}
	}
}

func TestBatchPrefixRetriesOriginalTargetsAfterPartialFailure(t *testing.T) {
	a, backend, settle := prefixFixture(t, []string{"test/a/b", "test/b/c"}, map[string]string{"test/b/c": "vault_busy"})
	if a.batch.prefixFrom != "test" || a.batch.prefixTo != "test" {
		t.Fatal("common folder prefix was not prefilled")
	}
	a.batch.prefixTo = "test0"
	a.startBatchMove()
	settle()
	if a.batch.result == nil || !a.batch.result.prefix || a.batch.result.completed != 1 || !a.batch.selected["test/b/c"] {
		t.Fatal("partial prefix result was not preserved")
	}
	delete(backend.failures, "test/b/c")
	a.batch.prefixTo = "unrelated"
	a.retryBatch()
	settle()
	moves := backend.actions("move")
	if len(moves) != 3 || moves[2].Destination != "test0/b/c" || a.notice != "已修改 1 项的文件夹前缀" {
		t.Fatalf("retry lost its original destination: %#v, %q", moves, a.notice)
	}
}

func TestBatchPrefixLockedMovePreservesUnsentEntriesForRetry(t *testing.T) {
	a, backend, settle := prefixFixture(t, []string{"test/a/b", "test/b/c"}, map[string]string{"test/a/b": "locked"})
	a.batch.prefixTo = "test0"
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

func TestBatchPrefixLockAfterSnapshotDoesNotRestoreHiddenNames(t *testing.T) {
	a, backend, _ := prefixFixture(t, []string{"test/a/b", "test/b/c"}, map[string]string{"test/b/c": "vault_busy"})
	a.service = &interruptedBatchBackend{batchBackend: backend, hideNames: true}
	a.state.Settings.BrowseWhileLocked = false
	a.batch.prefixTo = "test0"
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
	a, backend, _ := prefixFixture(t, []string{"test/a/b", "test/b/c"}, nil)
	a.service = &interruptedBatchBackend{batchBackend: backend, hideNames: true}
	a.state.Settings.BrowseWhileLocked = false
	a.setBatchMoveMode(false)
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
	a, _, _ := prefixFixture(t, []string{"test/a/b", "test/b/c"}, nil)
	a.batch.prefixTo = "test0"
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

func TestBatchPrefixSessionLockStopsWorkerAndPreservesResult(t *testing.T) {
	a, backend, settle := prefixFixture(t, []string{"test/a/b", "test/b/c"}, nil)
	interrupted := &interruptedBatchBackend{batchBackend: backend, started: make(chan struct{}), release: make(chan struct{})}
	a.service = interrupted
	a.batch.prefixTo = "test0"
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

func TestBatchPrefixUIAndResponsivePreview(t *testing.T) {
	for _, test := range []struct {
		language string
		width    int
	}{{"zh-CN", 1180}, {"en", 1180}, {"en", 860}} {
		t.Run(fmt.Sprintf("%s-%d", test.language, test.width), func(t *testing.T) {
			a, backend, settle := prefixFixture(t, []string{"test/a/b", "test/b/c"}, nil)
			a.batch.moveMode = ""
			a.state.Settings.Language = test.language
			view := ui.NewTester(testView(a), test.width, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			view.Advance(time.Second)
			if err := view.Click(a.t("修改前缀")); err != nil {
				t.Fatal(err)
			}
			for step := 0; step < 8 && !view.Focused(a.t("新文件夹前缀")); step++ {
				view.Key(0, ui.KeyTab)
			}
			if !view.Focused(a.t("新文件夹前缀")) {
				t.Fatal("new prefix input is not reachable by keyboard")
			}
			view.Key(ui.Cmd, ui.KeyA)
			view.Type("test0")
			view.Frame()
			if !view.HasText("test0/a/b") || !view.HasText("test0/b/c") {
				t.Fatalf("preview did not show the preserved hierarchy: from=%q to=%q mode=%q texts=%v", a.batch.prefixFrom, a.batch.prefixTo, a.batch.moveMode, view.Texts())
			}
			managerResizeScreenshot(t, view, fmt.Sprintf("batch-prefix-%s-%d", test.language, test.width))
			if err := view.Click(a.t("修改 %d 项", 2)); err != nil {
				t.Fatal(err)
			}
			settle()
			if len(backend.actions("move")) != 2 {
				t.Fatal("prefix action did not send both moves")
			}
		})
	}
}

func TestBatchPrefixUIWithRealVaultPreservesContents(t *testing.T) {
	service, err := app.New(app.Options{DataDir: t.TempDir(), Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	settings := service.Execute(app.Request{Action: "configure"}).Data.Settings
	settings.Language = "zh-CN"
	if response := service.Execute(app.Request{Action: "settings", Settings: &settings}); response.Status != "ok" {
		t.Fatal(response.Message)
	}
	contents := map[string]string{"test/a/b": "fixture-secret-b\nusername: fixture-b\n", "test/b/c": "fixture-secret-c\nusername: fixture-c\n"}
	for path, content := range contents {
		if response := service.Execute(app.Request{Action: "save", Entry: path, Contents: content, Mode: "create"}); response.Status != "ok" {
			t.Fatal(response.Code)
		}
	}
	a := newApplication(service)
	a.state = service.Execute(app.Request{Action: "configure"}).Data
	callbacks := make(chan func(), 16)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	view := ui.NewTester(testView(a), 1180, 800)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	for _, label := range []string{"选择", "选择条目 test/a/b", "选择条目 test/b/c", "修改前缀"} {
		if err := view.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	for step := 0; step < 8 && !view.Focused("新文件夹前缀"); step++ {
		view.Key(0, ui.KeyTab)
	}
	if !view.Focused("新文件夹前缀") {
		t.Fatal("new prefix input is not reachable by keyboard")
	}
	view.Key(ui.Cmd, ui.KeyA)
	view.Type("test0")
	if err := view.Click("修改 2 项"); err != nil {
		t.Fatal(err)
	}
	for a.busy {
		awaitMain(t, callbacks)
	}
	view.Frame()
	for original, content := range contents {
		destination := "test0/" + original[len("test/"):]
		response := service.Execute(app.Request{Action: "fetch", Entry: destination})
		if response.Status != "ok" || response.Data.Contents != content {
			t.Fatalf("real prefix move changed contents at %q: %s", destination, response.Code)
		}
		if _, err := os.Stat(filepath.Join(a.state.Directory, filepath.FromSlash(original)+".gpg")); !os.IsNotExist(err) {
			t.Fatalf("original encrypted file still exists: %q, %v", original, err)
		}
		if info, err := os.Stat(filepath.Join(a.state.Directory, filepath.FromSlash(destination)+".gpg")); err != nil || info.Size() == 0 {
			t.Fatalf("destination encrypted file is missing: %q, %v", destination, err)
		}
	}
}
