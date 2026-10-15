package desktop

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine-neo/client/app"
)

func TestSettingsSaveOnChangeWithoutSaveButton(t *testing.T) {
	a := fixture()
	a.page = "settings"
	callbacks := make(chan func(), 1)
	requests := make(chan app.Request, 1)
	a.runOnMain = func(f func()) { callbacks <- f }
	a.service = backendFunc(func(req app.Request) app.Response {
		requests <- req
		return app.Response{Status: "ok", Data: app.Data{Settings: *req.Settings}}
	})
	view := ui.NewTester(testView(a), 1120, 760)
	if view.HasText("保存设置") {
		t.Fatal("settings still require a manual save")
	}
	if err := view.Click("关闭后继续运行"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	request := <-requests
	if request.Action != "settings" || request.Settings.CloseToTray {
		t.Fatal("changing a setting did not save it")
	}
	view.Frame()
	if !view.HasText("已保存") {
		t.Fatal("saved setting did not show simple feedback")
	}
}
func TestSettingsSaveCoalescesWithoutLosingNewChanges(t *testing.T) {
	a := fixture()
	a.settings = app.DefaultSettings()
	a.settingsSaved = a.settings
	a.settingsLoaded = true
	callbacks := make(chan func(), 2)
	requests := make(chan app.Request, 2)
	release := make(chan struct{})
	a.runOnMain = func(f func()) { callbacks <- f }
	a.service = backendFunc(func(req app.Request) app.Response {
		requests <- req
		if !req.Settings.AutoSync {
			<-release
		}
		return app.Response{Status: "ok", Data: app.Data{Settings: *req.Settings}}
	})
	a.settings.Theme = "dark"
	a.settingsDirty = true
	a.saveSettingsAutomatically()
	select {
	case <-requests:
	case <-time.After(3 * time.Second):
		t.Fatal("save never started")
	}
	a.settings.AutoSync = true
	a.settingsDirty = true
	a.page = "entries"
	close(release)
	awaitMain(t, callbacks)
	awaitMain(t, callbacks)
	request := <-requests
	if request.Settings.Theme != "dark" || !request.Settings.AutoSync || a.settingsSaving || a.settingsDirty || a.settings != a.settingsSaved {
		t.Fatal("a previous save overwrote a more recent change")
	}
}
func TestLockedListShowsNamesWithoutGeneratorOrSecrets(t *testing.T) {
	a := fixture()
	a.clearSecrets()
	a.state.Locked = true
	view := ui.NewTester(testView(a), 1120, 760)
	if !view.HasText("GitHub") || !view.HasText("Figma") {
		t.Fatal("locked sidebar hides entry names")
	}
	for _, label := range []string{"我的收藏", "密码生成器", "fake-Secret-42!"} {
		if view.HasText(label) {
			t.Fatalf("sidebar contains an unwanted tool or secret: %s", label)
		}
	}
}
