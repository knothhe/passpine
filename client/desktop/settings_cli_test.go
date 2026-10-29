package desktop

import (
	"bytes"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/internal/commandlink"
)

func cliSettingsFixture(t *testing.T) (*application, commandlink.Manager, chan func()) {
	t.Helper()
	a := fixture()
	a.state.Preview = false
	root := t.TempDir()
	executable := filepath.Join(root, "Application with spaces")
	if err := os.WriteFile(executable, []byte("application"), 0o700); err != nil {
		t.Fatal(err)
	}
	executable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	m := commandlink.Manager{Executable: executable, Directory: filepath.Join(root, "bin with spaces"), Name: "passpine-dev"}
	a.cli.manager = func() (commandlink.Manager, error) { return m, nil }
	callbacks := make(chan func(), 10)
	a.runOnMain = func(f func()) { callbacks <- f }
	return a, m, callbacks
}

func TestCLISettingsInstallStatusAliasAndUninstall(t *testing.T) {
	a, m, callbacks := cliSettingsFixture(t)
	if runtime.GOOS == "windows" {
		a.manageCLI("install")
		view := ui.NewTester(a.cliSettingsView, 700, 600)
		if !view.HasText(a.t("Windows 版暂不支持命令行，请在 macOS 或 Linux 上使用 %s。", a.t("命令行工具"))) || a.cli.operation != "" {
			t.Fatal("Windows did not explain the unsupported feature")
		}
		if _, err := os.Stat(m.Directory); !os.IsNotExist(err) {
			t.Fatal("Windows created a command directory")
		}
		return
	}
	a.settingsState.Category = "cli"
	a.loadPreferencesPage("settings")
	awaitMain(t, callbacks)
	view := ui.NewTester(a.cliSettingsView, 700, 650)
	if !a.cli.loaded || a.cli.status.Main.Exists || !view.HasText("未安装") {
		t.Fatal("opening settings did not inspect commands")
	}
	if err := view.Click("安装命令"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	view.Frame()
	if !a.cli.status.Main.Removable || a.cli.status.Alias.Exists || !view.HasText("已安装") {
		t.Fatal("installation did not respect the default alias choice")
	}
	if err := view.Click("同时安装 pass 别名"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(m.Directory, commandlink.Alias)); !os.IsNotExist(err) {
		t.Fatal("changing the choice installed the alias immediately")
	}
	if err := view.Click("安装命令"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	view.Frame()
	if !a.cli.status.Alias.Removable {
		t.Fatal("selected alias was not installed")
	}
	if err := os.Remove(filepath.Join(m.Directory, commandlink.Alias)); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("刷新状态"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	view.Frame()
	if a.cli.status.Alias.Exists {
		t.Fatal("manual refresh did not detect an external change")
	}
	if err := view.Click("卸载"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	view.Frame()
	if a.cli.status.Main.Exists || a.cli.status.Alias.Exists || !view.HasText("未安装") {
		t.Fatal("uninstall did not refresh status")
	}
}

func TestCLISettingsConflictAndRetryPreserveAliasChoice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix link operation; Windows UI is covered separately")
	}
	a, m, callbacks := cliSettingsFixture(t)
	if err := os.MkdirAll(m.Directory, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(m.Directory, commandlink.Alias)
	if err := os.WriteFile(alias, []byte("foreign pass"), 0o700); err != nil {
		t.Fatal(err)
	}
	a.cli.includeAlias = true
	a.manageCLI("install")
	awaitMain(t, callbacks)
	view := ui.NewTester(a.cliSettingsView, 700, 650)
	if !view.HasText(a.t("%s 已存在且不是当前 Passpine，未覆盖。", alias)) || !a.cli.includeAlias || !a.cli.status.Alias.Exists || a.cli.status.Main.Exists {
		t.Fatal("conflict lost the choice or misreported actual status")
	}
	if data, _ := os.ReadFile(alias); string(data) != "foreign pass" {
		t.Fatal("conflict overwrote pass")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("安装命令"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	if a.cli.err != nil || !a.cli.status.Alias.Removable {
		t.Fatal("retry did not install the selected alias")
	}
}

func TestCLIInstallClickKeepsControlsAndCapturesAliasDraft(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows rejects command-link installation")
	}
	a, m, callbacks := cliSettingsFixture(t)
	if err := m.Install(false); err != nil {
		t.Fatal(err)
	}
	a.cli.loaded, a.cli.directory = true, m.Directory
	var err error
	a.cli.status, err = m.Status()
	if err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}, 2), make(chan struct{})
	var calls atomic.Int32
	a.cli.manager = func() (commandlink.Manager, error) {
		calls.Add(1)
		started <- struct{}{}
		<-release
		return m, nil
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		if a.cli.operation != "" {
			awaitMain(t, callbacks)
		}
	})
	view := ui.NewTester(a.cliSettingsView, 700, 600)
	button := visualBounds(t, view, a.t("安装命令"))
	view.Move(button.X+button.W/2, button.Y+button.H/2)
	before := append([]byte(nil), view.Image().Pix...)
	if err := view.Click(a.t("安装命令")); err != nil {
		t.Fatal(err)
	}
	<-started
	a.cli.started = view.Now()
	view.Frame()
	if !bytes.Equal(before, view.Image().Pix) {
		t.Fatal("clicking Install changed the controls before a result arrived")
	}
	for _, label := range []string{"安装命令", "卸载", "刷新状态"} {
		if err := view.Click(a.t(label)); err != nil {
			t.Fatal(err)
		}
		view.Key(0, ui.KeyEnter)
		view.Key(0, ui.KeySpace)
	}
	if calls.Load() != 1 || a.cli.operation != "install" {
		t.Fatal("repeated actions started another worker")
	}
	if err := view.Click(a.t("同时安装 pass 别名")); err != nil {
		t.Fatal(err)
	}
	close(release)
	awaitMain(t, callbacks)
	view.Frame()
	if !a.cli.includeAlias || a.cli.status.Alias.Exists {
		t.Fatal("editing the next install's choice changed the in-flight request")
	}
	if err := view.Click(a.t("安装命令")); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	if !a.cli.status.Alias.Removable || calls.Load() != 2 {
		t.Fatal("the next install did not use the retained alias choice")
	}
}

func TestCLISettingsWorkerSurvivesNavigationAndLanguageChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows rejects link workers; covered by the platform test")
	}
	a, m, callbacks := cliSettingsFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	a.cli.manager = func() (commandlink.Manager, error) {
		calls.Add(1)
		close(started)
		<-release
		return m, nil
	}
	a.manageCLI("install")
	<-started
	a.manageCLI("uninstall") // Repeated actions must not start another worker.
	a.settings.Language = "en"
	a.hidePreferences()
	a.generation++ // A vault change does not invalidate this app-wide result.
	view := ui.NewTester(a.cliSettingsView, 700, 650)
	a.cli.started = view.Now().Add(-cliProgressDelay)
	view.Frame()
	if _, ok := view.Find("Installing commands…"); !ok || calls.Load() != 1 {
		t.Fatal("language change restarted or hid the operation")
	}
	close(release)
	awaitMain(t, callbacks)
	view.Frame()
	if !view.HasText("Installed") || !a.cli.status.Main.Removable || a.cli.operation != "" {
		t.Fatal("navigation discarded the operation result")
	}
	a.settings.Language = "zh-CN"
	view.Frame()
	if !view.HasText("已安装") || calls.Load() != 1 {
		t.Fatal("completed feedback did not follow the current language")
	}
}

