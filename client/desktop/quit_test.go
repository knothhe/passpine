package desktop

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

type shutdownBackend struct {
	backendFunc
	shutdown func(context.Context) error
}

func (s shutdownBackend) Shutdown(ctx context.Context) error { return s.shutdown(ctx) }

func quitFixture(t *testing.T) (*application, chan func(), *int) {
	a := fixture()
	a.settings, a.settingsSaved, a.settingsObserved, a.settingsLoaded = a.state.Settings, a.state.Settings, a.state.Settings, true
	callbacks := make(chan func(), 20)
	a.runOnMain = func(f func()) { callbacks <- f }
	count := new(int)
	a.quit.quit = func() { *count++ }
	t.Cleanup(func() {
		if a.quit.timer != nil {
			a.quit.timer.Stop()
		}
	})
	return a, callbacks, count
}

func TestQuitWaitsForSettingsAndServiceDrainAndRunsOnlyOnce(t *testing.T) {
	a, callbacks, count := quitFixture(t)
	writing, release := make(chan struct{}), make(chan struct{})
	draining, drained := make(chan struct{}), make(chan struct{})
	a.service = shutdownBackend{backendFunc: func(request app.Request) app.Response {
		close(writing)
		<-release
		return app.Response{Status: "ok", Data: app.Data{Settings: *request.Settings}}
	}, shutdown: func(context.Context) error { close(draining); <-drained; return nil }}
	a.settings.Theme = "dark"
	event := &mygo.QuitEvent{}
	a.beforeQuit(event)
	<-writing
	if !event.DefaultPrevented() || a.closing || *count != 0 {
		t.Fatal("quit interrupted the pending save")
	}
	a.beforeQuit(&mygo.QuitEvent{})
	close(release)
	for !a.quit.draining {
		awaitMain(t, callbacks)
	}
	<-draining
	if a.settingsSaved.Theme != "dark" || *count != 0 {
		t.Fatal("quit did not drain settings first")
	}
	close(drained)
	awaitMain(t, callbacks)
	if !a.quit.ready || *count != 1 {
		t.Fatal("quit did not finish exactly once")
	}
	event = &mygo.QuitEvent{}
	a.beforeQuit(event)
	if event.DefaultPrevented() || *count != 1 {
		t.Fatal("completed quit re-entered the barrier")
	}
}

func TestFailedSettingsSaveCancelsQuitAndPreservesInput(t *testing.T) {
	a, callbacks, count := quitFixture(t)
	a.service = backendFunc(func(app.Request) app.Response { return app.Response{Status: "error", Message: "disk full"} })
	a.settings.Theme = "dark"
	a.requestQuit()
	awaitMain(t, callbacks)
	if a.quit.requested || a.closing || *count != 0 || a.settings.Theme != "dark" || a.errorText != "disk full" {
		t.Fatal("failed save allowed quit or lost input")
	}
}

func TestQuitDirtyEditorCanCancelWithoutLosingDraft(t *testing.T) {
	a, _, count := quitFixture(t)
	a.editEntry()
	a.editor.password = "changed"
	a.editor.compose()
	a.requestQuit()
	if !a.discardOpen || a.quit.requested || a.closing {
		t.Fatal("quit bypassed the dirty editor")
	}
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("继续编辑"); err != nil {
		t.Fatal(err)
	}
	if !a.editor.open || a.editor.password != "changed" || a.quit.requested || *count != 0 {
		t.Fatal("canceling quit lost the draft")
	}
}

func TestShutdownFailureKeepsApplicationAvailableForRetry(t *testing.T) {
	a, callbacks, count := quitFixture(t)
	a.service = shutdownBackend{shutdown: func(context.Context) error { return errors.New("still running") }}
	a.requestQuit()
	awaitMain(t, callbacks)
	if a.closing || a.quit.requested || a.quit.draining || *count != 0 || a.errorText == "" {
		t.Fatal("failed shutdown left application unusable")
	}
	select {
	case <-a.stop:
		t.Fatal("clock stopped before successful shutdown")
	default:
	}
}

func TestQuitWaitHasDeadlineAndPreservesBusyWork(t *testing.T) {
	a, _, count := quitFixture(t)
	a.busy = true
	a.quit.requested = true
	a.quit.started = time.Now().Add(-31 * time.Second)
	a.continueQuit()
	if a.quit.requested || a.closing || !a.busy || *count != 0 {
		t.Fatal("deadline interrupted work or trapped quit")
	}
}

func TestQuitAfterLockIgnoresObsoleteBackgroundPresentationFlag(t *testing.T) {
	a, callbacks, count := quitFixture(t)
	a.backgroundBusy = true // A lock may invalidate a metadata response first.
	a.requestQuit()
	awaitMain(t, callbacks)
	if !a.quit.ready || *count != 1 {
		t.Fatal("obsolete presentation state blocked service shutdown")
	}
}
