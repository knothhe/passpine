package desktop

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestDockPolicyFollowsMainWindowAndDoesNotTreatQuickPanelAsManager(t *testing.T) {
	a := fixture()
	var visible bool
	a.lifecycle.setDockVisible = func(value bool) { visible = value }
	for _, policy := range []string{app.DockIconShow, app.DockIconHide, app.DockIconFollowWindow} {
		a.state.Settings.DockIcon = policy
		a.quick.visible = true
		a.mainWindowVisibilityChanged(false)
		if visible != (policy == app.DockIconShow) {
			t.Fatalf("policy %q while hidden showed dock = %v", policy, visible)
		}
		a.mainWindowVisibilityChanged(true)
		if visible != (policy != app.DockIconHide) {
			t.Fatalf("policy %q while shown showed dock = %v", policy, visible)
		}
	}
}

func TestDockVisibilityWaitsForNativeSetter(t *testing.T) {
	for _, policy := range []string{app.DockIconShow, app.DockIconHide} {
		t.Run(policy, func(t *testing.T) {
			a := fixture()
			a.state.Settings.DockIcon = policy
			a.updateDockVisibility()
			var calls []bool
			a.lifecycle.setDockVisible = func(visible bool) { calls = append(calls, visible) }
			a.updateDockVisibility()
			a.updateDockVisibility()
			if want := []bool{policy == app.DockIconShow}; !reflect.DeepEqual(calls, want) {
				t.Fatalf("initial Dock policy was skipped or reapplied: got %v, want %v", calls, want)
			}
		})
	}
}

func TestDockVisibilityChangesOnlyWhenEffectivePolicyChanges(t *testing.T) {
	a := fixture()
	var calls []bool
	a.lifecycle.setDockVisible = func(visible bool) { calls = append(calls, visible) }
	a.mainWindowVisibilityChanged(true)
	a.state.Settings.DockIcon = app.DockIconFollowWindow
	a.applyNativeSettings() // A new policy can still mean the Dock stays visible.
	a.mainWindowVisibilityChanged(false)
	a.state.Settings.DockIcon = app.DockIconHide
	a.applyNativeSettings()
	a.mainWindowVisibilityChanged(true)
	a.state.Settings.DockIcon = app.DockIconFollowWindow
	a.applyNativeSettings()
	if want := []bool{true, false, true}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("Dock calls did not follow effective visibility: got %v, want %v", calls, want)
	}
}

func TestFollowWindowInitializationWaitsForMainWindowWithoutChangingDock(t *testing.T) {
	for _, dockVisible := range []bool{true, false} {
		name := "cold startup"
		if !dockVisible {
			name = "lightweight resume"
		}
		t.Run(name, func(t *testing.T) {
			a := fixture()
			a.initializing = true
			a.state.Settings.DockIcon = app.DockIconFollowWindow
			a.lifecycle.dockVisible, a.lifecycle.dockVisibilityApplied = dockVisible, true
			var calls []bool
			a.lifecycle.setDockVisible = func(visible bool) { calls = append(calls, visible) }
			a.applyNativeSettings()
			if len(calls) != 0 || a.lifecycle.mainVisible {
				t.Fatalf("pending main window changed the Dock or pretended to be visible: %v", calls)
			}
			a.initializing = false
			a.mainWindowVisibilityChanged(true)
			var want []bool
			if !dockVisible {
				want = []bool{true}
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("showing the main window changed the Dock unexpectedly: got %v, want %v", calls, want)
			}
			a.mainWindowVisibilityChanged(false)
			a.mainWindowVisibilityChanged(true)
			want = append(want, false, true)
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("runtime Dock visibility stopped following the main window: got %v, want %v", calls, want)
			}
		})
	}
}

func TestBackgroundInitializationKeepsWindowsHiddenAndAppliesDockPolicy(t *testing.T) {
	for _, policy := range []string{app.DockIconShow, app.DockIconHide, app.DockIconFollowWindow} {
		t.Run(policy, func(t *testing.T) {
			a, window := quickSearchFixture()
			a.initializing, a.quick.visible = true, false
			a.lifecycle.resumeTarget = "" // Login starts the service without opening a window.
			a.state.Settings.DockIcon = policy
			a.lifecycle.dockVisible, a.lifecycle.dockVisibilityApplied = true, true
			var calls []bool
			a.lifecycle.setDockVisible = func(visible bool) { calls = append(calls, visible) }
			a.applyNativeSettings()
			a.finishUIInitialization()
			var want []bool
			if policy != app.DockIconShow {
				want = []bool{false}
			}
			if !reflect.DeepEqual(calls, want) || a.initializing || a.lifecycle.mainVisible || a.quick.visible || window.shown != 0 || window.focused != 0 {
				t.Fatalf("background initialization opened a window or changed the Dock policy: %v", calls)
			}
		})
	}
}

