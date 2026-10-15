// Package desktop is the native MyGo application. Onevra owns the controls,
// dialogs and window chrome; this package owns presentation state only.
package desktop

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine-neo/client/app"
	"github.com/knothhe/passpine-neo/client/vault"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"
	"github.com/oneo-me/onevra/source/theme"
)

type Backend interface {
	Execute(app.Request) app.Response
}

type application struct {
	service                                                                        Backend
	runOnMain                                                                      func(func())
	window                                                                         *mygo.Window
	shell                                                                          *sidebar.Shell
	state                                                                          app.Data
	detail                                                                         app.Data
	busy                                                                           bool
	errorText, notice, page, filter, query, selected                               string
	searchRef                                                                      ui.Ref
	list                                                                           components.ListState
	selectedIndex                                                                  int
	filtered                                                                       []app.Entry
	listWidth                                                                      float32
	settings                                                                       app.Settings
	settingsState                                                                  components.SettingsState
	settingsLoaded                                                                 bool
	setup                                                                          bool
	setupMode                                                                      int
	setupDirectory, setupKey, setupName, setupEmail, passphrase, confirmPassphrase string
	reveal                                                                         bool
	editor                                                                         editorState
	deleteOpen, discardOpen, purgeOpen                                             bool
	purgeID                                                                        string
	recovery                                                                       []app.RecoveryItem
	activity                                                                       []app.Activity
	recoveryKind                                                                   int
	restoreDestination                                                             string
	generator                                                                      generatorState
	clipboard                                                                      string
	clipboardTimer                                                                 *time.Timer
	generation                                                                     uint64
	closing                                                                        bool
	lastInput                                                                      time.Time
	unlockTime                                                                     time.Time
	stop                                                                           chan struct{}
	lastTouch                                                                      time.Time
	backupOpen                                                                     bool
	backupPath, backupPassphrase                                                   string
	entriesVersion, cachedEntriesVersion                                           uint64
	cacheReady                                                                     bool
	sortedEntries                                                                  []app.Entry
	searchPaths                                                                    []string
	cachedQuery, cachedFilter                                                      string
	cachedResults                                                                  []app.Entry
	folders                                                                        []string
	tray                                                                           *mygo.Tray
	setupRemote, gitRemote                                                         string
	pendingRefresh                                                                 bool
	pendingChanged                                                                 bool
	metadataEpoch                                                                  uint64
}

// Run starts a native GPU-rendered window, with native menus, file panels,
// clipboard and system lock/sleep notifications. It does not create a WebView.
func Run(service *app.Service) error {
	a := newApplication(service)
	mygo.App.SetName("Passpine Neo")
	mygo.App.SetPath(mygo.PathUserData, service.DataDirectory())
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	mygo.App.OnSecondInstance(func([]string, string) { a.show() })
	mygo.App.OnActivate(func(bool) { a.show() })
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.closing = true; close(a.stop); a.clearSecrets(); go service.Close() })
	mygo.App.OnWindowAllClosed(func() { mygo.App.Quit() })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		mygo.GlobalShortcut.UnregisterAll()
		if a.tray != nil {
			a.tray.Destroy()
			a.tray = nil
		}
	})
	mygo.Power.OnSuspend(func() { a.lock() })
	mygo.Power.OnLockScreen(func() { a.lock() })
	service.SetOnOpen(func() { mygo.RunOnMain(a.show) })
	service.SetOnChange(func() { mygo.RunOnMain(func() { a.queueMetadataRefresh(true) }) })
	service.SetOnLock(func() {
		mygo.RunOnMain(func() {
			if a.closing || !service.IsLocked() {
				return
			}
			a.generation++
			a.busy = false
			a.clearSecrets()
			a.state.Locked = true
			a.invalidate()
		})
	})
	service.SetClipboard(func(value string, seconds int) error {
		lease := service.ClipboardLease()
		var err error
		mygo.RunOnMain(func() {
			if a.closing {
				err = errors.New("application is closing")
				return
			}
			err = lease(func() { a.writeClipboard(value, seconds) })
		})
		return err
	})
	mygo.App.WhenReady(func() {
		a.shell = sidebar.NewShell(sidebar.ShellOptions{
			Title: "Passpine Neo", Width: 1120, Height: 760, MinWidth: 860, MinHeight: 620,
			StateKey: "passpine-main", LeftWidth: 204, LeftMinWidth: 184, LeftMaxWidth: 300,
			Theme: theme.Onevra, Left: a.sidebarView, Content: a.contentView,
			ContentTitle: a.header, LeftToolbar: a.sidebarToolbar,
			LeftToggleLabel: "切换侧栏", LeftResizeLabel: "调整侧栏宽度", CloseToastLabel: "关闭提示",
		})
		a.window = a.shell.Window()
		a.window.OnFocus(func() {
			if a.state.Configured {
				a.queueMetadataRefresh(false)
			}
		})
		a.window.OnClose(func(event *mygo.CloseEvent) {
			if !a.closing && a.tray != nil && a.state.Settings.CloseToTray {
				event.PreventDefault()
				a.window.Hide()
			}
		})
		a.installMenu()
		a.callBackground(app.Request{Action: "configure"}, func(d app.Data) {
			a.settings = d.Settings
			a.settingsLoaded = true
			a.applyTheme()
			a.startNativeIntegrations()
		})
		go a.clock()
	})
	return mygo.App.Run()
}

