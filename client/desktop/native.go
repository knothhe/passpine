package desktop

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/knothhe/passpine/client/assets"
	"github.com/knothhe/passpine/client/internal/traybutton"
)

type trayButtonController interface {
	SetHighlighted(bool)
	Close()
}

func (a *application) updateTrayHighlight() {
	if a.lifecycle.trayButton != nil {
		a.lifecycle.trayButton.SetHighlighted(a.quick.visible && a.quick.mode == quickSearchTray)
	}
}

func (a *application) startNativeIntegrations() {
	a.lifecycle.nativeReady = true
	if runtime.GOOS == "darwin" {
		// MyGo starts with the regular activation policy. Reapplying Show
		// would activate the app even though its Dock icon is already visible.
		a.lifecycle.dockVisible, a.lifecycle.dockVisibilityApplied = true, true
		a.lifecycle.setDockVisible = func(visible bool) {
			if visible {
				mygo.App.Dock.Show()
			} else {
				mygo.App.Dock.Hide()
			}
		}
	}
	if !a.state.Preview {
		a.lifecycle.setOpenAtLogin = mygo.App.SetOpenAtLogin
		// System Settings and Task Manager can change this outside the app.
		// Read the real registration; startup must not silently re-enable it.
		if enabled := mygo.App.OpenAtLogin(); enabled != a.settings.LaunchAtLogin {
			a.settings.LaunchAtLogin = enabled
			a.settingsSaved.LaunchAtLogin = enabled
			a.settingsObserved = a.settings
			a.state.Settings.LaunchAtLogin = enabled
			a.settingsDirty = true
			a.saveSettingsAutomatically()
		}
	}
	if a.tray != nil {
		return
	}
	a.updateNativeIcons()
	icon, template := assets.IconForStyle(a.themeSettings().ThemeStyle), false
	if runtime.GOOS == "darwin" {
		icon, template = assets.MenuBarTemplatePNG, true
	}
	tray, err := newNativeTray(mygo.TrayOptions{Icon: icon, IconIsTemplate: template, ToolTip: "Passpine"})
	if err != nil {
		a.warn(a.t("菜单栏图标不可用：%s", err))
		a.registerGlobalShortcut()
		return
	}
	a.tray = tray
	a.installTrayMenu()
	tray.OnClick(a.trayClicked)
	if runtime.GOOS != "linux" {
		tray.OnRightClick(a.showTrayMenu)
	}
	// MyGo has no status-button highlight API. Keep its click handler as a
	// fallback if the native button cannot be found during installation.
	if button, err := traybutton.Install("Passpine", a.trayClicked); err == nil {
		a.lifecycle.trayButton = button
		a.updateTrayHighlight()
	}
	a.registerGlobalShortcut()
}

func (a *application) updateNativeIcons() {
	if a.closing || !a.lifecycle.nativeReady {
		return
	}
	if runtime.GOOS == "darwin" {
		_ = mygo.App.Dock.SetIcon(assets.DockIconForStyle(a.themeSettings().ThemeStyle))
		return // The menu bar uses the OS-colored template in both styles.
	}
	icon := assets.IconForStyle(a.themeSettings().ThemeStyle)
	if a.window != nil {
		_ = a.window.SetIcon(icon)
	}
	if window, ok := a.quick.window.(*mygo.Window); ok {
		_ = window.SetIcon(icon)
	}
	if a.tray != nil {
		_ = a.tray.SetIcon(icon, false)
	}
}

func (a *application) registerGlobalShortcut() {
	if err := a.changeGlobalShortcut(a.settings.GlobalShortcut); err != nil {
		a.shortcut.errorText = err.Error()
		a.warn(a.t("全局搜索快捷键不可用：%s", err))
	}
}

func (a *application) installTrayMenu() {
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: a.t("搜索密码"), Click: func(*mygo.MenuItem, *mygo.Window) { a.trayQuickPanel() }},
		{Label: a.t("设置…"), Click: func(*mygo.MenuItem, *mygo.Window) { a.show(); a.navigate("settings", ""); a.invalidate() }},
		{Label: a.t("锁定密码库"), Click: func(*mygo.MenuItem, *mygo.Window) { a.lock() }},
		mygo.Separator(),
		{Label: a.t("退出 Passpine"), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	a.lifecycle.trayMenu = menu
	if runtime.GOOS == "linux" {
		a.tray.SetMenu(menu)
	} else {
		// Attaching an NSStatusItem menu swallows its left-click action.
		// Keep the menu for explicit right-click tracking instead.
		a.tray.SetMenu(nil)
	}
}

func (a *application) showTrayMenu() {
	if a.tray == nil || a.lifecycle.trayMenu == nil || a.closing {
		return
	}
	a.tray.SetMenu(a.lifecycle.trayMenu)
	defer func() {
		a.tray.SetMenu(nil)
		// Menu tracking clears AppKit's highlight after a menu action may
		// already have opened the quick panel.
		a.updateTrayHighlight()
	}()
	a.tray.PopUpMenu()
}
