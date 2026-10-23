package desktop

import (
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func readyGitHeaderState() app.Data {
	return app.Data{Configured: true, VaultID: "active-vault", GitAvailable: true, GitInitialized: true, GitRemote: "origin", GitStatusAt: "2026-10-23T12:00:00Z"}
}

func TestManagerLeavesGitStatusToHeader(t *testing.T) {
	a := fixture()
	state := readyGitHeaderState()
	state.GitError, state.GitConflict = "merge conflict", true
	a.applyGitState(state)
	view := ui.NewTester(a.managerColumnsView, 900, 600)
	if view.HasText("存在冲突") || view.HasText("merge conflict") {
		t.Fatal("vault area repeated the header Git status")
	}
	header := ui.NewTester(a.header, 900, 60)
	if !header.HasText("存在冲突") {
		t.Fatal("header did not show the Git status")
	}
}

func TestGitHeaderStatusOpensSyncSettingsAndPreservesInlineDraft(t *testing.T) {
	a := fixture()
	a.state.GitConflict = true
	a.editEntry()
	a.editor.password = "unsaved-local-password"
	a.editor.compose()
	before := a.editor
	view := ui.NewTester(a.header, 900, 60)
	if err := view.Click(a.gitStatusLabel()); err != nil {
		t.Fatal(err)
	}
	if a.page != "settings" || a.settingsState.Category != "vault" || a.editor != before || a.discardOpen {
		t.Fatal("sync settings lost the retained editor")
	}
}

func TestGitHeaderSyncUsesIndependentGitLaneAndPreservesEditor(t *testing.T) {
	a := fixture()
	git := readyGitHeaderState()
	git.GitError = "remote authentication rejected"
	a.applyGitState(git)
	a.state.VaultID = git.VaultID
	a.editor = editorState{open: true, path: "draft", contents: "unsaved draft", originalContents: "original"}
	a.busy, a.activeRequestAction = true, "save"
	beforeDetail, beforeEditor, beforeEntries, beforeSelection := a.detail, a.editor, append([]app.Entry(nil), a.state.Entries...), a.selected
	callbacks, requests, release := make(chan func(), 1), make(chan app.Request, 2), make(chan struct{})
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		<-release
		return app.Response{Status: "ok", Data: app.Data{VaultID: git.VaultID, GitAvailable: true, GitInitialized: true, GitRemote: "origin", GitStatusAt: git.GitStatusAt, Entries: []app.Entry{{Path: "must-not-replace"}}, Password: "must-not-replace"}}
	})
	view := ui.NewTester(a.header, 900, 60)
	if err := view.Click(a.t("同步密码库")); err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-requests:
		if request.Action != "git-sync" || request.VaultID != git.VaultID {
			t.Fatal("wrong sync request", request)
		}
	case <-time.After(time.Second):
		t.Fatal("sync did not start")
	}
	if !a.gitActionPending || !a.busy || a.activeRequestAction != "save" {
		t.Fatal("sync changed the credential queue")
	}
	if a.gitStatusLabel() != a.t("正在处理 Git 操作…") {
		t.Fatal("pending Git action is not shown by the header status")
	}
	close(release)
	awaitMain(t, callbacks)
	if a.gitActionPending || a.notice != a.t("同步完成") || a.state.GitError != "" {
		t.Fatal("successful sync did not complete and clear the failure")
	}
	if !a.busy || a.activeRequestAction != "save" || a.pendingRefresh {
		t.Fatal("Git response modified credential queue")
	}
	if !reflect.DeepEqual(a.detail, beforeDetail) || a.editor != beforeEditor || a.selected != beforeSelection || !reflect.DeepEqual(a.state.Entries, beforeEntries) {
		t.Fatal("Git sync replaced credentials, editor input or entry metadata")
	}
}
