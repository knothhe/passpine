package desktop

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine-neo/client/app"
)

func generatorFixture() (*application, chan func(), chan app.Request) {
	a := fixture()
	a.editEntry()
	callbacks := make(chan func(), 8)
	requests := make(chan app.Request, 8)
	a.runOnMain = func(f func()) { callbacks <- f }
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		data := state
		data.Password = strings.Repeat("x", request.Length)
		return app.Response{Status: "ok", Data: data}
	})
	return a, callbacks, requests
}

func nextGeneratorRequest(t *testing.T, requests <-chan app.Request) app.Request {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(3 * time.Second):
		t.Fatal("generator did not dispatch a request")
		return app.Request{}
	}
}

func nextGeneratorCallback(t *testing.T, callbacks <-chan func()) func() {
	t.Helper()
	select {
	case callback := <-callbacks:
		return callback
	case <-time.After(3 * time.Second):
		t.Fatal("generator did not publish a main-thread callback")
		return nil
	}
}

func TestGeneratorUsesIndependentRequestState(t *testing.T) {
	a, callbacks, requests := generatorFixture()
	a.busy, a.backgroundBusy, a.activeRequestAction = true, true, "configure"
	a.openGenerator()
	request := nextGeneratorRequest(t, requests)
	if request.Action != "generate" || !a.generator.busy || !a.busy || !a.backgroundBusy || a.activeRequestAction != "configure" {
		t.Fatal("generator interfered with the main request lane")
	}
	awaitMain(t, callbacks)
	if !a.generatorReady() || !a.busy || !a.backgroundBusy || a.activeRequestAction != "configure" || a.state.Password != "" {
		t.Fatal("generator completion replaced global request state or metadata")
	}
}

func TestGeneratorCoalescesChangesAndDiscardsOldOptions(t *testing.T) {
	a, callbacks, requests := generatorFixture()
	a.openGenerator()
	first := nextGeneratorRequest(t, requests)
	for _, length := range []float64{32, 48, 33.6} {
		a.generator.length = length
		a.generator.upper, a.generator.symbols, a.generator.exclude = false, false, false
		a.requestGeneratorUpdate()
	}
	if !a.generator.pending || a.generatorReady() || a.busy {
		t.Fatal("option changes did not stay pending in the generator lane")
	}
	select {
	case request := <-requests:
		t.Fatalf("an intermediate option started a concurrent request: %#v", request)
	default:
	}
	awaitMain(t, callbacks)
	latest := nextGeneratorRequest(t, requests)
	if first.Length != 24 || latest.Length != 34 || latest.Uppercase || latest.Symbols || latest.ExcludeAmbiguous || !latest.Lowercase || !latest.Numbers {
		t.Fatalf("latest request options were not coalesced: %#v", latest)
	}
	if a.generator.password != "" || !a.generator.busy || a.generatorReady() {
		t.Fatal("old options were presented while the latest result was pending")
	}
	awaitMain(t, callbacks)
	if !a.generatorReady() || len(a.generator.password) != 34 {
		t.Fatal("latest options did not produce the usable password")
	}
	select {
	case request := <-requests:
		t.Fatalf("an unnecessary intermediate request was dispatched: %#v", request)
	default:
	}
}

func TestGeneratorRetainsPreviewButCannotApplyStaleOptions(t *testing.T) {
	a, callbacks, requests := generatorFixture()
	a.openGenerator()
	nextGeneratorRequest(t, requests)
	awaitMain(t, callbacks)
	previousPreview, previousPassword := a.generator.password, a.editor.password
	a.generator.length = 40
	if a.generatorReady() {
		t.Fatal("an option change left the previous password usable")
	}
	a.applyGeneratedPassword()
	if a.editor.password != previousPassword {
		t.Fatal("stale options were applied to the editor")
	}
	a.requestGeneratorUpdate()
	nextGeneratorRequest(t, requests)
	if a.generator.password != previousPreview || a.generatorReady() {
		t.Fatal("updating options flashed the preview or left it usable")
	}
	awaitMain(t, callbacks)
	a.applyGeneratedPassword()
	if a.generator.open || a.generator.busy || a.generator.pending || a.generator.password != "" || len(a.editor.password) != 40 {
		t.Fatal("applying the result did not close and clear generation state")
	}
}

