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

// The form reads top to bottom: destination, what to keep, preview, then the
// move button. Trash sits beside the count, apart from the move form.
func TestBatchMoveFormOrdersFieldsAboveItsButton(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		for _, width := range []int{1180, 860} {
			t.Run(fmt.Sprintf("%s-%d", language, width), func(t *testing.T) {
				a, _, _ := moveFixture(t, []string{"test/a/b", "test/b/c"}, nil)
				a.chooseBatchFolder("test0")
				a.state.Settings.Language = language
				view := ui.NewTester(testView(a), width, 760)
				view.SetPreferences(ui.Preferences{ReduceMotion: true})
				view.Advance(time.Second)
				count := visualBounds(t, view, a.t("已选 %d 项", 2))
				trash := visualBounds(t, view, a.t("移到回收站"))
				if trash.Y > count.Y+count.H || trash.X < count.X+count.W {
					t.Fatalf("trash is not beside the selection count: count=%v trash=%v", count, trash)
				}
				search := visualBounds(t, view, a.t("搜索目标文件夹"))
				keep := visualBounds(t, view, a.t("保留原路径"))
				preview := visualBounds(t, view, a.t("原路径与新路径"))
				move := visualBounds(t, view, a.t("移动 %d 项", 2))
				if !(trash.Y < search.Y && search.Y < keep.Y && keep.Y < preview.Y && preview.Y+preview.H <= move.Y) {
					t.Fatalf("form is out of order: search=%v keep=%v preview=%v move=%v", search, keep, preview, move)
				}
				if move.Y+move.H > 760 {
					t.Fatalf("move button is not visible: %v", move)
				}
				for _, removed := range []string{"修改前缀", "保留前缀", "不保留前缀"} {
					if _, found := view.Find(removed); found {
						t.Fatalf("the merged form still shows %q", removed)
					}
				}
				managerResizeScreenshot(t, view, fmt.Sprintf("batch-move-form-%s-%d", language, width))
			})
		}
	}
}

