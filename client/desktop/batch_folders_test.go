package desktop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestBatchCreateNestedFolderSelectsVisibleLeafAndMovesEntries(t *testing.T) {
	a, backend, settle := batchFixture(t, nil)
	a.enterBatch()
	a.batch.flatten = true
	a.batch.selected["开发/GitHub"] = true
	delete(a.batch.expanded, "工作")
	view := ui.NewTester(testView(a), 1180, 800)
	view.Advance(time.Second)
	if err := view.Click("搜索目标文件夹"); err != nil {
		t.Fatal(err)
	}
	view.Type("工作/test/a/b/c")
	if err := view.Click("新建文件夹 工作/test/a/b/c"); err != nil {
		t.Fatal(err)
	}
	if !a.busy || a.batch.chosen || a.batch.folderQuery != "工作/test/a/b/c" {
		t.Fatal("folder was selected before creation completed")
	}
	settle()
	if !a.batch.chosen || a.batch.destination != "工作/test/a/b/c" || a.batch.folderQuery != "工作/test/a/b/c" || !a.batch.expanded["工作"] {
		t.Fatalf("created leaf was not selected and revealed: %#v", a.batch)
	}
	view.Frame()
	if err := view.Click("搜索目标文件夹"); err != nil {
		t.Fatal(err)
	}
	view.Key(ui.Cmd, ui.KeyA)
	view.Key(0, ui.KeyBackspace)
	for _, folder := range []string{"工作/test", "工作/test/a", "工作/test/a/b", "工作/test/a/b/c"} {
		if _, found := view.Find("选择文件夹 " + folder); !found {
			t.Fatalf("new folder %q is missing from the tree", folder)
		}
	}
	if !view.HasText("已创建并选中文件夹“工作/test/a/b/c”") {
		t.Fatal("creation success feedback is missing")
	}
	if err := view.Click("选择文件夹 工作/test/a/b/c"); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("移动 1 项"); err != nil {
		t.Fatal(err)
	}
	settle()
	moves := backend.actions("move")
	if len(moves) != 1 || moves[0].Destination != "工作/test/a/b/c/GitHub" {
		t.Fatalf("move did not use the newly created leaf: %#v", moves)
	}
}

func TestBatchCreateFolderFailuresKeepInputAndSelection(t *testing.T) {
	for _, code := range []string{"operation_failed", "vault_busy"} {
		t.Run(code, func(t *testing.T) {
			a, backend, settle := batchFixture(t, map[string]string{"": code})
			a.enterBatch()
			a.batch.selected["开发/GitHub"] = true
			a.batch.folderQuery = "test/a/b/c"
			a.batch.destination, a.batch.chosen = "归档", true
			a.createBatchFolder()
			settle()
			if a.errorText == "" || a.batch.folderQuery != "test/a/b/c" || a.batch.destination != "归档" || !a.batch.chosen || !a.batch.selected["开发/GitHub"] {
				t.Fatalf("creation failure lost feedback or recoverable state: %q %#v", a.errorText, a.batch)
			}
			view := ui.NewTester(testView(a), 1180, 800)
			if !view.HasText("操作未完成") || !view.HasText("已选 1 项") {
				t.Fatal("creation failure or preserved selection was not displayed")
			}
			delete(backend.failures, "")
			a.createBatchFolder()
			settle()
			if a.batch.destination != "test/a/b/c" || !a.batch.chosen || len(backend.actions("create-folder")) != 2 {
				t.Fatal("creation could not be retried with the preserved input")
			}
		})
	}
}

