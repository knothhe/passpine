package clientshell

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/theme"
)

func TestLeadingHeaderNavigationStaysBeforeSidebarToggle(t *testing.T) {
	for _, width := range []int{860, 1120} {
		for _, nativeInset := range []float32{0, 80} {
			t.Run(fmt.Sprintf("width=%d/nativeInset=%g", width, nativeInset), func(t *testing.T) {
				var backClicks, forwardClicks int
				var title ui.Rect
				s := newShell(ShellOptions{
					LeftWidth: 310, LeadingHeaderWidth: 64, Theme: theme.Onevra,
					LeadingHeader: func(c *ui.Context) {
						if IconButton(c, lucide.ChevronLeft(), "Back").Clicked() {
							backClicks++
						}
						if IconButton(c, lucide.ChevronRight(), "Forward").Clicked() {
							forwardClicks++
						}
					},
					ContentTitle: func(c *ui.Context) {
						ui.Text(c, "Main title").SingleLine().DrawOver(func(_ *ui.Painter, rect ui.Rect) { title = rect })
					},
					Left: func(c *ui.Context) {
						ui.Column(c).Fill().Padding(HeaderHeight(c), 0, 0, 0).Children(func() { ui.Text(c, "Sidebar content") })
					},
					Content: func(c *ui.Context) {
						ui.Column(c).Fill().Padding(HeaderHeight(c), 0, 0, 0).Children(func() { ui.Text(c, "Main content") })
					},
				})
				view := ui.NewTester(s.view, width, 760)
				view.SetTitleBar(ui.TitleBar{Height: 48, Left: nativeInset})
				view.SetPreferences(ui.Preferences{ReduceMotion: true})
				for iteration, expanded := range []bool{true, false, true} {
					if s.LeftExpanded() != expanded {
						t.Fatalf("iteration %d: sidebar expansion is %v, want %v", iteration, s.LeftExpanded(), expanded)
					}
					back, hasBack := view.Find("Back")
					forward, hasForward := view.Find("Forward")
					toggle, hasToggle := view.Find("Toggle sidebar")
					if !hasBack || !hasForward || !hasToggle {
						t.Fatalf("expanded=%v: header navigation disappeared: back=%v forward=%v toggle=%v", expanded, hasBack, hasForward, hasToggle)
					}
					for label, rect := range map[string]ui.Rect{"Back": back, "Forward": forward, "Toggle sidebar": toggle} {
						if rect.W <= 0 || rect.H <= 0 || rect.X < nativeInset || rect.Y < 0 || rect.Y+rect.H > 48 || rect.X+rect.W > float32(width) {
							t.Fatalf("expanded=%v: %s is outside the visible header: %+v", expanded, label, rect)
						}
					}
					if back.X+back.W > forward.X || forward.X+forward.W > toggle.X {
						t.Fatalf("expanded=%v: navigation must precede the sidebar toggle without overlap: back=%+v forward=%+v toggle=%+v", expanded, back, forward, toggle)
					}
					if !view.HasText("Main title") || title.W <= 0 || title.X < toggle.X+toggle.W || title.X+title.W > float32(width)-8 || title.Y < 0 || title.Y+title.H > 48 {
						t.Fatalf("expanded=%v: main title is hidden or overlaps header controls: title=%+v toggle=%+v", expanded, title, toggle)
					}
					for _, label := range []string{"Back", "Forward"} {
						if err := view.Click(label); err != nil {
							t.Fatal(err)
						}
					}
					if backClicks != iteration+1 || forwardClicks != iteration+1 || s.LeftExpanded() != expanded || !view.HasText("Main title") {
						t.Fatalf("expanded=%v: navigation did not receive its own clicks: back=%d forward=%d", expanded, backClicks, forwardClicks)
					}
					if iteration < 2 {
						if err := view.Click("Toggle sidebar"); err != nil {
							t.Fatal(err)
						}
						view.Frame()
					}
				}
			})
		}
	}
}
