package desktop

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

type managerLayoutBackend struct{ directory string }

func (backend managerLayoutBackend) DataDirectory() string { return backend.directory }
func (managerLayoutBackend) Execute(app.Request) app.Response {
	return app.Response{Status: "ok"}
}

func managerLayoutFixture(t *testing.T, directory string, width int) (*application, *ui.Tester, chan func()) {
	t.Helper()
	initial := fixture()
	// Construct a fresh application and exercise its startup read before the
	// first frame. No window or in-memory layout state is shared across restarts.
	a := newApplication(managerLayoutBackend{directory: directory})
	a.state, a.detail, a.selected = initial.state, initial.detail, initial.selected
	callbacks := make(chan func(), 16)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	t.Cleanup(a.saveManagerLayout)
	view := ui.NewTester(testView(a), width, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	return a, view, callbacks
}

func assertManagerLayoutStoredWidth(t *testing.T, directory string, want float32) {
	t.Helper()
	width, found, err := loadManagerLayout(filepath.Join(directory, managerLayoutFile))
	if err != nil || !found || width != want {
		t.Fatalf("saved list width = %g, found=%t, error=%v; want %g", width, found, err, want)
	}
}

func takeManagerLayoutCallback(t *testing.T, callbacks <-chan func()) func() {
	t.Helper()
	select {
	case callback := <-callbacks:
		return callback
	case <-time.After(3 * time.Second):
		t.Fatal("layout debounce did not enqueue its main-thread save")
		return nil
	}
}

func TestManagerLayoutPersistsPointerAndKeyboardWidthBeforeFirstRestartFrame(t *testing.T) {
	directory := t.TempDir()
	a, view, _ := managerLayoutFixture(t, directory, 1120)
	dragManagerListTo(t, a, view, 390)
	if !a.managerLayout.dirty || a.managerLayout.timer == nil {
		t.Fatal("dragging did not queue layout persistence")
	}
	// Quitting immediately after a drag must persist without waiting for the
	// debounce. Closing must not suppress this last synchronous flush.
	a.closing = true
	a.saveManagerLayout()
	assertManagerLayoutStoredWidth(t, directory, 390)
	restarted, next, callbacks := managerLayoutFixture(t, directory, 1120)
	assertManagerListWidth(t, next, 390)
	if restarted.managerListPreferredWidth != 390 || restarted.managerLayout.dirty || restarted.managerLayout.timer != nil {
		t.Fatal("startup did not restore the preference before rendering or queued a redundant save")
	}
	if err := next.Click(restarted.t("调整条目列表宽度")); err != nil {
		t.Fatal(err)
	}
	next.Key(0, ui.KeyRight)
	awaitMain(t, callbacks)
	assertManagerLayoutStoredWidth(t, directory, 400)
	_, final, _ := managerLayoutFixture(t, directory, 1120)
	assertManagerListWidth(t, final, 400)
}

func TestManagerLayoutResponsiveClampAndIdleFramesNeverRewriteSavedPreference(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, managerLayoutFile)
	if err := storeManagerLayout(path, 480); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	a, view, _ := managerLayoutFixture(t, directory, 961)
	assertManagerListWidth(t, view, 961-a.shell.LeftWidth()-360-2*(managerFrameInset+managerFrameBorder))
	for _, width := range []int{860, 1120, 961} {
		view.SetSize(width, 760)
		view.Frame()
	}
	if err := view.Click(a.t("调整条目列表宽度")); err != nil {
		t.Fatal(err)
	}
	view.Key(0, ui.KeyRight) // Already at the temporary available-width maximum.
	a.saveManagerLayout()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !info.ModTime().Equal(stamp) || a.managerListPreferredWidth != 480 || a.managerLayout.dirty || a.managerLayout.timer != nil {
		t.Fatal("layout rendering or temporary clamping rewrote the saved user preference")
	}
	_, restarted, _ := managerLayoutFixture(t, directory, 860)
	assertManagerListWidth(t, restarted, 480)
}

func TestManagerLayoutDoubleClickPersistsResponsiveDefaultAcrossRestart(t *testing.T) {
	directory := t.TempDir()
	if err := storeManagerLayout(filepath.Join(directory, managerLayoutFile), 430); err != nil {
		t.Fatal(err)
	}
	a, view, _ := managerLayoutFixture(t, directory, 1120)
	if err := view.Click(a.t("调整条目列表宽度")); err != nil {
		t.Fatal(err)
	}
	view.Advance(100 * time.Millisecond)
	if err := view.Click(a.t("调整条目列表宽度")); err != nil {
		t.Fatal(err)
	}
	a.saveManagerLayout()
	assertManagerLayoutStoredWidth(t, directory, 0)
	restarted, next, _ := managerLayoutFixture(t, directory, 1120)
	assertManagerListWidth(t, next, 300)
	next.SetSize(860, 760)
	assertManagerListWidth(t, next, 280)
	if restarted.managerListPreferredWidth != 0 {
		t.Fatal("restart turned a reset into a fixed width")
	}
}

