package desktop

import (
	"github.com/egoist/mygo"
	"github.com/knothhe/passpine/client/internal/linuxdesktop"
)

func newNativeTray(options mygo.TrayOptions) (nativeTray, error) {
	return linuxdesktop.NewTray(options)
}
