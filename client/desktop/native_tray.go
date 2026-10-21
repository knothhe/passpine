package desktop

import "github.com/egoist/mygo"

type nativeTray interface {
	SetIcon([]byte, bool) error
	SetMenu(*mygo.Menu)
	Bounds() mygo.Rectangle
	PopUpMenu()
	Destroy()
	OnClick(func()) func()
	OnRightClick(func()) func()
}
