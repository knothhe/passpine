package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestHistoryNavigationDismissesNarrowSidebar(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 860, 620)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	a.navigate("settings", "")
	view.Frame()
	if err := view.Click(a.t("切换侧栏")); err != nil {
		t.Fatal(err)
	}
	if !a.shell.LeftExpanded() {
		t.Fatal("settings sidebar did not open")
	}
	if err := view.Click(a.t("后退")); err != nil {
		t.Fatal(err)
	}
	if a.page != "entries" || a.shell.LeftExpanded() {
		t.Fatal("history left the narrow sidebar over the entry manager")
	}
}

func TestRecoveryNavigationQueuesReadDuringCredentialRequest(t *testing.T) {
	for _, action := range []string{"configure", "fetch"} {
		t.Run(action, func(t *testing.T) {
			a := fixture()
			a.busy, a.backgroundBusy, a.activeRequestAction = true, action == "configure", action
			callbacks, requests := make(chan func(), 1), make(chan app.Request, 1)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			state := a.state
			a.service = backendFunc(func(request app.Request) app.Response {
				requests <- request
				return app.Response{Status: "ok", Data: state}
			})
			a.navigateRecovery(1)
			if a.page != "recovery" || a.recoveryKind != 1 || !a.busy || a.preferences.pendingRead == nil || a.preferences.pendingRead.kind != "draft" {
				t.Fatal("recovery navigation was dropped instead of queuing its read behind the current request")
			}
			if len(requests) != 0 {
				t.Fatal("recovery read started before the credential lane was free")
			}
			a.receiveResponse(a.generation, app.Response{Status: "ok", Data: state}, nil)
			awaitMain(t, callbacks)
			request := <-requests
			if request.Action != "recovery" || request.Kind != "draft" || a.page != "recovery" || a.busy || a.preferences.pendingRead != nil {
				t.Fatalf("queued recovery read did not finish for the requested destination: %+v", request)
			}
		})
	}
}
