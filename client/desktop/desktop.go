// Package desktop is the native MyGo application. Onevra owns the controls,
// dialogs and window chrome; this package owns presentation state only.
package desktop

import (
	"errors"
	"fmt"
	"os"
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
	service                                                              Backend
	runOnMain                                                            func(func())
	window                                                               *mygo.Window
	shell                                                                *sidebar.Shell
	state                                                                app.Data
	detail                                                               app.Data
	busy                                                                 bool
	backgroundBusy                                                       bool
	activeRequestAction                                                  string
	initializing                                                         bool
	detailLoadingStarted                                                 time.Time
	pendingSelection, pendingSelectionVaultID                            string
	pendingSelectionGeneration                                           uint64
	pendingCopyEntry, pendingCopyVaultID                                 string
	errorText, notice, page, filter, query, selected, folderQuery        string
	folderSelection, folderSelectionFilter                               string
	searchRef                                                            ui.Ref
	list                                                                 components.ListState
	selectedIndex                                                        int
	filtered                                                             []app.Entry
	vaultSelection                                                       string
	settings                                                             app.Settings
	shortcut                                                             shortcutState
	settingsState                                                        components.SettingsState
	settingsLoaded                                                       bool
	settingsSaved                                                        app.Settings
	settingsObserved                                                     app.Settings
	settingsDirty, settingsSaving                                        bool
	settingsSavedUntil                                                   time.Time
	setup                                                                bool
	setupMode                                                            int
	defaultVaultDirectory                                                string
	setupDirectory, setupName, setupEmail, passphrase, confirmPassphrase string
	reveal                                                               bool
	editor                                                               editorState
	deleteOpen, discardOpen, purgeOpen                                   bool
	purgeID                                                              string
	recovery                                                             []app.RecoveryItem
	activity                                                             []app.Activity
	recoveryKind                                                         int
	restoreDestination                                                   string
	generator                                                            generatorState
	clipboard                                                            string
	clipboardTimer                                                       *time.Timer
	generation                                                           uint64
	closing                                                              bool
	lastInput                                                            time.Time
	unlockTime                                                           time.Time
	stop                                                                 chan struct{}
	lastTouch                                                            time.Time
	backupOpen                                                           bool
	backupPath, backupPassphrase                                         string
	entriesVersion, cachedEntriesVersion                                 uint64
	cacheReady                                                           bool
	sortedEntries                                                        []app.Entry
	searchPaths                                                          []string
	cachedQuery, cachedFilter                                            string
	cachedResults                                                        []app.Entry
	folders                                                              []string
	tray                                                                 *mygo.Tray
	setupRemote, gitRemote                                               string
	gitRemoteObserved                                                    string
	gitRemoteVaultID                                                     string
	gitRemoteSaveAt                                                      time.Time
	gitRemoteSaving                                                      bool
	gitActionPending                                                     bool
	gitActionSerial                                                      uint64
	pendingRefresh                                                       bool
	pendingChanged                                                       bool
	metadataEpoch                                                        uint64
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
	service.SetOnOpen(func() {
		mygo.RunOnMain(func() {
			a.navigate("entries", "")
			a.show()
		})
	})
	service.SetOnChange(func() { mygo.RunOnMain(func() { a.queueMetadataRefresh(true) }) })
	service.SetOnGitChange(func() { mygo.RunOnMain(func() { a.queueMetadataRefresh(false) }) })
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
		a.initializing = true
		a.shell = sidebar.NewShell(sidebar.ShellOptions{
			Hidden: true, Title: "Passpine Neo", Width: 1120, Height: 760, MinWidth: 860, MinHeight: 620,
			StateKey: "passpine-main", LeftWidth: 310, LeftMinWidth: 260, LeftMaxWidth: 440,
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
			if home, err := os.UserHomeDir(); err == nil {
				a.useDefaultVault(home)
			}
			a.settings = d.Settings
			a.settingsSaved = d.Settings
			a.settingsObserved = d.Settings
			a.settingsLoaded = true
			a.applyTheme()
			a.startNativeIntegrations()
		})
		go a.clock()
	})
	return mygo.App.Run()
}