func TestManagerLayoutDamagedOrMissingDataKeepsStartupDefaults(t *testing.T) {
	for _, test := range []struct{ name, data string }{
		{"missing", ""},
		{"truncated", `{"listWidth":`},
		{"null", `{"listWidth":null}`},
		{"out of range", `{"listWidth":900}`},
		{"trailing value", `{"listWidth":400} {}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, managerLayoutFile)
			if test.data != "" {
				if err := os.WriteFile(path, []byte(test.data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			a, view, _ := managerLayoutFixture(t, directory, 1120)
			assertManagerListWidth(t, view, 300)
			view.SetSize(860, 760)
			assertManagerListWidth(t, view, 280)
			if a.managerListPreferredWidth != 0 || a.managerLayout.saved || a.managerLayout.dirty || a.managerLayout.timer != nil || a.busy {
				t.Fatal("invalid layout data affected startup or triggered a write")
			}
		})
	}
}

func TestManagerLayoutDebounceDiscardsLateCallbacksAndFlushesNewestWidth(t *testing.T) {
	directory := t.TempDir()
	if err := storeManagerLayout(filepath.Join(directory, managerLayoutFile), 320); err != nil {
		t.Fatal(err)
	}
	a, view, callbacks := managerLayoutFixture(t, directory, 1120)
	if err := view.Click(a.t("调整条目列表宽度")); err != nil {
		t.Fatal(err)
	}
	view.Key(0, ui.KeyRight)
	stale := takeManagerLayoutCallback(t, callbacks)
	view.Key(0, ui.KeyRight)
	view.Key(0, ui.KeyRight)
	stale()
	assertManagerLayoutStoredWidth(t, directory, 320)
	if !a.managerLayout.dirty || a.managerLayout.timer == nil {
		t.Fatal("a superseded callback flushed or canceled the newer debounce")
	}
	awaitMain(t, callbacks)
	assertManagerLayoutStoredWidth(t, directory, 350)
	view.Key(0, ui.KeyRight)
	lateQuitCallback := takeManagerLayoutCallback(t, callbacks)
	a.closing = true
	a.saveManagerLayout()
	lateQuitCallback()
	assertManagerLayoutStoredWidth(t, directory, 360)
	if a.managerLayout.dirty || a.managerLayout.timer != nil {
		t.Fatal("quit left a pending layout save after flushing")
	}
	_, restarted, _ := managerLayoutFixture(t, directory, 1120)
	assertManagerListWidth(t, restarted, 360)
}

func TestManagerLayoutSaveFailureKeepsUIUsableAndCanRetry(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, managerLayoutFile)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	a, view, callbacks := managerLayoutFixture(t, directory, 1120)
	dragManagerListTo(t, a, view, 410)
	a.saveManagerLayout()
	assertManagerListWidth(t, view, 410)
	if !a.managerLayout.dirty || !a.managerLayout.saveFailed || a.managerLayout.saved || a.busy || a.notice == "" {
		t.Fatal("persistence failure blocked resizing, lost the pending value or hid the failure")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	a.queueManagerLayoutSave()
	awaitMain(t, callbacks)
	assertManagerLayoutStoredWidth(t, directory, 410)
	if a.managerLayout.dirty || a.managerLayout.saveFailed || !a.managerLayout.saved {
		t.Fatal("retry did not clear the failed persistence state")
	}
	_, restarted, _ := managerLayoutFixture(t, directory, 1120)
	assertManagerListWidth(t, restarted, 410)
}

func TestManagerLayoutSidebarDragPersistsAcrossRestartAndResetsToDefault(t *testing.T) {
	directory := t.TempDir()
	// Legacy layout files contain only the middle list's width.
	if err := storeManagerLayout(filepath.Join(directory, managerLayoutFile), 390); err != nil {
		t.Fatal(err)
	}
	a, view, _ := managerLayoutFixture(t, directory, 1120)
	if a.shell.LeftWidth() != 200 {
		t.Fatal("legacy layout did not use the default sidebar width")
	}
	handle := visualBounds(t, view, a.t("调整侧栏宽度"))
	x, y := handle.X+handle.W/2, handle.Y+handle.H/2
	view.Press(x, y)
	view.Move(x+60, y)
	view.Release(x+60, y)
	a.saveManagerLayout()
	restarted, next, _ := managerLayoutFixture(t, directory, 1120)
	if restarted.shell.LeftWidth() != 260 {
		t.Fatalf("restart lost the sidebar width: %g", restarted.shell.LeftWidth())
	}
	assertManagerListWidth(t, next, 390)
	next.SetSize(860, 760)
	restarted.saveManagerLayout()
	next.SetSize(1120, 760)
	if restarted.shell.LeftWidth() != 260 {
		t.Fatal("narrow-window clipping overwrote the sidebar preference")
	}
	next.Advance(time.Second)
	if err := next.Click(restarted.t("调整侧栏宽度")); err != nil {
		t.Fatal(err)
	}
	next.Advance(100 * time.Millisecond)
	if err := next.Click(restarted.t("调整侧栏宽度")); err != nil {
		t.Fatal(err)
	}
	restarted.saveManagerLayout()
	final, finalView, _ := managerLayoutFixture(t, directory, 1120)
	if final.shell.LeftWidth() != 200 {
		t.Fatal("sidebar reset did not persist the original default width")
	}
	assertManagerListWidth(t, finalView, 390)
}
