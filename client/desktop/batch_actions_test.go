package desktop

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func assertBatchPeerActions(t *testing.T, a *application, view *ui.Tester, executeLabel string) {
	t.Helper()
	move := visualBounds(t, view, a.t("移动到"))
	prefix := visualBounds(t, view, a.t("修改前缀"))
	trash := visualBounds(t, view, a.t("移到回收站"))
	if !visualNear(move.Y, prefix.Y) || !visualNear(move.Y, trash.Y) || !visualNear(move.H, prefix.H) {
		t.Fatalf("organization actions are not peers in one row: move=%v prefix=%v trash=%v", move, prefix, trash)
	}
	execute := visualBounds(t, view, executeLabel)
	if execute.Y < move.Y+move.H {
		t.Fatalf("execution still precedes its action chooser: actions=%v execute=%v", move, execute)
	}
}

func TestBatchActionsArePeersAboveTheirExecution(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		for _, width := range []int{1180, 860} {
			t.Run(fmt.Sprintf("%s-%d", language, width), func(t *testing.T) {
				a, _, _ := prefixFixture(t, []string{"test/a/b", "test/b/c"}, nil)
				a.chooseBatchPrefix("test")
				a.setBatchMoveMode(false)
				a.batch.flatten = true
				a.chooseBatchFolder("test")
				a.state.Settings.Language = language
				view := ui.NewTester(testView(a), width, 760)
				view.SetPreferences(ui.Preferences{ReduceMotion: true})
				view.Advance(time.Second)
				assertBatchPeerActions(t, a, view, a.t("移动 %d 项", 2))
				if language == "zh-CN" && width == 1180 || language == "en" && width == 860 {
					managerResizeScreenshot(t, view, fmt.Sprintf("batch-peer-move-%s-%d", language, width))
				}
				if err := view.Click(a.t("修改前缀")); err != nil {
					t.Fatal(err)
				}
				a.batch.prefixTo = "test0"
				view.Frame()
				assertBatchPeerActions(t, a, view, a.t("修改 %d 项", 2))
				if language == "zh-CN" && width == 1180 || language == "en" && width == 860 {
					managerResizeScreenshot(t, view, fmt.Sprintf("batch-peer-prefix-%s-%d", language, width))
				}
			})
		}
	}
}

