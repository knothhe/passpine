package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/godbus/dbus/v5"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/assets"
	"github.com/knothhe/passpine/client/internal/linuxdesktop"
)

func TestMain(m *testing.M) {
	if directory := os.Getenv("PASSPINE_LINUX_INTERACTIONS_CHILD"); directory != "" {
		runNativeLinuxInteractions(directory)
		return
	}
	os.Exit(m.Run())
}

func runNativeLinuxInteractions(directory string) {
	configureNativeRendering()
	fail := func(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	mygo.App.SetName("Passpine Linux Test")
	mygo.App.SetPath(mygo.PathUserData, directory)
	mygo.App.WhenReady(func() {
		a := fixture()
		snapshot := a.state
		a.service = backendFunc(func(request app.Request) app.Response {
			data := snapshot
			if request.Settings != nil {
				data.Settings = *request.Settings
			}
			return app.Response{Status: "ok", Data: data}
		})
		a.settings = app.DefaultSettings()
		a.settingsSaved = a.settings
		a.settingsObserved = a.settings
		a.settingsLoaded = true
		a.newMainWindow()
		a.lifecycle.nativeReady = true
		tray, err := newNativeTray(mygo.TrayOptions{Icon: assets.MenuBarTemplatePNG, ToolTip: "Passpine"})
		if err != nil {
			fail(err)
		}
		a.tray = tray
		a.installTrayMenu()
		tray.OnClick(a.trayClicked)
		mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.closing = true })
		registry, err := linuxdesktop.NewShortcuts()
		if err != nil {
			fail(err)
		}
		a.shortcut.registry = registry
		initialShortcut := "Ctrl+Alt+F23"
		if os.Getenv("PASSPINE_NATIVE_RECORDING_ONLY") == "1" {
			initialShortcut = "Ctrl+Alt+Shift+P"
		}
		if err := a.changeGlobalShortcut(initialShortcut); err != nil {
			fail(err)
		}
		mygo.App.OnWillQuit(func(*mygo.QuitEvent) { registry.Close(); tray.Destroy() })
		if os.Getenv("PASSPINE_NATIVE_RECORDING_ONLY") == "1" {
			go runNativeRecorder(a, fail)
			return
		}
		go func() {
			conn, err := dbus.ConnectSessionBus()
			if err != nil {
				fail(err)
			}
			defer conn.Close()
			data, err := exec.Command("hyprctl", "-j", "binds").Output()
			if err != nil {
				fail(err)
			}
			var binds []struct{ Key, Description string }
			if err := json.Unmarshal(data, &binds); err != nil {
				fail(err)
			}
			var shortcutName string
			for _, bind := range binds {
				if bind.Key == "F23" && strings.HasPrefix(bind.Description, "Passpine shortcut ") {
					shortcutName = strings.TrimPrefix(bind.Description, "Passpine shortcut ")
				}
			}
			if shortcutName == "" {
				fail(fmt.Errorf("shortcut missing from compositor"))
			}
			var items []string
			if err := conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher").Call("org.freedesktop.DBus.Properties.Get", 0, "org.kde.StatusNotifierWatcher", "RegisteredStatusNotifierItems").Store(&items); err != nil {
				fail(err)
			}
			var trayName string
			for _, item := range items {
				name := strings.SplitN(item, "/", 2)[0]
				var pid uint32
				if conn.BusObject().Call("org.freedesktop.DBus.GetConnectionUnixProcessID", 0, name).Store(&pid) == nil && pid == uint32(os.Getpid()) {
					trayName = name
				}
			}
			if trayName == "" {
				fail(fmt.Errorf("tray missing from panel watcher: %v", items))
			}
			check := func(mode string, visible bool) {
				deadline := time.Now().Add(3 * time.Second)
				for time.Now().Before(deadline) {
					done := make(chan bool, 1)
					mygo.RunOnMain(func() {
						done <- a.quick.visible == visible && (!visible || a.quick.mode == mode && a.quick.window.(*mygo.Window).IsVisible())
					})
					if <-done {
						return
					}
					time.Sleep(20 * time.Millisecond)
				}
				fail(fmt.Errorf("panel did not reach mode %q, visible=%v", mode, visible))
			}
			activateSearch := func() {
				if err := conn.Object(shortcutName, "/Shortcuts").Call("com.knothhe.Passpine.Shortcuts.Activate", 0, "Ctrl+Alt+F23").Err; err != nil {
					fail(err)
				}
			}
			activateSearch()
			check(quickSearchGlobal, true)
			activateSearch()
			check(quickSearchGlobal, false)
			if err := conn.Object(trayName, "/StatusNotifierItem").Call("org.kde.StatusNotifierItem.Activate", 0, int32(0), int32(0)).Err; err != nil {
				fail(err)
			}
			check(quickSearchTray, true)
			done := make(chan error, 1)
			mygo.RunOnMain(func() {
				image, err := a.quick.window.(*mygo.Window).CapturePage()
				if err == nil {
					err = os.WriteFile(filepath.Join(directory, "tray-panel.png"), image, 0600)
				}
				done <- err
			})
			if err := <-done; err != nil {
				fail(err)
			}
			activateSearch()
			check(quickSearchGlobal, true)
			activateSearch()
			check(quickSearchGlobal, false)
			// Exercise the right-click menu's first action through dbusmenu.
			if err := conn.Object(trayName, "/MenuBar").Call("com.canonical.dbusmenu.Event", 0, int32(1), "clicked", dbus.MakeVariant(int32(0)), uint32(0)).Err; err != nil {
				fail(err)
			}
			check(quickSearchTray, true)
			// Reproduce the multi-window path: leave the tray panel for the
			// manager, then for settings, allowing real GTK frames each time.
			for i := 0; i < 3; i++ {
				for _, action := range []func(){a.openQuickSearchInManager, a.openQuickSearchSettings} {
					ready := make(chan bool, 1)
					mygo.RunOnMain(func() { action(); ready <- a.window != nil && a.window.IsVisible() && !a.quick.visible })
					if !<-ready {
						fail(fmt.Errorf("manager did not open from quick panel"))
					}
					time.Sleep(250 * time.Millisecond)
					frame := make(chan error, 1)
					mygo.RunOnMain(func() {
						data, err := a.window.CapturePage()
						if err == nil {
							err = os.WriteFile(filepath.Join(directory, "manager-"+a.page+".png"), data, 0600)
						}
						frame <- err
					})
					if err := <-frame; err != nil {
						fail(err)
					}
					mygo.RunOnMain(func() { a.window.Hide(); a.trayQuickPanel() })
					check(quickSearchTray, true)
				}
			}
			fmt.Println("Linux manager and settings transitions verified")
			fmt.Println("Linux shortcut, tray activation and tray menu verified")
			mygo.RunOnMain(mygo.App.Quit)
		}()
	})
	if err := mygo.App.Run(); err != nil {
		fail(err)
	}
}

