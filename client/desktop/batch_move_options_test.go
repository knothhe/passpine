package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestBatchMovePreservesFullPrefixByDefaultAndFlattenIsExplicit(t *testing.T) {
	a, _, _ := prefixFixture(t, []string{"test/a/b", "test/c/d", "root-entry"}, nil)
	a.setBatchMoveMode(false)
	a.chooseBatchFolder("test0")
	if a.batch.flatten {
		t.Fatal("a new selection did not default to preserving folder prefixes")
	}
	for _, entry := range a.selectedBatchEntries() {
		if got, want := a.batchEntryDestination(entry), "test0/"+entry.Path; got != want {
			t.Fatalf("default move destination = %q, want %q", got, want)
		}
	}
	a.setBatchMoveFlatten(true)
	for _, entry := range a.selectedBatchEntries() {
		if got, want := a.batchEntryDestination(entry), "test0/"+entry.Name; got != want {
			t.Fatalf("explicit flattened destination = %q, want %q", got, want)
		}
	}
	// This option belongs only to Move to; prefix replacement keeps its own
	// segment-preserving destination rules under either stored option.
	a.setBatchMoveMode(true)
	a.chooseBatchPrefix("test")
	a.batch.prefixTo = "test0"
	for _, flatten := range []bool{false, true} {
		a.setBatchMoveFlatten(flatten)
		for _, entry := range a.selectedBatchEntries() {
			want := entry.Path
			if entry.Folder == "test/a" {
				want = "test0/a/b"
			} else if entry.Folder == "test/c" {
				want = "test0/c/d"
			}
			if got := a.batchEntryDestination(entry); got != want {
				t.Fatalf("flatten=%t altered prefix replacement: %q, want %q", flatten, got, want)
			}
		}
	}
}