func TestBatchPeerActionsKeepDraftsAndWaitForExecution(t *testing.T) {
	a, backend, settle := prefixFixture(t, []string{"test/a/b", "test/b/c"}, nil)
	a.chooseBatchPrefix("test")
	a.setBatchMoveMode(false)
	a.batch.flatten = true
	view := ui.NewTester(testView(a), 1180, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	if err := view.Click(a.t("选择文件夹 %s", "test")); err != nil {
		t.Fatal(err)
	}
	if err := view.Click(a.t("修改前缀")); err != nil {
		t.Fatal(err)
	}
	// Represent an in-progress prefix draft, then leave and return through the
	// actual peer action controls. Switching must never apply either operation.
	a.batch.prefixFrom, a.batch.prefixTo = "test", "test0"
	view.Frame()
	if err := view.Click(a.t("移动到")); err != nil {
		t.Fatal(err)
	}
	if a.batch.destination != "test" || a.batch.folderQuery != "test" || !a.batch.chosen {
		t.Fatal("switching away from the folder action lost its selected destination")
	}
	if err := view.Click(a.t("搜索目标文件夹")); err != nil {
		t.Fatal(err)
	}
	view.Key(ui.Cmd, ui.KeyA)
	view.Type("new/path")
	if err := view.Click(a.t("修改前缀")); err != nil {
		t.Fatal(err)
	}
	if a.batch.prefixFrom != "test" || a.batch.prefixTo != "test0" || !view.HasText(a.t("将修改 %d 项，%d 项不修改", 2, 0)) {
		t.Fatal("returning to the prefix action lost its draft or preview")
	}
	if err := view.Click(a.t("移动到")); err != nil {
		t.Fatal(err)
	}
	if a.batch.folderQuery != "new/path" || a.batch.chosen {
		t.Fatal("returning to the folder action lost its unfinished search or chose an unrelated destination")
	}
	if len(a.batch.selected) != 2 || len(backend.actions("move")) != 0 || len(backend.actions("trash")) != 0 || len(backend.actions("create-folder")) != 0 {
		t.Fatal("choosing a peer action modified the entries or their selection")
	}
	if err := view.Click(a.t("修改前缀")); err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{"archive", ""} {
		a.batch.prefixTo = to
		view.Frame()
		if err := view.Click(a.t("移动到")); err != nil {
			t.Fatal(err)
		}
		if err := view.Click(a.t("修改前缀")); err != nil {
			t.Fatal(err)
		}
		if a.batch.prefixFrom != "test" || a.batch.prefixTo != to {
			t.Fatalf("returning to an unfinished prefix draft changed its source or input: from=%q to=%q, want test/%q", a.batch.prefixFrom, a.batch.prefixTo, to)
		}
	}
	a.batch.prefixFrom, a.batch.prefixTo = "test", "test0"
	view.Frame()
	if err := view.Click(a.t("修改 %d 项", 2)); err != nil {
		t.Fatal(err)
	}
	settle()
	moves := backend.actions("move")
	if len(moves) != 2 || moves[0].Destination != "test0/a/b" || moves[1].Destination != "test0/b/c" {
		t.Fatalf("the prefix execution did not use its preserved draft: %#v", moves)
	}
}

func TestBatchPeerActionsAndFormRemainKeyboardReachable(t *testing.T) {
	for _, ready := range []bool{false, true} {
		t.Run(fmt.Sprintf("execution-ready=%t", ready), func(t *testing.T) {
			a, _, _ := prefixFixture(t, []string{"test/a/b", "test/b/c"}, nil)
			a.chooseBatchPrefix("test")
			if ready {
				a.batch.prefixTo = "test0"
			}
			view := ui.NewTester(testView(a), 860, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			if err := view.Click(a.t("修改前缀")); err != nil {
				t.Fatal(err)
			}
			labels := []string{a.t("移到回收站")}
			if ready {
				labels = append(labels, a.t("修改 %d 项", 2))
			}
			labels = append(labels, a.t("原文件夹前缀"), a.t("复制原文件夹前缀"), a.t("新文件夹前缀"))
			for _, label := range labels {
				view.Key(0, ui.KeyTab)
				if !view.Focused(label) {
					t.Fatalf("keyboard could not reach %q in action-to-form order", label)
				}
			}
		})
	}
}

func TestBatchPeerActionsCannotSwitchDuringExecution(t *testing.T) {
	a, backend, settle := prefixFixture(t, []string{"test/a/b", "test/b/c"}, nil)
	a.chooseBatchPrefix("test")
	a.batch.prefixTo = "test0"
	interrupted := &interruptedBatchBackend{batchBackend: backend, started: make(chan struct{}), release: make(chan struct{})}
	released := false
	release := func() {
		if !released {
			close(interrupted.release)
			released = true
		}
	}
	defer release()
	a.service = interrupted
	view := ui.NewTester(testView(a), 1180, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	if err := view.Click(a.t("修改 %d 项", 2)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-interrupted.started:
	case <-time.After(time.Second):
		t.Fatal("prefix execution did not start")
	}
	view.Frame()
	for _, key := range []string{"移动到", "修改前缀", "移到回收站"} {
		if err := view.Click(a.t(key)); err != nil {
			t.Fatal(err)
		}
	}
	if a.batch.moveMode != "prefix" || !a.busy || len(backend.actions("trash")) != 0 {
		t.Fatal("a running operation allowed another peer action")
	}
	// Release the worker explicitly before settling; the deferred close protects
	// failing assertions without leaving a test goroutine blocked.
	release()
	settle()
}
