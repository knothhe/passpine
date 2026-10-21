package clientshell

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/theme"
)

type overlayFixture struct {
	shell                         *Shell
	view                          *ui.Tester
	content, scroll, lastRow      ui.Rect
	sidebarScroll, sidebarLastRow ui.Rect
	editor                        string
	contentClicks, sidebarClicks  int
	backClicks, forwardClicks     int
}

func newOverlayFixture(width int) *overlayFixture {
	f := &overlayFixture{}
	shell, build := Layout(ShellOptions{
		LeftWidth: 224, LeftMinWidth: 180, LeftMaxWidth: 320,
		LeftOverlayBelow: 960, LeftOverlayCloseLabel: "Dismiss navigation", Theme: theme.Onevra,
		LeadingHeaderWidth: 64,
		LeadingHeader: func(c *ui.Context) {
			if IconButton(c, lucide.ChevronLeft(), "Back").Clicked() {
				f.backClicks++
			}
			if IconButton(c, lucide.ChevronRight(), "Forward").Clicked() {
				f.forwardClicks++
			}
		},
		ContentTitle: func(c *ui.Context) { ui.Text(c, "Current collection").SingleLine() },
		Left: func(c *ui.Context) {
			ui.Column(c).Fill().Padding(HeaderHeight(c), 12, 12, 12).Children(func() {
				if ui.Button(c, "Sidebar action").Clicked() {
					f.sidebarClicks++
				}
				ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).
					DrawOver(func(_ *ui.Painter, r ui.Rect) { f.sidebarScroll = r }).Children(func() {
					for i := range 40 {
						row := ui.Text(c, fmt.Sprintf("Folder %d", i)).Height(44).Shrink(0)
						if i == 39 {
							row.DrawOver(func(_ *ui.Painter, r ui.Rect) { f.sidebarLastRow = r })
						}
					}
				})
			})
		},
		Content: func(c *ui.Context) {
			ui.Column(c).Fill().Padding(HeaderHeight(c), 12, 12, 12).
				DrawOver(func(_ *ui.Painter, r ui.Rect) { f.content = r }).Children(func() {
				ui.TextInput(c, &f.editor).Label("Editor").FillWidth()
				if ui.Button(c, "Content action").Clicked() {
					f.contentClicks++
				}
				ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).
					DrawOver(func(_ *ui.Painter, r ui.Rect) { f.scroll = r }).Children(func() {
					for i := range 40 {
						row := ui.Text(c, fmt.Sprintf("Content row %d", i)).Height(44).Shrink(0)
						if i == 39 {
							row.DrawOver(func(_ *ui.Painter, r ui.Rect) { f.lastRow = r })
						}
					}
				})
			})
		},
	})
	f.shell = shell
	f.view = ui.NewTester(build, width, 760)
	f.view.SetTitleBar(ui.TitleBar{Height: 48, Left: 80})
	f.view.SetPreferences(ui.Preferences{ReduceMotion: true})
	return f
}

func clickShell(t *testing.T, view *ui.Tester, label string) {
	t.Helper()
	if err := view.Click(label); err != nil {
		t.Fatal(err)
	}
}

func TestNarrowSidebarOverlaysWithoutResizingContent(t *testing.T) {
	f := newOverlayFixture(860)
	if !f.shell.LeftOverlayActive() || f.shell.LeftExpanded() || !f.shell.LeftDockedExpanded() {
		t.Fatal("narrow windows must hide the sidebar without changing the docked preference")
	}
	before := f.content
	if before.X != 0 || before.W != 860 || f.shell.ContentWidth(860) != 860 {
		t.Fatalf("narrow content does not fill the viewport: %+v", before)
	}
	if _, ok := f.view.Find("Resize sidebar"); ok {
		t.Fatal("the narrow overlay must not expose a docked resize handle")
	}
	clickShell(t, f.view, "Toggle sidebar")
	if !f.shell.LeftExpanded() || f.content != before || f.shell.ContentWidth(860) != 860 {
		t.Fatalf("opening the overlay resized the content: before=%+v after=%+v", before, f.content)
	}
	clickShell(t, f.view, "Sidebar action")
	if f.sidebarClicks != 1 || !f.shell.LeftExpanded() {
		t.Fatal("sidebar actions must receive their clicks above the backdrop")
	}
	f.view.ClickAt(100, 500)
	if !f.shell.LeftExpanded() {
		t.Fatal("blank sidebar space must not dismiss the backdrop")
	}
	// A click over an underlying control closes the overlay and never reaches
	// that control. The body retains exactly the same bounds throughout.
	clickShell(t, f.view, "Content action")
	if f.shell.LeftExpanded() || f.contentClicks != 0 || f.content != before {
		t.Fatal("the backdrop did not consume the content click and close the sidebar")
	}
	clickShell(t, f.view, "Toggle sidebar")
	clickShell(t, f.view, "Dismiss navigation")
	if f.shell.LeftExpanded() || f.content != before {
		t.Fatal("the localized backdrop did not dismiss the sidebar")
	}
	clickShell(t, f.view, "Toggle sidebar")
	f.view.Key(0, ui.KeyEscape)
	if f.shell.LeftExpanded() || !f.shell.LeftDockedExpanded() || f.content != before {
		t.Fatal("Escape must close the overlay and retain the docked preference and content bounds")
	}
}

