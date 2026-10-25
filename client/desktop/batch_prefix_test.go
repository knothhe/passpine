package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/i18n"
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

func selectBatchPrefixUI(t *testing.T, a *application, view *ui.Tester, folder string) {
	t.Helper()
	if err := view.Click(a.t("修改前缀")); err != nil {
		t.Fatal(err)
	}
	for step := 0; step < 8 && !view.Focused(a.t("原文件夹前缀")); step++ {
		view.Key(0, ui.KeyTab)
	}
	if !view.Focused(a.t("原文件夹前缀")) {
		t.Fatal("source prefix selector is not reachable by keyboard")
	}
	view.Key(0, ui.KeyEnter)
	if err := view.Click(a.t("选择原文件夹前缀 %s", folder)); err != nil {
		t.Fatal(err)
	}
	if a.batch.prefixFrom != folder {
		t.Fatalf("source prefix selection = %q, want %q", a.batch.prefixFrom, folder)
	}
}

func TestBatchPrefixSourceDefaultsToFirstSelectedFolderAndOffersAncestors(t *testing.T) {
	a, _, _ := prefixFixture(t, []string{"second/folder/Zulu", "first/nested/Alpha"}, nil)
	if a.batch.prefixFrom != "first/nested" || a.batch.prefixTo != "first/nested" {
		t.Fatalf("source should default to the first selected entry in list order: %q / %q", a.batch.prefixFrom, a.batch.prefixTo)
	}
	options := a.batchPrefixOptions()
	for _, want := range []string{"first", "first/nested", "second", "second/folder"} {
		if !slices.Contains(options, want) {
			t.Fatalf("source selector omitted ancestor %q: %v", want, options)
		}
	}
	if len(options) != 4 || slices.Contains(options, "") {
		t.Fatalf("source options contain duplicates or an empty prefix: %v", options)
	}
	a.chooseBatchPrefix("first")
	if a.batch.prefixFrom != "first" || a.batch.prefixTo != "first" {
		t.Fatal("an untouched replacement did not follow the newly chosen source")
	}
	for _, to := range []string{"archive", ""} {
		a.batch.prefixTo = to
		a.chooseBatchPrefix("second")
		a.chooseBatchPrefix("first")
		if a.batch.prefixTo != to {
			t.Fatalf("changing the source replaced the custom/unfinished replacement %q", to)
		}
	}
	a.batch.prefixTo = "custom/destination"
	delete(a.batch.selected, "first/nested/Alpha")
	a.syncBatchPrefixSource()
	if a.batch.prefixFrom != "second/folder" || a.batch.prefixTo != "custom/destination" {
		t.Fatal("removing the selected source did not choose the remaining entry folder and preserve the replacement draft")
	}
}

