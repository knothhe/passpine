package desktop

import (
	"github.com/egoist/mygo"
	"github.com/knothhe/passpine/client/buildinfo"
)

const (
	quickSearchGlobal = ""
	quickSearchTray   = "tray"
)

func (a *application) quickSearchTitle() string {
	if a.quick.mode == quickSearchTray {
		return a.t("%s 快捷面板", buildinfo.Name())
	}
	return a.t("%s 快速搜索", buildinfo.Name())
}

// The menu-bar panel stays attached to the actual status item. Bottom taskbars
// use the space above their icon; narrow and secondary displays stay clamped.
func trayQuickPanelBounds(anchor, area mygo.Rectangle) mygo.Rectangle {
	width, height := min(420, area.Width), min(440, area.Height)
	x := anchor.X + anchor.Width/2 - width/2
	y := anchor.Y + anchor.Height + 6
	if y+height > area.Y+area.Height {
		y = anchor.Y - height - 6
	}
	return mygo.Rectangle{
		X:     max(area.X, min(x, area.X+area.Width-width)),
		Y:     max(area.Y, min(y, area.Y+area.Height-height)),
		Width: width, Height: height,
	}
}

func (a *application) trayQuickPanelAnchor() (mygo.Rectangle, mygo.Rectangle) {
	anchor := mygo.Rectangle{}
	if a.tray != nil {
		anchor = a.tray.Bounds()
	}
	point := mygo.Screen.CursorScreenPoint()
	if anchor.Width > 0 && anchor.Height > 0 {
		point = mygo.Point{X: anchor.X + anchor.Width/2, Y: anchor.Y + anchor.Height/2}
	} else {
		anchor = mygo.Rectangle{X: point.X, Y: point.Y}
	}
	return anchor, mygo.Screen.DisplayNearestPoint(point).WorkArea
}

func (a *application) setQuickSearchMode(mode string) bool {
	if a.quick.mode == mode {
		return false
	}
	a.hideQuickSearch() // Flush a dragged global position before changing mode.
	if a.quick.mode == quickSearchTray {
		a.quick.trayQuery = a.quick.query
	} else {
		a.quick.globalQuery = a.quick.query
	}
	a.quick.mode = mode
	if mode == quickSearchTray {
		a.quick.query = a.quick.trayQuery
	} else {
		a.quick.query = a.quick.globalQuery
	}
	a.clearQuickSearchSecrets()
	return true
}

func (a *application) restoreGlobalQuickSearchBounds() {
	if _, native := a.quick.window.(*mygo.Window); !native {
		return
	}
	area := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint()).WorkArea
	options := quickSearchOptions(area)
	var areas []mygo.Rectangle
	for _, display := range mygo.Screen.Displays() {
		areas = append(areas, display.WorkArea)
	}
	a.restoreQuickSearchPosition(&options, areas, area)
	a.quick.window.SetBounds(mygo.Rectangle{X: options.X, Y: options.Y, Width: options.Width, Height: options.Height})
}