func TestResponsiveSidebarRestoresDockedChoiceAndHeaderControls(t *testing.T) {
	f := newOverlayFixture(1120)
	for _, width := range []int{1120, 860, 1120, 960, 961, 860} {
		f.view.SetSize(width, 760)
		overlay := width <= 960
		if f.shell.LeftOverlayActive() != overlay || f.shell.LeftExpanded() == overlay {
			t.Fatalf("width=%d: wrong automatic expansion state", width)
		}
		wantInset := float32(224)
		if overlay {
			wantInset = 0
		}
		if f.content.X != wantInset || f.content.W != float32(width)-wantInset || f.shell.ContentWidth(float32(width)) != f.content.W {
			t.Fatalf("width=%d: content is outside its intended region: %+v", width, f.content)
		}
		for _, label := range []string{"Back", "Forward", "Toggle sidebar", "Current collection"} {
			r, ok := f.view.Find(label)
			if !ok || r.X < 80 || r.Y < 0 || r.Y+r.H > 48 || r.X+r.W > float32(width) {
				t.Fatalf("width=%d: header control %q is hidden or overlaps native controls: %+v", width, label, r)
			}
		}
		clickShell(t, f.view, "Back")
		clickShell(t, f.view, "Forward")
	}
	if f.backClicks != 6 || f.forwardClicks != 6 {
		t.Fatal("persistent header navigation stopped receiving clicks")
	}
	clickShell(t, f.view, "Toggle sidebar")
	if !f.shell.LeftExpanded() {
		t.Fatal("narrow toggle did not open the overlay")
	}
	f.view.SetSize(1120, 760)
	if !f.shell.LeftExpanded() || !f.shell.LeftDockedExpanded() {
		t.Fatal("returning to a wide window did not restore the open docked sidebar")
	}
	clickShell(t, f.view, "Toggle sidebar")
	if f.shell.LeftExpanded() || f.shell.LeftDockedExpanded() || f.content.X != 0 {
		t.Fatal("manual wide collapse did not remove its reserved width")
	}
	f.view.SetSize(860, 760)
	clickShell(t, f.view, "Toggle sidebar")
	f.view.SetSize(1120, 760)
	if f.shell.LeftExpanded() || f.shell.LeftDockedExpanded() || f.content.W != 1120 {
		t.Fatal("a narrow overlay overwrote the manually collapsed docked preference")
	}
}

func TestResponsiveSidebarRetainsEditorFocusAndScroll(t *testing.T) {
	f := newOverlayFixture(1120)
	clickShell(t, f.view, "Editor")
	f.view.Type("unsaved")
	f.view.Scroll(f.scroll.X+f.scroll.W/2, f.scroll.Y+f.scroll.H/2, 0, -380)
	rowY := f.lastRow.Y
	if !f.view.Focused("Editor") || rowY >= 1800 {
		t.Fatal("fixture must start with a focused editor and scrolled content")
	}
	for _, width := range []int{860, 1120, 860} {
		f.view.SetSize(width, 760)
		if !f.view.Focused("Editor") || f.lastRow.Y != rowY {
			t.Fatalf("width=%d: resize lost editor focus or scroll position: row=%g want=%g", width, f.lastRow.Y, rowY)
		}
		f.view.Type("!")
	}
	if f.editor != "unsaved!!!" {
		t.Fatalf("resizing lost editor text or caret state: %q", f.editor)
	}
	// Opening from code does not steal the editor's focus, and Escape returns
	// to the exact same content rather than replacing its widgets.
	f.shell.SetLeftExpanded(true)
	f.view.Frame()
	f.shell.SetLabels(Labels{LeftOverlayClose: "关闭导航"})
	f.view.Frame()
	if _, ok := f.view.Find("关闭导航"); !ok || !f.view.Focused("Editor") {
		t.Fatal("changing labels lost overlay accessibility or editor focus")
	}
	f.view.Key(0, ui.KeyEscape)
	if !f.view.Focused("Editor") || f.lastRow.Y != rowY || f.shell.LeftExpanded() {
		t.Fatal("closing the overlay lost focus or scrolling")
	}
	f.view.Type("done")
	if f.editor != "unsaved!!!done" {
		t.Fatalf("overlay changed editor state: %q", f.editor)
	}
}

func TestResponsiveSidebarRetainsFolderScroll(t *testing.T) {
	f := newOverlayFixture(1120)
	f.view.Scroll(f.sidebarScroll.X+f.sidebarScroll.W/2, f.sidebarScroll.Y+f.sidebarScroll.H/2, 0, -380)
	rowY := f.sidebarLastRow.Y
	if rowY >= 1800 {
		t.Fatal("fixture must start with a scrolled folder list")
	}
	f.view.SetSize(860, 760)
	clickShell(t, f.view, "Toggle sidebar")
	if f.sidebarLastRow.Y != rowY {
		t.Fatalf("opening the narrow overlay lost the folder scroll: got=%g want=%g", f.sidebarLastRow.Y, rowY)
	}
	f.view.SetSize(1120, 760)
	if f.sidebarLastRow.Y != rowY {
		t.Fatal("returning to a docked sidebar lost the folder scroll")
	}
}