func TestBatchPrefixRootFirstSelectionKeepsAvailableSourcesReachable(t *testing.T) {
	for _, test := range []struct {
		path, want string
	}{{"test/Zulu", "test"}, {"test/a/Zulu", ""}} {
		t.Run(test.path, func(t *testing.T) {
			a, _, _ := prefixFixture(t, []string{"Alpha", test.path}, nil)
			if a.batch.prefixFrom != test.want {
				t.Fatalf("root-first source = %q, want %q", a.batch.prefixFrom, test.want)
			}
			view := ui.NewTester(testView(a), 860, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			if test.want == "" {
				selectBatchPrefixUI(t, a, view, "test")
				if a.batch.prefixFrom != "test" {
					t.Fatal("root-first selection made its non-root prefixes unreachable")
				}
			} else if a.batch.prefixTo != "test" {
				t.Fatal("the sole available source was not used as the read-only default")
			}
		})
	}
}

func TestBatchPrefixSourceSelectionCannotAcceptTypedPathsAndCopiesCurrentSource(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		t.Run(language, func(t *testing.T) {
			a, backend, _ := prefixFixture(t, []string{"test/a/b", "test/b/c"}, nil)
			a.state.Settings.Language = language
			view := ui.NewTester(testView(a), 1180, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			selectBatchPrefixUI(t, a, view, "test")
			view.Type("../arbitrary-input")
			if a.batch.prefixFrom != "test" {
				t.Fatal("typing into a source selector created an arbitrary prefix")
			}
			copied := ""
			a.clipboardWriter = func(value string, sensitive bool) error {
				if sensitive {
					t.Fatal("a folder prefix was copied as a credential")
				}
				copied = value
				return nil
			}
			if err := view.Click(a.t("复制原文件夹前缀")); err != nil {
				t.Fatal(err)
			}
			if copied != "test" || !view.HasText(a.t("已复制")) {
				t.Fatalf("copy did not use the current source or report success: %q", copied)
			}
			a.clipboardWriter = func(string, bool) error { return errors.New("clipboard unavailable") }
			if err := view.Click(a.t("复制原文件夹前缀")); err != nil {
				t.Fatal(err)
			}
			if !view.HasText(a.t(i18n.ClipboardWriteFailed)) || view.HasText(a.t("已复制")) {
				t.Fatal("clipboard failure was not reported or retained stale success feedback")
			}
			a.clipboardWriter = func(value string, sensitive bool) error { copied = value; return nil }
			if err := view.Click(a.t("复制原文件夹前缀")); err != nil {
				t.Fatal(err)
			}
			if !view.HasText(a.t("已复制")) || view.HasText(a.t(i18n.ClipboardWriteFailed)) || a.errorText != "" {
				t.Fatal("retrying the copy retained an obsolete clipboard error")
			}
			if len(backend.actions("move")) != 0 || len(backend.actions("unlock")) != 0 {
				t.Fatal("choosing or copying a metadata prefix started a vault action")
			}
		})
	}
}

func TestBatchPrefixSingleSourceIsReadOnlyAndRootEntriesHaveNoSource(t *testing.T) {
	t.Run("one option", func(t *testing.T) {
		a, _, _ := prefixFixture(t, []string{"test/account"}, nil)
		if options := a.batchPrefixOptions(); !slices.Equal(options, []string{"test"}) {
			t.Fatalf("single-level source options = %v", options)
		}
		view := ui.NewTester(testView(a), 860, 760)
		view.SetPreferences(ui.Preferences{ReduceMotion: true})
		if err := view.Click(a.t("修改前缀")); err != nil {
			t.Fatal(err)
		}
		copyReached := false
		for step := 0; step < 6 && !view.Focused(a.t("新文件夹前缀")); step++ {
			view.Key(0, ui.KeyTab)
			if view.Focused(a.t("原文件夹前缀")) {
				t.Fatal("one source was rendered as an editable or selectable control")
			}
			copyReached = copyReached || view.Focused(a.t("复制原文件夹前缀"))
		}
		if !copyReached || !view.Focused(a.t("新文件夹前缀")) {
			t.Fatal("read-only source copy or replacement input is not keyboard reachable")
		}
		copied := ""
		a.clipboardWriter = func(value string, sensitive bool) error { copied = value; return nil }
		if err := view.Click(a.t("复制原文件夹前缀")); err != nil || copied != "test" {
			t.Fatalf("read-only source could not be copied: %q, %v", copied, err)
		}
	})
	t.Run("root entries", func(t *testing.T) {
		a, backend, _ := prefixFixture(t, []string{"Alpha", "Zulu"}, nil)
		if len(a.batchPrefixOptions()) != 0 || a.batch.prefixFrom != "" || a.batchMoveConfigured() {
			t.Fatal("root entries invented a replaceable folder prefix")
		}
		writes := 0
		a.clipboardWriter = func(string, bool) error { writes++; return nil }
		view := ui.NewTester(testView(a), 860, 760)
		if err := view.Click(a.t("复制原文件夹前缀")); err != nil {
			t.Fatal(err)
		}
		if !view.HasText(a.t("所选条目没有可替换的文件夹前缀。")) || writes != 0 {
			t.Fatal("missing source was not explained or allowed a meaningless clipboard write")
		}
		a.batch.prefixTo = "archive"
		a.startBatchMove()
		if len(backend.actions("move")) != 0 {
			t.Fatal("a missing source sent a prefix move")
		}
	})
}

func prefixTextCount(view *ui.Tester, text string) int {
	count := 0
	for _, got := range view.Texts() {
		if got == text {
			count++
		}
	}
	return count
}

func TestBatchPrefixSummaryGroupsReasonsAndListsOnlyMismatchedPaths(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		t.Run(language, func(t *testing.T) {
			a, backend, _ := prefixFixture(t, []string{"test/a/b", "test/b/c", "unrelated/d", "elsewhere/e"}, nil)
			backend.paths = append(backend.paths, "next/a/b")
			a.state.Entries = nil
			a.state = backend.metadata(a.state)
			a.cacheReady = false
			a.chooseBatchPrefix("test")
			a.batch.prefixTo = "next"
			a.state.Settings.Language = language
			view := ui.NewTester(testView(a), 860, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			if !view.HasText(a.t("将修改 %d 项，%d 项不修改", 1, 3)) || !view.HasText(a.t("不修改原因")) || prefixTextCount(view, a.t("前缀不匹配：%d 项", 2)) != 1 || prefixTextCount(view, a.t("目标路径已存在或重名：%d 项", 1)) != 1 || view.HasText(a.t("路径未变化：%d 项", 0)) {
				t.Fatalf("excluded summary did not group each nonzero reason once: %v", view.Texts())
			}
			// Every path appears in the entry list. Only mismatched paths should
			// appear again beneath their reason in the summary panel.
			assertPathCounts := func(mismatched ...string) {
				t.Helper()
				for _, path := range backend.paths {
					want := 1
					if slices.Contains(mismatched, path) {
						want = 2
					}
					if got := prefixTextCount(view, path); got != want {
						t.Fatalf("original path %q appeared %d times, want %d: %v", path, got, want, view.Texts())
					}
				}
			}
			assertPathCounts("unrelated/d", "elsewhere/e")
			if view.HasText("next/b/c") {
				t.Fatalf("summary included a successful target path: %v", view.Texts())
			}
			managerResizeScreenshot(t, view, "batch-prefix-excluded-"+language)
			a.batch.prefixTo = "test"
			view.Frame()
			if !view.HasText(a.t("将修改 %d 项，%d 项不修改", 0, 4)) || prefixTextCount(view, a.t("路径未变化：%d 项", 2)) != 1 || prefixTextCount(view, a.t("前缀不匹配：%d 项", 2)) != 1 || view.HasText(a.t("目标路径已存在或重名：%d 项", 0)) || view.HasText(a.t("目标路径已存在或重名：%d 项", 1)) {
				t.Fatalf("same-prefix summary did not group unchanged paths and hide zero reasons: %v", view.Texts())
			}
			assertPathCounts("unrelated/d", "elsewhere/e")
			a.batch.prefixTo = "next"
			a.chooseBatchPrefix("unrelated")
			view.Frame()
			if !view.HasText(a.t("将修改 %d 项，%d 项不修改", 1, 3)) || prefixTextCount(view, a.t("前缀不匹配：%d 项", 3)) != 1 || view.HasText(a.t("前缀不匹配：%d 项", 2)) || view.HasText(a.t("路径未变化：%d 项", 0)) || view.HasText(a.t("目标路径已存在或重名：%d 项", 0)) || view.HasText(a.t("目标路径已存在或重名：%d 项", 1)) {
				t.Fatalf("changing the source retained stale reason counts: %v", view.Texts())
			}
			assertPathCounts("test/a/b", "test/b/c", "elsewhere/e")
			if view.HasText("next/d") || view.HasText("next/b/c") {
				t.Fatalf("changing the source showed a successful target path: %v", view.Texts())
			}
			a.batch.prefixTo = ""
			view.Frame()
			a.startBatchMove()
			if a.batchMoveConfigured() || !view.HasText(a.t("请输入新文件夹前缀。")) || len(backend.actions("move")) != 0 {
				t.Fatal("an empty replacement removed the prefix instead of remaining an unfinished draft")
			}
		})
	}
}

func TestBatchPrefixPreservesHierarchyAndMatchesFolderBoundaries(t *testing.T) {
	for _, to := range []string{"test0", "archive/test0"} {
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
	for _, input := range []struct{ from, to string }{{"", "next"}, {"test/..", "next"}, {"test", "../escape"}, {"test", ""}} {
		a.batch.prefixFrom, a.batch.prefixTo = input.from, input.to
		a.startBatchMove()
		if a.batchMoveConfigured() || len(backend.actions("move")) != 0 {
			t.Fatalf("invalid prefix sent a move: %+v", input)
		}
	}
}

func TestBatchPrefixRetriesOriginalTargetsAfterPartialFailure(t *testing.T) {
	a, backend, settle := prefixFixture(t, []string{"test/a/b", "test/b/c"}, map[string]string{"test/b/c": "vault_busy"})
	if a.batch.prefixFrom != "test/a" || a.batch.prefixTo != "test/a" {
		t.Fatal("first selected entry folder was not prefilled")
	}
	a.chooseBatchPrefix("test")
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
	if len(moves) != 3 || moves[2].Destination != "test0/b/c" || a.notice != "已修改 2 项的文件夹前缀" {
		t.Fatalf("retry lost its original destination: %#v, %q", moves, a.notice)
	}
}

func TestBatchPrefixLockedMovePreservesUnsentEntriesForRetry(t *testing.T) {
	a, backend, settle := prefixFixture(t, []string{"test/a/b", "test/b/c"}, map[string]string{"test/a/b": "locked"})
	a.chooseBatchPrefix("test")
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
	a.chooseBatchPrefix("test")
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
	a.chooseBatchPrefix("test")
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
	a.chooseBatchPrefix("test")
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
			selectBatchPrefixUI(t, a, view, "test")
			for step := 0; step < 8 && !view.Focused(a.t("新文件夹前缀")); step++ {
				view.Key(0, ui.KeyTab)
			}
			if !view.Focused(a.t("新文件夹前缀")) {
				t.Fatal("new prefix input is not reachable by keyboard")
			}
			view.Key(ui.Cmd, ui.KeyA)
			view.Type("test0")
			view.Frame()
			if !view.HasText(a.t("将修改 %d 项，%d 项不修改", 2, 0)) || view.HasText("test0/a/b") || view.HasText("test0/b/c") {
				t.Fatalf("summary should replace per-entry successful target previews: %v", view.Texts())
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
	selectBatchPrefixUI(t, a, view, "test")
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
