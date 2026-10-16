package desktop

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestTypographyCombinesSavedScaleAndSystemTextScaleOnce(t *testing.T) {
	a := fixture()
	a.state.Settings.FontScale = 120
	var themeSize, explicitSize float32
	view := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.clientTheme(c))
		themeSize, explicitSize = c.Theme().FontSize, scaledFontSize(c, 13)
	}, 200, 100)
	base := clientPalette(a.state.Settings.ThemeStyle, false).FontSize
	if math.Abs(float64(themeSize-base*1.2)) > .001 {
		t.Fatalf("saved typography scale = %g, want %g", themeSize, base*1.2)
	}
	before := explicitSize
	view.SetPreferences(ui.Preferences{TextScale: 1.5})
	if math.Abs(float64(themeSize-base*1.2*1.5)) > .001 || math.Abs(float64(explicitSize-before*1.5)) > .001 {
		t.Fatal("system text scale did not combine once with the saved scale and explicit fonts")
	}
}

func TestTextSizeSegmentedControlPersistsAndUpdatesBothSurfaces(t *testing.T) {
	a, _ := quickSearchFixture()
	a.page = "settings"
	callbacks, requests := make(chan func(), 2), make(chan app.Request, 2)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: app.Data{Settings: *request.Settings}}
	})
	view := ui.NewTester(testView(a), 1120, 760)
	for _, scale := range []int{80, 120} {
		if err := view.Click(fmt.Sprintf("%d%%", scale)); err != nil {
			t.Fatal(err)
		}
		awaitMain(t, callbacks)
		if request := <-requests; request.Action != "settings" || request.Settings.FontScale != scale || a.settings.FontScale != scale {
			t.Fatal("segmented text size choice did not save")
		}
		var mainSize, panelSize float32
		ui.NewTester(func(c *ui.Context) { mainSize = a.clientTheme(c).FontSize }, 200, 100)
		ui.NewTester(func(c *ui.Context) {
			a.quickSearchView(c)
			panelSize = c.Theme().FontSize
		}, 420, 440)
		if mainSize != panelSize || math.Abs(float64(mainSize-clientPalette(a.settings.ThemeStyle, false).FontSize*float32(scale)/100)) > .001 {
			t.Fatal("text size did not reach both manager and panel themes")
		}
	}
}

func TestTextSizeExtremesKeepMainAndPanelControlsInsideTheirWindows(t *testing.T) {
	for _, scale := range []int{80, 120} {
		t.Run(fmt.Sprintf("%d", scale), func(t *testing.T) {
			a := fixture()
			a.state.Settings.FontScale = scale
			main := ui.NewTester(testView(a), 860, 620)
			assertTypographyBounds(t, main, 860, 620, "编辑条目", "显示或隐藏密码", "复制密码")
			for _, mode := range []string{quickSearchGlobal, quickSearchTray} {
				panel, _ := quickSearchFixture()
				panel.state.Settings.FontScale = scale
				panel.quick.mode = mode
				width, height := 660, 460
				if mode == quickSearchTray {
					width, height = 420, 440
				}
				view := ui.NewTester(panel.quickSearchView, width, height)
				assertTypographyBounds(t, view, width, height, "快速搜索条目", "GitHub", "复制密码", "关闭快速搜索")
				panel.state.Locked, panel.state.Settings.BrowseWhileLocked = true, false
				view.Frame()
				assertTypographyBounds(t, view, width, height, "解锁以快速搜索", "快速搜索私钥口令", "解锁")
			}
		})
	}
}

func TestTextSizeExtremesScaleExplicitMainAndPanelText(t *testing.T) {
	for _, surface := range []string{"main", "global", "tray"} {
		t.Run(surface, func(t *testing.T) {
			var widths []float32
			for _, scale := range []int{80, 120} {
				a := fixture()
				a.state.Settings.FontScale = scale
				build, width, height := testView(a), 860, 620
				if surface != "main" {
					build, width, height = a.quickSearchView, 660, 460
					if surface == "tray" {
						a.quick.mode, width, height = quickSearchTray, 420, 440
					}
				}
				view := ui.NewTester(build, width, height)
				text, ok := view.Find("复制密码")
				if !ok {
					t.Fatal("font measurement text is missing")
				}
				widths = append(widths, text.W)
			}
			if widths[1] <= widths[0]+3 {
				t.Fatalf("explicit text did not grow with saved text size: %v", widths)
			}
		})
	}
}

func assertTypographyBounds(t *testing.T, view *ui.Tester, width, height int, labels ...string) {
	t.Helper()
	for _, label := range labels {
		bounds, ok := view.Find(label)
		if !ok || bounds.W <= 0 || bounds.H <= 0 || bounds.X < 0 || bounds.Y < 0 || bounds.X+bounds.W > float32(width)+.5 || bounds.Y+bounds.H > float32(height)+.5 {
			t.Errorf("text size clipped %q outside %dx%d: %+v", label, width, height, bounds)
		}
	}
}
