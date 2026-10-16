package desktop

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/nativehost"
)

func preferencesReadFixture() (*application, chan func(), chan app.Request) {
	a := fixture()
	a.state.VaultID = "current-vault"
	callbacks := make(chan func(), 4)
	requests := make(chan app.Request, 4)
	a.runOnMain = func(f func()) { callbacks <- f }
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		data := state
		data.Activity = []app.Activity{{Action: "loaded"}}
		data.Recovery = []app.RecoveryItem{{ID: request.Kind + "-loaded"}}
		return app.Response{Status: "ok", Data: data}
	})
	return a, callbacks, requests
}

func TestPreferencesListReadWaitsForMainOperation(t *testing.T) {
	for _, page := range []string{"activity", "recovery"} {
		for _, action := range []string{"configure", "save"} {
			t.Run(page+"/"+action, func(t *testing.T) {
				a, callbacks, requests := preferencesReadFixture()
				a.callBackground(app.Request{Action: action}, nil)
				a.navigate(page, "")
				if a.preferences.pendingRead == nil {
					t.Fatal("busy navigation discarded the list read")
				}
				awaitMain(t, callbacks)
				awaitMain(t, callbacks)
				first, second := <-requests, <-requests
				if first.Action != action || second.Action != page || second.VaultID != "current-vault" {
					t.Fatalf("unexpected read sequence: %#v then %#v", first, second)
				}
				if page == "activity" && len(a.activity) != 1 || page == "recovery" && len(a.recovery) != 1 {
					t.Fatal("the queued destination did not receive its list")
				}
			})
		}
	}
}

func TestPreferencesPendingReadHasDestinationAndSessionLease(t *testing.T) {
	for _, cancel := range []string{"navigate", "epoch", "generation", "lock"} {
		t.Run(cancel, func(t *testing.T) {
			a, callbacks, requests := preferencesReadFixture()
			a.callBackground(app.Request{Action: "configure"}, nil)
			a.navigate("activity", "")
			awaitMain(t, callbacks) // Capture the queued activity callback next.
			select {
			case callback := <-callbacks:
				switch cancel {
				case "navigate":
					a.navigate("entries", "")
				case "epoch":
					a.hidePreferences()
				case "generation":
					a.generation++
				case "lock":
					a.clearSecrets()
					a.state.Locked = true
					a.generation++
				}
				callback()
			case <-time.After(3 * time.Second):
				t.Fatal("list read did not finish")
			}
			if len(a.activity) != 0 {
				t.Fatal("a late list response replaced the newer destination or session")
			}
			<-requests
			<-requests
		})
	}
}

func TestPreferencesPendingReadRejectsChangedVaultBeforeDispatch(t *testing.T) {
	a, callbacks, requests := preferencesReadFixture()
	state := a.state
	state.VaultID = "other-vault"
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: state}
	})
	a.callBackground(app.Request{Action: "configure"}, nil)
	a.navigate("activity", "")
	awaitMain(t, callbacks)
	if a.preferences.pendingRead != nil {
		t.Fatal("a list request for the previous vault remained pending")
	}
	<-requests
	select {
	case request := <-requests:
		t.Fatalf("the previous vault dispatched a list read: %#v", request)
	default:
	}
}

func TestPreferencesPendingReadUsesLatestDestination(t *testing.T) {
	a, callbacks, requests := preferencesReadFixture()
	a.callBackground(app.Request{Action: "configure"}, nil)
	a.navigate("activity", "")
	a.navigate("recovery", "")
	a.recoveryKind = 1
	a.loadRecovery()
	awaitMain(t, callbacks)
	awaitMain(t, callbacks)
	<-requests
	request := <-requests
	if request.Action != "recovery" || request.Kind != "draft" || len(a.activity) != 0 || len(a.recovery) != 1 || a.recovery[0].ID != "draft-loaded" {
		t.Fatalf("pending list did not follow the latest destination: %#v", request)
	}
}

func TestPreferencesPendingReadIsCancelledBeforeDispatch(t *testing.T) {
	a, callbacks, requests := preferencesReadFixture()
	a.callBackground(app.Request{Action: "configure"}, nil)
	a.navigate("activity", "")
	a.navigate("entries", "")
	awaitMain(t, callbacks)
	if a.preferences.pendingRead != nil {
		t.Fatal("navigation retained a pending list read")
	}
	<-requests
	select {
	case request := <-requests:
		t.Fatalf("cancelled page dispatched a read: %#v", request)
	default:
	}
}

func TestPreferencesShareManagerAndPreserveUnsavedEditor(t *testing.T) {
	a := fixture()
	a.query = "github"
	a.editEntry()
	a.editor.password = "unsaved-value"
	a.editor.compose()
	before := a.editor
	a.navigate("settings", "")
	if a.page != "settings" || a.query != "github" || a.selected != "开发/GitHub" || a.editor != before {
		t.Fatal("inline preferences changed the manager draft or selection")
	}
	view := ui.NewTester(testView(a), 860, 620)
	if view.HasText("编辑条目") || !view.HasText("通用") {
		t.Fatal("the retained editor appeared over inline preferences")
	}
	a.moveHistory(-1)
	view.Frame()
	if a.page != "entries" || a.editor != before || !view.HasText("编辑条目") {
		t.Fatal("returning from preferences lost the inline draft")
	}
}

