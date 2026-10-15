package desktop

import (
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine-neo/client/app"
)

type fakeShortcutRegistry struct {
	bindings map[string]func()
	conflict string
}

func (r *fakeShortcutRegistry) Register(value string, callback func()) error {
	if value == r.conflict {
		return errors.New("already in use")
	}
	if r.bindings == nil {
		r.bindings = map[string]func(){}
	}
	r.bindings[value] = callback
	return nil
}

func (r *fakeShortcutRegistry) Unregister(value string) { delete(r.bindings, value) }

func shortcutFixture(t *testing.T) (*application, *fakeShortcutRegistry) {
	t.Helper()
	a := fixture()
	a.settings = app.DefaultSettings()
	a.settings.Language = "zh-CN"
	a.settingsSaved = a.settings
	a.settingsObserved = a.settings
	a.settingsLoaded = true
	registry := &fakeShortcutRegistry{}
	a.shortcut.registry = registry
	if err := a.changeGlobalShortcut(a.settings.GlobalShortcut); err != nil {
		t.Fatal(err)
	}
	return a, registry
}

func TestShortcutConflictNeverPersistsOrRemovesWorkingBinding(t *testing.T) {
	a, registry := shortcutFixture(t)
	registry.conflict = "Ctrl+Alt+P"
	a.service = backendFunc(func(app.Request) app.Response {
		t.Error("conflicting shortcut reached persistence")
		return app.Response{Status: "error"}
	})
	a.recordShortcut(registry.conflict)
	if a.settings.GlobalShortcut != app.DefaultGlobalShortcut || a.settingsSaved != a.settings || a.shortcut.errorText == "" {
		t.Fatal("conflict did not restore the visible saved shortcut")
	}
	if registry.bindings[app.DefaultGlobalShortcut] == nil || len(registry.bindings) != 1 {
		t.Fatal("failed replacement removed the working global shortcut")
	}
}

func TestShortcutSaveFailureKeepsOldBindingAndRemovesProvisionalBinding(t *testing.T) {
	a, registry := shortcutFixture(t)
	callbacks := make(chan func(), 1)
	requests := make(chan app.Request, 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "error", Message: "disk is full"}
	})
	a.recordShortcut("Ctrl+Alt+P")
	if registry.bindings[app.DefaultGlobalShortcut] == nil || registry.bindings["Ctrl+Alt+P"] == nil {
		t.Fatal("old binding was removed before persistence completed")
	}
	awaitMain(t, callbacks)
	if request := <-requests; request.Settings.GlobalShortcut != "Ctrl+Alt+P" {
		t.Fatal("wrong shortcut was saved")
	}
	if a.settings.GlobalShortcut != app.DefaultGlobalShortcut || a.settingsSaved != a.settings || a.shortcut.errorText == "" || a.settingsSaving {
		t.Fatal("save failure left an unsaved shortcut displayed as active")
	}
	if registry.bindings[app.DefaultGlobalShortcut] == nil || len(registry.bindings) != 1 {
		t.Fatal("failed save did not remove only the provisional shortcut")
	}
}

func TestRecordingShortcutSavesCompleteCombinationAndCanCancel(t *testing.T) {
	a, registry := shortcutFixture(t)
	a.page = "settings"
	callbacks := make(chan func(), 1)
	requests := make(chan app.Request, 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: app.Data{Settings: *request.Settings}}
	})
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("录制全局搜索快捷键"); err != nil {
		t.Fatal(err)
	}
	view.Key(0, ui.KeyP)
	if !a.shortcut.recording || a.shortcut.errorText == "" || a.settings.GlobalShortcut != app.DefaultGlobalShortcut {
		t.Fatal("recording accepted a typing key without a modifier")
	}
	view.Key(0, ui.KeyEscape)
	if a.shortcut.recording || a.shortcut.errorText != "" {
		t.Fatal("Escape did not cancel recording")
	}
	if err := view.Click("录制全局搜索快捷键"); err != nil {
		t.Fatal(err)
	}
	view.Key(ui.Ctrl|ui.Alt, ui.KeyP)
	awaitMain(t, callbacks)
	if request := <-requests; request.Settings.GlobalShortcut != "Ctrl+Alt+P" {
		t.Fatal("recorder did not save the complete chord")
	}
	view.Frame()
	if a.shortcut.recording || a.settingsSaved.GlobalShortcut != "Ctrl+Alt+P" || !view.HasText("Ctrl + Alt + P") {
		t.Fatal("recorded shortcut was not displayed as saved")
	}
	if registry.bindings["Ctrl+Alt+P"] == nil || len(registry.bindings) != 1 {
		t.Fatal("successful replacement did not retire the old binding")
	}
	if err := view.Click("恢复默认"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	if request := <-requests; request.Settings.GlobalShortcut != app.DefaultGlobalShortcut || a.settingsSaved.GlobalShortcut != app.DefaultGlobalShortcut {
		t.Fatal("restoring the default did not persist it")
	}
	if registry.bindings[app.DefaultGlobalShortcut] == nil || len(registry.bindings) != 1 {
		t.Fatal("restoring the default did not replace the custom binding")
	}
}

func TestShortcutRecordingCapturesApplicationNavigationChord(t *testing.T) {
	a, _ := shortcutFixture(t)
	a.page = "settings"
	callbacks := make(chan func(), 1)
	requests := make(chan app.Request, 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: app.Data{Settings: *request.Settings}}
	})
	view := ui.NewTester(testView(a), 1120, 760)
	if err := view.Click("录制全局搜索快捷键"); err != nil {
		t.Fatal(err)
	}
	view.Key(ui.Cmd, ui.KeyN)
	awaitMain(t, callbacks)
	want := "Ctrl+N"
	if ui.Cmd == ui.Super {
		want = "Cmd+N"
	}
	if request := <-requests; request.Settings.GlobalShortcut != want || a.editor.open || a.page != "settings" {
		t.Fatal("application shortcut navigated instead of being recorded")
	}
}