func newApplication(service Backend) *application {
	a := &application{service: service, runOnMain: mygo.RunOnMain, page: "entries", filter: "all", selectedIndex: -1, listWidth: 300, setupName: "我的密码库", generator: generatorState{length: 24, lower: true, upper: true, numbers: true, symbols: true, exclude: true}, lastInput: time.Now(), stop: make(chan struct{})}
	a.list = components.ListState{Selected: &a.selectedIndex, Key: func(i int) any { return a.filtered[i].Path }, Label: func(i int) string { return a.filtered[i].Name }, OnChange: func() {
		if a.selectedIndex >= 0 && a.selectedIndex < len(a.filtered) {
			a.selectEntry(a.filtered[a.selectedIndex].Path)
		}
	}, OnSubmit: func(i int) {
		if i >= 0 && i < len(a.filtered) {
			a.selectEntry(a.filtered[i].Path)
		}
	}}
	return a
}

func (a *application) show() {
	if a.window != nil && !a.closing {
		if a.window.IsMinimized() {
			a.window.Restore()
		}
		a.window.Show()
		a.window.Focus()
	}
}
func (a *application) invalidate() {
	if a.window != nil && !a.closing {
		a.window.Invalidate()
	}
}
func (a *application) applyTheme() {
	source := a.state.Settings.Theme
	if source == "" {
		source = "system"
	}
	mygo.Theme.SetSource(mygo.ThemeSource(source))
}

// All application state is confined to MyGo's main thread. The generation
// check discards late decrypted results after a lock or vault switch.
func (a *application) call(req app.Request, done func(app.Data)) {
	a.startCall(req, done, true)
}

// Refreshes run without user input, so they must not extend the idle session.
func (a *application) callBackground(req app.Request, done func(app.Data)) {
	a.startCall(req, done, false)
}

func (a *application) startCall(req app.Request, done func(app.Data), userAction bool) {
	if a.busy || a.closing {
		return
	}
	a.busy, a.errorText = true, ""
	if userAction {
		a.lastInput = time.Now()
	}
	generation := a.generation
	go func() {
		r := a.service.Execute(req)
		a.runOnMain(func() { a.receiveResponse(generation, r, done) })
	}()
	a.invalidate()
}

func (a *application) receiveResponse(generation uint64, r app.Response, done func(app.Data)) {
	if a.closing || generation != a.generation {
		return
	}
	defer func() {
		if a.pendingChanged {
			a.dropDetail()
		}
		a.flushMetadataRefresh()
	}()
	a.busy = false
	if a.state.VaultID != r.Data.VaultID || len(a.state.Entries) != len(r.Data.Entries) {
		a.cacheReady = false
	}
	a.state = r.Data
	a.entriesVersion = r.Data.EntriesVersion
	clearDataSecrets(&a.state)
	if a.state.Locked {
		a.clearSecrets()
	}
	if r.Status != "ok" {
		a.errorText = r.Message
		if a.errorText == "" {
			a.errorText = r.Code
		}
		a.invalidate()
		return
	}
	if done != nil {
		done(r.Data)
	}
	a.invalidate()
}