func TestBatchCreateFolderRejectsInvalidInputWithFeedback(t *testing.T) {
	a, backend, _ := batchFixture(t, nil)
	a.enterBatch()
	a.batch.selected["开发/GitHub"] = true
	view := ui.NewTester(testView(a), 1180, 800)
	view.Advance(time.Second)
	if err := view.Click("搜索目标文件夹"); err != nil {
		t.Fatal(err)
	}
	view.Type("test/../escape")
	if err := view.Click("新建文件夹 test/../escape"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !view.HasText("文件夹路径无效，请使用 / 分隔层级，并避免空名称和特殊字符。") || a.batch.folderQuery != "test/../escape" || a.batch.chosen || len(backend.actions("create-folder")) != 0 {
		t.Fatal("invalid input was silently accepted or discarded")
	}
}

func TestBatchCreateFolderWaitsForUnlockAndPreservesInputOnLanguageChange(t *testing.T) {
	a, backend, settle := batchFixture(t, nil)
	backend.locked, a.state.Locked = true, true
	a.enterBatch()
	a.batch.selected["开发/GitHub"] = true
	a.batch.folderQuery = "test/a/b/c"
	a.createBatchFolder()
	if a.batch.pending != "create-folder" || len(backend.actions("create-folder")) != 0 {
		t.Fatal("locked creation did not wait for authentication")
	}
	if got := a.batchUnlockDescription(); got != "解锁后将新建文件夹“test/a/b/c”。" {
		t.Fatal("unlock description does not describe the folder creation", got)
	}
	a.state.Settings.Language = "en"
	if got := a.batchUnlockDescription(); got != "After unlocking, folder “test/a/b/c” will be created." || a.batch.folderQuery != "test/a/b/c" || !a.batch.selected["开发/GitHub"] {
		t.Fatal("language change lost the pending creation or its input", got)
	}
	a.passphrase = "test-passphrase"
	a.unlock("")
	settle()
	if a.batch.pending != "" || !a.batch.chosen || a.batch.destination != "test/a/b/c" || len(backend.actions("create-folder")) != 1 {
		t.Fatal("folder creation did not resume once after unlocking")
	}
}

func TestBatchCreateFolderIgnoresSuccessAfterLeavingSelection(t *testing.T) {
	for _, reenter := range []bool{false, true} {
		a, backend, settle := batchFixture(t, nil)
		a.enterBatch()
		a.batch.selected["开发/GitHub"] = true
		a.batch.folderQuery = "test/a/b/c"
		a.createBatchFolder()
		a.exitBatch()
		if reenter {
			a.enterBatch()
			a.batch.selected["工作/Figma"] = true
			a.batch.folderQuery = "another/folder"
		}
		settle()
		if a.batch.active != reenter || a.batch.chosen || a.notice != "" || len(backend.actions("create-folder")) != 1 {
			t.Fatal("late creation response restored an abandoned selection")
		}
		if a.state.VaultID != "vault" {
			t.Fatal("late response changed the vault")
		}
	}
}

func TestBatchCreateFolderWithRealVault(t *testing.T) {
	for _, test := range []struct {
		language string
		width    int
	}{{"zh-CN", 1180}, {"en", 960}} {
		t.Run(test.language, func(t *testing.T) {
			service, err := app.New(app.Options{DataDir: t.TempDir(), Preview: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { service.Close() })
			settings := service.Execute(app.Request{Action: "configure"}).Data.Settings
			settings.Language = test.language
			if response := service.Execute(app.Request{Action: "settings", Settings: &settings}); response.Status != "ok" {
				t.Fatal(response.Message)
			}
			a := newApplication(service)
			a.state = service.Execute(app.Request{Action: "configure"}).Data
			callbacks := make(chan func(), 16)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.enterBatch()
			a.batch.selected["工作/GitHub"] = true
			a.batch.flatten = true
			view := ui.NewTester(testView(a), test.width, 700)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			settle := func() {
				t.Helper()
				for a.busy {
					awaitMain(t, callbacks)
				}
				view.Frame()
				view.Advance(time.Second)
			}
			if err := view.Click(a.t("搜索目标文件夹")); err != nil {
				t.Fatal(err)
			}
			view.Type("test/a/b/c")
			if err := view.Click(a.t("新建文件夹 %s", "test/a/b/c")); err != nil {
				t.Fatal(err)
			}
			settle()
			if !a.batch.chosen || a.batch.destination != "test/a/b/c" {
				t.Fatal("real folder creation did not select the leaf")
			}
			if info, err := os.Stat(filepath.Join(a.state.Directory, "test/a/b/c")); err != nil || !info.IsDir() {
				t.Fatal("UI did not create the actual hierarchy", err)
			}
			leaf, found := view.Find(a.t("选择文件夹 %s", "test/a/b/c"))
			panel, _ := view.Find(a.t("选择目标文件夹"))
			if !found || leaf.Y < panel.Y || leaf.Y+leaf.H > panel.Y+panel.H {
				t.Fatalf("selected leaf was not scrolled into view: leaf=%v panel=%v", leaf, panel)
			}
			managerResizeScreenshot(t, view, "batch-folder-created-"+test.language)
			if err := view.Click(a.t("移动 %d 项", 1)); err != nil {
				t.Fatal(err)
			}
			settle()
			moved := service.Execute(app.Request{Action: "fetch", Entry: "test/a/b/c/GitHub"})
			if moved.Status != "ok" || moved.Data.Username != "alex@example.com" {
				t.Fatal("move into the created hierarchy failed", moved.Code)
			}
			if _, err := os.Stat(filepath.Join(a.state.Directory, "工作", "GitHub.gpg")); !os.IsNotExist(err) {
				t.Fatal("move left the original entry behind", err)
			}
		})
	}
}
