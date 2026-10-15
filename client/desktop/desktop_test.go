package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine-neo/client/app"
	"github.com/knothhe/passpine-neo/client/vault"
	"github.com/oneo-me/onevra/source/theme"
)

type backendFunc func(app.Request) app.Response

func (f backendFunc) Execute(req app.Request) app.Response { return f(req) }

func awaitMain(t *testing.T, callbacks <-chan func()) {
	t.Helper()
	select {
	case callback := <-callbacks:
		callback()
	case <-time.After(3 * time.Second):
		t.Fatal("backend did not return a main-thread callback")
	}
}

func fixture() *application {
	a := newApplication(nil)
	a.state = app.Data{Configured: true, VaultName: "个人密码库", Preview: true, Settings: app.DefaultSettings(), Entries: []app.Entry{
		{Path: "开发/GitHub", Name: "GitHub", Folder: "开发", Favorite: true},
		{Path: "工作/Figma", Name: "Figma", Folder: "工作", Favorite: true},
		{Path: "生活/Apple", Name: "Apple", Folder: "生活"},
		{Path: "开发/Cloudflare", Name: "Cloudflare", Folder: "开发"},
	}}
	a.selected = "开发/GitHub"
	a.detail = app.Data{Entry: "开发/GitHub", Password: "fake-Secret-42!", Username: "pine@example.test", URL: "https://github.com", Notes: "工作与开源项目使用的账户。", Contents: "fake-Secret-42!\nusername: pine@example.test\nurl: https://github.com\nnotes: 工作与开源项目使用的账户。\n", Revision: "fixture-revision"}
	return a
}

func testView(a *application) func(*ui.Context) {
	return func(c *ui.Context) {
		c.SetTheme(theme.Onevra(c))
		ui.Row(c).Fill().Children(func() {
			ui.Column(c).Width(204).FillHeight().Background(c.Theme().Surface).Children(func() { a.sidebarView(c) })
			ui.Column(c).Grow(1).FillHeight().Children(func() { a.contentView(c) })
		})
	}
}

func TestPasswordHiddenAndSearchLocal(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 1120, 760)
	if view.HasText(a.detail.Password) {
		t.Fatal("password was visible before explicit reveal")
	}
	if err := view.Click("显示或隐藏密码"); err != nil {
		t.Fatal(err)
	}
	if !view.HasText(a.detail.Password) {
		t.Fatal("explicit reveal did not show password")
	}
	if err := view.Click("搜索条目"); err != nil {
		t.Fatal(err)
	}
	view.Type("cloud")
	if got := a.entries(); len(got) != 1 || got[0].Name != "Cloudflare" {
		t.Fatalf("local search mismatch: %#v", got)
	}
	// No backend exists in this test: search and reveal must remain local.
}

func TestEditPasswordPreservesOtherBytes(t *testing.T) {
	raw := "old-password\r\nusername: Alice\r\nusername: keep duplicate\r\nX-Unknown: retain\r\n\r\nnotes: two\r\nlines\r\n\r\n"
	e := editorState{contents: raw, originalContents: raw}
	e.readFields()
	e.password = "new-password"
	e.compose()
	want := strings.Replace(raw, "old-password", "new-password", 1)
	if e.contents != want {
		t.Fatalf("unrelated content changed:\n got %q\nwant %q", e.contents, want)
	}
}

func TestNewEntryKeepsActiveEditor(t *testing.T) {
	for _, originalPath := range []string{"", "work/existing"} {
		a := fixture()
		before := editorState{open: true, path: "work/renamed", originalPath: originalPath, contents: "unsaved secret", originalContents: "old secret", revision: "original-revision", draftID: "saved-draft"}
		a.editor = before
		a.newEntry()
		if a.editor != before {
			t.Fatal("new-entry menu action replaced the active editor")
		}
	}
}

func TestEditingEmailUsernameAlias(t *testing.T) {
	raw := "secret\r\nEmail: old@example.test\r\ncustom: keep\r\n"
	e := editorState{contents: raw, originalContents: raw}
	e.readFields()
	if e.username != "old@example.test" {
		t.Fatal("email alias was absent from username form")
	}
	e.username = "new@example.test"
	e.compose()
	if e.contents != strings.Replace(raw, "old@example.test", "new@example.test", 1) {
		t.Fatalf("email alias was not edited in place: %q", e.contents)
	}
	if doc := vault.ParseDocument("work/account", e.contents, ""); doc.Username != e.username {
		t.Fatalf("core reads username %q after form saved %q", doc.Username, e.username)
	}
	if extra := extraFields(e.contents); len(extra) != 1 || extra[0][0] != "custom" {
		t.Fatalf("recognized username alias duplicated in custom fields: %#v", extra)
	}
}

