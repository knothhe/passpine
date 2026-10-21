package desktop

import (
	"github.com/egoist/mygo"
	"github.com/knothhe/passpine/client/internal/linuxdesktop"
)

func platformShortcutRegistry() (shortcutRegistry, error) {
	if linuxdesktop.HyprlandSession() {
		return linuxdesktop.NewShortcuts()
	}
	return mygo.GlobalShortcut, nil
}

func captureNativeShortcut(window *mygo.Window, callback func(string)) (func(), error) {
	if window == nil {
		return func() {}, nil
	}
	return linuxdesktop.CaptureKeyboard(window.NativeHandle(), callback)
}
