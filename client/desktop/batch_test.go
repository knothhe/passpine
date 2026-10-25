package desktop

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

// batchBackend keeps entry paths in memory and fails chosen entries with a code.
// While locked it rejects trash and restore, and moves into policy, a folder
// standing in for one whose recipients require re-encryption.
type batchBackend struct {
	mu       sync.Mutex
	locked   bool
	policy   string
	paths    []string
	folders  []string
	failures map[string]string
	trash    map[string]string
	requests []app.Request
}

func (b *batchBackend) Execute(r app.Request) app.Response {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.requests = append(b.requests, r)
	data := app.Data{VaultID: "vault", Configured: true, Settings: app.DefaultSettings()}
	data.Settings.Language, data.Settings.BrowseWhileLocked = "zh-CN", true
	if code := b.failures[r.Entry]; code != "" && r.Action != "list" {
		return app.Response{Status: "error", Code: code, Data: b.metadata(data)}
	}
	if r.Action == "unlock" {
		b.locked = false
	}
	data.Locked = b.locked
	reencrypt := r.Action == "move" && b.policy != "" && strings.HasPrefix(r.Destination, b.policy+"/")
	if b.locked && (reencrypt || r.Action == "trash" || r.Action == "restore") {
		return app.Response{Status: "error", Code: "locked", Data: b.metadata(data)}
	}
	switch r.Action {
	case "create-folder":
		parts := strings.Split(r.Destination, "/")
		for i := range parts {
			folder := strings.Join(parts[:i+1], "/")
			if !containsFolder(b.folders, folder) {
				b.folders = append(b.folders, folder)
			}
		}
		data.Destination = r.Destination
	case "move":
		b.rename(r.Entry, r.Destination)
		data.Entry = r.Destination
		data.Revision = "moved:" + r.Destination
	case "trash":
		b.rename(r.Entry, "")
		data.TrashID = "trash:" + r.Entry
		b.trash[data.TrashID] = r.Entry
	case "restore":
		data.Entry = b.trash[r.ID]
		b.paths = append(b.paths, data.Entry)
	}
	return app.Response{Status: "ok", Data: b.metadata(data)}
}

func (b *batchBackend) metadata(data app.Data) app.Data {
	data.Folders = append([]string{}, b.folders...)
	data.EntriesVersion = uint64(len(b.requests))
	for _, path := range b.paths {
		folder, name := splitEntryPath(path)
		data.Entries = append(data.Entries, app.Entry{Path: path, Name: name, Folder: folder})
	}
	return data
}

func (b *batchBackend) rename(from, to string) {
	kept := b.paths[:0]
	for _, path := range b.paths {
		if path != from {
			kept = append(kept, path)
		}
	}
	b.paths = kept
	if to != "" {
		b.paths = append(b.paths, to)
	}
}

func (b *batchBackend) actions(action string) []app.Request {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []app.Request{}
	for _, r := range b.requests {
		if r.Action == action {
			out = append(out, r)
		}
	}
	return out
}

func batchFixture(t *testing.T, failures map[string]string) (*application, *batchBackend, func()) {
	t.Helper()
	backend := &batchBackend{failures: failures, trash: map[string]string{}, paths: []string{
		"开发/Cloudflare", "开发/GitHub", "工作/Figma", "工作/Linear", "工作/客户项目/GitHub", "归档/Linear",
		"工作/共享账号/Notion", "工作/财务/Stripe", "工作/招聘/Portal", "家庭/设备/NAS",
	}}
	a := fixture()
	a.state = backend.Execute(app.Request{Action: "list"}).Data
	backend.requests = nil
	a.service = backend
	callbacks := make(chan func(), 64)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	settle := func() {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for a.busy {
			select {
			case callback := <-callbacks:
				callback()
			case <-deadline:
				t.Fatal("batch operation did not finish")
			}
		}
	}
	return a, backend, settle
}

