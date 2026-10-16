package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func clickBooleanSettingSegment(t *testing.T, view *ui.Tester, label string, enabled bool) {
	t.Helper()
	bounds, ok := view.Find(label)
	if !ok {
		t.Fatalf("boolean setting group is missing: %s", label)
	}
	fraction := float32(.75)
	if enabled {
		fraction = .25
	}
	view.ClickAt(bounds.X+bounds.W*fraction, bounds.Y+bounds.H/2)
}

func TestDockAndTraySegmentsPreserveAndSaveEveryChoice(t *testing.T) {
	for _, test := range []struct {
		title  string
		values []string
		labels []string
		value  func(*app.Settings) *string
	}{
		{"Dock 图标", []string{app.DockIconShow, app.DockIconHide, app.DockIconFollowWindow}, []string{"展示", "隐藏", "跟随窗口"}, func(s *app.Settings) *string { return &s.DockIcon }},
		{"点击托盘图标", []string{app.TrayClickQuickPanel, app.TrayClickMainWindow}, []string{"打开快捷面板", "打开主窗口"}, func(s *app.Settings) *string { return &s.TrayClickAction }},
	} {
		t.Run(test.title, func(t *testing.T) {
			a := fixture()
			a.page, a.settingsState.Query = "settings", test.title
			*test.value(&a.state.Settings) = test.values[len(test.values)-1]
			callbacks, requests := make(chan func(), 1), make(chan app.Request, 1)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.service = backendFunc(func(request app.Request) app.Response {
				requests <- request
				return app.Response{Status: "ok", Data: app.Data{Settings: *request.Settings}}
			})
			view := ui.NewTester(testView(a), 860, 620)
			if *test.value(&a.settings) != test.values[len(test.values)-1] {
				t.Fatal("rendering replaced an existing enum preference")
			}
			for i, label := range test.labels {
				if err := view.Click(label); err != nil {
					t.Fatal(err)
				}
				awaitMain(t, callbacks)
				if request := <-requests; request.Action != "settings" || *test.value(request.Settings) != test.values[i] || *test.value(&a.settingsSaved) != test.values[i] {
					t.Fatal("a visible segment did not save its existing enum value")
				}
				view.Frame()
			}
		})
	}
}
