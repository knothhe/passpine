package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestLockedNewEntryResumesInSelectedFolderAfterUnlock(t *testing.T) {
	a := fixture()
	a.state.VaultID, a.state.Locked = "test-vault", true
	a.filter = "folder:工作"
	callbacks, requests := make(chan func(), 1), make(chan app.Request, 1)
	state := a.state
	state.Locked = false
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: state}
	})
	a.newEntry()
	view := ui.NewTester(testView(a), 1120, 760)
	if a.editor.open || a.pendingUnlock.action != "new-entry" || !view.HasText("解锁密码库") || view.HasText("请先解锁，然后新建条目。") {
		t.Fatal("locked creation did not enter the unlock flow")
	}
	view.Type("synthetic-passphrase")
	view.Key(0, ui.KeyEnter)
	awaitMain(t, callbacks)
	request := <-requests
	if request.Action != "unlock" || request.Passphrase != "synthetic-passphrase" || !a.editor.open || a.editor.path != "工作/" || a.passphrase != "" {
		t.Fatalf("creation did not resume after authentication: %#v editor=%#v", request, a.editor)
	}
}

func TestLockedNewEntryStartsAvailableDeviceAuthentication(t *testing.T) {
	a := fixture()
	a.state.VaultID, a.state.Locked = "test-vault", true
	a.state.Settings.DeviceUnlock = true
	a.state.DeviceUnlockAvailable, a.state.DeviceUnlockEnrolled = true, true
	callbacks, requests := make(chan func(), 1), make(chan app.Request, 1)
	state := a.state
	state.Locked = false
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: state}
	})
	a.newEntry()
	awaitMain(t, callbacks)
	if request := <-requests; request.Action != "unlock" || request.Mode != "device" || !a.editor.open {
		t.Fatalf("creation did not authenticate and resume automatically: %#v", request)
	}
}

func TestPendingCreationCannotCrossNavigationOrVaultBoundary(t *testing.T) {
	for _, boundary := range []string{"navigate", "setup", "lock", "vault", "generation"} {
		t.Run(boundary, func(t *testing.T) {
			a := fixture()
			a.state.VaultID, a.state.Locked = "test-vault", true
			a.newEntry()
			switch boundary {
			case "navigate":
				a.navigate("entries", "all")
			case "setup":
				a.openSetup()
			case "lock":
				a.clearSecrets()
			case "vault":
				a.state.VaultID = "other-vault"
			case "generation":
				a.generation++
			}
			a.state.Locked = false
			if a.resumeUnlockAction() || a.editor.open {
				t.Fatal("stale unlock resumed creation in a superseded destination")
			}
		})
	}
}

func TestNewEntryDuringMetadataRefreshIsNotDropped(t *testing.T) {
	a := fixture()
	callbacks := make(chan func(), 1)
	state := a.state
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(app.Request) app.Response { return app.Response{Status: "ok", Data: state} })
	a.queueMetadataRefresh(false)
	a.newEntry()
	awaitMain(t, callbacks)
	if !a.editor.open || a.pendingNewEntry.action != "" {
		t.Fatal("background metadata refresh swallowed the creation action")
	}
}

func TestQueuedCreationEntersUnlockWhenMetadataDiscoversExpiredSession(t *testing.T) {
	a := fixture()
	a.state.VaultID = "test-vault"
	callbacks := make(chan func(), 2)
	state := a.state
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		data := state
		data.Locked = request.Action == "configure"
		return app.Response{Status: "ok", Data: data}
	})
	a.queueMetadataRefresh(false)
	a.newEntry()
	awaitMain(t, callbacks)
	if !a.state.Locked || a.editor.open || a.pendingUnlock.action != "new-entry" {
		t.Fatal("the newly discovered lock swallowed the user's creation click")
	}
	a.passphrase = "synthetic-passphrase"
	a.unlock("")
	awaitMain(t, callbacks)
	if a.state.Locked || !a.editor.open {
		t.Fatal("the pending creation did not resume after the expired session was unlocked")
	}
}
