package desktop

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/buildinfo"
	"github.com/knothhe/passpine/client/update"
)

func TestDevelopmentNeverStartsReleaseUpdaterEvenWithAnExplicitConfiguration(t *testing.T) {
	if buildinfo.IsRelease() {
		t.Skip("development updater boundary")
	}
	a := fixture()
	a.buildVersion = "0.1.3-dev"
	a.state.Preview = false
	a.startUpdates(update.Config{FeedURL: "https://example.invalid/must-not-be-read", PublicKey: "a supplied release key"})
	if a.updates.manager != nil || a.updates.starting || a.updates.ctx != nil || a.updates.cancel != nil || a.updates.snapshot.State != update.StateDisabled {
		t.Fatal("development build initialized a release updater")
	}
	for _, locale := range []string{"zh-CN", "en"} {
		a.state.Settings.Language = locale
		title, description := a.updateStatus()
		if title != a.t("开发版本") || description != a.t("开发版本不会检查、下载或安装正式更新。请从 GitHub Release 获取正式版本。") {
			t.Fatal("development build lacks localized updater explanation")
		}
	}
}

type fakeUpdater struct {
	snapshot   update.Snapshot
	calls      chan string
	installErr error
}

func (f *fakeUpdater) Start(context.Context)          { f.calls <- "start" }
func (f *fakeUpdater) Snapshot() update.Snapshot      { return f.snapshot }
func (f *fakeUpdater) Check(context.Context) error    { f.calls <- "check"; return nil }
func (f *fakeUpdater) Download(context.Context) error { f.calls <- "download"; return nil }
func (f *fakeUpdater) Install(context.Context) error  { f.calls <- "install"; return f.installErr }
func (f *fakeUpdater) SetAutoDownload(bool)           { f.calls <- "preference" }
func (f *fakeUpdater) SetProtectedPaths([]string)     {}

func updateFixture(state update.State) (*application, *fakeUpdater) {
	a := fixture()
	a.page, a.settingsState.Category, a.buildVersion = "settings", "updates", "0.1.3"
	f := &fakeUpdater{snapshot: update.Snapshot{State: state, CurrentVersion: "0.1.3", Version: "0.2.0", Configured: true, AutoDownload: true, UsedDelta: true, Progress: .42, Downloaded: 4200000, Total: 10000000, Notes: map[string]string{"en": "Faster vault search.", "zh-CN": "改善密码库搜索。"}}, calls: make(chan string, 8)}
	a.updates.manager, a.updates.snapshot = f, f.snapshot
	return a, f
}

func TestUpdateStatesRenderBilinguallyWithoutChangingVaultOrEditor(t *testing.T) {
	for _, state := range []update.State{update.StateIdle, update.StateChecking, update.StateLatest, update.StateAvailable, update.StateDownloading, update.StateVerifying, update.StateReady, update.StateFailed, update.StateDisabled} {
		for _, language := range []string{"zh-CN", "en"} {
			t.Run(fmt.Sprintf("%s/%s", state, language), func(t *testing.T) {
				a, _ := updateFixture(state)
				a.state.Settings.Language = language
				a.updates.snapshot.ErrorCode = update.ErrorVerification
				a.editor = editorState{open: true, contents: "user data 原始输入", originalContents: "old input"}
				before, editor := a.detail, a.editor
				view := ui.NewTester(testView(a), 860, 620)
				view.SetPreferences(ui.Preferences{ReduceMotion: true})
				view.SetDark(language == "en")
				title, _ := a.updateStatus()
				if !view.HasText(title) || !view.HasText(a.t("版本 %s", "0.1.3")) {
					t.Fatalf("state %s is missing: %s", state, title)
				}
				if state == update.StateReady && !view.HasText(a.t("请先保存当前编辑，并等待正在进行的操作完成，再重启安装。")) {
					t.Fatal("dirty editor guard is missing")
				}
				if state == update.StateDownloading && !view.HasText(a.t("已下载 %.1f / %.1f MB · %.0f%%", 4.2, 10.0, 42.0)) {
					t.Fatal("download progress is missing")
				}
				managerResizeScreenshot(t, view, "updates-"+string(state)+"-"+language)
				a.settings.Language = "en"
				if language == "en" {
					a.settings.Language = "zh-CN"
				}
				view.Frame()
				if !reflect.DeepEqual(a.detail, before) || a.editor != editor {
					t.Fatal("update rendering or locale switch changed user data")
				}
			})
		}
	}
}