// Browser changes and background Git syncs notify us once. Work already in
// progress keeps its own result and draft; a pending notification is coalesced
// until the editor closes and the service becomes idle.
func (a *application) queueMetadataRefresh(changed bool) {
	if a.closing {
		return
	}
	a.pendingRefresh = true
	if changed {
		a.pendingChanged = true
		a.metadataEpoch++
		a.dropDetail()
	}
	a.flushMetadataRefresh()
	a.invalidate()
}

func (a *application) dropDetail() {
	a.detail = app.Data{}
	a.reveal = false
	a.selected = ""
	a.selectedIndex = -1
	a.deleteOpen = false
}

func (a *application) refreshReady() bool {
	return a.pendingRefresh && !a.busy && !a.editor.open && !a.closing
}

func (a *application) flushMetadataRefresh() {
	if !a.refreshReady() {
		return
	}
	a.pendingRefresh = false
	a.pendingChanged = false
	version := a.state.EntriesVersion
	a.callBackground(app.Request{Action: "configure"}, func(d app.Data) {
		if d.EntriesVersion != version {
			a.metadataEpoch++
			a.dropDetail()
			a.cacheReady = false
		}
	})
}

func (a *application) lock() {
	if a.closing {
		return
	}
	a.clearSecrets()
	a.state.Locked = true
	a.generation++
	a.busy = false
	go func() { a.service.Execute(app.Request{Action: "lock"}) }()
	a.invalidate()
}

func (a *application) clearSecrets() {
	a.detail = app.Data{}
	clearDataSecrets(&a.state)
	if !a.state.Settings.BrowseWhileLocked {
		a.state.Entries = nil
		a.entriesVersion++
		a.sortedEntries = nil
		a.searchPaths = nil
		a.cachedResults = nil
		a.folders = nil
	}
	a.editor = editorState{}
	a.reveal = false
	a.passphrase, a.confirmPassphrase, a.generator.password = "", "", ""
	a.backupOpen, a.backupPassphrase = false, ""
	a.recovery = nil
	if a.clipboardTimer != nil {
		a.clipboardTimer.Stop()
		a.clipboardTimer = nil
	}
	if a.clipboard != "" && mygo.Clipboard.ReadText() == a.clipboard {
		mygo.Clipboard.Clear()
	}
	a.clipboard = ""
}

func (a *application) clock() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.stop:
			return
		case <-ticker.C:
		}
		mygo.RunOnMain(func() {
			if a.closing {
				return
			}
			if !a.state.Locked && a.state.Configured {
				if a.detail.Contents != "" {
					document := vault.ParseDocument(a.detail.Entry, a.detail.Contents, a.detail.Revision)
					if document.OTP != "" {
						if otp, err := vault.TOTP(document.OTP, time.Now()); err == nil {
							a.detail.OTP, a.detail.OTPSeconds = otp.Code, otp.Remaining
							a.invalidate()
						}
					}
				}
				if a.lastInput.After(a.lastTouch) {
					a.lastTouch = time.Now()
					if service, ok := a.service.(interface{ Touch() }); ok {
						go service.Touch()
					}
				}
				if a.editor.open && a.editor.dirty() && !a.editor.draftSaved && !a.editor.draftAttempted && a.editor.path != "" && a.editor.contents != "" && time.Since(a.lastInput) > 600*time.Millisecond && !a.busy {
					a.saveDraft()
				}
				s := a.state.Settings
				idle := s.IdleMinutes > 0 && time.Since(a.lastInput) >= time.Duration(s.IdleMinutes)*time.Minute && mygo.Power.IdleTime() >= time.Duration(s.IdleMinutes)*time.Minute
				maximum := s.MaxSessionMinutes > 0 && !a.unlockTime.IsZero() && time.Since(a.unlockTime) >= time.Duration(s.MaxSessionMinutes)*time.Minute
				if idle || maximum {
					a.lock()
				}
			}
		})
	}
}

