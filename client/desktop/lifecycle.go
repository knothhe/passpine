package desktop

import (
	"runtime/debug"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	sidebar "github.com/knothhe/passpine/client/internal/clientshell"
	"github.com/oneo-me/onevra/source/components"
)

const lightweightIdleDelay = time.Minute

// The native service, tray, shortcuts and clipboard lease outlive UI windows.
// All lifecycle state and injected OS callbacks belong to the main thread.
type lifecycleState struct {
	nativeReady, mainVisible, suspended bool
	resumeTarget                        string
	hiddenAt                            time.Time
	timer                               *time.Timer
	timerSerial                         uint64
	leftWidth                           float32
	leftCollapsed                       bool
	dockVisible, dockVisibilityApplied  bool
	setDockVisible                      func(bool)
	setOpenAtLogin                      func(bool) error
	configureDone                       func(app.Data)
	trayMenu                            *mygo.Menu
}

func (a *application) newMainWindow() {
	width := defaultSidebarWidth
	if a.lifecycle.leftWidth > 0 {
		width = int(a.lifecycle.leftWidth)
	}
	a.shell = sidebar.NewShell(sidebar.ShellOptions{
		Hidden: true, Title: "Passpine", Width: 1120, Height: 760, MinWidth: 860, MinHeight: 620,
		StateKey: "passpine-main", LeftWidth: width, LeftMinWidth: 280, LeftMaxWidth: 460,
		LeftCollapsed: a.lifecycle.leftCollapsed,
		Theme:         a.clientTheme, Left: a.sidebarView, Content: a.contentView,
		ContentTitle: a.header, LeftToolbar: a.sidebarToolbar,
		LeadingHeader: a.historyControls, LeadingHeaderWidth: 64,
		LeftToggleLabel: "切换侧栏", LeftResizeLabel: "调整侧栏宽度", CloseToastLabel: "关闭提示",
	})
	window := a.shell.Window()
	a.window = window
	window.SetVibrancy(mygo.VibrancyNone)
	window.OnFocus(func() {
		if a.state.Configured {
			a.queueMetadataRefresh(false)
		}
	})
	window.OnShow(func() { a.mainWindowVisibilityChanged(true) })
	window.OnHide(func() { a.mainWindowVisibilityChanged(false) })
	window.OnClose(a.handleMainWindowClose)
	window.OnClosed(func() { a.mainWindowClosed(window) })
}

func (a *application) mainWindowClosed(window *mygo.Window) {
	if a.window == window {
		a.window, a.shell = nil, nil
	}
	a.lifecycle.mainVisible = false
	if !a.lifecycle.suspended {
		a.closeQuickSearch()
	}
	a.updateDockVisibility()
}

func dockIconVisible(policy string, mainVisible bool) bool {
	switch policy {
	case app.DockIconHide:
		return false
	case app.DockIconFollowWindow:
		return mainVisible
	default:
		return true
	}
}

func (a *application) updateDockVisibility() {
	if a.lifecycle.setDockVisible == nil || a.closing {
		return
	}
	visible := dockIconVisible(a.state.Settings.DockIcon, a.lifecycle.mainVisible)
	if a.lifecycle.dockVisibilityApplied && a.lifecycle.dockVisible == visible {
		return
	}
	// Dock.Show also activates the macOS app. Retain the applied policy across
	// UI suspension, and record it before native calls can emit window events.
	a.lifecycle.dockVisible, a.lifecycle.dockVisibilityApplied = visible, true
	a.lifecycle.setDockVisible(visible)
}

func (a *application) mainWindowVisibilityChanged(visible bool) {
	a.lifecycle.mainVisible = visible
	a.updateDockVisibility()
	a.updateLightweightTimer(time.Now())
}

func (a *application) applyNativeSettings() {
	a.updateDockVisibility()
	a.updateLightweightTimer(time.Now())
}

func (a *application) trayClicked() {
	if a.state.Settings.TrayClickAction == app.TrayClickMainWindow {
		a.hideQuickSearch()
		a.show()
		return
	}
	a.trayQuickPanel()
}

// Native registration happens before persisting the setting. Persistence
// failure restores the previous OS registration as well as the UI setting.
func (a *application) prepareLaunchAtLogin(enabled bool) (func(bool), error) {
	previous := a.settingsSaved.LaunchAtLogin
	if a.state.Preview || enabled == previous || a.lifecycle.setOpenAtLogin == nil {
		return func(bool) {}, nil
	}
	if err := a.lifecycle.setOpenAtLogin(enabled); err != nil {
		return nil, err
	}
	return func(saved bool) {
		if !saved {
			if err := a.lifecycle.setOpenAtLogin(previous); err != nil {
				a.warn(a.t("无法恢复开机自启设置：%s", err))
			}
		}
	}, nil
}