func TestBatchMovePrefixOptionsPreserveDraftsWithoutExecuting(t *testing.T) {
	for _, test := range []struct {
		language string
		width    int
	}{{"zh-CN", 1180}, {"en", 860}} {
		t.Run(test.language, func(t *testing.T) {
			a, backend, _ := prefixFixture(t, []string{"test/a/b", "test/c/d"}, nil)
			a.setBatchMoveMode(false)
			a.chooseBatchFolder("test")
			a.batch.prefixTo = "unfinished/prefix"
			a.state.Settings.Language = test.language
			view := ui.NewTester(testView(a), test.width, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			if !view.HasText(a.t("将原始文件夹路径完整保留在目标文件夹下。")) {
				t.Fatal("default prefix behavior was not explained")
			}
			if err := view.Click(a.t("不保留前缀")); err != nil {
				t.Fatal(err)
			}
			if !a.batch.flatten || !view.HasText(a.t("直接移入目标文件夹，只保留条目名称。")) {
				t.Fatal("discarding the prefix did not change the choice or its explanation")
			}
			for _, label := range []string{"修改前缀", "移动到"} {
				if err := view.Click(a.t(label)); err != nil {
					t.Fatal(err)
				}
			}
			if !a.batch.flatten || a.batch.destination != "test" || a.batch.folderQuery != "test" || a.batch.prefixTo != "unfinished/prefix" || !a.batch.chosen {
				t.Fatal("switching peer actions lost a move option or draft")
			}
			a.batch.folderQuery = "unfinished/folder"
			a.syncBatchFolderQuery()
			view.Frame()
			if err := view.Click(a.t("保留前缀")); err != nil {
				t.Fatal(err)
			}
			if a.batch.flatten || a.batch.folderQuery != "unfinished/folder" || a.batch.chosen || len(a.batch.selected) != 2 || a.batch.prefixTo != "unfinished/prefix" {
				t.Fatal("switching prefix behavior discarded a draft or selection")
			}
			a.busy = true
			view.Frame()
			if err := view.Click(a.t("不保留前缀")); err != nil {
				t.Fatal(err)
			}
			if a.batch.flatten {
				t.Fatal("a running operation allowed its move behavior to change")
			}
			a.busy = false
			if len(backend.actions("move")) != 0 || len(backend.actions("unlock")) != 0 || len(backend.actions("create-folder")) != 0 {
				t.Fatal("choosing move behavior executed a vault action")
			}
			view.Frame()
			managerResizeScreenshot(t, view, "batch-move-prefix-options-"+test.language)
		})
	}
}

func TestBatchMovePrefixOptionsRecomputeConflictsAndRootNoOps(t *testing.T) {
	a, _, _ := prefixFixture(t, []string{"test/a/b", "test/c/b", "TEST0/test/a/b", "test0/b"}, nil)
	delete(a.batch.selected, "TEST0/test/a/b")
	delete(a.batch.selected, "test0/b")
	a.setBatchMoveMode(false)
	a.chooseBatchFolder("test0")
	ready, conflicts, unchanged := a.batchMovePlan()
	if len(ready) != 1 || ready[0].Path != "test/c/b" || len(conflicts) != 1 || conflicts[0].Path != "test/a/b" || len(unchanged) != 0 {
		t.Fatalf("preserving prefixes ignored full-path conflicts: ready=%v conflicts=%v unchanged=%v", ready, conflicts, unchanged)
	}
	a.setBatchMoveFlatten(true)
	ready, conflicts, unchanged = a.batchMovePlan()
	if len(ready) != 0 || len(conflicts) != 2 || len(unchanged) != 0 {
		t.Fatal("flattening did not recompute duplicate-name conflicts")
	}
	a.setBatchMoveFlatten(false)
	a.chooseBatchFolder("")
	ready, conflicts, unchanged = a.batchMovePlan()
	if len(ready) != 0 || len(conflicts) != 0 || len(unchanged) != 2 {
		t.Fatal("preserving prefixes at the vault root should leave paths unchanged")
	}
	a.setBatchMoveFlatten(true)
	ready, conflicts, unchanged = a.batchMovePlan()
	if len(ready) != 0 || len(conflicts) != 2 || len(unchanged) != 0 {
		t.Fatal("flattening to the root should still refuse duplicate names")
	}
}

func TestBatchMovePrefixOptionsWithRealVaultPreserveContents(t *testing.T) {
	for _, flatten := range []bool{false, true} {
		t.Run(fmt.Sprintf("flatten=%t", flatten), func(t *testing.T) {
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
			contents := map[string]string{"test/a/b": "fixture-move-b\nusername: fixture-b\n", "test/c/d": "fixture-move-d\nusername: fixture-d\n"}
			for path, content := range contents {
				if response := service.Execute(app.Request{Action: "save", Entry: path, Contents: content, Mode: "create"}); response.Status != "ok" {
					t.Fatal(response.Code)
				}
			}
			if response := service.Execute(app.Request{Action: "create-folder", Destination: "test0"}); response.Status != "ok" {
				t.Fatal(response.Code)
			}
			a := newApplication(service)
			a.state = service.Execute(app.Request{Action: "configure"}).Data
			callbacks := make(chan func(), 16)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.enterBatch()
			for path := range contents {
				a.batch.selected[path] = true
			}
			a.chooseBatchFolder("test0")
			view := ui.NewTester(testView(a), 1180, 800)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			if flatten {
				if err := view.Click("不保留前缀"); err != nil {
					t.Fatal(err)
				}
			}
			if a.batch.flatten != flatten {
				t.Fatal("the UI did not retain the requested move behavior")
			}
			if err := view.Click("移动 2 项"); err != nil {
				t.Fatal(err)
			}
			for a.busy {
				awaitMain(t, callbacks)
			}
			for original, content := range contents {
				relative := original
				if flatten {
					_, relative = splitEntryPath(original)
				}
				destination := "test0/" + relative
				response := service.Execute(app.Request{Action: "fetch", Entry: destination})
				if response.Status != "ok" || response.Data.Contents != content {
					t.Fatalf("move changed contents at %q: %s", destination, response.Code)
				}
				if _, err := os.Stat(filepath.Join(a.state.Directory, filepath.FromSlash(original)+".gpg")); !os.IsNotExist(err) {
					t.Fatalf("original encrypted file still exists: %q, %v", original, err)
				}
				if info, err := os.Stat(filepath.Join(a.state.Directory, filepath.FromSlash(destination)+".gpg")); err != nil || info.Size() == 0 {
					t.Fatalf("destination encrypted file is missing: %q, %v", destination, err)
				}
			}
		})
	}
}