func TestBatchMoveFormRemainsKeyboardReachable(t *testing.T) {
	a, _, _ := moveFixture(t, []string{"test/a/b", "test/a/c/d"}, nil)
	a.chooseBatchFolder("test0")
	view := ui.NewTester(testView(a), 860, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	order := []string{a.t("移到回收站"), a.t("搜索目标文件夹"), a.t("去掉公共前缀"), a.t("要去掉的公共前缀"), a.t("移动 %d 项", 2)}
	next := 0
	for step := 0; step < 40 && next < len(order); step++ {
		view.Key(0, ui.KeyTab)
		if view.Focused(order[next]) {
			next++
		}
	}
	if next != len(order) {
		t.Fatalf("keyboard could not reach %q in form order", order[next])
	}
}

func TestBatchMoveFormCannotChangeDuringExecution(t *testing.T) {
	a, backend, settle := moveFixture(t, []string{"test/a/b", "test/b/c"}, nil)
	a.chooseBatchFolder("test0")
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
	if err := view.Click(a.t("移动 %d 项", 2)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-interrupted.started:
	case <-time.After(time.Second):
		t.Fatal("move did not start")
	}
	view.Frame()
	if !view.HasText(a.t("正在移动 %d/%d", 0, 2)) {
		t.Fatalf("running move did not show its progress: %v", view.Texts())
	}
	for _, key := range []string{"仅条目名", "完整路径", "移到回收站"} {
		_ = view.Click(a.t(key))
	}
	if a.batch.keep != "" || !a.busy || len(backend.actions("trash")) != 0 {
		t.Fatal("a running move allowed its option or another action to change")
	}
	// Release the worker explicitly before settling; the deferred close protects
	// failing assertions without leaving a test goroutine blocked.
	release()
	settle()
	moves := backend.actions("move")
	if len(moves) != 2 || moves[0].Destination != "test0/a/b" || moves[1].Destination != "test0/b/c" {
		t.Fatalf("move did not use the option chosen before it started: %#v", moves)
	}
}

func TestBatchMovePreviewListsEachPathAndSkippedReasons(t *testing.T) {
	for _, test := range []struct {
		language string
		width    int
	}{{"zh-CN", 1180}, {"en", 1180}, {"en", 860}} {
		t.Run(fmt.Sprintf("%s-%d", test.language, test.width), func(t *testing.T) {
			a, backend, settle := moveFixture(t, []string{"test/a/b", "test/b/c", "test/b/d", "archive/d", "archive/c"}, nil)
			delete(a.batch.selected, "archive/d")
			delete(a.batch.selected, "archive/c")
			a.state.Settings.Language = test.language
			a.chooseBatchFolder("archive")
			a.batch.keep = batchKeepName
			view := ui.NewTester(testView(a), test.width, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			for _, text := range []string{a.t("将移动 %d 项，跳过 %d 项", 1, 2), "archive/b", "test/b/c", "test/b/d", a.t("目标位置已有同名条目，跳过")} {
				if !view.HasText(text) {
					t.Fatalf("preview missing %q: %v", text, view.Texts())
				}
			}
			// Skipped entries come first so their reasons are visible.
			list := visualBounds(t, view, a.t("原路径与新路径"))
			skipped := visualBoundsIn(t, view, a.t("目标位置已有同名条目，跳过"), list)
			ready := visualBoundsIn(t, view, "test/a/b", list)
			if skipped.Y > ready.Y {
				t.Fatalf("skipped entries were listed after ready ones: skipped=%v ready=%v", skipped, ready)
			}
			managerResizeScreenshot(t, view, fmt.Sprintf("batch-move-preview-%s-%d", test.language, test.width))
			a.chooseBatchFolder("")
			a.batch.keep = batchKeepFull
			view.Frame()
			if !view.HasText(a.t("路径不变，跳过")) || !view.HasText(a.t("没有可移动的条目，请换一个目标文件夹或保留方式。")) {
				t.Fatalf("unchanged paths were not explained: %v", view.Texts())
			}
			a.chooseBatchFolder("archive")
			a.batch.keep = ""
			view.Frame()
			if err := view.Click(a.t("移动 %d 项", 3)); err != nil {
				t.Fatal(err)
			}
			settle()
			if len(backend.actions("move")) != 3 || a.batch.active {
				t.Fatal("the default move did not send every ready entry")
			}
		})
	}
}

func TestBatchMovePreviewDuplicatesAreMarked(t *testing.T) {
	a, _, _ := moveFixture(t, []string{"test/a/x", "test/b/x"}, nil)
	a.chooseBatchFolder("next")
	a.batch.keep = batchKeepName
	view := ui.NewTester(testView(a), 1180, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	if !view.HasText(a.t("与其他所选条目重名，跳过")) || !view.HasText(a.t("将移动 %d 项，跳过 %d 项", 0, 2)) {
		t.Fatalf("duplicate targets were not marked: %v", view.Texts())
	}
}

func TestBatchMoveStripWithRealVaultPreservesContents(t *testing.T) {
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
	for _, label := range []string{"选择", "选择条目 test/a/b", "选择条目 test/b/c", "搜索目标文件夹"} {
		if err := view.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	view.Type("test0")
	if err := view.Click("新建文件夹 test0"); err != nil {
		t.Fatal(err)
	}
	for a.busy {
		awaitMain(t, callbacks)
	}
	view.Frame()
	if !view.HasText("test0/a/b") || !view.HasText("test0/b/c") {
		t.Fatalf("preview did not remove the shared folder: %v", view.Texts())
	}
	if err := view.Click("移动 2 项"); err != nil {
		t.Fatal(err)
	}
	for a.busy {
		awaitMain(t, callbacks)
	}
	for original, content := range contents {
		destination := "test0/" + original[len("test/"):]
		response := service.Execute(app.Request{Action: "fetch", Entry: destination})
		if response.Status != "ok" || response.Data.Contents != content {
			t.Fatalf("real move changed contents at %q: %s", destination, response.Code)
		}
		if _, err := os.Stat(filepath.Join(a.state.Directory, filepath.FromSlash(original)+".gpg")); !os.IsNotExist(err) {
			t.Fatalf("original encrypted file still exists: %q, %v", original, err)
		}
		if info, err := os.Stat(filepath.Join(a.state.Directory, filepath.FromSlash(destination)+".gpg")); err != nil || info.Size() == 0 {
			t.Fatalf("destination encrypted file is missing: %q, %v", destination, err)
		}
	}
}
