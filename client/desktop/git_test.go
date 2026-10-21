package desktop

import (
	"testing"
	"time"

	"github.com/knothhe/passpine/client/app"
)

func TestGitStatusRefreshPreservesCredentialRequestAndEditorState(t *testing.T) {
	a := fixture()
	a.state.VaultID = "active-vault"
	a.editor = editorState{open: true, path: "draft", contents: "unsaved draft", originalContents: "original"}
	beforeDetail, beforeSelection, beforeEntries := a.detail, a.selected, a.state.Entries
	callbacks := make(chan func(), 1)
	requests := make(chan app.Request, 1)
	release := make(chan struct{})
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		<-release
		return app.Response{Status: "ok", Data: app.Data{VaultID: "active-vault", GitSyncing: true, GitChanges: 3, Entries: []app.Entry{{Path: "unrelated"}}, Password: "must not replace visible state"}}
	})
	a.refreshGitState()
	if a.busy || a.pendingRefresh || a.activeRequestAction != "" || a.gitActionPending {
		t.Fatal("Git status occupied the credential queue or scheduled configure")
	}
	select {
	case request := <-requests:
		if request.Action != "git-status" || request.VaultID != a.state.VaultID {
			t.Fatal("Git notification did not use the status-only request", request)
		}
	case <-time.After(time.Second):
		t.Fatal("Git status request did not start")
	}
	// Status refreshes also work while a foreground request already owns busy.
	a.busy, a.activeRequestAction = true, "save"
	close(release)
	awaitMain(t, callbacks)
	if !a.busy || a.activeRequestAction != "save" || a.pendingRefresh {
		t.Fatal("Git status modified the foreground credential request")
	}
	if !a.state.GitSyncing || a.state.GitChanges != 3 {
		t.Fatal("Git status was not applied")
	}
	if a.detail.Contents != beforeDetail.Contents || a.detail.Password != beforeDetail.Password || a.selected != beforeSelection || len(a.state.Entries) != len(beforeEntries) {
		t.Fatal("Git status replaced entry presentation or secrets")
	}
	if !a.editor.open || a.editor.contents != "unsaved draft" || a.editor.originalContents != "original" {
		t.Fatal("Git status discarded the editor input")
	}
}