func (a *application) stopLightweightTimer() {
	a.lifecycle.timerSerial++
	if a.lifecycle.timer != nil {
		a.lifecycle.timer.Stop()
		a.lifecycle.timer = nil
	}
}

func (a *application) updateLightweightTimer(now time.Time) {
	if a.closing || !a.state.Settings.LightweightMode || a.lifecycle.mainVisible || a.quick.visible || a.lifecycle.suspended {
		a.stopLightweightTimer()
		a.lifecycle.hiddenAt = time.Time{}
		return
	}
	if a.lifecycle.hiddenAt.IsZero() {
		a.lifecycle.hiddenAt = now
	}
	if a.lifecycle.timer == nil {
		a.scheduleLightweightCheck(max(time.Millisecond, lightweightIdleDelay-now.Sub(a.lifecycle.hiddenAt)))
	}
}

func (a *application) scheduleLightweightCheck(delay time.Duration) {
	a.stopLightweightTimer()
	serial := a.lifecycle.timerSerial
	a.lifecycle.timer = time.AfterFunc(delay, func() {
		a.runOnMain(func() {
			if serial != a.lifecycle.timerSerial {
				return
			}
			a.lifecycle.timer = nil
			if !a.suspendLightweightUI(time.Now()) {
				a.updateLightweightTimer(time.Now())
			}
		})
	})
}

func (a *application) lightweightSuspensionReady(now time.Time) bool {
	return a.state.Settings.LightweightMode && !a.closing && !a.initializing && !a.lifecycle.suspended &&
		!a.lifecycle.mainVisible && !a.quick.visible && !a.lifecycle.hiddenAt.IsZero() &&
		now.Sub(a.lifecycle.hiddenAt) >= lightweightIdleDelay &&
		!a.busy && !a.settingsSaving && !a.gitRemoteSaving && !a.gitActionPending &&
		!a.editor.open && !a.generator.busy && a.quick.operation == ""
}

func (a *application) suspendLightweightUI(now time.Time) bool {
	if !a.lightweightSuspensionReady(now) {
		// Keep the original close time while an in-flight operation finishes.
		if !a.lifecycle.hiddenAt.IsZero() && now.Sub(a.lifecycle.hiddenAt) >= lightweightIdleDelay &&
			a.state.Settings.LightweightMode && !a.lifecycle.mainVisible && !a.quick.visible && !a.closing {
			a.scheduleLightweightCheck(time.Second)
		}
		return false
	}
	a.stopLightweightTimer()
	a.lifecycle.suspended = true // Closing the last window must retain the process.
	if a.shell != nil {
		a.lifecycle.leftWidth = a.shell.LeftWidth()
		a.lifecycle.leftCollapsed = !a.shell.LeftExpanded()
	}
	window := a.window
	a.window, a.shell = nil, nil
	a.closeQuickSearch()
	if window != nil {
		window.Destroy()
	}
	a.releaseUIPresentation()
	go debug.FreeOSMemory()
	return true
}

func (a *application) releaseUIPresentation() {
	a.cancelUnlockAction()
	a.hidePreferences()
	a.clearNotifications()
	a.dropDetail()
	a.clearQuickSearchSecrets()
	a.closeGenerator()
	a.editor = editorState{}
	a.passphrase, a.confirmPassphrase, a.backupPassphrase = "", "", ""
	clearDataSecrets(&a.state)
	a.state.Entries, a.filtered, a.sortedEntries, a.cachedResults = nil, nil, nil, nil
	a.searchPaths, a.folders = nil, nil
	a.cacheReady = false
	a.recovery, a.activity = nil, nil
	a.searchRef = ui.Ref{}
	a.shortcut.ref = ui.Ref{}
	a.detailScroll = ui.ScrollState{}
	a.initializeEntryList()
	a.quick.list = components.ListState{}
	a.quick.searchRef, a.quick.passphraseRef = ui.Ref{}, ui.Ref{}
	a.pendingRefresh = true
	a.preferences.pendingRead = nil
}

func (a *application) resumeLightweightUI(target string) bool {
	if !a.lifecycle.suspended {
		return false
	}
	a.stopLightweightTimer()
	a.lifecycle.hiddenAt = time.Time{}
	a.lifecycle.suspended = false
	a.lifecycle.resumeTarget = target
	a.initializing = true
	a.pendingRefresh, a.pendingChanged = false, false
	a.callBackground(app.Request{Action: "configure"}, func(app.Data) {
		if isPreferencesPage(a.page) {
			a.loadPreferencesPage(a.page)
		}
	})
	return true
}

func (a *application) finishUIInitialization() {
	a.initializing = false
	a.lifecycle.configureDone = nil
	target := a.lifecycle.resumeTarget
	a.lifecycle.resumeTarget = ""
	switch target {
	case "quick":
		a.quickSearch()
	case "tray":
		a.trayQuickPanel()
	case "main":
		a.show()
	default:
		a.updateLightweightTimer(time.Now())
	}
}