func TestGeneratorRunsOnlyLatestCompletionCallback(t *testing.T) {
	a, callbacks, requests := generatorFixture()
	a.openGenerator()
	nextGeneratorRequest(t, requests)
	awaitMain(t, callbacks)
	oldCalled, latestCalled := false, false
	a.generate(func(string) { oldCalled = true })
	nextGeneratorRequest(t, requests)
	a.generator.length = 35
	a.generate(func(password string) { latestCalled = len(password) == 35 })
	awaitMain(t, callbacks)
	nextGeneratorRequest(t, requests)
	awaitMain(t, callbacks)
	if oldCalled || !latestCalled {
		t.Fatal("a superseded generation callback ran or the latest one was lost")
	}
}

func TestGeneratorLateResponseCannotReviveInvalidSurface(t *testing.T) {
	for _, action := range []string{"cancel", "lock", "editor", "vault", "generation", "closing"} {
		t.Run(action, func(t *testing.T) {
			a, callbacks, requests := generatorFixture()
			a.openGenerator()
			nextGeneratorRequest(t, requests)
			switch action {
			case "cancel":
				a.closeGenerator()
			case "lock":
				a.state.Locked = true
				a.clearSecrets()
			case "editor":
				a.editor = editorState{open: true, contents: "replacement-password"}
				a.editor.readFields()
			case "vault":
				a.state.VaultID = "another-vault"
			case "generation":
				a.generation++
			case "closing":
				a.closing = true
			}
			password := a.editor.password
			awaitMain(t, callbacks)
			if a.generator.open || a.generator.busy || a.generator.pending || a.generator.password != "" || a.editor.password != password {
				t.Fatal("a late generation revived a dismissed or replaced surface")
			}
		})
	}
}

func TestGeneratorReopeningDoesNotLetOldResponseClearNewRequest(t *testing.T) {
	a, callbacks, requests := generatorFixture()
	a.openGenerator()
	nextGeneratorRequest(t, requests)
	oldCallback := nextGeneratorCallback(t, callbacks)
	a.closeGenerator()
	a.generator.length = 64
	a.openGenerator()
	nextGeneratorRequest(t, requests)
	newCallback := nextGeneratorCallback(t, callbacks)
	oldCallback()
	if !a.generator.busy || !a.generator.open || a.generator.password != "" {
		t.Fatal("an old response cleared or populated the reopened generator")
	}
	newCallback()
	if !a.generatorReady() || len(a.generator.password) != 64 {
		t.Fatal("the reopened generator did not accept its own request")
	}
}

func TestGeneratorOptionsNormalizeLengths(t *testing.T) {
	for _, sample := range []struct {
		input float64
		want  int
	}{{4, 8}, {999, 128}, {15.6, 16}, {math.NaN(), 24}, {math.Inf(1), 24}} {
		g := generatorState{length: sample.input}
		if options := g.options(); options.length != sample.want || g.length != float64(sample.want) {
			t.Fatalf("length %v was normalized to %v instead of %d", sample.input, g.length, sample.want)
		}
	}
}

func TestGeneratorLockObservationRevokesOlderMainLaneResponse(t *testing.T) {
	a, callbacks, requests := generatorFixture()
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		data := state
		if request.Action == "fetch" {
			data.Entry, data.Password, data.Contents = "old-entry", "old-secret", "old-secret\n"
		} else {
			data.Locked, data.Password = true, "must-not-appear"
		}
		return app.Response{Status: "ok", Data: data}
	})
	a.call(app.Request{Action: "fetch", Entry: "old-entry"}, func(data app.Data) { a.detail = data })
	nextGeneratorRequest(t, requests)
	oldMainCallback := nextGeneratorCallback(t, callbacks)
	generation := a.generation
	a.openGenerator()
	nextGeneratorRequest(t, requests)
	awaitMain(t, callbacks)
	oldMainCallback()
	if a.generation != generation+1 || !a.state.Locked || a.busy || a.backgroundBusy || a.editor.open || a.generator.open || a.detail.Password != "" || a.state.Password != "" {
		t.Fatal("an older main-lane response revived state after the generator observed a lock")
	}
}

