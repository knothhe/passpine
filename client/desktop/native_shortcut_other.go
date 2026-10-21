//go:build !linux

package desktop

import "github.com/egoist/mygo"

func platformShortcutRegistry() (shortcutRegistry, error) { return mygo.GlobalShortcut, nil }

func captureNativeShortcut(*mygo.Window, func(string)) (func(), error) { return func() {}, nil }