func (a *application) copy(value, label string) {
	if value == "" || a.state.Locked {
		return
	}
	field := ""
	if a.detail.Entry != "" {
		switch {
		case label == "密码" && value == a.detail.Password:
			field = "password"
		case label == "用户名" && value == a.detail.Username:
			field = "username"
		case strings.HasPrefix(label, "一次性密码") && value == a.detail.OTP:
			field = "otp"
		}
	}
	if field != "" {
		a.call(app.Request{Action: "copy", Entry: a.detail.Entry, Revision: a.detail.Revision, Field: field}, func(app.Data) { a.notice = "已复制" + label })
		return
	}
	service, ok := a.service.(interface{ ClipboardLease() func(func()) error })
	if !ok {
		return
	}
	seconds := a.state.Settings.ClipboardSeconds
	generation := a.generation
	metadataEpoch := a.metadataEpoch
	go func() {
		lease := service.ClipboardLease()
		mygo.RunOnMain(func() {
			if a.closing || a.state.Locked || generation != a.generation || metadataEpoch != a.metadataEpoch {
				return
			}
			if err := lease(func() { a.writeClipboard(value, seconds) }); err != nil {
				a.errorText = err.Error()
			} else {
				a.notice = "已复制" + label
			}
			a.invalidate()
		})
	}()
}

func (a *application) writeClipboard(value string, seconds int) {
	mygo.Clipboard.WriteText(value)
	a.clipboard = value
	if a.clipboardTimer != nil {
		a.clipboardTimer.Stop()
	}
	if seconds > 0 {
		a.clipboardTimer = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
			mygo.RunOnMain(func() {
				if a.clipboard == value && mygo.Clipboard.ReadText() == value {
					mygo.Clipboard.Clear()
				}
				if a.clipboard == value {
					a.clipboard = ""
				}
			})
		})
	}
}

func (a *application) selectEntry(path string) {
	if a.state.Locked || a.busy {
		return
	}
	a.selected, a.reveal = path, false
	a.detail = app.Data{}
	a.call(app.Request{Action: "fetch", Entry: path}, func(d app.Data) { a.detail = d })
}

func (a *application) navigate(page, filter string) {
	a.lastInput = time.Now()
	a.page, a.filter = page, filter
	a.errorText = ""
	if page == "recovery" {
		a.loadRecovery()
	}
	if page == "settings" {
		a.settings = a.state.Settings
		a.settingsLoaded = true
		a.gitRemote = a.state.GitRemote
		a.call(app.Request{Action: "git-status"}, func(d app.Data) { a.gitRemote = d.GitRemote })
	}
	if page == "activity" {
		a.call(app.Request{Action: "activity"}, func(d app.Data) { a.activity = d.Activity })
	}
}

func clearDataSecrets(d *app.Data) {
	d.Contents = ""
	d.Password = ""
	d.Username = ""
	d.URL = ""
	d.Notes = ""
	d.OTP = ""
}

func (a *application) entries() []app.Entry {
	changed := !a.cacheReady || a.entriesVersion != a.cachedEntriesVersion
	if changed {
		a.cachedEntriesVersion = a.entriesVersion
		a.cacheReady = true
		a.sortedEntries = append([]app.Entry(nil), a.state.Entries...)
		sort.SliceStable(a.sortedEntries, func(i, j int) bool {
			if a.sortedEntries[i].Favorite != a.sortedEntries[j].Favorite {
				return a.sortedEntries[i].Favorite
			}
			return strings.ToLower(a.sortedEntries[i].Path) < strings.ToLower(a.sortedEntries[j].Path)
		})
		a.searchPaths = make([]string, len(a.sortedEntries))
		for i, e := range a.sortedEntries {
			a.searchPaths[i] = strings.ToLower(e.Path)
		}
		folders := map[string]bool{}
		for _, e := range a.sortedEntries {
			if e.Folder != "" {
				folders[e.Folder] = true
			}
		}
		a.folders = a.folders[:0]
		for f := range folders {
			a.folders = append(a.folders, f)
		}
		sort.Strings(a.folders)
	}
	q := strings.ToLower(strings.TrimSpace(a.query))
	if !changed && q == a.cachedQuery && a.filter == a.cachedFilter {
		return a.cachedResults
	}
	a.cachedQuery, a.cachedFilter = q, a.filter
	result := a.cachedResults[:0]
	if cap(result) < len(a.sortedEntries) {
		result = make([]app.Entry, 0, len(a.sortedEntries))
	}
	for i, e := range a.sortedEntries {
		if a.filter == "favorites" && !e.Favorite {
			continue
		}
		if strings.HasPrefix(a.filter, "folder:") && e.Folder != strings.TrimPrefix(a.filter, "folder:") {
			continue
		}
		if q != "" && !strings.Contains(a.searchPaths[i], q) {
			continue
		}
		result = append(result, e)
	}
	a.cachedResults = result
	return result
}

