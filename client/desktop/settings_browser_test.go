package desktop

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/nativehost"
)

func TestManualBrowserFormFitsNarrowLayouts(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, width := range []int{360, 680} {
			t.Run(fmt.Sprintf("%s/%d", locale, width), func(t *testing.T) {
				a := fixture()
				a.state.Preview = false
				a.settings.Language = locale
				a.preferences.browser.open = true
				a.preferences.browser.value.Kind = "chromium"
				view := ui.NewTester(a.manualBrowserView, width, 800)
				assertTypographyBounds(t, view, width, 800, a.t("浏览器名称"), a.t("浏览器程序路径"), a.t("选择程序…"), a.t("浏览器兼容类型"), a.t("保存浏览器配置"), a.t("取消"))
				managerResizeScreenshot(t, view, fmt.Sprintf("manual-browser-%s-%d", locale, width))
			})
		}
	}
}

func TestManualBrowserDraftSurvivesLanguageChangeAndSendsExactUserPaths(t *testing.T) {
	a := fixture()
	a.state.Preview = false
	a.page = "settings"
	a.settingsState.Category = "browser"
	requests, callbacks := make(chan app.Request, 1), make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	state := a.state
	a.service = backendFunc(func(r app.Request) app.Response { requests <- r; return app.Response{Status: "ok", Data: state} })
	view := ui.NewTester(a.manualBrowserView, 760, 640)
	if err := view.Click("添加浏览器…"); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("浏览器名称"); err != nil {
		t.Fatal(err)
	}
	view.Type("未翻译的浏览器")
	if err := view.Click("浏览器程序路径"); err != nil {
		t.Fatal(err)
	}
	view.Type("/Applications/Browser With Spaces.app")
	before := a.preferences.browser.value
	a.settings.Language = "en"
	view.Frame()
	if a.preferences.browser.value != before || !view.HasText("Configure browsers manually") {
		t.Fatal("language change lost input or failed to translate controls")
	}
	if err := view.Click("Save browser configuration"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	r := <-requests
	if r.Action != "browser-save" || r.BrowserConfiguration == nil || *r.BrowserConfiguration != before {
		t.Fatalf("manual browser request changed user data: %+v", r)
	}
	if a.preferences.browser.open {
		t.Fatal("successful save did not close editor")
	}
}

func TestManualBrowserPickerDiscardsLateResultAndPreviewDoesNotOpen(t *testing.T) {
	a := fixture()
	a.page = "settings"
	a.preferences.browser.open = true
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	called := make(chan mygo.OpenDialogOptions, 1)
	a.preferences.openDialog = func(options mygo.OpenDialogOptions) ([]string, error) {
		called <- options
		return []string{"/chosen/browser"}, nil
	}
	a.state.Preview = true
	a.chooseBrowserProgram()
	if a.preferences.browser.picking {
		t.Fatal("preview opened browser picker")
	}
	a.state.Preview = false
	a.chooseBrowserProgram()
	options := <-called
	if options.Title != a.t("选择浏览器程序") {
		t.Fatal("picker title is not localized")
	}
	a.preferences.epoch++
	awaitMain(t, callbacks)
	if a.preferences.browser.value.Path != "" || a.preferences.browser.picking {
		t.Fatal("late picker result changed the draft")
	}
}

func TestManualBrowserMissingProgramCanBeEditedOrRemoved(t *testing.T) {
	a := fixture()
	a.state.Preview = false
	a.state.BrowserHelperAvailable = true
	configuration := nativehost.BrowserConfiguration{ID: "custom-example", Name: "浏览器用户名称", Path: "/missing/browser", Kind: "chromium"}
	a.state.BrowserRegistrations = []nativehost.BrowserRegistration{{ID: configuration.ID, Name: configuration.Name, Status: "browser_missing", Configuration: &configuration}}
	view := ui.NewTester(a.browserRegistrationView, 760, 200)
	if !view.HasText("浏览器程序不存在") {
		t.Fatal("missing program is not explained")
	}
	if err := view.Click("编辑 浏览器用户名称 的浏览器配置"); err != nil {
		t.Fatal(err)
	}
	if !a.preferences.browser.open || a.preferences.browser.value != configuration {
		t.Fatal("edit did not retain configuration")
	}
}
