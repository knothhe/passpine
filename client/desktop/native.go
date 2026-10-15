package desktop

import (
	"bytes"
	"image/png"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func (a *application) startNativeIntegrations() {
	if a.tray != nil {
		return
	}
	// Reuse the Lucide asset rather than maintaining an independently drawn icon.
	img := ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		components.Icon(c, lucide.KeyRound).Size(22, 22).TextColor(ui.Hex("#000000"))
	}, 22, 22, 2)
	var icon bytes.Buffer
	if err := png.Encode(&icon, img); err != nil {
		a.errorText = err.Error()
		return
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: icon.Bytes(), IconIsTemplate: true, ToolTip: "Passpine Neo"})
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
	if !a.state.Preview {
		if err := mygo.GlobalShortcut.Register("Ctrl+Alt+Space", a.quickSearch); err != nil {
			a.notice = "全局搜索快捷键不可用：" + err.Error()
		}
	}
}

func (a *application) quickSearch() {
	a.page = "entries"
	a.show()
	a.searchRef.Focus()
	a.invalidate()
}