func (a *application) sidebarView(c *ui.Context) {
	a.entries()
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Gap(20).Children(func() {
		ui.Column(c).FillWidth().Padding(10, 20, 0, 20).Gap(6).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
				components.Icon(c, lucide.KeyRound).Size(25, 25).TextColor(c.Theme().Accent)
				components.Text(c, "Passpine").FontSize(21).Bold()
			})
		})
		components.ScrollView(c, components.ScrollViewOptions{}).Grow(1).FillWidth().Children(func() {
			components.Sidebar(c).Children(func() {
				components.SidebarSection(c, "密码库", func() {
					if components.SidebarItem(c, "所有条目", lucide.Layers(), a.page == "entries" && a.filter == "all").Clicked() {
						a.navigate("entries", "all")
					}
					if components.SidebarItem(c, "我的收藏", lucide.Star(), a.page == "entries" && a.filter == "favorites").Clicked() {
						a.navigate("entries", "favorites")
					}
				})
				names := a.folders
				if len(names) > 0 {
					components.SidebarSection(c, "文件夹", func() {
						for _, f := range names {
							if components.SidebarItem(c, f, lucide.Folder(), a.page == "entries" && a.filter == "folder:"+f).Clicked() {
								a.navigate("entries", "folder:"+f)
							}
						}
					})
				}
				components.SidebarSection(c, "工具", func() {
					if components.SidebarItem(c, "密码生成器", lucide.Sparkles(), a.page == "generator").Clicked() {
						a.navigate("generator", "")
					}
					if components.SidebarItem(c, "恢复中心", lucide.ArchiveRestore(), a.page == "recovery").Clicked() {
						a.navigate("recovery", "")
					}
					if components.SidebarItem(c, "操作记录", lucide.Clock3(), a.page == "activity").Clicked() {
						a.navigate("activity", "")
					}
				})
			})
		})
		ui.Column(c).FillWidth().Padding(0, 16, 12, 16).Gap(8).Children(func() {
			if a.state.Preview {
				components.Badge(c, "隔离演示 · 虚构数据").FontSize(10)
			}
			name := a.state.VaultName
			if name == "" {
				name = "尚未连接密码库"
			}
			components.Text(c, name).Bold().FontSize(12).SingleLine()
			status := "已锁定"
			if a.state.Configured && !a.state.Locked {
				status = "已解锁 · 本地加密"
			}
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				components.Icon(c, lucide.ShieldCheck).Size(13, 13).TextColor(c.Theme().Success)
				components.Text(c, status).FontSize(11).TextColor(c.Theme().TextMuted)
			})
		})
	})
}

func (a *application) sidebarToolbar(c *ui.Context) {
	if iconButton(c, lucide.Settings(), "设置", components.ButtonGhost).Clicked() {
		a.navigate("settings", "")
	}
	if iconButton(c, lucide.FolderPlus(), "打开或创建密码库", components.ButtonGhost).Clicked() {
		a.setup = true
	}
	ui.Spacer(c)
	if iconButton(c, lucide.LockKeyhole(), "锁定密码库", components.ButtonGhost).Clicked() {
		a.lock()
	}
}

func (a *application) header(c *ui.Context) {
	title := map[string]string{"entries": "密码库", "generator": "密码生成器", "recovery": "恢复中心", "settings": "设置", "activity": "操作记录"}[a.page]
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		components.Text(c, title).Bold()
		ui.Spacer(c)
		if a.state.Configured {
			if iconButton(c, lucide.RefreshCw(), "同步密码库", components.ButtonOutline).Clicked() {
				a.call(app.Request{Action: "git-sync"}, func(app.Data) { a.notice = "同步完成" })
			}
		}
		if iconButton(c, lucide.Plus(), "新建条目", components.ButtonOutline).Clicked() {
			a.newEntry()
		}
	})
}