func TestNarrowOverlayAnimationKeepsContentBounds(t *testing.T) {
	f := newOverlayFixture(860)
	f.view.SetPreferences(ui.Preferences{})
	before := f.content
	clickShell(t, f.view, "Toggle sidebar")
	for range 6 {
		f.view.Advance(sidebarDuration / 6)
		if f.content != before {
			t.Fatalf("opening animation moved the content: before=%+v after=%+v", before, f.content)
		}
	}
	f.view.Key(0, ui.KeyEscape)
	for range 6 {
		f.view.Advance(sidebarDuration / 6)
		if f.content != before {
			t.Fatalf("closing animation moved the content: before=%+v after=%+v", before, f.content)
		}
	}
}

func TestDockedSidebarStillResizesAndResets(t *testing.T) {
	f := newOverlayFixture(1120)
	f.view.Press(224, 400)
	f.view.Move(264, 400)
	f.view.Release(264, 400)
	if f.shell.LeftWidth() != 264 || f.content.X != 264 {
		t.Fatalf("dragging did not resize the docked sidebar: width=%g content=%+v", f.shell.LeftWidth(), f.content)
	}
	f.view.Advance(time.Second)
	clickShell(t, f.view, "Resize sidebar")
	f.view.Key(0, ui.KeyRight)
	if f.shell.LeftWidth() != 274 || !f.view.Focused("Resize sidebar") {
		t.Fatal("the focused docked separator did not handle keyboard resize")
	}
	f.view.Key(0, ui.KeyLeft)
	if f.shell.LeftWidth() != 264 {
		t.Fatal("left-arrow resize did not restore the previous width")
	}
	f.view.Advance(time.Second)
	clickShell(t, f.view, "Resize sidebar")
	f.view.Advance(100 * time.Millisecond)
	clickShell(t, f.view, "Resize sidebar")
	if f.shell.LeftWidth() != 224 || f.content.X != 224 {
		t.Fatal("double-click no longer resets the docked separator")
	}
	f.shell.SetLeftWidth(320)
	f.view.Frame()
	clickShell(t, f.view, "Resize sidebar")
	f.view.Key(0, ui.KeyRight)
	if f.shell.LeftWidth() != 320 {
		t.Fatal("keyboard resize exceeded the configured sidebar maximum")
	}
}

func TestContentWidthIsPureAndReplacementPagesUseFullWidth(t *testing.T) {
	s := newShell(ShellOptions{LeftWidth: 224, LeftOverlayBelow: 960})
	for viewport, want := range map[float32]float32{860: 860, 960: 960, 961: 737, 1120: 896, -1: 0} {
		if got := s.ContentWidth(viewport); got != want {
			t.Fatalf("ContentWidth(%g)=%g, want %g", viewport, got, want)
		}
	}
	if !s.LeftExpanded() || !s.LeftDockedExpanded() || s.LeftOverlayActive() {
		t.Fatal("ContentWidth queries must not mutate the current layout state")
	}
	s.SetPage(func(c *ui.Context) { ui.Text(c, "Replacement page") })
	if s.ContentWidth(1120) != 1120 {
		t.Fatal("replacement pages should receive the full viewport width")
	}
	s.SetPage(nil)
	if s.ContentWidth(1120) != 896 {
		t.Fatal("restoring the main page lost the docked sidebar preference")
	}
}

func TestNarrowSidebarEscapeDefersToContentOverlays(t *testing.T) {
	for _, modal := range []bool{false, true} {
		t.Run(fmt.Sprintf("modal=%v", modal), func(t *testing.T) {
			open := false
			shell, build := Layout(ShellOptions{
				LeftOverlayBelow: 960,
				Content: func(c *ui.Context) {
					anchor := ui.Button(c, "Content overlay").Absolute().Left(300).Top(100)
					if modal {
						components.Dialog(c, &open, components.DialogOptions{}, func(panel ui.Element) {
							ui.Button(c, "Dialog action")
						})
					} else {
						components.Popover(c, anchor, &open, func(panel ui.Element) {
							ui.Button(c, "Popover action")
						})
					}
				},
			})
			view := ui.NewTester(build, 860, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			shell.SetLeftExpanded(true)
			open = true
			view.Frame()
			view.Key(0, ui.KeyEscape)
			if open || !shell.LeftExpanded() {
				t.Fatalf("Escape closed the sidebar instead of the content overlay: overlay=%v sidebar=%v", open, shell.LeftExpanded())
			}
			view.Key(0, ui.KeyEscape)
			if shell.LeftExpanded() {
				t.Fatal("a second Escape did not close the sidebar")
			}
		})
	}
}