func TestUpdateControlsUseIndependentLaneAndRetryCorrectOperation(t *testing.T) {
	a, f := updateFixture(update.StateLatest)
	a.busy, a.activeRequestAction = true, "fetch"
	a.service = backendFunc(func(app.Request) app.Response { t.Error("updater used credential backend"); return app.Response{} })
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("检查更新"); err != nil {
		t.Fatal(err)
	}
	if got := <-f.calls; got != "check" {
		t.Fatal(got)
	}
	if !a.busy || a.activeRequestAction != "fetch" {
		t.Fatal("manual update check changed credential lane")
	}
	a.receiveUpdate(update.Snapshot{State: update.StateReady, Version: "0.2.0", Configured: true})
	if a.updates.confirmOpen || !a.busy {
		t.Fatal("background completion interrupted operation")
	}
	a.updates.snapshot = update.Snapshot{State: update.StateFailed, Version: "0.2.0", ErrorCode: update.ErrorVerification, Configured: true}
	a.retryUpdate()
	if got := <-f.calls; got != "download" {
		t.Fatal("verification retry did not redownload")
	}
	a.updates.snapshot.ErrorCode = update.ErrorNetwork
	a.retryUpdate()
	if got := <-f.calls; got != "check" {
		t.Fatal("check failure did not retry checking")
	}
}

func TestUpdateInstallRequiresConfirmationAndPreservesPendingWork(t *testing.T) {
	a, f := updateFixture(update.StateReady)
	a.editor = editorState{open: true, contents: "unsaved", originalContents: "saved"}
	a.requestUpdateInstall()
	if a.updates.confirmOpen {
		t.Fatal("dirty editor allowed restart")
	}
	a.editor = editorState{}
	a.busy = true
	a.requestUpdateInstall()
	if a.updates.confirmOpen {
		t.Fatal("active request allowed restart")
	}
	a.busy = false
	a.installUpdate()
	select {
	case action := <-f.calls:
		t.Fatalf("install without confirmation: %s", action)
	default:
	}
	a.requestUpdateInstall()
	if !a.updates.confirmOpen {
		t.Fatal("explicit install action did not open confirmation")
	}
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("稍后"); err != nil {
		t.Fatal(err)
	}
	if a.updates.confirmOpen || a.updates.snapshot.State != update.StateReady {
		t.Fatal("Later lost pending update")
	}
	a.requestUpdateInstall()
	callbacks := make(chan func(), 2)
	a.runOnMain = func(fn func()) { callbacks <- fn }
	quit := false
	a.updates.quit = func() { quit = true }
	a.installUpdate()
	if !a.updates.preparing || !a.updates.confirmOpen || quit {
		t.Fatal("installation was not kept modal until validated")
	}
	if got := <-f.calls; got != "install" {
		t.Fatal(got)
	}
	awaitMain(t, callbacks)
	if !quit {
		t.Fatal("successful helper launch did not request restart")
	}
}

func TestInstallValidationFailureKeepsApplicationRunning(t *testing.T) {
	a, f := updateFixture(update.StateReady)
	f.installErr = errors.New("unsafe package detail")
	callbacks := make(chan func(), 2)
	a.runOnMain = func(fn func()) { callbacks <- fn }
	quit := false
	a.updates.quit = func() { quit = true }
	a.requestUpdateInstall()
	a.installUpdate()
	<-f.calls
	awaitMain(t, callbacks)
	if quit || a.closing || a.updates.preparing || a.updates.confirmOpen {
		t.Fatal("failed preflight closed app or left modal blocked")
	}
	if a.updates.snapshot.State != update.StateFailed || a.updates.snapshot.ErrorCode != update.ErrorInstall {
		t.Fatal("failed preflight is not retryable")
	}
}

