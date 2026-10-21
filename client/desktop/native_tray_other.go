//go:build !linux

package desktop

import "github.com/egoist/mygo"

func newNativeTray(options mygo.TrayOptions) (nativeTray, error) { return mygo.NewTray(options) }