func TestClearSecretsAndSafeURL(t *testing.T) {
	a := fixture()
	a.editor = editorState{open: true, contents: "private"}
	a.passphrase = "private"
	a.generator.password = "private"
	a.clearSecrets()
	if a.detail.Contents != "" || a.editor.contents != "" || a.passphrase != "" || a.generator.password != "" {
		t.Fatal("secret presentation state survived clear")
	}
	for _, u := range []string{"file:///etc/passwd", "javascript:alert(1)", "https://user:password@example.com"} {
		if safeWebsite(u) {
			t.Fatalf("unsafe website allowed: %s", u)
		}
	}
	if !safeWebsite("https://example.com/account") {
		t.Fatal("https link rejected")
	}
}

func TestLockedErrorDiscardsVisibleSecrets(t *testing.T) {
	a := fixture()
	a.editor = editorState{open: true, contents: "unsaved-secret"}
	a.passphrase = "passphrase"
	a.receiveResponse(a.generation, app.Response{Status: "error", Code: "locked", Data: app.Data{Configured: true, Locked: true}}, nil)
	if a.editor.open || a.editor.contents != "" || a.detail.Password != "" || a.passphrase != "" {
		t.Fatal("locked error retained editor or decrypted contents")
	}
}

func TestOldResponseCannotReviveSecretsOrFinishNewRequest(t *testing.T) {
	a := fixture()
	a.clearSecrets()
	a.generation = 2
	a.busy = true
	a.receiveResponse(1, app.Response{Status: "ok", Data: app.Data{Configured: true, Contents: "stale-secret", Password: "stale-secret"}}, func(app.Data) { t.Fatal("stale callback ran") })
	if !a.busy || a.detail.Password != "" || a.state.Password != "" {
		t.Fatal("stale response modified new request state")
	}
}

func TestExternalRefreshPreservesDraftAndRemovesStaleDetail(t *testing.T) {
	a := fixture()
	a.editor = editorState{open: true, path: "工作/新名称", originalPath: "开发/GitHub", contents: "unsaved draft", revision: "original-revision"}
	a.selectedIndex = 1
	a.queueMetadataRefresh(true)
	if !a.pendingRefresh || a.refreshReady() {
		t.Fatal("external refresh was not deferred while editing")
	}
	if a.detail.Password != "" || a.selectedIndex != -1 {
		t.Fatal("external change left stale detail selectable")
	}
	if a.editor.contents != "unsaved draft" || a.editor.revision != "original-revision" {
		t.Fatal("external change overwrote the user's draft")
	}
	// An already-running fetch must not put its old secret back while the
	// queued refresh is waiting for this editor to finish.
	a.receiveResponse(a.generation, app.Response{Status: "ok", Data: a.state}, func(app.Data) { a.detail = app.Data{Password: "stale"} })
	if a.detail.Password != "" {
		t.Fatal("old request revived detail during pending refresh")
	}
	a.editor.open = false
	if !a.refreshReady() {
		t.Fatal("queued refresh was not ready after the editor closed")
	}
}

func TestFocusRefreshCoalescesWithoutClearingUnchangedDetail(t *testing.T) {
	a := fixture()
	a.busy = true
	a.queueMetadataRefresh(false)
	a.queueMetadataRefresh(false)
	if !a.pendingRefresh || a.pendingChanged || a.refreshReady() {
		t.Fatal("focus refresh did not wait for the current request")
	}
	if a.detail.Password == "" {
		t.Fatal("focus alone removed the current detail before checking its version")
	}
	a.busy = false
	if !a.refreshReady() {
		t.Fatal("coalesced focus refresh was not ready when idle")
	}
}

func TestBackgroundRefreshDoesNotExtendIdleSession(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprintf("external_change_%t", changed), func(t *testing.T) {
			a := fixture()
			idleSince := time.Now().Add(-30 * time.Minute)
			a.lastInput, a.lastTouch = idleSince, idleSince.Add(time.Second)
			callbacks := make(chan func(), 1)
			requests := make(chan app.Request, 1)
			state := a.state
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.service = backendFunc(func(req app.Request) app.Response {
				requests <- req
				return app.Response{Status: "ok", Data: state}
			})
			a.queueMetadataRefresh(changed)
			awaitMain(t, callbacks)
			if req := <-requests; req.Action != "configure" {
				t.Fatalf("unexpected refresh request: %#v", req)
			}
			if !a.lastInput.Equal(idleSince) || a.lastInput.After(a.lastTouch) {
				t.Fatal("metadata refresh renewed idle time or scheduled a core activity touch")
			}
		})
	}
}