func TestAutomaticDownloadPreferenceAppliesOnlyAfterSuccessfulSave(t *testing.T) {
	a, f := updateFixture(update.StateAvailable)
	a.settings, a.settingsSaved, a.settingsObserved = a.state.Settings, a.state.Settings, a.state.Settings
	a.settingsLoaded = true
	callbacks := make(chan func(), 2)
	a.runOnMain = func(fn func()) { callbacks <- fn }
	fail := true
	a.service = backendFunc(func(req app.Request) app.Response {
		if fail {
			return app.Response{Status: "error", Message: "failure"}
		}
		return app.Response{Status: "ok", Data: app.Data{Settings: *req.Settings}}
	})
	a.settings.AutoDownloadUpdates = false
	a.settingsDirty = true
	a.saveSettingsAutomatically()
	awaitMain(t, callbacks)
	select {
	case <-f.calls:
		t.Fatal("failed settings save changed updater policy")
	default:
	}
	fail = false
	a.settingsDirty = true
	a.saveSettingsAutomatically()
	awaitMain(t, callbacks)
	if got := <-f.calls; got != "preference" {
		t.Fatal(got)
	}
	a.settings.Language = "en"
	a.settingsDirty = true
	a.saveSettingsAutomatically()
	awaitMain(t, callbacks)
	select {
	case <-f.calls:
		t.Fatal("locale change restarted updater")
	default:
	}
}

func TestUpdatePreparationBlocksShortcutAndIndependentRequests(t *testing.T) {
	a, _ := updateFixture(update.StateReady)
	a.updates.preparing = true
	a.service = backendFunc(func(app.Request) app.Response {
		t.Error("request started during installation preparation")
		return app.Response{Status: "ok"}
	})
	a.openQuickSearch(quickSearchGlobal)
	if a.quick.visible {
		t.Fatal("shortcut opened quick search over restart confirmation")
	}
	a.quick.visible = true
	a.callQuickSearch(app.Request{Action: "unlock"})
	a.startGitCall(app.Request{Action: "git-sync"}, nil, true)
	if a.busy || a.gitActionPending {
		t.Fatal("independent request started during preparation")
	}
}

func TestUpdateReadyAndRestartConfirmationLayout(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		a, _ := updateFixture(update.StateReady)
		a.state.Settings.Language = language
		view := ui.NewTester(testView(a), 1120, 760)
		view.SetPreferences(ui.Preferences{ReduceMotion: true})
		view.SetDark(language == "en")
		if err := view.Click(a.t("重启并安装")); err != nil {
			t.Fatal(err)
		}
		view.Advance(time.Second)
		if !a.updates.confirmOpen || !view.HasText(a.t("重启并安装更新？")) {
			t.Fatal("restart action did not present confirmation")
		}
		for _, label := range []string{a.t("稍后"), a.t("重启并安装更新？")} {
			box, ok := view.Find(label)
			if !ok || box.X < 0 || box.Y < 0 || box.X+box.W > 1120 || box.Y+box.H > 760 {
				t.Fatalf("confirmation overflow: %s %+v", label, box)
			}
		}
		managerResizeScreenshot(t, view, "updates-confirm-"+language)
	}
}

func TestUpdateIgnoresOutOfOrderBackgroundCallbacks(t *testing.T) {
	a, _ := updateFixture(update.StateReady)
	a.updates.snapshot.Revision = 8
	a.receiveUpdate(update.Snapshot{State: update.StateDownloading, Revision: 7})
	if a.updates.snapshot.State != update.StateReady {
		t.Fatal("late progress replaced verified update")
	}
}