func newApplication(service Backend) *application {
	a := &application{service: service, runOnMain: mygo.RunOnMain, page: "entries", filter: "all", selectedIndex: -1, setupName: "我的密码库", generator: generatorState{length: 24, lower: true, upper: true, numbers: true, symbols: true, exclude: true}, lastInput: time.Now(), stop: make(chan struct{})}
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
	if a.window != nil && !a.closing && !a.initializing {
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
	if a.window == nil {
		return
	}
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
	if req.Action == "git-status" || req.Action == "git-init" || req.Action == "git-sync" {
		a.startGitCall(req, done, userAction)
		return
	}
	if a.busy || a.closing {
		return
	}
	a.busy = true
	a.backgroundBusy = !userAction
	a.activeRequestAction = req.Action
	if userAction {
		a.errorText = ""
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
		// First paint follows configuration and theme setup, so reopening an
		// existing vault never briefly displays onboarding or the wrong theme.
		if a.initializing {
			a.initializing = false
			a.show()
		}
		if a.pendingChanged {
			a.dropDetail()
		}
		a.flushMetadataRefresh()
		a.flushPendingSelection()
	}()
	a.busy = false
	a.backgroundBusy = false
	a.activeRequestAction = ""
	if a.state.VaultID != r.Data.VaultID {
		a.vaultSelection = r.Data.VaultID
	}
	if a.state.VaultID != r.Data.VaultID || len(a.state.Entries) != len(r.Data.Entries) {
		a.cacheReady = false
	}
	wasLocked := a.state.Locked
	a.state = r.Data
	a.entriesVersion = r.Data.EntriesVersion
	clearDataSecrets(&a.state)
	if a.state.Locked {
		if !wasLocked {
			a.clearSecrets()
		} else {
			a.detail = app.Data{}
			a.reveal = false
		}
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
	a.pendingCopyEntry, a.pendingCopyVaultID = "", ""
	a.detail = app.Data{}
	a.detailLoadingStarted = time.Time{}
	a.pendingSelection, a.pendingSelectionVaultID = "", ""
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
	a.setShortcutRecording(false)
	a.detail = app.Data{}
	a.detailLoadingStarted = time.Time{}
	a.activeRequestAction = ""
	a.pendingSelection, a.pendingSelectionVaultID = "", ""
	a.pendingCopyEntry, a.pendingCopyVaultID = "", ""
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
			if !a.settingsSavedUntil.IsZero() && time.Now().After(a.settingsSavedUntil) {
				a.settingsSavedUntil = time.Time{}
				a.invalidate()
			}
			if !a.gitRemoteSaveAt.IsZero() && time.Now().After(a.gitRemoteSaveAt) {
				a.saveGitRemoteAutomatically()
			}

			if !a.state.Locked && a.state.Configured {
				if a.detail.Contents != "" {
					document := vault.ParseDocument(a.detail.Entry, a.detail.Contents, a.detail.Revision)
					if document.OTP != "" {
						if otp, err := vault.TOTP(document.OTP, time.Now()); err == nil {
							changed := a.detail.OTP != otp.Code
							a.detail.OTP, a.detail.OTPSeconds = otp.Code, otp.Remaining
							if changed {
								a.invalidate()
							}
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
	if a.pendingCopyEntry != "" && a.pendingCopyEntry != path {
		a.pendingCopyEntry, a.pendingCopyVaultID = "", ""
	}
	if a.busy {
		// Keep the latest click through metadata refreshes and earlier reads.
		// Other foreground operations retain their own navigation context.
		if (a.backgroundBusy || a.activeRequestAction == "fetch") && !a.state.Locked {
			a.pendingSelection, a.pendingSelectionVaultID = path, a.state.VaultID
			a.pendingSelectionGeneration = a.generation
			a.page, a.selected = "entries", path
			a.detail, a.reveal = app.Data{}, false
			a.detailLoadingStarted = time.Now()
		}
		return
	}
	a.page = "entries"
	if a.selected == path && a.detail.Entry == path && !a.state.Locked {
		a.lastInput = time.Now()
		return
	}
	a.selected, a.reveal = path, false
	a.detail = app.Data{}
	a.detailLoadingStarted = time.Now()
	if a.state.Locked {
		return
	}
	a.call(app.Request{Action: "fetch", Entry: path}, func(d app.Data) {
		if a.selected == path {
			a.detail = d
		}
	})
}

func (a *application) flushPendingSelection() {
	if a.busy || a.pendingSelection == "" || a.closing {
		return
	}
	path, vaultID, generation := a.pendingSelection, a.pendingSelectionVaultID, a.pendingSelectionGeneration
	a.pendingSelection, a.pendingSelectionVaultID = "", ""
	if a.setup || a.page != "entries" || a.state.Locked || a.state.VaultID != vaultID || a.generation != generation {
		return
	}
	for _, entry := range a.state.Entries {
		if entry.Path == path {
			a.selectEntry(path)
			return
		}
	}
	a.dropDetail()
}

func (a *application) navigate(page, filter string) {
	a.pendingCopyEntry, a.pendingCopyVaultID = "", ""
	a.pendingSelection, a.pendingSelectionVaultID = "", ""
	a.setShortcutRecording(false)
	a.lastInput = time.Now()
	a.page = page
	if filter != "" {
		a.filter = filter
	}
	a.errorText = ""
	if page == "recovery" {
		a.loadRecovery()
	}
	if page == "settings" {
		if !a.settingsLoaded {
			a.settings = a.state.Settings
			a.settingsSaved = a.state.Settings
			a.settingsObserved = a.state.Settings
			a.settingsLoaded = true
		}
		if a.gitRemoteSaveAt.IsZero() && !a.gitRemoteSaving {
			a.gitRemote = a.state.GitRemote
			a.gitRemoteObserved = a.gitRemote
		}
		a.call(app.Request{Action: "git-status"}, func(d app.Data) {
			if a.gitRemoteSaveAt.IsZero() && !a.gitRemoteSaving {
				a.gitRemote = d.GitRemote
				a.gitRemoteObserved = a.gitRemote
			}
		})
	}
	if page == "activity" {
		a.call(app.Request{Action: "activity"}, func(d app.Data) { a.activity = d.Activity })
	}
	a.invalidate()
}

func (a *application) openSetup() {
	a.pendingCopyEntry, a.pendingCopyVaultID = "", ""
	a.pendingSelection, a.pendingSelectionVaultID = "", ""
	a.setShortcutRecording(false)
	a.setup = true
	a.invalidate()
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

			return strings.ToLower(a.sortedEntries[i].Path) < strings.ToLower(a.sortedEntries[j].Path)
		})
		a.searchPaths = make([]string, len(a.sortedEntries))
		for i, e := range a.sortedEntries {
			a.searchPaths[i] = strings.ToLower(e.Path)
		}
		folders := map[string]bool{}
		for _, e := range a.sortedEntries {
			if e.Folder != "" {
				parts := strings.Split(e.Folder, "/")
				for i := range parts {
					folders[strings.Join(parts[:i+1], "/")] = true
				}
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
		if strings.HasPrefix(a.filter, "folder:") {
			folder := strings.TrimPrefix(a.filter, "folder:")
			if e.Folder != folder && !strings.HasPrefix(e.Folder, folder+"/") {
				continue
			}
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
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Children(func() {
		a.listView(c)
	})
}

func (a *application) sidebarToolbar(c *ui.Context) {
	if a.page != "entries" || a.setup {
		if iconButton(c, lucide.ChevronLeft(), "返回密码库", components.ButtonGhost).Clicked() {
			a.setup = false
			a.navigate("entries", "")
		}
	}
	if iconButton(c, lucide.Settings(), "设置", components.ButtonGhost).Clicked() {
		a.navigate("settings", "")
	}
	ui.Spacer(c)
	iconButton(c, lucide.Ellipsis(), "更多操作", components.ButtonGhost).Menu(func(menu *ui.Menu) {
		if menu.Item("恢复中心").Chosen() {
			a.navigate("recovery", "")
		}
		if menu.Item("操作记录").Chosen() {
			a.navigate("activity", "")
		}
		menu.Separator()
		if menu.Item("打开或创建密码库").Chosen() {
			a.openSetup()
		}
	})
}

func (a *application) header(c *ui.Context) {
	title := map[string]string{"entries": "密码库", "recovery": "恢复中心", "settings": "设置", "activity": "操作记录"}[a.page]
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		components.Text(c, title).Bold()
		ui.Spacer(c)
		if a.state.Configured {
			a.vaultSelector(c)
			a.gitHeaderStatus(c)
			lockIcon, lockLabel := lucide.LockKeyhole(), "锁定密码库"
			if a.state.Locked {
				lockIcon, lockLabel = lucide.LockOpen(), "解锁密码库"
			}
			if iconButton(c, lockIcon, lockLabel, components.ButtonGhost).Clicked() {
				if a.state.Locked {
					a.navigate("entries", "")
				} else {
					a.lock()
				}
			}
			if iconButton(c, lucide.RefreshCw(), "同步密码库", components.ButtonOutline).Disabled(a.gitActionPending || a.state.GitSyncing || !a.state.GitInitialized).Clicked() {
				a.call(app.Request{Action: "git-sync"}, func(app.Data) { a.notice = "同步完成" })
			}
		}
		if iconButton(c, lucide.Plus(), "新建条目", components.ButtonOutline).Clicked() {
			a.newEntry()
		}
	})
}

func (a *application) vaultSelector(c *ui.Context) {
	if a.vaultSelection == "" {
		a.vaultSelection = a.state.VaultID
	}
	sel := components.Select(c, &a.vaultSelection)
	sel.Trigger.Width(170).MinWidth(100).Disabled(a.busy || a.editor.open).Label("切换密码库").Gap(6).Children(func() {
		components.Icon(c, lucide.FolderKey()).Size(14, 14)
		components.Text(c, a.state.VaultName).SingleLine()
	})
	sel.Popup(func(panel ui.Element) {
		panel.Width(240).Label("密码库选项")
		components.ScrollView(c, components.ScrollViewOptions{}).FillWidth().MaxHeight(240).Children(func() {
			found := false
			for _, v := range a.state.Vaults {
				v := v
				found = found || v.ID == a.state.VaultID
				sel.Item(v.ID).Label("切换到密码库 " + v.Name).Children(func() { components.Text(c, v.Name).SingleLine() })
			}
			if !found {
				sel.Item(a.state.VaultID).Label("当前密码库").Children(func() { components.Text(c, a.state.VaultName).SingleLine() })
			}
			sel.Item("open-vault").Label("打开或创建密码库").Children(func() {
				components.Icon(c, lucide.FolderPlus()).Size(14, 14)
				components.Text(c, "打开或创建密码库").SingleLine()
			})
		})
	})
	if sel.Trigger.Changed() {
		id := a.vaultSelection
		a.vaultSelection = a.state.VaultID
		if id == "open-vault" {
			a.openSetup()
		} else if id != a.state.VaultID {
			a.switchVault(id)
		}
	}
}

func (a *application) contentView(c *ui.Context) {
	a.flushMetadataRefresh()
	if !a.shortcut.recording {
		c.OnShortcut(ui.Cmd, ui.KeyF, func() { a.navigate("entries", ""); a.searchRef.Focus() })
		c.OnShortcut(ui.Cmd, ui.KeyN, a.newEntry)
		c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyL, a.lock)
		c.OnShortcut(ui.Cmd, ui.KeyComma, func() { a.navigate("settings", "") })
		c.OnShortcut(ui.Cmd, ui.KeyQ, mygo.App.Quit)
	}
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Children(func() {
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
		if a.page != "settings" && a.page != "recovery" && a.page != "activity" {
			a.vaultView(c)
			return
		}
		if a.state.Locked && a.page != "settings" && a.page != "activity" {
			a.unlockView(c)
			return
		}
		switch a.page {
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
		return &mygo.MenuItem{Label: label, Accelerator: shortcut, Click: func(*mygo.MenuItem, *mygo.Window) {
			if a.shortcut.recording {
				a.recordShortcut(shortcut)
				return
			}
			fn()
		}}
	}
	editMenu := &mygo.MenuItem{Role: mygo.RoleEditMenu}
	windowMenu := &mygo.MenuItem{Role: mygo.RoleWindowMenu}
	if a.shortcut.recording {
		editMenu = &mygo.MenuItem{Label: "编辑", Disabled: true}
		windowMenu = &mygo.MenuItem{Label: "窗口", Disabled: true}
	}
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{action("设置…", "CmdOrCtrl+,", func() { a.navigate("settings", "") }), mygo.Separator(), action("退出 Passpine Neo", "CmdOrCtrl+Q", mygo.App.Quit)}},
		{Label: "文件", Submenu: []*mygo.MenuItem{action("新建条目", "CmdOrCtrl+N", a.newEntry), action("打开密码库…", "", a.openSetup), mygo.Separator(), action("锁定密码库", "CmdOrCtrl+Shift+L", a.lock)}},
		editMenu,
		{Label: "显示", Submenu: []*mygo.MenuItem{
			action("搜索", "CmdOrCtrl+F", func() { a.navigate("entries", ""); a.searchRef.Focus() }),
			action("恢复中心", "", func() { a.navigate("recovery", "") }),
			action("操作记录", "", func() { a.navigate("activity", "") }),
		}},
		windowMenu,
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
