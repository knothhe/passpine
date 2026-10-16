package desktop

import (
	"testing"
	"time"

	"github.com/knothhe/passpine/client/app"
)

type sessionBackend struct {
	locked   bool
	requests []app.Request
	response app.Data
	started  chan<- app.Request
	complete <-chan struct{}
}

func (s *sessionBackend) IsLocked() bool { return s.locked }
func (s *sessionBackend) Execute(r app.Request) app.Response {
	s.requests = append(s.requests, r)
	if s.started != nil {
		s.started <- r
	}
	if s.complete != nil {
		<-s.complete
	}
	return app.Response{Status: "ok", Data: s.response}
}

func TestVaultTransitionsPreservePollingAndHonorLockNotifications(t *testing.T) {
	for _, action := range []string{"create", "open", "switch"} {
		for _, event := range []string{"poll", "notification"} {
			t.Run(action+"/"+event, func(t *testing.T) {
				a := fixture()
				a.state.VaultID = "old-vault"
				a.openSetup()
				a.setupDirectory = t.TempDir()
				a.passphrase = "test-only setup passphrase"
				a.confirmPassphrase = a.passphrase
				data := a.state
				data.VaultID, data.Locked = "new-vault", action == "switch"
				started, complete := make(chan app.Request, 1), make(chan struct{})
				t.Cleanup(func() {
					select {
					case <-complete:
					default:
						close(complete)
					}
				})
				s := &sessionBackend{locked: true, response: data, started: started, complete: complete}
				a.service = s
				callbacks := make(chan func(), 1)
				a.runOnMain = func(callback func()) { callbacks <- callback }
				switch action {
				case "create":
					a.setupMode = 0
					a.submitSetup()
				case "open":
					a.setupMode = 1
					a.submitSetup()
				case "switch":
					a.switchVault("new-vault")
				}
				select {
				case request := <-started:
					if request.Action != action {
						t.Fatalf("requested %s, want %s", request.Action, action)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("vault transition did not start")
				}
				// The replacement vault is locked while setup is still in progress.
				generation := a.generation
				if event == "poll" {
					for range 3 {
						a.refreshSessionLock()
					}
					s.locked = data.Locked
				} else {
					a.handleSessionLock()
				}
				close(complete)
				awaitMain(t, callbacks)
				if event == "poll" {
					if a.generation != generation || a.setup || a.state.VaultID != data.VaultID || a.state.Locked != data.Locked || a.busy {
						t.Fatal("session polling discarded the completed vault transition")
					}
				} else if a.generation == generation || !a.state.Locked || a.state.VaultID != "old-vault" || a.busy || a.passphrase != "" || a.confirmPassphrase != "" {
					t.Fatal("lock notification did not invalidate the pending vault transition")
				}
			})
		}
	}
}

func TestLockNotificationDiscardsPendingUnlockResponse(t *testing.T) {
	for _, surface := range []string{"manager", "quick search"} {
		t.Run(surface, func(t *testing.T) {
			a, _ := quickSearchFixture()
			a.state.Locked = true
			a.detail = app.Data{}
			a.passphrase, a.quick.passphrase = "private", "private"
			data := a.state
			data.Locked = false
			s := &sessionBackend{response: data}
			a.service = s
			callbacks := make(chan func(), 1)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			if surface == "manager" {
				a.unlock("")
			} else {
				a.unlockQuickSearch("")
			}
			var callback func()
			select {
			case callback = <-callbacks:
			case <-time.After(3 * time.Second):
				t.Fatal("unlock did not queue its response")
			}
			// Authentication succeeded, but the browser locked the shared vault
			// before the successful response reached the desktop UI.
			s.locked = true
			generation := a.generation
			a.handleSessionLock()
			callback()
			if !a.state.Locked || a.generation == generation || a.busy || a.passphrase != "" || a.quick.passphrase != "" {
				t.Fatal("lock notification did not invalidate the pending unlock and clear its secrets")
			}
			if len(s.requests) != 1 || s.requests[0].Action != "unlock" {
				t.Fatal("stale unlock response dispatched a follow-up operation")
			}
		})
	}
}

func TestSessionPollingPreservesPendingUnlock(t *testing.T) {
	a := fixture()
	a.service = &sessionBackend{locked: true}
	a.state.Locked = true
	a.busy = true
	a.activeRequestAction = "unlock"
	a.passphrase = "private"
	generation := a.generation
	for range 3 {
		a.refreshSessionLock()
	}
	if a.generation != generation || !a.busy || a.activeRequestAction != "unlock" || a.passphrase != "private" {
		t.Fatal("session polling cancelled an in-progress unlock")
	}
}

func TestBrowserSessionIgnoresStaleDesktopActivity(t *testing.T) {
	a := fixture()
	s := &sessionBackend{}
	a.service = s
	a.state.Settings.IdleMinutes = 1
	a.state.Settings.MaxSessionMinutes = 30
	a.lastInput = time.Now().Add(-time.Hour)
	a.lastTouch = a.lastInput.Add(time.Second)

	// The browser has authenticated a fresh shared session while the desktop
	// has not received input for longer than either configured timeout.
	for range 3 {
		a.refreshSessionLock()
	}
	if a.state.Locked || a.detail.Contents == "" || len(s.requests) != 0 {
		t.Fatal("stale desktop activity locked a live shared session")
	}

	// Expiry comes from the shared vault, even if the desktop just had input.
	s.locked = true
	a.lastInput = time.Now()
	a.refreshSessionLock()
	if !a.state.Locked || a.detail.Contents != "" || len(s.requests) != 0 {
		t.Fatal("shared expiry did not clear secrets or dispatched another lock")
	}
}

func TestDelayedLockNotificationCannotClearNewBrowserSession(t *testing.T) {
	a := fixture()
	s := &sessionBackend{locked: true}
	a.service = s
	a.refreshSessionLock()
	generation := a.generation
	a.refreshSessionLock()
	if a.generation != generation {
		t.Fatal("duplicate expiry invalidated the UI again")
	}
	s.locked = false
	a.state.Locked = false
	a.detail.Contents = "fixture-new-session"
	a.handleSessionLock()
	if a.state.Locked || a.detail.Contents == "" || a.generation != generation {
		t.Fatal("delayed lock notification cleared a new browser session")
	}
}