func TestInitializationSwitchToPanelReconcilesDeferredDockVisibility(t *testing.T) {
	for _, target := range []string{"quick", "tray"} {
		t.Run(target, func(t *testing.T) {
			a, window := quickSearchFixture()
			a.initializing, a.quick.visible, a.busy = true, false, true
			a.state.Settings.DockIcon = app.DockIconFollowWindow
			a.lifecycle.dockVisible, a.lifecycle.dockVisibilityApplied = true, true
			var calls []bool
			a.lifecycle.setDockVisible = func(visible bool) { calls = append(calls, visible) }
			a.applyNativeSettings()
			if len(calls) != 0 {
				t.Fatalf("pending main window hid the Dock before the panel request: %v", calls)
			}
			a.lifecycle.resumeTarget = target
			a.finishUIInitialization()
			mode := quickSearchGlobal
			if target == "tray" {
				mode = quickSearchTray
			}
			if !reflect.DeepEqual(calls, []bool{false}) || a.initializing || a.lifecycle.mainVisible || !a.quick.visible || a.quick.mode != mode || window.shown != 1 || window.focused != 1 || window.focusCalls != 0 {
				t.Fatalf("panel startup changed the manager, repeated focus or retained the Dock: %v", calls)
			}
		})
	}
}

func TestLightweightSuspensionDoesNotReapplyDockVisibility(t *testing.T) {
	for _, policy := range []string{app.DockIconShow, app.DockIconHide, app.DockIconFollowWindow} {
		t.Run(policy, func(t *testing.T) {
			a := fixture()
			a.state.Settings.DockIcon, a.state.Settings.LightweightMode = policy, true
			t.Cleanup(a.stopLightweightTimer)
			var calls []bool
			a.lifecycle.setDockVisible = func(visible bool) { calls = append(calls, visible) }
			a.mainWindowVisibilityChanged(true)
			a.mainWindowVisibilityChanged(false)
			beforeSuspension := len(calls)
			now := time.Now()
			a.lifecycle.hiddenAt = now.Add(-lightweightIdleDelay)
			if !a.suspendLightweightUI(now) {
				t.Fatal("hidden UI did not suspend after one minute")
			}
			// Exercise the native OnClosed handler without creating an OS window.
			// Suspension clears the window reference before Destroy emits OnClosed.
			a.mainWindowClosed(nil)
			a.applyNativeSettings()
			if len(calls) != beforeSuspension {
				t.Fatalf("suspension reapplied the Dock policy and could steal focus: %v", calls)
			}
		})
	}
}

func TestTrayClickUsesConfiguredDestination(t *testing.T) {
	a, window := quickSearchFixture()
	a.quick.visible = false
	a.busy = true
	a.state.Settings.TrayClickAction = app.TrayClickQuickPanel
	a.trayClicked()
	if !a.quick.visible || window.shown != 1 {
		t.Fatal("tray did not open quick panel")
	}
	a.initializing = true
	a.lifecycle.resumeTarget = "quick"
	a.state.Settings.TrayClickAction = app.TrayClickMainWindow
	a.trayClicked()
	if a.quick.visible || a.lifecycle.resumeTarget != "main" {
		t.Fatal("tray main-window action did not dismiss panel and request manager")
	}
}

func TestLightweightModeWaitsForBothSurfacesAndFullMinute(t *testing.T) {
	a := fixture()
	a.state.Settings.LightweightMode = true
	now := time.Now()
	a.updateLightweightTimer(now)
	t.Cleanup(a.stopLightweightTimer)
	if a.lightweightSuspensionReady(now.Add(59*time.Second)) || !a.lightweightSuspensionReady(now.Add(time.Minute)) {
		t.Fatal("lightweight suspension did not wait a full minute")
	}
	a.quick.visible = true
	a.updateLightweightTimer(now.Add(30 * time.Second))
	if !a.lifecycle.hiddenAt.IsZero() || a.lifecycle.timer != nil {
		t.Fatal("visible quick panel did not cancel suspension")
	}
	a.quick.visible = false
	a.updateLightweightTimer(now.Add(40 * time.Second))
	if a.lightweightSuspensionReady(now.Add(time.Minute)) {
		t.Fatal("closing the panel reused the manager's earlier idle deadline")
	}
	a.mainWindowVisibilityChanged(true)
	if !a.lifecycle.hiddenAt.IsZero() || a.lifecycle.timer != nil {
		t.Fatal("showing the manager did not cancel suspension")
	}
	a.state.Settings.LightweightMode = false
	a.lifecycle.mainVisible = false
	a.updateLightweightTimer(now)
	if a.lifecycle.timer != nil {
		t.Fatal("disabled lightweight mode scheduled suspension")
	}
}

