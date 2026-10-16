package desktop

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
)

type shortcutRegistry interface {
	Register(string, func()) error
	Unregister(string)
}

type shortcutState struct {
	recording  bool
	visible    bool
	errorText  string
	registered string
	ref        ui.Ref
	registry   shortcutRegistry
}

func (a *application) shortcutControls(c *ui.Context) {
	a.shortcut.visible = true
	ui.Column(c).AlignSelf(ui.Center).Shrink(0).Gap(6).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			value, _ := app.NormalizeGlobalShortcut(a.settings.GlobalShortcut)
			components.Text(c, strings.ReplaceAll(value, "+", " + ")).FontSize(12).SingleLine().Shrink(0)
			label := a.t("录制快捷键")
			if a.shortcut.recording {
				label = a.t("请按下组合键…")
			}
			button := components.Button(c, label, components.ButtonOutline).Label(a.t("录制全局搜索快捷键")).Ref(&a.shortcut.ref).Disabled(a.settingsSaving)
			button.HandleInput(a.shortcutInput)
			if button.Clicked() {
				a.setShortcutRecording(!a.shortcut.recording)
				a.shortcut.errorText = ""
				a.shortcut.ref.Focus()
			}
			if a.shortcut.recording {
				if components.Button(c, a.t("取消"), components.ButtonGhost).Clicked() {
					a.setShortcutRecording(false)
					a.shortcut.errorText = ""
				}
			} else if components.Button(c, a.t("恢复默认"), components.ButtonGhost).Disabled(a.settingsSaving || value == app.DefaultGlobalShortcut).Clicked() {
				a.recordShortcut(app.DefaultGlobalShortcut)
			}
		})
		if a.shortcut.errorText != "" {
			components.Text(c, a.t(a.shortcut.errorText)).FontSize(11).TextColor(c.Theme().Danger).Wrap().MaxWidth(340)
		}
	})
}

func (a *application) shortcutInput(event ui.InputEvent) bool {
	if !a.shortcut.recording || event.Kind != ui.InputKeyDown {
		return false
	}
	if event.Key == ui.KeyEscape && event.Mods == 0 || event.Key == ui.KeyTab && event.Mods == 0 {
		a.setShortcutRecording(false)
		a.shortcut.errorText = ""
		a.invalidate()
		return event.Key != ui.KeyTab
	}
	key := event.Key
	if key == ui.KeyUnknown {
		key = event.PhysicalKey
	}
	value := shortcutFromKey(key, event.Mods)
	if value != "" && !event.Repeat {
		a.recordShortcut(value)
	}
	return true
}

func shortcutFromKey(key ui.Key, modifiers ui.Modifiers) string {
	name := ""
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		name = string(rune('A' + key - ui.KeyA))
	case key >= ui.Key0 && key <= ui.Key9:
		name = string(rune('0' + key - ui.Key0))
	case key >= ui.KeyF1 && key <= ui.KeyF12:
		name = fmt.Sprintf("F%d", key-ui.KeyF1+1)
	default:
		name = map[ui.Key]string{
			ui.KeySpace: "Space", ui.KeyTab: "Tab", ui.KeyEnter: "Enter", ui.KeyEscape: "Escape",
			ui.KeyBackspace: "Backspace", ui.KeyDelete: "Delete", ui.KeyInsert: "Insert", ui.KeyHome: "Home", ui.KeyEnd: "End", ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown",
			ui.KeyUp: "Up", ui.KeyDown: "Down", ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyMinus: "-", ui.KeyEqual: "=", ui.KeyComma: ",", ui.KeyPeriod: ".", ui.KeySlash: "/", ui.KeySemicolon: ";", ui.KeyQuote: "'", ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]", ui.KeyBackslash: "\\", ui.KeyBackquote: "`",
		}[key]
	}
	if name == "" {
		return ""
	}
	parts := make([]string, 0, 5)
	for _, modifier := range []struct {
		key  ui.Modifiers
		name string
	}{{ui.Super, "Cmd"}, {ui.Ctrl, "Ctrl"}, {ui.Alt, "Alt"}, {ui.Shift, "Shift"}} {
		if modifiers&modifier.key != 0 {
			parts = append(parts, modifier.name)
		}
	}
	return strings.Join(append(parts, name), "+")
}

func (a *application) recordShortcut(value string) {
	if value == "" || a.settingsSaving {
		return
	}
	shortcut, err := app.NormalizeGlobalShortcut(value)
	if err != nil {
		a.shortcut.errorText = err.Error()
		a.invalidate()
		return
	}
	// Recording produces one complete value. Partial chords never enter settings.
	a.setShortcutRecording(false)
	a.shortcut.errorText = ""
	a.settings.GlobalShortcut = shortcut
	a.settingsObserved = a.settings
	a.settingsDirty = a.settings != a.settingsSaved
	a.saveSettingsAutomatically()
	a.invalidate()
}

func nativeShortcut(value string) (string, error) {
	replacement := "Ctrl"
	if runtime.GOOS == "darwin" {
		replacement = "Cmd"
	}
	value = strings.ReplaceAll(value, "CmdOrCtrl", replacement)
	return app.NormalizeGlobalShortcut(value)
}

// Keep the previous binding until the replacement is accepted by both the OS
// and persistence. On a failed save, only the provisional binding is removed.
func (a *application) prepareGlobalShortcut(value string) (func(bool), error) {
	shortcut, err := nativeShortcut(value)
	if err != nil {
		return nil, err
	}
	if shortcut == a.shortcut.registered {
		return func(bool) {}, nil
	}
	registry := a.shortcut.registry
	if registry == nil && !a.state.Preview {
		registry = mygo.GlobalShortcut
	}
	if registry != nil {
		if err = registry.Register(shortcut, a.globalShortcutActivated); err != nil {
			message := a.t("快捷键不可用，请更换组合")
			if a.shortcut.registered != "" {
				message = a.t("快捷键不可用，请更换组合。原快捷键保持有效")
			}
			return nil, fmt.Errorf("%s: %w", message, err)
		}
	}
	previous := a.shortcut.registered
	return func(commit bool) {
		if commit {
			if registry != nil && previous != "" {
				registry.Unregister(previous)
			}
			a.shortcut.registered = shortcut
			a.shortcut.errorText = ""
		} else if registry != nil {
			registry.Unregister(shortcut)
		}
	}, nil
}

func (a *application) changeGlobalShortcut(value string) error {
	finish, err := a.prepareGlobalShortcut(value)
	if err != nil {
		return err
	}
	finish(true)
	return nil
}

func (a *application) globalShortcutActivated() {
	if a.shortcut.recording {
		a.setShortcutRecording(false)
		a.shortcut.errorText = ""
		a.invalidate()
		return
	}
	a.toggleQuickSearch()
}

func (a *application) setShortcutRecording(recording bool) {
	if a.shortcut.recording == recording {
		return
	}
	a.shortcut.recording = recording
	if a.window != nil && !a.closing {
		a.installMenu()
	}
}
