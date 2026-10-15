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
		a.warn(a.t("菜单栏图标不可用：%s", err))
		return
	}
	a.tray = tray
	a.installTrayMenu()
	if runtime.GOOS != "linux" {
		tray.OnClick(a.quickSearch)
		tray.OnRightClick(func() { tray.PopUpMenu() })
	}
	a.registerGlobalShortcut()
}

func (a *application) registerGlobalShortcut() {
	if err := a.changeGlobalShortcut(a.settings.GlobalShortcut); err != nil {
		a.shortcut.errorText = err.Error()
		a.warn(a.t("全局搜索快捷键不可用：%s", err))
	}
}

func (a *application) installTrayMenu() {
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: a.t("搜索密码"), Click: func(*mygo.MenuItem, *mygo.Window) { a.quickSearch() }},
		{Label: a.t("设置…"), Click: func(*mygo.MenuItem, *mygo.Window) { a.show(); a.navigate("settings", ""); a.invalidate() }},
		{Label: a.t("锁定密码库"), Click: func(*mygo.MenuItem, *mygo.Window) { a.lock() }},
		mygo.Separator(),
		{Label: a.t("退出 Passpine Neo"), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	a.tray.SetMenu(menu)
}
