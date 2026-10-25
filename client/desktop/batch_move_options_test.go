package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestBatchCommonFoldersOfferOnlyFoldersEverySelectedEntryShares(t *testing.T) {
	entry := func(path string) app.Entry {
		folder, name := splitEntryPath(path)
		return app.Entry{Path: path, Folder: folder, Name: name}
	}
	for _, test := range []struct {
		paths []string
		want  []string
	}{
		{[]string{"test/a/b", "test/a/c/d"}, []string{"test", "test/a"}},
		{[]string{"test/a/c/d"}, []string{"test", "test/a", "test/a/c"}},
		{[]string{"test/a/b", "testing/c"}, nil},
		{[]string{"test/a/b", "test/A/c"}, []string{"test"}},
		{[]string{"test/a/b", "root-entry"}, nil},
		{nil, nil},
	} {
		entries := []app.Entry{}
		for _, path := range test.paths {
			entries = append(entries, entry(path))
		}
		if got := batchCommonFolders(entries); !slices.Equal(got, test.want) {
			t.Errorf("batchCommonFolders(%v) = %v, want %v", test.paths, got, test.want)
		}
	}
}

func TestBatchMoveDefaultsToRemovingTheDeepestSharedFolder(t *testing.T) {
	a, _, _ := moveFixture(t, []string{"test/a/b", "test/a/c/d"}, nil)
	a.chooseBatchFolder("test0")
	want := map[string]map[string]string{
		"":             {"test/a/b": "test0/b", "test/a/c/d": "test0/c/d"},
		batchKeepFull:  {"test/a/b": "test0/test/a/b", "test/a/c/d": "test0/test/a/c/d"},
		batchKeepName:  {"test/a/b": "test0/b", "test/a/c/d": "test0/d"},
		batchKeepStrip: {"test/a/b": "test0/b", "test/a/c/d": "test0/c/d"},
	}
	for _, mode := range []string{"", batchKeepFull, batchKeepName, batchKeepStrip} {
		a.batch.keep = mode
		plan := a.batchMovePlan()
		for path, target := range want[mode] {
			if plan.target[path] != target {
				t.Fatalf("keep %q moved %q to %q, want %q", mode, path, plan.target[path], target)
			}
		}
	}
	a.chooseBatchStripFolder("test")
	if a.batch.keep != batchKeepStrip || a.batch.stripFrom != "test" || a.batchMovePlan().target["test/a/c/d"] != "test0/a/c/d" {
		t.Fatal("choosing a shallower shared folder did not keep the structure beneath it")
	}
	a.chooseBatchStripFolder("test/a/c")
	a.chooseBatchStripFolder("unrelated")
	if a.batch.stripFrom != "test" {
		t.Fatal("a folder not shared by every selected entry became the removed prefix")
	}
}

func TestBatchMoveWithoutSharedFolderFallsBackToFullPath(t *testing.T) {
	a, _, _ := moveFixture(t, []string{"test/a/b", "other/c", "root-entry"}, nil)
	a.chooseBatchFolder("archive")
	mode, _, common := a.batchKeep(a.selectedBatchEntries())
	if mode != batchKeepFull || len(common) != 0 {
		t.Fatalf("no shared folder resolved to %q with %v", mode, common)
	}
	a.setBatchKeep(batchKeepStrip)
	if a.batch.keep != "" || a.batchMovePlan().target["test/a/b"] != "archive/test/a/b" {
		t.Fatal("the unavailable option could still be chosen")
	}
	// An explicit choice survives selection changes and applies again once the
	// selection shares a folder.
	a.batch.keep = batchKeepStrip
	if a.batchMovePlan().target["other/c"] != "archive/other/c" {
		t.Fatal("an unavailable strip choice did not fall back to the full path")
	}
	delete(a.batch.selected, "other/c")
	delete(a.batch.selected, "root-entry")
	if a.batchMovePlan().target["test/a/b"] != "archive/b" {
		t.Fatal("the strip choice did not return once the selection shared a folder")
	}
}

func TestBatchMoveOptionsRecomputeConflictsAndRootNoOps(t *testing.T) {
	a, _, _ := moveFixture(t, []string{"test/a/b", "test/c/b", "TEST0/test/a/b", "test0/b"}, nil)
	delete(a.batch.selected, "TEST0/test/a/b")
	delete(a.batch.selected, "test0/b")
	a.chooseBatchFolder("test0")
	a.setBatchKeep(batchKeepFull)
	plan := a.batchMovePlan()
	if len(plan.ready) != 1 || plan.ready[0].Path != "test/c/b" || plan.status["test/a/b"] != batchMoveExists {
		t.Fatalf("keeping full paths ignored case-insensitive existing paths: %#v", plan)
	}
	a.setBatchKeep(batchKeepName)
	plan = a.batchMovePlan()
	if len(plan.ready) != 0 || plan.status["test/a/b"] != batchMoveExists || plan.status["test/c/b"] != batchMoveExists {
		t.Fatalf("entry names did not recompute conflicts: %#v", plan)
	}
	a.chooseBatchFolder("")
	a.setBatchKeep(batchKeepFull)
	plan = a.batchMovePlan()
	if len(plan.ready) != 0 || plan.status["test/a/b"] != batchMoveUnchanged || plan.status["test/c/b"] != batchMoveUnchanged {
		t.Fatalf("keeping full paths at the vault root should leave paths unchanged: %#v", plan)
	}
	a.setBatchKeep(batchKeepName)
	plan = a.batchMovePlan()
	if len(plan.ready) != 0 || plan.status["test/a/b"] != batchMoveDuplicate || plan.status["test/c/b"] != batchMoveDuplicate {
		t.Fatalf("entry names at the root should refuse duplicate names: %#v", plan)
	}
}