func TestLightweightModeProtectsInflightWorkAndUnsavedEditor(t *testing.T) {
	a := fixture()
	a.state.Settings.LightweightMode = true
	now := time.Now()
	a.lifecycle.hiddenAt = now.Add(-time.Minute)
	for _, block := range []func(){
		func() { a.busy = true },
		func() { a.settingsSaving = true },
		func() { a.editor.open = true },
		func() { a.quick.operation = "device-unlock" },
	} {
		block()
		if a.lightweightSuspensionReady(now) {
			t.Fatal("lightweight mode would discard in-flight work or an editor")
		}
		a.busy, a.settingsSaving, a.editor.open, a.quick.operation = false, false, false, ""
	}
}

func TestLightweightSuspensionDropsShortcutUIBindingButKeepsNativeRegistration(t *testing.T) {
	a := fixture()
	registry := &fakeShortcutRegistry{}
	a.shortcut.registry, a.shortcut.registered = registry, app.DefaultGlobalShortcut
	view := ui.NewTester(func(c *ui.Context) { ui.Box(c).Ref(&a.shortcut.ref) }, 100, 100)
	view.Frame()
	if reflect.ValueOf(a.shortcut.ref).IsZero() {
		t.Fatal("fixture did not bind the shortcut UI handle")
	}
	a.state.Settings.LightweightMode = true
	now := time.Now()
	a.lifecycle.hiddenAt = now.Add(-time.Minute)
	if !a.suspendLightweightUI(now) {
		t.Fatal("fixture did not enter lightweight suspension")
	}
	if !reflect.ValueOf(a.shortcut.ref).IsZero() {
		t.Fatal("suspension retained the shortcut handle's old UI engine")
	}
	if a.shortcut.registry != registry || a.shortcut.registered != app.DefaultGlobalShortcut {
		t.Fatal("dropping the shortcut UI binding removed its native registration")
	}
}

func TestLightweightSuspensionReleasesPresentationKeepsServiceAndClipboardAndReloads(t *testing.T) {
	a, window := quickSearchFixture()
	a.quick.visible = false
	a.state.Settings.LightweightMode = true
	a.clipboard = "existing clipboard lease"
	a.sortedEntries, a.filtered, a.cachedResults = a.state.Entries, a.state.Entries, a.state.Entries
	a.cacheReady = true
	a.quick.results, a.quick.cachedResults = a.state.Entries, a.state.Entries
	now := time.Now()
	a.lifecycle.hiddenAt = now.Add(-time.Minute)
	callbacks, requests := make(chan func(), 2), make(chan app.Request, 2)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: app.Data{Configured: true, Settings: a.state.Settings, Entries: []app.Entry{{Path: "reloaded", Name: "Reloaded"}}}}
	})
	if !a.suspendLightweightUI(now) || !a.lifecycle.suspended || window.destroyed != 1 || a.quick.window != nil {
		t.Fatal("idle lightweight mode did not destroy native panel resources")
	}
	if len(a.state.Entries) != 0 || a.detail.Contents != "" || len(a.sortedEntries) != 0 || len(a.quick.cachedResults) != 0 || a.cacheReady {
		t.Fatal("suspension retained presentation entry caches or decrypted detail")
	}
	if a.service == nil || a.clipboard != "existing clipboard lease" {
		t.Fatal("suspension removed a service or clipboard component needed by the browser")
	}
	a.queueMetadataRefresh(true)
	if !a.pendingRefresh || a.busy {
		t.Fatal("background notification reloaded presentation while suspended")
	}
	a.show()
	if a.lifecycle.suspended || !a.initializing {
		t.Fatal("reopening did not start a metadata reload")
	}
	awaitMain(t, callbacks)
	if request := <-requests; request.Action != "configure" {
		t.Fatalf("reopening requested %q instead of reloading metadata", request.Action)
	}
	if a.initializing || a.busy || len(a.state.Entries) != 1 || a.state.Entries[0].Path != "reloaded" {
		t.Fatal("reloaded presentation did not become ready")
	}
}