func TestCLISettingsPreviewAndRepaintDoNotTouchFilesystem(t *testing.T) {
	a := fixture()
	a.cli.manager = func() (commandlink.Manager, error) {
		t.Error("preview or repaint accessed the filesystem")
		return commandlink.Manager{}, errors.New("unexpected")
	}
	a.settingsState.Category = "cli"
	a.loadPreferencesPage("settings")
	a.manageCLI("install")
	a.manageCLI("uninstall")
	view := ui.NewTester(a.cliSettingsView, 700, 500)
	view.Frame()
	if runtime.GOOS != "windows" && !view.HasText("预览模式不会安装或移除命令链接。") {
		t.Fatal("preview restriction is missing")
	}
	a.state.Preview = false
	view.Frame()
	if a.cli.operation != "" {
		t.Fatal("rendering started a worker")
	}
}

func TestCLISettingsReadFailureCanRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no status worker; covered separately")
	}
	a, m, callbacks := cliSettingsFixture(t)
	a.cli.manager = func() (commandlink.Manager, error) { return commandlink.Manager{}, errors.New("read failure") }
	a.manageCLI("status")
	awaitMain(t, callbacks)
	view := ui.NewTester(a.cliSettingsView, 700, 650)
	if a.cli.loaded || !view.HasText("命令行操作失败：read failure") {
		t.Fatal("read failure appeared as not installed")
	}
	a.cli.manager = func() (commandlink.Manager, error) { return m, nil }
	if err := view.Click("刷新状态"); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	if !a.cli.loaded || a.cli.err != nil {
		t.Fatal("status retry did not recover")
	}
}