func TestBatchKeepOptionsChangeOnlyThePreview(t *testing.T) {
	for _, test := range []struct {
		language string
		width    int
	}{{"zh-CN", 1180}, {"en", 1180}, {"en", 860}} {
		t.Run(fmt.Sprintf("%s-%d", test.language, test.width), func(t *testing.T) {
			a, backend, _ := moveFixture(t, []string{"test/a/b", "test/a/c/d"}, nil)
			a.state.Settings.Language = test.language
			view := ui.NewTester(testView(a), test.width, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			if !view.HasText(a.t("选择目标文件夹后，这里会列出每一项的新路径。")) || !view.HasText("…/c/d") || !view.HasText("…/test/a/c/d") || !view.HasText("test/a") {
				t.Fatalf("options without a destination did not show their examples: %v", view.Texts())
			}
			a.chooseBatchFolder("archive")
			view.Frame()
			for _, text := range []string{"archive/c/d", "archive/test/a/c/d", "archive/d", a.t("将移动 %d 项", 2), "test/a/b"} {
				if !view.HasText(text) {
					t.Fatalf("preview or examples missing %q: %v", text, view.Texts())
				}
			}
			managerResizeScreenshot(t, view, fmt.Sprintf("batch-move-keep-%s-%d", test.language, test.width))
			for step := 0; step < 40 && !view.Focused(a.t("要去掉的公共前缀")); step++ {
				view.Key(0, ui.KeyTab)
			}
			if !view.Focused(a.t("要去掉的公共前缀")) {
				t.Fatal("the removed prefix selector is not reachable by keyboard")
			}
			view.Key(0, ui.KeyEnter)
			if err := view.Click(a.t("去掉公共前缀 %s", "test")); err != nil {
				t.Fatal(err)
			}
			if a.batch.keep != batchKeepStrip || a.batch.stripFrom != "test" || !view.HasText("archive/a/c/d") {
				t.Fatalf("choosing the removed prefix did not choose that option: %q %q", a.batch.keep, a.batch.stripFrom)
			}
			if err := view.Click(a.t("仅条目名")); err != nil {
				t.Fatal(err)
			}
			if a.batch.keep != batchKeepName || a.batchMovePlan().target["test/a/c/d"] != "archive/d" || !view.HasText("archive/d") || view.HasText(a.t("要去掉的公共前缀")) {
				t.Fatalf("choosing entry names did not update the preview: %v", view.Texts())
			}
			a.busy = true
			view.Frame()
			_ = view.Click(a.t("完整路径"))
			a.setBatchKeep(batchKeepFull)
			if a.batch.keep != batchKeepName {
				t.Fatal("a running operation allowed its move option to change")
			}
			a.busy = false
			if len(backend.actions("move")) != 0 || len(backend.actions("unlock")) != 0 || len(a.batch.selected) != 2 {
				t.Fatal("choosing a move option executed a vault action or changed the selection")
			}
		})
	}
}

func TestBatchKeepUnavailableOptionExplainsWhy(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		t.Run(language, func(t *testing.T) {
			a, _, _ := moveFixture(t, []string{"test/a/b", "other/c"}, nil)
			a.state.Settings.Language = language
			a.chooseBatchFolder("archive")
			view := ui.NewTester(testView(a), 860, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			if !view.HasText(a.t("所选条目没有共同的上级文件夹。")) || !view.HasText("archive/test/a/b") {
				t.Fatalf("missing shared folder was not explained or full paths were not the default: %v", view.Texts())
			}
			_ = view.Click(a.t("去掉公共前缀"))
			if a.batch.keep != "" {
				t.Fatal("the disabled option could be chosen")
			}
			managerResizeScreenshot(t, view, "batch-move-keep-unavailable-"+language)
		})
	}
}

func TestBatchKeepOptionsWithRealVaultPreserveContents(t *testing.T) {
	for _, mode := range []string{"", batchKeepFull, batchKeepName} {
		t.Run("keep="+mode, func(t *testing.T) {
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
			if label := map[string]string{batchKeepFull: "完整路径", batchKeepName: "仅条目名"}[mode]; label != "" {
				if err := view.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			if err := view.Click("移动 2 项"); err != nil {
				t.Fatal(err)
			}
			for a.busy {
				awaitMain(t, callbacks)
			}
			for original, content := range contents {
				destination := map[string]string{"": "test0/" + original[len("test/"):], batchKeepFull: "test0/" + original, batchKeepName: "test0/" + original[len(original)-1:]}[mode]
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