func TestBatchMoveSkipsConflictsAndKeepsThemSelected(t *testing.T) {
	a, backend, settle := batchFixture(t, nil)
	a.enterBatch()
	a.batch.keep = batchKeepName // Exercise conflicts when keeping only entry names.
	for _, path := range []string{"开发/Cloudflare", "开发/GitHub", "工作/Linear", "工作/客户项目/GitHub"} {
		a.batch.selected[path] = true
	}
	a.batch.destination, a.batch.chosen = "归档", true
	plan := a.batchMovePlan()
	if len(plan.ready) != 1 || len(plan.skipped) != 3 || plan.status["工作/Linear"] != batchMoveExists || plan.status["开发/GitHub"] != batchMoveDuplicate {
		t.Fatalf("plan should skip the existing Linear and both GitHubs: %#v", plan)
	}
	a.startBatchMove()
	if a.batch.progress == nil || a.batch.progress.total != 1 {
		t.Fatal("move did not report progress for the entries it sends")
	}
	settle()
	moves := backend.actions("move")
	if len(moves) != 1 || moves[0].Entry != "开发/Cloudflare" || moves[0].Destination != "归档/Cloudflare" || moves[0].VaultID != "vault" {
		t.Fatalf("unexpected move requests: %#v", moves)
	}
	if !a.batch.active || len(a.batch.selected) != 3 || a.batch.selected["开发/Cloudflare"] || a.batch.chosen {
		t.Fatalf("skipped entries should stay selected for another folder: %#v", a.batch.selected)
	}
	if a.notice != "已将 1 项移动到“归档”" || a.notifications.action != "撤销" || a.notifications.onAction == nil {
		t.Fatalf("move did not report its outcome with an undo action: %q %q", a.notice, a.notifications.action)
	}
	a.notifications.onAction()
	settle()
	if !a.batch.active || !a.batch.selected["开发/Cloudflare"] || len(backend.actions("move")) != 2 {
		t.Fatal("undo did not restore the moved entry")
	}
}

func TestBatchMovePartialFailureRetriesOnlyRetryableEntries(t *testing.T) {
	a, backend, settle := batchFixture(t, map[string]string{"工作/Figma": "vault_busy", "工作/Linear": "source_remains"})
	a.enterBatch()
	a.batch.keep = batchKeepName
	for _, path := range []string{"开发/Cloudflare", "工作/Figma", "工作/Linear"} {
		a.batch.selected[path] = true
	}
	a.batch.destination, a.batch.chosen = "家庭/设备", true
	a.startBatchMove()
	settle()
	result := a.batch.result
	if result == nil || result.completed != 1 || len(result.failures) != 2 || len(a.batch.selected) != 2 {
		t.Fatalf("partial failure was not kept for review: %#v %#v", result, a.batch.selected)
	}
	delete(backend.failures, "工作/Figma")
	delete(backend.failures, "工作/Linear")
	a.retryBatch()
	settle()
	moves := backend.actions("move")
	if last := moves[len(moves)-1]; len(moves) != 4 || last.Entry != "工作/Figma" {
		t.Fatalf("retry should resend only the busy entry: %#v", moves)
	}
	if a.batch.result == nil || len(a.batch.result.failures) != 1 || a.batch.result.failures[0].code != "source_remains" {
		t.Fatal("an entry whose source remains must not be retried automatically")
	}
}

func TestBatchTrashOffersUndoThatRestoresTheSelection(t *testing.T) {
	a, backend, settle := batchFixture(t, nil)
	a.enterBatch()
	a.batch.selected["开发/Cloudflare"], a.batch.selected["工作/Figma"] = true, true
	a.startBatchTrash()
	settle()
	if a.batch.active || len(a.state.Entries) != 8 || len(backend.actions("trash")) != 2 {
		t.Fatal("trash did not move both entries and leave selection mode")
	}
	if a.notice != "已将 2 项移到回收站" || a.notifications.description != "恢复副本仅保存在此设备" || a.notifications.action != "撤销" || a.notifications.timeout != 8*time.Second {
		t.Fatalf("trash toast is missing its undo: %#v", a.notifications)
	}
	a.notifications.onAction()
	settle()
	if len(backend.actions("restore")) != 2 || len(a.state.Entries) != 10 {
		t.Fatal("undo did not restore both entries")
	}
	if !a.batch.active || !a.batch.selected["开发/Cloudflare"] || !a.batch.selected["工作/Figma"] || a.notice != "已撤销" {
		t.Fatal("undo should return to the restored selection")
	}
}

func TestBatchFolderTreeStartsCollapsedAndSearchShowsPaths(t *testing.T) {
	a, _, _ := batchFixture(t, nil)
	a.enterBatch()
	a.batch.selected["开发/GitHub"] = true
	view := ui.NewTester(testView(a), 1180, 800)
	view.Advance(time.Second)
	if !view.HasText("已选 1 项") || !view.HasText("移动到") {
		t.Fatal("selection panel is missing its count or move section")
	}
	if _, found := view.Find("展开“工作”"); !found {
		t.Fatal("every parent folder should offer an expand toggle")
	}
	if _, found := view.Find("展开“家庭”"); !found {
		t.Fatal("a folder with one subfolder must also offer an expand toggle")
	}
	if _, found := view.Find("展开“开发”"); found {
		t.Fatal("a leaf folder must not offer an expand toggle")
	}
	if _, found := view.Find("客户项目"); found {
		t.Fatal("the tree should start fully collapsed")
	}
	if err := view.Click("展开“工作”"); err != nil {
		t.Fatal(err)
	}
	if _, found := view.Find("客户项目"); !found || !a.batch.expanded["工作"] {
		t.Fatal("expanding did not show subfolders")
	}
	if err := view.Click("搜索目标文件夹"); err != nil {
		t.Fatal(err)
	}
	view.Type("客户")
	_, path := view.Find("工作/客户项目")
	_, other := view.Find("选择文件夹 家庭")
	if !path || other {
		t.Fatalf("search should list matching full paths only: %v", view.Texts())
	}
	if err := view.Click("选择文件夹 工作/客户项目"); err != nil {
		t.Fatal(err)
	}
	if !a.batch.chosen || a.batch.destination != "工作/客户项目" {
		t.Fatal("choosing a search result did not set the destination")
	}
}

