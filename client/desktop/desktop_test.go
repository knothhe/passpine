package desktop

import (
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/buildinfo"
	sidebar "github.com/knothhe/passpine/client/internal/clientshell"
	"github.com/knothhe/passpine/client/nativehost"
	"github.com/knothhe/passpine/client/vault"
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
		{Path: "开发/GitHub", Name: "GitHub", Folder: "开发"},
		{Path: "工作/Figma", Name: "Figma", Folder: "工作"},
		{Path: "生活/Apple", Name: "Apple", Folder: "生活"},
		{Path: "开发/Cloudflare", Name: "Cloudflare", Folder: "开发"},
	}}
	a.state.Settings.Language = "zh-CN"
	a.selected = "开发/GitHub"
	a.detail = app.Data{Entry: "开发/GitHub", Password: "fake-Secret-42!", Username: "pine@example.test", URL: "https://github.com", Notes: "工作与开源项目使用的账户。", Contents: "fake-Secret-42!\nusername: pine@example.test\nurl: https://github.com\nnotes: 工作与开源项目使用的账户。\n", Revision: "fixture-revision"}
	return a
}

func testView(a *application) func(*ui.Context) {
	shell, view := sidebar.Layout(a.mainShellOptions())
	a.shell = shell
	return view
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
	screens := []string{"entries", "settings", "settings-system", "settings-about", "recovery", "setup", "locked", "editor", "editor-custom", "generator"}
	directory := os.Getenv("PASSPINE_SNAPSHOT_DIR")
	if directory != "" {
		screens = append(screens, "settings-security", "settings-vault", "settings-browser", "settings-config-sync", "settings-cli", "settings-updates", "activity", "recovery-items", "drafts", "setup-import", "setup-import-files", "setup-import-files-en", "setup-import-files-busy", "setup-import-files-error", "setup-git", "locked-device", "editor-raw", "empty", "git-locked", "git-syncing", "confirm-delete", "confirm-discard", "confirm-purge", "backup")
	}
	type viewport struct {
		name          string
		width, height int
		dark          bool
	}
	sizes := []viewport{{"", 1120, 760, false}}
	if directory != "" {
		sizes = append(sizes, viewport{"-narrow", 860, 620, false}, viewport{"-dark", 1120, 760, true})
	}
	for _, screen := range screens {
		for _, size := range sizes {
			t.Run(screen+size.name, func(t *testing.T) {
				a := fixture()
				a.page = screen
				switch screen {
				case "settings-system", "settings-security", "settings-vault", "settings-browser", "settings-config-sync", "settings-cli", "settings-updates", "settings-about":
					a.page = "settings"
					a.settingsState.Category = strings.TrimPrefix(screen, "settings-")
					a.state.DeviceUnlockAvailable, a.state.DeviceUnlockEnrolled = true, true
					if screen == "settings-about" {
						a.buildVersion = "0.1.0"
						a.aboutExpanded = map[string]bool{"crypto": true}
					}
					if screen == "settings-browser" {
						a.state.BrowserHelperAvailable = true
						a.state.BrowserRegistrations = []nativehost.BrowserRegistration{
							{ID: "chrome", Name: "Chrome", Registered: true, Status: "registered", ExtensionID: "example-chromium-extension"},
							{ID: "firefox", Name: "Firefox", Status: "missing", ExtensionID: "example@passpine.test"},
							{ID: "brave", Name: "Brave", Status: "invalid", ExtensionID: "example-chromium-extension"},
						}
					}
				case "setup", "setup-import", "setup-import-files", "setup-import-files-en", "setup-import-files-busy", "setup-import-files-error", "setup-git":
					a.state.Configured = false
					if strings.HasPrefix(screen, "setup-import") {
						a.setupMode = 1
						a.setupDirectory = "/Users/example/.password-store"
					}
					if strings.HasPrefix(screen, "setup-import-files") {
						a.setupKeySource = 1
						a.setupPrivateKey = "/Users/example/Keys/private.asc"
					}
					if screen == "setup-import-files-en" {
						a.state.Settings.Language = "en"
					}
					if screen == "setup-import-files-busy" {
						a.busy, a.activeRequestAction = true, "open"
					}
					if screen == "setup-import-files-error" {
						a.errorText = "私钥口令错误或私钥不可用"
					}
					if screen == "setup-git" {
						a.setupMode = 2
					}
				case "locked", "locked-device":
					a.page = "entries"
					a.state.Locked = true
					if screen == "locked-device" {
						a.state.DeviceUnlockAvailable, a.state.DeviceUnlockEnrolled = true, true
					}
				case "confirm-delete", "confirm-discard", "confirm-purge", "backup":
					a.page = "entries"
					a.deleteOpen = screen == "confirm-delete"
					a.discardOpen = screen == "confirm-discard"
					a.purgeOpen = screen == "confirm-purge"
					a.backupOpen = screen == "backup"
					a.backupPath = "/tmp/example-recovery.asc"
				case "editor", "editor-raw", "editor-custom", "generator":
					a.page = "entries"
					a.editEntry()
					a.editor.showRaw = screen == "editor-raw"
					if screen == "editor-custom" {
						a.editor.fields.add("account-id", "pine-account-42", -1)
						a.editor.fields.add("api-token", "fake-token-value", -1)
						a.editor.compose()
					}
					if screen == "generator" {
						a.generator.open = true
						a.generator.target = a.editor.ensureIdentity()
						a.generator.password = "fake-Generated-Secret-42!"
						a.generator.generated = a.generator.options()
					}
				case "empty":
					a.page = "entries"
					a.state.Entries = nil
					a.detail = app.Data{}
				case "recovery-items", "drafts":
					a.page = "recovery"
					if screen == "drafts" {
						a.recoveryKind = 1
					}
					a.recovery = []app.RecoveryItem{{ID: "fixture", Path: "工作/演示条目", CreatedAt: "2026-10-15 16:00"}}
				case "activity":
					a.activity = []app.Activity{{Time: "2026-10-15T08:00:00Z", Action: "copy", Source: "browser", Outcome: "ok"}}
				case "git-locked", "git-syncing":
					a.page = "entries"
					a.state.GitAvailable, a.state.GitInitialized = true, true
					a.state.GitBranch, a.state.GitSummary = "main", "已同步"
					a.state.Locked = screen == "git-locked"
					a.state.GitSyncing = screen == "git-syncing"
				}
				if a.backupOpen || a.purgeOpen {
					a.page = "settings"
				}
				v := ui.NewTester(testView(a), size.width, size.height)
				v.SetDark(size.dark)
				if screen == "settings-about" {
					v.Advance(time.Second)
				}
				if directory != "" {
					// Capture the settled palette, rather than the navigation fade's
					// translucent first frame over the previous window surface.
					v.Advance(time.Second)
				}
				v.Frame()
				if v.Image().Bounds().Empty() {
					t.Fatal("empty rendered frame")
				}
				if directory != "" {
					if err := os.MkdirAll(directory, 0755); err != nil {
						t.Fatal(err)
					}
					file, err := os.Create(filepath.Join(directory, screen+size.name+".png"))
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
}

func TestDefaultVaultDiscovery(t *testing.T) {
	for _, scenario := range []string{"existing", "missing", "empty", "file", "symlink", "configured", "preview", "manual"} {
		t.Run(scenario, func(t *testing.T) {
			home := t.TempDir()
			directory := filepath.Join(home, ".password-store")
			if scenario == "file" {
				if err := os.WriteFile(directory, []byte("not a directory"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if scenario != "missing" {
				store := directory
				if scenario == "symlink" {
					store = filepath.Join(home, "real-store")
				}
				if err := os.Mkdir(store, 0700); err != nil {
					t.Fatal(err)
				}
				if scenario != "empty" {
					if err := os.WriteFile(filepath.Join(store, ".gpg-id"), []byte("fixture-recipient\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "symlink" {
					var err error
					if runtime.GOOS == "windows" {
						// Directory junctions exercise the same reparse-point boundary
						// without requiring Developer Mode or elevated tests.
						err = exec.Command("cmd.exe", "/c", "mklink", "/J", directory, store).Run()
					} else {
						err = os.Symlink(store, directory)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			a := newApplication(nil)
			a.state.Settings.Language = "zh-CN"
			a.state.Configured = scenario == "configured"
			a.state.Preview = scenario == "preview"
			if scenario == "manual" {
				a.setupDirectory = filepath.Join(home, "chosen-store")
			}
			before := a.setupDirectory
			a.useDefaultVault(home)
			if scenario == "existing" && buildinfo.IsRelease() {
				if a.setupMode != 1 || a.setupDirectory != directory {
					t.Fatalf("default store was not selected for import: mode=%d directory=%q", a.setupMode, a.setupDirectory)
				}
				view := ui.NewTester(testView(a), 1120, 760)
				if !view.HasText("已找到默认密码库 ~/.password-store。选择密钥来源后即可导入。") {
					t.Fatal("import screen did not explain the detected store")
				}
				files, err := os.ReadDir(directory)
				if err != nil || len(files) != 1 || files[0].Name() != ".gpg-id" {
					t.Fatal("discovery modified the existing store", err)
				}
			} else if a.setupMode != 0 || a.setupDirectory != before || a.defaultVaultDirectory != "" {
				t.Fatal("discovery changed an unrelated setup")
			}
		})
	}
}

func TestDefaultVaultIsOnlyPrefilledForImport(t *testing.T) {
	a := newApplication(nil)
	a.state.Settings.Language = "zh-CN"
	a.defaultVaultDirectory = filepath.Join(t.TempDir(), ".password-store")
	a.setupDirectory = a.defaultVaultDirectory
	a.setupMode = 1
	view := ui.NewTester(testView(a), 1120, 760)
	for _, tab := range []string{"创建密码库", "从 Git 下载"} {
		if err := view.Click(tab); err != nil {
			t.Fatal(err)
		}
		if a.setupDirectory != "" {
			t.Fatal("existing default store was reused as a new store destination")
		}
		if err := view.Click("导入密码库"); err != nil {
			t.Fatal(err)
		}
		if a.setupDirectory != a.defaultVaultDirectory {
			t.Fatal("returning to import did not restore the default store")
		}
	}
	a.setupDirectory = filepath.Join(t.TempDir(), "chosen-store")
	chosen := a.setupDirectory
	if err := view.Click("创建密码库"); err != nil {
		t.Fatal(err)
	}
	if a.setupDirectory != chosen {
		t.Fatal("tab change replaced a manually selected directory")
	}
}

func TestOpenVaultUsesSystemGPGWithoutKeyFile(t *testing.T) {
	a := newApplication(nil)
	a.state.Settings.Language = "zh-CN"
	a.setupMode = 1
	a.setupDirectory = filepath.Join(t.TempDir(), ".password-store")
	a.passphrase = "fixture-system-gpg-passphrase"
	callbacks := make(chan func(), 1)
	requests := make(chan app.Request, 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(req app.Request) app.Response {
		requests <- req
		return app.Response{Status: "error", Message: "fixture response"}
	})
	view := ui.NewTester(testView(a), 1120, 760)
	if view.HasText("OpenPGP 私钥文件") || view.HasText("选择私钥文件") {
		t.Fatal("opening a system GPG store still asks for a recovery key file")
	}
	if err := view.Click("打开并解锁"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	req := <-requests
	if req.Action != "open" || req.Mode != "system-gpg" || req.KeyFile != "" || req.Directory != a.setupDirectory || req.Passphrase != a.passphrase {
		t.Fatalf("unexpected system GPG open request: action=%s mode=%s", req.Action, req.Mode)
	}
}

func BenchmarkSearch10000(b *testing.B) {
	a := newApplication(nil)
	a.state.Settings.Language = "zh-CN"
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
	a.state.Settings.Language = "zh-CN"
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