func TestCLISettingsQuitAndUpdateWaitForWorker(t *testing.T) {
	a, callbacks, count := quitFixture(t)
	a.cli.operation = "install"
	if !a.updateInstallBlocked() {
		t.Fatal("update could replace an application during link installation")
	}
	a.requestQuit()
	if a.closing || a.quit.draining || *count != 0 {
		t.Fatal("quit did not wait for the link operation")
	}
	a.cli.operation = ""
	a.continueQuit()
	awaitMain(t, callbacks)
	if !a.quit.ready || *count != 1 {
		t.Fatal("quit did not resume after installation")
	}
}

func TestCLISettingsHasIndependentTabAndRefreshesOnEntry(t *testing.T) {
	a, _, callbacks := cliSettingsFixture(t)
	a.page = "settings"
	view := ui.NewTester(testView(a), 1120, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	if _, ok := view.Find(a.t("安装命令")); ok {
		t.Fatal("General still includes CLI controls")
	}
	if err := view.Click(a.t("命令行")); err != nil {
		t.Fatal(err)
	}
	if a.settingsState.Category != "cli" {
		t.Fatal("sidebar did not open the independent CLI tab")
	}
	if runtime.GOOS != "windows" {
		awaitMain(t, callbacks)
		view.Frame()
		if !a.cli.loaded || !view.HasText(a.t("安装命令")) {
			t.Fatal("entering the tab did not load its actual command status")
		}
	}
	if err := view.Click(a.t("通用")); err != nil {
		t.Fatal(err)
	}
	if a.cli.operation != "" {
		t.Fatal("entering General started another command inspection")
	}
	if err := view.Click(a.t("搜索设置")); err != nil {
		t.Fatal(err)
	}
	view.Type("CLI")
	if !view.HasText(a.t("命令行")) || runtime.GOOS != "windows" && !view.HasText(a.t("安装命令")) {
		t.Fatal("CLI settings are no longer searchable")
	}
}

func TestCLISettingsUsageTipsAreOptionalAndPreserveChoices(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows only shows the unsupported state")
	}
	a, _, _ := cliSettingsFixture(t)
	a.cli.loaded, a.cli.directory = true, "/Users/demo/.local/bin"
	view := ui.NewTester(a.cliSettingsView, 500, 650)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	hint := a.t("若终端找不到命令，将 %s 加入 PATH。", a.cli.directory)
	if bounds, ok := view.Find(hint); ok && bounds.H > 0 {
		t.Fatal("optional instructions clutter the initial page")
	}
	if err := view.Click(a.t("同时安装 pass 别名")); err != nil {
		t.Fatal(err)
	}
	if err := view.Click(a.t("使用说明")); err != nil {
		t.Fatal(err)
	}
	view.Advance(time.Second)
	if !a.cli.helpExpanded || !view.HasText(hint) || !a.cli.includeAlias {
		t.Fatal("opening the instructions lost the install choice or hid PATH help")
	}
	a.settings.Language = "en"
	view.Frame()
	if !a.cli.helpExpanded || !a.cli.includeAlias || !view.HasText("Usage tips") || a.cli.operation != "" {
		t.Fatal("language change reset the page or performed a filesystem action")
	}
}

func TestCLISettingsTabAndLayout(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, variant := range []struct {
			width int
			dark  bool
		}{{460, false}, {460, true}, {1120, false}, {1120, true}} {
			width := variant.width
			a := fixture()
			a.state.Preview = false
			a.state.Settings.Language, a.state.Settings.FontScale = locale, 120
			a.page, a.settingsState.Category = "settings", "cli"
			a.cli.loaded = true
			a.cli.directory = "/Users/demo/.local/bin"
			a.cli.status = commandlink.Status{Main: commandlink.Entry{Name: "passpine-dev", Path: "/Applications/Passpine Dev.app/Contents/MacOS/passpine", Exists: true, Owned: true, Removable: true}, Alias: commandlink.Entry{Name: "pass", Path: "/Users/demo/.local/bin/pass"}}
			view := ui.NewTester(testView(a), width, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			view.SetDark(variant.dark)
			view.Advance(time.Second)
			if !view.HasText(a.t("命令行")) {
				t.Fatal("CLI tab is missing its content")
			}
			if runtime.GOOS != "windows" {
				for _, label := range []string{"安装命令", "卸载", "刷新状态", "同时安装 pass 别名"} {
					bounds, ok := view.Find(a.t(label))
					if !ok || bounds.X < 0 || bounds.X+bounds.W > float32(width)+1 || bounds.Y+bounds.H > 760 {
						t.Fatalf("%s clipped at %s/%d: %v", label, locale, width, bounds)
					}
				}
			}
			if dir := os.Getenv("PASSPINE_CLI_SCREENSHOT_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("cli-%s-%d-dark-%t.png", locale, width, variant.dark)))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, view.Image())
				closeErr := f.Close()
				if err != nil || closeErr != nil {
					t.Fatal(err, closeErr)
				}
			}
		}
	}
}