func TestLaunchAtLoginRegistrationRollsBackWhenSettingsCannotPersist(t *testing.T) {
	for _, saved := range []bool{false, true} {
		t.Run(map[bool]string{false: "failed save", true: "successful save"}[saved], func(t *testing.T) {
			a := fixture()
			a.state.Preview = false
			a.settings, a.settingsSaved = app.DefaultSettings(), app.DefaultSettings()
			a.settings.LaunchAtLogin, a.settingsDirty = true, true
			var registrations []bool
			a.lifecycle.setOpenAtLogin = func(value bool) error { registrations = append(registrations, value); return nil }
			callbacks := make(chan func(), 1)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.service = backendFunc(func(request app.Request) app.Response {
				if !saved {
					return app.Response{Status: "error", Message: "disk is full"}
				}
				return app.Response{Status: "ok", Data: app.Data{Settings: *request.Settings}}
			})
			a.saveSettingsAutomatically()
			awaitMain(t, callbacks)
			want := []bool{true}
			if !saved {
				want = append(want, false)
			}
			if !reflect.DeepEqual(registrations, want) || a.settings.LaunchAtLogin != saved || a.settingsSaved.LaunchAtLogin != saved || a.settingsSaving {
				t.Fatalf("native registration/persistence diverged: registrations=%v, settings=%+v", registrations, a.settings)
			}
		})
	}
}

func TestLaunchAtLoginNativeErrorRestoresToggleWithoutSaving(t *testing.T) {
	a := fixture()
	a.state.Preview = false
	a.settings, a.settingsSaved = app.DefaultSettings(), app.DefaultSettings()
	a.settings.LaunchAtLogin, a.settingsDirty = true, true
	a.lifecycle.setOpenAtLogin = func(bool) error { return errors.New("an app bundle is required") }
	a.service = backendFunc(func(app.Request) app.Response {
		t.Error("native failure must not persist settings")
		return app.Response{}
	})
	a.saveSettingsAutomatically()
	if a.settings.LaunchAtLogin || a.settingsSaving || a.settingsDirty || a.errorText == "" {
		t.Fatal("native login error was not surfaced or toggle was not restored")
	}
}

func TestLaunchAtLoginPreviewNeverChangesOSRegistration(t *testing.T) {
	a := fixture()
	a.lifecycle.setOpenAtLogin = func(bool) error {
		t.Fatal("preview attempted to change the real login registration")
		return nil
	}
	finish, err := a.prepareLaunchAtLogin(true)
	if err != nil {
		t.Fatal(err)
	}
	finish(false)
}

type lifecycleLockBackend struct {
	execute func(app.Request) app.Response
}

func (backend lifecycleLockBackend) Execute(request app.Request) app.Response {
	return backend.execute(request)
}

func (lifecycleLockBackend) IsLocked() bool { return true }

func TestLightweightResumeRetriesConfigurationInvalidatedBySessionLock(t *testing.T) {
	a := fixture()
	a.state.Locked = false
	a.lifecycle.suspended = true
	settings := a.state.Settings
	callbacks, requests := make(chan func(), 2), make(chan app.Request, 2)
	releaseFirst := make(chan struct{})
	a.runOnMain = func(callback func()) { callbacks <- callback }
	first := true
	a.service = lifecycleLockBackend{execute: func(request app.Request) app.Response {
		requests <- request
		if first {
			first = false
			<-releaseFirst
			return app.Response{Status: "ok", Data: app.Data{Configured: true, Locked: false, Settings: settings, Entries: []app.Entry{{Path: "stale"}}}}
		}
		return app.Response{Status: "ok", Data: app.Data{Configured: true, Locked: true, Settings: settings, Entries: []app.Entry{{Path: "current"}}}}
	}}
	a.show()
	select {
	case <-requests:
	case <-time.After(3 * time.Second):
		t.Fatal("resume configuration never started")
	}
	a.handleSessionLock()
	close(releaseFirst)
	awaitMain(t, callbacks)
	if !a.initializing || !a.state.Locked || len(a.state.Entries) == 1 && a.state.Entries[0].Path == "stale" {
		t.Fatal("stale resume response restored an unlocked session")
	}
	select {
	case request := <-requests:
		if request.Action != "configure" {
			t.Fatal("invalidated initialization did not retry configuration")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("session lock left resumed UI stuck initializing")
	}
	awaitMain(t, callbacks)
	if a.initializing || a.busy || !a.state.Locked || len(a.state.Entries) != 1 || a.state.Entries[0].Path != "current" {
		t.Fatal("fresh locked configuration did not finish reopening")
	}
}

func TestInitializationRetryRetainsOnlyItsConfigurationCallback(t *testing.T) {
	a := fixture()
	a.initializing = true
	a.generation = 2
	configured := false
	a.lifecycle.configureDone = func(app.Data) { configured = true }
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		if request.Action != "configure" {
			t.Errorf("initialization retry requested %q", request.Action)
		}
		return app.Response{Status: "ok", Data: app.Data{Settings: app.DefaultSettings(), Locked: true}}
	})
	a.receiveResponse(1, app.Response{Status: "ok"}, func(app.Data) {
		t.Error("a stale entry response's callback was reused for configuration")
	})
	awaitMain(t, callbacks)
	if !configured || a.initializing || a.lifecycle.configureDone != nil {
		t.Fatal("initialization retry lost or retained its configuration callback")
	}
}