func TestPreferencesDisplaysIndividualBrowserRegistrations(t *testing.T) {
	a := fixture()
	a.page = "settings"
	a.settingsState.Category = "browser"
	a.state.BrowserHelperAvailable = true
	a.state.BrowserRegistrations = []nativehost.BrowserRegistration{
		{ID: "chrome", Name: "Chrome", Registered: true, Status: "registered", ExtensionID: "example-chrome"},
		{ID: "firefox", Name: "Firefox", Status: "missing", ExtensionID: "example@firefox"},
		{ID: "brave", Name: "Brave", Status: "invalid", ExtensionID: "example-brave"},
	}
	view := ui.NewTester(testView(a), 860, 620)
	for _, text := range []string{"Chrome", "Firefox", "Brave", "已注册", "未注册", "需要修复", "example-chrome", "example@firefox"} {
		if !view.HasText(text) {
			t.Fatalf("registration detail missing: %s", text)
		}
	}
}

func TestPreferencesRegistersOnlySelectedBrowser(t *testing.T) {
	a := fixture()
	a.state.Preview = false
	a.state.BrowserHelperAvailable = true
	a.state.BrowserRegistrations = []nativehost.BrowserRegistration{
		{ID: "chrome", Name: "Chrome", Status: "missing"},
		{ID: "firefox", Name: "Firefox", Status: "missing"},
	}
	requests := make(chan app.Request, 1)
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: state}
	})
	view := ui.NewTester(a.browserRegistrationView, 600, 260)
	if err := view.Click("注册 Firefox"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	request := <-requests
	if request.Action != "browser-install" || request.ID != "firefox" {
		t.Fatalf("browser registration did not select the requested browser: %+v", request)
	}
}

func TestPreferencesEmptyBrowserListExplainsDetection(t *testing.T) {
	a := fixture()
	a.state.Preview = false
	view := ui.NewTester(a.browserRegistrationView, 600, 260)
	if !view.HasText("未检测到支持的浏览器") {
		t.Fatal("empty browser list does not explain installation detection")
	}
	a.state.Preview = true
	view.Frame()
	if !view.HasText("预览模式不会注册个人浏览器") {
		t.Fatal("preview browser list does not explain why detection is unavailable")
	}
}

func TestRecoveryDraftCannotReplaceActiveEditor(t *testing.T) {
	a := fixture()
	a.editEntry()
	a.editor.password = "unsaved-value"
	a.editor.compose()
	before := a.editor
	a.page = "recovery"
	a.recoveryKind = 1
	a.recovery = []app.RecoveryItem{{ID: "other-draft", Path: "other-account"}}
	a.service = backendFunc(func(app.Request) app.Response {
		t.Error("a disabled draft recovery reached the backend")
		return app.Response{}
	})
	view := ui.NewTester(testView(a), 860, 620)
	if !view.HasText("请先保存或关闭当前编辑的条目。") {
		t.Fatal("active draft restriction is not explained")
	}
	_ = view.Click("继续编辑")
	if a.editor != before {
		t.Fatal("recovery replaced the active editor")
	}
}

func TestClosingPreferencesCancelsDelayedDraftOpen(t *testing.T) {
	a := fixture()
	a.page = "recovery"
	a.recoveryKind = 1
	a.recovery = []app.RecoveryItem{{ID: "saved-draft", Path: "draft-account"}}
	callbacks := make(chan func(), 1)
	a.runOnMain = func(f func()) { callbacks <- f }
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		state.Entry, state.Contents = "draft-account", "draft-secret"
		return app.Response{Status: "ok", Data: state}
	})
	view := ui.NewTester(testView(a), 860, 620)
	if err := view.Click("继续编辑"); err != nil {
		t.Fatal(err)
	}
	a.hidePreferences()
	awaitMain(t, callbacks)
	if a.editor.open || a.state.Contents != "" {
		t.Fatal("a delayed recovery response reopened a dismissed surface")
	}
}

func TestRecoveredDraftAppearsInMainEditor(t *testing.T) {
	a := fixture()
	a.page, a.recoveryKind = "recovery", 1
	a.recovery = []app.RecoveryItem{{ID: "saved-draft", Path: "draft-account"}}
	callbacks := make(chan func(), 1)
	a.runOnMain = func(f func()) { callbacks <- f }
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		state.Entry, state.Destination, state.Contents = "draft-account", "draft-account", "draft-secret"
		return app.Response{Status: "ok", Data: state}
	})
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("继续编辑"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	view.Frame()
	if a.page != "entries" || !a.editor.open || a.editor.path != "draft-account" || a.editor.password != "draft-secret" {
		t.Fatal("the recovered draft did not navigate into the main inline editor")
	}
	if _, ok := view.Find("账号名称"); !ok {
		t.Fatal("the recovered draft's form is not visible")
	}
}

func TestVaultNameCanBeChangedFromPreferences(t *testing.T) {
	a := fixture()
	a.state.VaultID = "fixture-vault"
	a.page = "settings"
	a.settingsState.Category = "vault"
	callbacks := make(chan func(), 1)
	requests := make(chan app.Request, 1)
	a.runOnMain = func(f func()) { callbacks <- f }
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		state.VaultName = request.Name
		return app.Response{Status: "ok", Data: state}
	})
	view := ui.NewTester(testView(a), 1000, 740)
	if err := view.Click("编辑密码库名称"); err != nil {
		t.Fatal(err)
	}
	view.Key(ui.Cmd, ui.KeyA)
	view.Type("Personal vault")
	if err := view.Click("保存密码库名称"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	request := <-requests
	if request.Action != "vault-rename" || request.ID != "fixture-vault" || request.Name != "Personal vault" || a.state.VaultName != request.Name {
		t.Fatal("vault name was not saved for the current vault")
	}
}
