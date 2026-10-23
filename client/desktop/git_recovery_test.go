package desktop

import (
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func readyGitRecoveryState() app.Data {
	return app.Data{Configured: true, VaultID: "active-vault", GitAvailable: true, GitInitialized: true, GitRemote: "origin", GitStatusAt: "2026-10-23T12:00:00Z"}
}

func TestGitRecoveryUsesReportedPrerequisitesAndSpecificRecovery(t *testing.T) {
	for _, test := range []struct {
		name           string
		change         func(*app.Data)
		setup, pending bool
		title, action  string
		progress       bool
	}{
		{name: "healthy"},
		{name: "unconfigured", change: func(s *app.Data) { s.Configured = false }},
		{name: "setup", setup: true},
		{name: "unknown", change: func(s *app.Data) { *s = app.Data{Configured: true} }},
		{name: "unknown error has no guessed retry", change: func(s *app.Data) { *s = app.Data{Configured: true, GitError: "permission denied"} }, title: "远端同步未完成"},
		{name: "conflict", change: func(s *app.Data) { s.GitConflict, s.GitError = true, "merge conflict" }, title: "存在同步冲突"},
		{name: "missing git with error", change: func(s *app.Data) { s.GitAvailable, s.GitError = false, "executable unavailable" }, title: "Git 不可用"},
		{name: "uninitialized", change: func(s *app.Data) { s.GitInitialized = false }, title: "未初始化 Git", action: "git-init"},
		{name: "no remote with error", change: func(s *app.Data) { s.GitRemote, s.GitError = "", "no remote" }, title: "未配置远端"},
		{name: "unknown empty remote with error", change: func(s *app.Data) { s.GitRemote, s.GitStatusAt, s.GitError = "", "", "unknown failure" }, title: "远端同步未完成"},
		{name: "retry", change: func(s *app.Data) { s.GitError = "authentication denied" }, title: "远端同步未完成", action: "git-sync"},
		{name: "syncing", change: func(s *app.Data) { s.GitSyncing, s.GitError = true, "stale failure" }, title: "正在同步", progress: true},
		{name: "mutation pending", pending: true, title: "正在处理 Git 操作…", progress: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := readyGitRecoveryState()
			if test.change != nil {
				test.change(&state)
			}
			before := state
			got := presentGitRecovery(state, test.setup, test.pending)
			if got.Title != test.title || got.Action != test.action || got.Progress != test.progress {
				t.Fatalf("recovery mismatch: %#v", got)
			}
			if !reflect.DeepEqual(state, before) {
				t.Fatal("presentation changed reported state")
			}
			if got.Progress && got.Error != "" {
				t.Fatal("old failure remained during a real operation")
			}
		})
	}
}

func TestGitRecoveryRetryUsesIndependentGitLaneAndPreservesEditor(t *testing.T) {
	a := fixture()
	git := readyGitRecoveryState()
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
	view := ui.NewTester(a.gitRecoveryView, 900, 180)
	if err := view.Click("重试同步"); err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-requests:
		if request.Action != "git-sync" || request.VaultID != git.VaultID {
			t.Fatal("wrong recovery request", request)
		}
	case <-time.After(time.Second):
		t.Fatal("retry did not start")
	}
	if !a.gitActionPending || !a.busy || a.activeRequestAction != "save" {
		t.Fatal("retry changed the credential queue")
	}
	if !view.HasText("正在处理 Git 操作…") {
		t.Fatal("pending Git action is not visible")
	}
	a.runGitRecoveryAction("git-sync")
	close(release)
	awaitMain(t, callbacks)
	select {
	case extra := <-requests:
		t.Fatal("duplicate retry started", extra)
	default:
	}
	if a.gitActionPending || a.notice != a.t("同步完成") || presentGitRecovery(a.state, false, false).Title != "" {
		t.Fatal("successful retry did not complete and remove recovery state")
	}
	if !a.busy || a.activeRequestAction != "save" || a.pendingRefresh {
		t.Fatal("Git response modified credential queue")
	}
	if !reflect.DeepEqual(a.detail, beforeDetail) || a.editor != beforeEditor || a.selected != beforeSelection || !reflect.DeepEqual(a.state.Entries, beforeEntries) {
		t.Fatal("Git retry replaced credentials, editor input or entry metadata")
	}
}

func TestGitRecoverySettingsPreservesInlineDraft(t *testing.T) {
	a := fixture()
	a.state.GitConflict = true
	a.editEntry()
	a.editor.password = "unsaved-local-password"
	a.editor.compose()
	before := a.editor
	view := ui.NewTester(a.gitRecoveryView, 900, 180)
	if err := view.Click("查看同步设置"); err != nil {
		t.Fatal(err)
	}
	if a.page != "settings" || a.settingsState.Category != "vault" || a.editor != before || a.discardOpen {
		t.Fatal("recovery settings lost the retained editor")
	}
}

func TestGitRecoveryInitializationAndStaleActions(t *testing.T) {
	a := fixture()
	state := readyGitRecoveryState()
	state.GitInitialized = false
	a.applyGitState(state)
	a.state.VaultID = state.VaultID
	callbacks, requests := make(chan func(), 1), make(chan app.Request, 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		state.GitInitialized = true
		state.GitRemote = ""
		return app.Response{Status: "ok", Data: state}
	})
	view := ui.NewTester(a.gitRecoveryView, 900, 180)
	if err := view.Click("初始化 Git"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	if request := <-requests; request.Action != "git-init" {
		t.Fatal("initialization did not use the existing Git action", request)
	}
	if a.notice != a.t("Git 已初始化") {
		t.Fatal("initialization completion was not announced")
	}
	for _, change := range []func(){
		func() { a.state.GitRemote, a.state.GitError = "", "no remote" },
		func() { a.state.GitRemote, a.state.GitConflict = "origin", true },
		func() { a.state.GitConflict, a.state.GitSyncing = false, true },
	} {
		change()
		a.runGitRecoveryAction("git-sync")
		select {
		case request := <-requests:
			t.Fatal("stale retry bypassed prerequisites", request)
		default:
		}
	}
}