func TestBatchSelectionEndsWithFolderChangesAndLock(t *testing.T) {
	a, _, _ := batchFixture(t, nil)
	a.enterBatch()
	a.batch.selected["开发/GitHub"] = true
	a.query = "git"
	a.filtered = a.entries()
	a.toggleBatchAll()
	if !a.batch.selected["工作/客户项目/GitHub"] || a.batch.selected["工作/Figma"] {
		t.Fatal("select all should cover only the visible search results")
	}
	a.query = ""
	if len(a.selectedBatchEntries()) != 2 {
		t.Fatal("clearing the search must keep the selection")
	}
	a.batch.pending = "move"
	a.lock()
	if !a.batch.active || len(a.batch.selected) != 2 || a.batch.pending != "" {
		t.Fatal("locking should keep the chosen paths but cancel the pending action")
	}
	a.navigateManager("folder:工作")
	if a.batch.active {
		t.Fatal("switching folders should end selection mode")
	}
}

func TestLockedSelectionMovesWithoutUnlock(t *testing.T) {
	a, backend, settle := batchFixture(t, nil)
	backend.locked = true
	a.state.Locked = true
	view := ui.NewTester(testView(a), 1180, 800)
	view.Advance(time.Second)
	if err := view.Click("选择"); err != nil || !a.batch.active {
		t.Fatalf("selection should be available while browsing locked: %v", err)
	}
	a.batch.keep = batchKeepName
	a.batch.selected["开发/Cloudflare"], a.batch.selected["工作/Figma"] = true, true
	view.Frame()
	if err := view.Click("展开“家庭”"); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("选择文件夹 家庭/设备"); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("移动 2 项"); err != nil {
		t.Fatal(err)
	}
	if a.batch.pending != "" || a.pendingUnlock.action != "" {
		t.Fatal("a move that changes no entry contents asked to unlock")
	}
	settle()
	moves := backend.actions("move")
	destinations := map[string]bool{}
	for _, move := range moves {
		destinations[move.Destination] = true
	}
	if len(moves) != 2 || !destinations["家庭/设备/Cloudflare"] || !destinations["家庭/设备/Figma"] || len(backend.actions("unlock")) != 0 {
		t.Fatalf("the locked move did not run directly: %#v", moves)
	}
	if a.batch.active || !a.state.Locked || a.notice != "已将 2 项移动到“家庭/设备”" {
		t.Fatalf("locked move did not finish normally: %q", a.notice)
	}
}

func TestLockedMoveAsksToUnlockOnlyForReencryption(t *testing.T) {
	a, backend, settle := batchFixture(t, nil)
	backend.locked, backend.policy, a.state.Locked = true, "家庭", true
	a.enterBatch()
	a.batch.keep = batchKeepName
	a.batch.selected["开发/Cloudflare"] = true
	a.chooseBatchFolder("家庭/设备")
	a.startBatchMove()
	settle()
	if len(backend.actions("move")) != 1 || a.batch.result == nil || a.batch.pending != "retry" || a.pendingUnlock.action != "batch" {
		t.Fatal("a move needing re-encryption did not ask to unlock and retry")
	}
	view := ui.NewTester(testView(a), 1180, 800)
	view.Advance(time.Second)
	if !view.HasText("解锁后将重试 1 项。") || !view.HasText("返回选择") {
		t.Fatalf("unlock view does not explain the pending retry: %v", view.Texts())
	}
	if err := view.Click("返回选择"); err != nil || a.batch.pending != "" || a.pendingUnlock.action != "" {
		t.Fatalf("cancelling should return to the result: %v", err)
	}
	a.retryBatch()
	if a.batch.pending != "retry" || len(backend.actions("move")) != 1 {
		t.Fatal("retrying a locked failure did not wait for unlock")
	}
	a.passphrase = "test-passphrase"
	a.unlock("")
	settle()
	if moves := backend.actions("move"); len(moves) != 2 || moves[1].Destination != "家庭/设备/Cloudflare" || a.batch.result != nil {
		t.Fatalf("the move did not continue after unlocking: %#v", moves)
	}
}