func TestGeneratorVaultChangeObservationRevokesOldSnapshotsAndRefreshes(t *testing.T) {
	a, callbacks, requests := generatorFixture()
	a.state.VaultID = "original-vault"
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		data := state
		switch request.Action {
		case "fetch":
			data.Entry, data.Password, data.Contents = "old-entry", "old-secret", "old-secret\n"
		case "generate":
			data.VaultID = "new-vault"
			return app.Response{Status: "error", Code: "vault_changed", Data: data}
		case "configure":
			data.VaultID, data.Locked = "new-vault", true
		}
		return app.Response{Status: "ok", Data: data}
	})
	a.call(app.Request{Action: "fetch", Entry: "old-entry"}, func(data app.Data) { a.detail = data })
	nextGeneratorRequest(t, requests)
	oldMainCallback := nextGeneratorCallback(t, callbacks)
	generation := a.generation
	a.openGenerator()
	nextGeneratorRequest(t, requests)
	// An option change must not defer a vault-change observation until the
	// latest pending generation. It cancels both request lanes immediately.
	a.generator.length = 40
	a.requestGeneratorUpdate()
	awaitMain(t, callbacks)
	refresh := nextGeneratorRequest(t, requests)
	if refresh.Action != "configure" || a.generation != generation+1 || a.generator.open || a.editor.open {
		t.Fatal("vault change did not revoke old surfaces and schedule metadata refresh")
	}
	oldMainCallback()
	if !a.state.Locked || a.detail.Password != "" {
		t.Fatal("an older main-lane response exposed the old vault during refresh")
	}
	awaitMain(t, callbacks)
	if a.state.VaultID != "new-vault" || !a.state.Locked || a.busy || a.detail.Password != "" || a.generator.open {
		t.Fatal("vault-change refresh did not settle on the current locked vault")
	}
}

func TestGeneratorControlsUpdateAutomaticallyWhileGenerating(t *testing.T) {
	a, callbacks, requests := generatorFixture()
	a.openGenerator()
	nextGeneratorRequest(t, requests)
	view := ui.NewTester(a.dialogs, 1120, 760)
	before, ok := view.Find("密码长度数值")
	if !ok {
		t.Fatal("length number input missing")
	}
	if err := view.Click("密码长度数值"); err != nil {
		t.Fatal(err)
	}
	view.Key(0, ui.KeyUp)
	if a.generator.length != 25 || !a.generator.pending || !view.Focused("密码长度数值") {
		t.Fatal("number input could not update while generation was pending")
	}
	if after, _ := view.Find("密码长度数值"); after != before {
		t.Fatal("generation moved the length input")
	}
	if err := view.Click("大写字母"); err != nil {
		t.Fatal(err)
	}
	if a.generator.upper || a.busy {
		t.Fatal("character toggles were disabled by generation")
	}
	awaitMain(t, callbacks)
	latest := nextGeneratorRequest(t, requests)
	if latest.Length != 25 || latest.Uppercase {
		t.Fatalf("controls did not automatically regenerate their latest options: %#v", latest)
	}
	awaitMain(t, callbacks)
	view.Frame()
	if !a.generatorReady() {
		t.Fatal("control updates did not finish with a usable result")
	}
}

func TestGeneratorRender(t *testing.T) {
	directory := os.Getenv("PASSPINE_GENERATOR_SCREENSHOTS")
	if directory == "" {
		t.Skip("set PASSPINE_GENERATOR_SCREENSHOTS to export generator previews")
	}
	for _, sample := range []struct {
		name, language string
		width, height  int
		dark           bool
		length         int
		compact        bool
	}{
		{"en-dark", "en", 1120, 760, true, 24, false},
		{"en-light", "en", 1120, 760, false, 24, false},
		{"zh", "zh-CN", 1120, 760, false, 24, false},
		{"small", "en", 860, 620, false, 24, false},
		{"long", "en", 1120, 760, true, 128, false},
		{"generator-en-dark-compact", "en", 460, 310, true, 24, true},
	} {
		t.Run(sample.name, func(t *testing.T) {
			a := fixture()
			a.state.Settings.Language = sample.language
			a.editEntry()
			a.generator.open, a.generator.target = true, a.editor.ensureIdentity()
			a.generator.generation, a.generator.vaultID = a.generation, a.state.VaultID
			a.generator.length = float64(sample.length)
			a.generator.password = strings.Repeat("aB3!xQ8@", 16)[:sample.length]
			a.generator.generated = a.generator.options()
			build := testView(a)
			if sample.compact {
				build = func(c *ui.Context) {
					c.SetTheme(clientTheme(c))
					a.generatorDialog(c)
				}
			}
			view := ui.NewTester(build, sample.width, sample.height)
			view.SetDark(sample.dark)
			view.Frame()
			if err := os.MkdirAll(directory, 0755); err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(filepath.Join(directory, sample.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := png.Encode(file, view.Image()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
