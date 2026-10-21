package desktop

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func recoveryColumnsFixture() *application {
	a := fixture()
	a.page = "recovery"
	a.recovery = []app.RecoveryItem{
		{ID: "first-copy", Path: "工作/First", CreatedAt: "2026-10-20 10:00"},
		{ID: "second-copy", Path: "工作/Second", CreatedAt: "2026-10-21 11:00"},
	}
	return a
}

func recoveryColumnsTestView(a *application, listWidth float32) func(*ui.Context) {
	return func(c *ui.Context) {
		c.SetTheme(a.clientTheme(c))
		a.recoveryColumnsView(c, listWidth)
	}
}

func TestRecoveryColumnsSelectionOnlyReadsMetadata(t *testing.T) {
	a := recoveryColumnsFixture()
	a.reveal = true
	a.service = backendFunc(func(app.Request) app.Response {
		t.Error("selecting recovery metadata requested decrypted contents")
		return app.Response{}
	})
	view := ui.NewTester(recoveryColumnsTestView(a, 300), 900, 620)
	a.restoreDestination = "custom/first-target"
	if err := view.Click("恢复副本 工作/Second"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if a.recoverySelection != "second-copy" || a.restoreDestination != "" {
		t.Fatal("a different copy retained the previous restore destination")
	}
	if view.HasText(a.detail.Password) || view.HasText(a.detail.Username) || view.HasText(a.detail.Notes) {
		t.Fatal("recovery metadata displayed the previously selected account's contents")
	}
	if !view.HasText("Second") || !view.HasText("2026-10-21 11:00") {
		t.Fatal("the detail did not follow the selected recovery copy")
	}
}

func TestRecoveryColumnsSelectedPathRetainsThemeTextContrast(t *testing.T) {
	for _, style := range []string{app.ThemeStyleMonochrome, app.ThemeStyleOnevra} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dark=%t", style, dark), func(t *testing.T) {
				a := recoveryColumnsFixture()
				palette := clientPalette(style, dark)
				view := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(palette)
					a.recoveryColumnsView(c, 300)
				}, 900, 620)
				bounds, found := view.Find("工作/First")
				if !found || bounds.X >= 300 {
					t.Fatal("the selected recovery row's path is missing")
				}
				background := palette.Accent.Alpha(.10).Over(palette.Surface)
				if isMonochromePalette(palette) {
					background = palette.Selection
				}
				var strongest float64
				rendered := view.Image()
				for y := int(math.Ceil(float64(bounds.Y))); y < int(bounds.Y+bounds.H); y++ {
					for x := int(math.Ceil(float64(bounds.X))); x < int(bounds.X+bounds.W); x++ {
						pixel := rendered.RGBAAt(x, y)
						strongest = max(strongest, themeContrast(ui.RGB(pixel.R, pixel.G, pixel.B), background))
					}
				}
				// Allow glyph antialiasing while rejecting the low-contrast primary
				// button foreground inherited from the selected state.
				if strongest < themeContrast(palette.Text, background)*.72 {
					t.Fatalf("selected path lost theme text contrast: %.2f", strongest)
				}
			})
		}
	}
}

func TestRecoveryColumnsFitNarrowDetailAndKeepDestination(t *testing.T) {
	a := recoveryColumnsFixture()
	for _, width := range []int{860, 640} {
		view := ui.NewTester(recoveryColumnsTestView(a, 280), width, 620)
		input, ok := view.Find("恢复到其他位置（可选）")
		if !ok || input.X < 280 || input.X+input.W > float32(width) {
			t.Fatalf("restore input escaped the narrow detail at width %v: %v", width, input)
		}
		if a.restoreDestination == "" {
			// The visible caption and input share a label. Click the field below
			// the caption so the input receives keyboard focus.
			view.ClickAt(input.X+input.W/2, input.Y+input.H+14)
			view.Type("work/recovered")
		}
		view.Frame()
		if a.restoreDestination != "work/recovered" {
			t.Fatalf("repainting or changing width lost the entered restore destination at %d: %q", width, a.restoreDestination)
		}
	}
}

func TestRecoveryColumnsFailedRestoreRetainsCopyAndDestination(t *testing.T) {
	a := recoveryColumnsFixture()
	callbacks := make(chan func(), 1)
	requests := make(chan app.Request, 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "error", Code: "conflict", Message: "目标已存在", Data: state}
	})
	view := ui.NewTester(recoveryColumnsTestView(a, 300), 900, 620)
	a.restoreDestination = "work/conflicting-name"
	if err := view.Click("恢复"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	view.Frame()
	request := <-requests
	if request.Action != "restore" || request.Kind != "trash" || request.ID != "first-copy" || request.Destination != "work/conflicting-name" {
		t.Fatalf("restore requested the wrong copy or destination: %#v", request)
	}
	if len(a.recovery) != 2 || a.recoverySelection != "first-copy" || a.restoreDestination != request.Destination || a.errorText != "目标已存在" {
		t.Fatal("failed restore discarded its copy, entered destination or error")
	}
}

func TestRecoveryColumnsSuccessfulRestoreRemovesOnlyRequestedCopy(t *testing.T) {
	a := recoveryColumnsFixture()
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	state := a.state
	a.service = backendFunc(func(app.Request) app.Response { return app.Response{Status: "ok", Data: state} })
	view := ui.NewTester(recoveryColumnsTestView(a, 300), 900, 620)
	if err := view.Click("恢复"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	view.Frame()
	if len(a.recovery) != 1 || a.recovery[0].ID != "second-copy" || a.recoverySelection != "second-copy" || a.notice != "已恢复条目" {
		t.Fatal("successful restore removed or selected the wrong recovery copy")
	}
}

func TestRecoveryColumnsDelayedDraftCannotReplaceNewSelection(t *testing.T) {
	a := recoveryColumnsFixture()
	a.recoveryKind = 1
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	state := a.state
	a.service = backendFunc(func(app.Request) app.Response {
		state.Entry, state.Contents = "工作/First", "synthetic-draft-secret"
		return app.Response{Status: "ok", Data: state}
	})
	view := ui.NewTester(recoveryColumnsTestView(a, 300), 900, 620)
	if err := view.Click("继续编辑"); err != nil {
		t.Fatal(err)
	}
	// A changed list/navigation selection invalidates the pending draft read.
	a.selectRecovery("second-copy")
	awaitMain(t, callbacks)
	if a.editor.open || a.page != "recovery" || a.state.Contents != "" {
		t.Fatal("a delayed draft response opened the previous selection or retained decrypted state")
	}
}

func TestRecoveryColumnsLockedStateHidesMetadataAndActions(t *testing.T) {
	a := recoveryColumnsFixture()
	a.state.Locked = true
	view := ui.NewTester(recoveryColumnsTestView(a, 300), 900, 620)
	for _, text := range []string{"First", "工作/First", "2026-10-20 10:00"} {
		if view.HasText(text) {
			t.Fatalf("locked recovery exposed metadata: %s", text)
		}
	}
	if _, found := view.Find("永久删除恢复副本"); found {
		t.Fatal("locked recovery exposed a destructive action")
	}
}
