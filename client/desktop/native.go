package desktop

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/knothhe/passpine-neo/client/assets"
)

func (a *application) startNativeIntegrations() {
	if a.tray != nil {
		return
	}
	if runtime.GOOS == "darwin" {
		_ = mygo.App.Dock.SetIcon(assets.AppIconPNG)
	} else if a.window != nil {
		_ = a.window.SetIcon(assets.AppIconPNG)
	}
	icon, template := assets.AppIconPNG, false
	if runtime.GOOS == "darwin" {
		icon, template = assets.MenuBarTemplatePNG, true
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: icon, IconIsTemplate: template, ToolTip: "Passpine Neo"})
	if err != nil {
		a.notice = "菜单栏图标不可用：" + err.Error()
		return
	}
	a.tray = tray
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "搜索密码", Click: func(*mygo.MenuItem, *mygo.Window) { a.quickSearch() }},
		{Label: "设置…", Click: func(*mygo.MenuItem, *mygo.Window) { a.navigate("settings", ""); a.show(); a.invalidate() }},
		{Label: "锁定密码库", Click: func(*mygo.MenuItem, *mygo.Window) { a.lock() }},
		mygo.Separator(),
		{Label: "退出 Passpine Neo", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	tray.SetMenu(menu)
	if runtime.GOOS != "linux" {
		tray.OnClick(a.quickSearch)
		tray.OnRightClick(func() { tray.PopUpMenu() })
	}
	a.registerGlobalShortcut()
}

func (a *application) registerGlobalShortcut() {
	if err := a.changeGlobalShortcut(a.settings.GlobalShortcut); err != nil {
		a.shortcut.errorText = err.Error()
		a.notice = "全局搜索快捷键不可用：" + err.Error()
	}
}

func (a *application) quickSearch() {
	a.page = "entries"
	a.show()
	a.searchRef.Focus()
	a.invalidate()
}