func runNativeRecorder(a *application, fail func(error)) {
	ready := make(chan struct{}, 1)
	mygo.RunOnMain(func() {
		a.navigateSettingsCategory("general")
		a.show()
		a.installMenu()
		ready <- struct{}{}
	})
	<-ready
	time.Sleep(300 * time.Millisecond)
	mygo.RunOnMain(func() {
		a.setShortcutRecording(true)
		a.shortcut.ref.Focus()
		a.invalidate()
		ready <- struct{}{}
	})
	<-ready
	time.Sleep(300 * time.Millisecond)
	key := os.Getenv("PASSPINE_NATIVE_RECORDING_KEY")
	if key == "" {
		key = "p"
	}
	if output, err := exec.Command("wtype", "-M", "ctrl", "-M", "alt", "-M", "shift", "-k", key, "-m", "shift", "-m", "alt", "-m", "ctrl").CombinedOutput(); err != nil {
		fail(fmt.Errorf("virtual keyboard: %v: %s", err, output))
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		done := make(chan bool, 1)
		mygo.RunOnMain(func() {
			done <- !a.shortcut.recording && a.shortcut.stopCapture == nil && a.settingsSaved.GlobalShortcut == "Ctrl+Alt+Shift+"+strings.ToUpper(key)
		})
		if <-done {
			mygo.RunOnMain(func() { a.setShortcutRecording(true); a.shortcut.ref.Focus(); a.invalidate(); ready <- struct{}{} })
			<-ready
			if output, err := exec.Command("wtype", "-k", "Escape").CombinedOutput(); err != nil {
				fail(fmt.Errorf("cancel recorder: %v: %s", err, output))
			}
			cancelled := make(chan bool, 1)
			mygo.RunOnMain(func() { cancelled <- !a.shortcut.recording && a.shortcut.stopCapture == nil })
			if !<-cancelled {
				fail(fmt.Errorf("Escape did not release shortcut capture"))
			}
			fmt.Println("native shortcut recording verified")
			mygo.RunOnMain(mygo.App.Quit)
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	mygo.RunOnMain(func() {
		fail(fmt.Errorf("recording=%v, saved=%s, error=%s, window-focused=%v", a.shortcut.recording, a.settingsSaved.GlobalShortcut, a.shortcut.errorText, a.window.IsFocused()))
	})
}

func TestNativeLinuxShortcutRecording(t *testing.T) {
	if os.Getenv("PASSPINE_NATIVE_SMOKE") != "1" || !linuxdesktop.HyprlandSession() {
		t.Skip("requires an interactive Wayland desktop and wtype")
	}
	if _, err := exec.LookPath("wtype"); err != nil {
		t.Skip("wtype is not installed")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, key := range []string{"p", "F24"} {
		t.Run(key, func(t *testing.T) {
			cmd := exec.CommandContext(ctx, executable)
			cmd.Env = append(os.Environ(), "PASSPINE_LINUX_INTERACTIONS_CHILD="+t.TempDir(), "PASSPINE_NATIVE_RECORDING_ONLY=1", "PASSPINE_NATIVE_RECORDING_KEY="+key)
			output, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "native shortcut recording verified") {
				t.Fatalf("recording failed: %v\n%s", err, output)
			}
		})
	}
}

func TestNativeLinuxInteractions(t *testing.T) {
	if os.Getenv("PASSPINE_NATIVE_SMOKE") != "1" || !linuxdesktop.HyprlandSession() {
		t.Skip("requires an interactive Hyprland desktop")
	}
	directory := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable)
	cmd.Env = append(os.Environ(), "PASSPINE_LINUX_INTERACTIONS_CHILD="+directory)
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "tray menu verified") {
		t.Fatalf("Linux native interaction failed: %v\n%s", err, output)
	}
	if destination := os.Getenv("PASSPINE_NATIVE_SMOKE_SCREENSHOT"); destination != "" {
		data, err := os.ReadFile(filepath.Join(directory, "tray-panel.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