func TestUserCallRecordsActivityBeforeDeferredRefresh(t *testing.T) {
	a := fixture()
	a.lastInput = time.Now().Add(-30 * time.Minute)
	a.lastTouch = a.lastInput.Add(time.Second)
	callbacks := make(chan func(), 2)
	requests := make(chan app.Request, 2)
	state := a.state
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(req app.Request) app.Response {
		requests <- req
		return app.Response{Status: "ok", Data: state}
	})
	a.call(app.Request{Action: "fetch", Entry: "开发/GitHub"}, nil)
	userInput := a.lastInput
	if !userInput.After(a.lastTouch) {
		t.Fatal("user request did not renew activity")
	}
	a.queueMetadataRefresh(true)
	awaitMain(t, callbacks) // Fetch completion dispatches the pending refresh.
	awaitMain(t, callbacks)
	if first, second := <-requests, <-requests; first.Action != "fetch" || second.Action != "configure" {
		t.Fatalf("unexpected request sequence: %q, %q", first.Action, second.Action)
	}
	if !a.lastInput.Equal(userInput) {
		t.Fatal("deferred metadata refresh replaced the actual user activity time")
	}
}

func TestRecoveryTabCannotChangeDuringLoad(t *testing.T) {
	a := fixture()
	a.page = "recovery"
	callbacks := make(chan func(), 1)
	state := a.state
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(req app.Request) app.Response {
		data := state
		data.Recovery = []app.RecoveryItem{{ID: req.Kind + "-id", Path: req.Kind + "-entry"}}
		return app.Response{Status: "ok", Data: data}
	})
	a.loadRecovery()
	view := ui.NewTester(testView(a), 1120, 760)
	// The first request has completed in the worker but is still waiting to
	// be delivered on the main thread, just like a slow vault read.
	_ = view.Click("加密草稿")
	if a.recoveryKind != 0 {
		t.Fatal("recovery category changed while its previous request was pending")
	}
	awaitMain(t, callbacks)
	if len(a.recovery) != 1 || a.recovery[0].ID != "trash-id" {
		t.Fatalf("unexpected trash response: %#v", a.recovery)
	}
	view.Frame()
	if err := view.Click("加密草稿"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	if a.recoveryKind != 1 || len(a.recovery) != 1 || a.recovery[0].ID != "draft-id" {
		t.Fatalf("selected tab and its response disagree: kind=%d items=%#v", a.recoveryKind, a.recovery)
	}
}

func TestActivitySurvivesMetadataRefresh(t *testing.T) {
	a := fixture()
	callbacks := make(chan func(), 1)
	state := a.state
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(req app.Request) app.Response {
		data := state
		if req.Action == "activity" {
			data.Activity = []app.Activity{{Time: "2026-10-15 12:00", Action: "test-activity-record", Source: "desktop", Outcome: "ok"}}
		}
		return app.Response{Status: "ok", Data: data}
	})
	a.navigate("activity", "")
	awaitMain(t, callbacks)
	view := ui.NewTester(testView(a), 1120, 760)
	if !view.HasText("test-activity-record") {
		t.Fatal("activity request did not populate the page")
	}
	a.queueMetadataRefresh(false)
	awaitMain(t, callbacks)
	view.Frame()
	if !view.HasText("test-activity-record") {
		t.Fatal("focus metadata refresh erased the operation history")
	}
}

func TestScreensRender(t *testing.T) {
	for _, screen := range []string{"entries", "generator", "settings", "recovery", "setup", "locked", "editor"} {
		t.Run(screen, func(t *testing.T) {
			a := fixture()
			a.page = screen
			switch screen {
			case "setup":
				a.state.Configured = false
			case "locked":
				a.state.Locked = true
			case "editor":
				a.page = "entries"
				a.editEntry()
			}
			v := ui.NewTester(testView(a), 1120, 760)
			v.Frame()
			if v.Image().Bounds().Empty() {
				t.Fatal("empty rendered frame")
			}
			if dir := os.Getenv("PASSPINE_SNAPSHOT_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				file, err := os.Create(filepath.Join(dir, screen+".png"))
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				if err = png.Encode(file, v.Image()); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSearch10000(b *testing.B) {
	a := newApplication(nil)
	for i := 0; i < 10000; i++ {
		a.state.Entries = append(a.state.Entries, app.Entry{Path: fmt.Sprintf("work/account-%05d", i), Name: fmt.Sprintf("account-%05d", i), Folder: "work"})
	}
	a.entries()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			a.query = "account-000"
		} else {
			a.query = "account-099"
		}
		a.entries()
	}
}

func BenchmarkIdleList10000(b *testing.B) {
	a := newApplication(nil)
	for i := 0; i < 10000; i++ {
		a.state.Entries = append(a.state.Entries, app.Entry{Path: fmt.Sprintf("work/account-%05d", i)})
	}
	a.entries()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		a.entries()
	}
}
