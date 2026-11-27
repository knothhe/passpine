package desktop

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/i18n"
	"github.com/knothhe/passpine/client/nativehost"
)

func TestManualBrowserPickerSelectsMacApplicationBundle(t *testing.T) {
	a := fixture()
	a.state.Preview = false
	a.preferences.browser.open = true
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	chosen := "/Volumes/External Apps/Browser With Spaces.app"
	var options mygo.OpenDialogOptions
	a.preferences.openDialog = func(o mygo.OpenDialogOptions) ([]string, error) {
		options = o
		return []string{chosen}, nil
	}
	for _, wantDirectory := range []string{"/Applications", filepath.Dir(chosen)} {
		a.chooseBrowserProgram()
		awaitMain(t, callbacks)
		if len(options.Filters) != 0 || options.Directory || options.TreatPackagesAsDirectories {
			t.Fatalf("picker must allow whole application bundles: %+v", options)
		}
		if options.DefaultPath != wantDirectory || a.preferences.browser.value.Path != chosen {
			t.Fatalf("picker opened at %q, selected %q", options.DefaultPath, a.preferences.browser.value.Path)
		}
	}
}

func TestMacBrowserDirectoryIsOptionalAndCustomSelectionSurvivesRetry(t *testing.T) {
	a := fixture()
	a.state.Preview = false
	a.settings.Language = "en"
	draft := &a.preferences.browser
	draft.open = true
	draft.value = nativehost.BrowserConfiguration{Kind: "chromium", Path: "/Applications/Example.app"}
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	selected := "/Volumes/User Data/NativeMessagingHosts"
	var options mygo.OpenDialogOptions
	a.preferences.openDialog = func(o mygo.OpenDialogOptions) ([]string, error) {
		options = o
		return []string{selected}, nil
	}
	state := a.state
	requests := make(chan app.Request, 1)
	a.service = backendFunc(func(r app.Request) app.Response {
		requests <- r
		if r.BrowserConfiguration.Directory == "" {
			return app.Response{Status: "error", Data: state, Message: i18n.BrowserDestinationUnknown}
		}
		return app.Response{Status: "ok", Data: state}
	})
	view := ui.NewTester(a.manualBrowserView, 760, 640)
	click := func(key string) {
		t.Helper()
		if err := view.Click(a.t(key)); err != nil {
			t.Fatal(err)
		}
	}
	if view.HasText(a.t("Native Messaging 目录")) || view.HasText(a.t("选择文件夹…")) {
		t.Fatal("automatic mode asked for a connection directory")
	}
	click("自定义连接目录")
	click("选择文件夹…")
	awaitMain(t, callbacks)
	if !options.Directory || options.Title != a.t("选择浏览器连接目录") || draft.value.Directory != selected {
		t.Fatalf("directory picker options = %+v, result = %q", options, draft.value.Directory)
	}
	click("自定义连接目录")
	click("保存浏览器配置")
	awaitMain(t, callbacks)
	if config := (<-requests).BrowserConfiguration; config.Directory != "" || !draft.open || a.errorText != a.t(i18n.BrowserDestinationUnknown) {
		t.Fatalf("automatic directory request = %+v, draft open = %v, error = %q", config, draft.open, a.errorText)
	}
	a.settings.Language = "zh-CN"
	view.Frame()
	click("自定义连接目录")
	click("保存浏览器配置")
	awaitMain(t, callbacks)
	if config := (<-requests).BrowserConfiguration; config.Directory != selected || config.Path != "/Applications/Example.app" || draft.open {
		t.Fatalf("custom directory request = %+v, draft open = %v", config, draft.open)
	}
}

func TestMacBrowserSavedDirectorySurvivesEditing(t *testing.T) {
	a := fixture()
	a.state.Preview = false
	value := nativehost.BrowserConfiguration{Kind: "chromium", Path: "/Applications/Example.app", ID: "custom-example", Directory: "/other/NativeMessagingHosts"}
	a.preferences.browser = browserConfigurationDraft{open: true, value: value}
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	state := a.state
	requests := make(chan app.Request, 1)
	a.service = backendFunc(func(r app.Request) app.Response {
		requests <- r
		return app.Response{Status: "ok", Data: state}
	})
	view := ui.NewTester(a.manualBrowserView, 760, 640)
	if view.HasText(a.t("自定义连接目录")) || view.HasText(a.t("选择文件夹…")) {
		t.Fatal("existing registered destination offered a directory override")
	}
	if err := view.Click(a.t("保存浏览器配置")); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	if config := (<-requests).BrowserConfiguration; *config != value {
		t.Fatalf("editing changed the saved registration destination: %+v", config)
	}
}