func (a *application) contentView(c *ui.Context) {
	a.flushMetadataRefresh()
	c.OnShortcut(ui.Cmd, ui.KeyF, func() { a.page = "entries"; a.searchRef.Focus() })
	c.OnShortcut(ui.Cmd, ui.KeyN, a.newEntry)
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyL, a.lock)
	c.OnShortcut(ui.Cmd, ui.KeyComma, func() { a.navigate("settings", "") })
	c.OnShortcut(ui.Cmd, ui.KeyQ, mygo.App.Quit)
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Children(func() {
		if a.busy {
			components.Progress(c, -1).Height(2).Label("正在处理")
		}
		if a.errorText != "" {
			a.banner(c, a.errorText, true)
		}
		if a.notice != "" {
			a.banner(c, a.notice, false)
		}
		if !a.state.Configured || a.setup {
			a.setupView(c)
			return
		}
		if a.state.Locked {
			a.unlockView(c)
			return
		}
		switch a.page {
		case "generator":
			a.generatorView(c)
		case "recovery":
			a.recoveryView(c)
		case "settings":
			a.settingsView(c)
		case "activity":
			a.activityView(c)
		default:
			a.vaultView(c)
		}
	})
	a.dialogs(c)
	a.flushMetadataRefresh()
}

func (a *application) banner(c *ui.Context, text string, bad bool) {
	color := c.Theme().Success
	if bad {
		color = c.Theme().Danger
	}
	ui.Row(c).FillWidth().Padding(10, 20).Gap(12).Background(color.Alpha(.08)).AlignItems(ui.Center).Children(func() {
		components.Text(c, text).Wrap().Grow(1).FontSize(12).TextColor(color)
		if iconButton(c, lucide.X(), "关闭消息", components.ButtonGhost).Clicked() {
			if bad {
				a.errorText = ""
			} else {
				a.notice = ""
			}
		}
	})
}

func (a *application) installMenu() {
	action := func(label, shortcut string, fn func()) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Accelerator: shortcut, Click: func(*mygo.MenuItem, *mygo.Window) { fn() }}
	}
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{action("设置…", "CmdOrCtrl+,", func() { a.navigate("settings", "") }), mygo.Separator(), {Role: mygo.RoleQuit}}},
		{Label: "文件", Submenu: []*mygo.MenuItem{action("新建条目", "CmdOrCtrl+N", a.newEntry), action("打开密码库…", "", func() { a.setup = true; a.invalidate() }), mygo.Separator(), action("锁定密码库", "CmdOrCtrl+Shift+L", a.lock)}},
		{Role: mygo.RoleEditMenu},
		{Label: "显示", Submenu: []*mygo.MenuItem{action("搜索", "CmdOrCtrl+F", func() { a.page = "entries"; a.searchRef.Focus(); a.invalidate() }), action("密码生成器", "", func() { a.navigate("generator", ""); a.invalidate() })}},
		{Role: mygo.RoleWindowMenu},
	}))
}

func field(c *ui.Context, label string, value *string, placeholder string, password bool) ui.Element {
	var input ui.Element
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		components.Text(c, label).FontSize(12).Bold()
		input = components.TextBox(c, value, components.TextBoxOptions{}).FillWidth().Label(label).Placeholder(placeholder)
		if password {
			input.Password()
		}
	})
	return input
}
func section(c *ui.Context, title, description string, build func()) {
	ui.Column(c).FillWidth().Gap(14).Children(func() {
		components.Text(c, title).FontSize(20).Bold()
		if description != "" {
			components.Text(c, description).Wrap().FontSize(12).TextColor(c.Theme().TextMuted)
		}
		if build != nil {
			build()
		}
	})
}
func muted(c *ui.Context, text string) {
	components.Text(c, text).Wrap().FontSize(12).TextColor(c.Theme().TextMuted)
}

// Released Onevra exposes Button and Icon independently. Compose them without
// implementing a parallel hit target, focus behavior or painted control.
func iconButton(c *ui.Context, icon *ui.SVG, label string, variant components.ButtonVariant) ui.Element {
	button := components.Button(c, "", variant).Label(label).Size(32, 32).Padding(7)
	button.Children(func() { components.Icon(c, icon).Size(17, 17) })
	return button
}
func entryName(path string) string { return strings.TrimSuffix(filepath.Base(path), ".gpg") }
func countLabel(n int) string      { return fmt.Sprintf("%d 个条目", n) }
