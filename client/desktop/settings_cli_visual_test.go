package desktop

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/internal/commandlink"
)

func TestCLISettingsOperationKeepsDetailsInPlace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not start command-link operations")
	}
	for _, operation := range []string{"install", "uninstall", "status", "retry"} {
		for _, variant := range []struct {
			locale string
			width  int
			dark   bool
		}{{"zh-CN", 1120, false}, {"en", 460, true}} {
			t.Run(fmt.Sprintf("%s/%s/%d", operation, variant.locale, variant.width), func(t *testing.T) {
				a, m, callbacks := cliSettingsFixture(t)
				if operation == "uninstall" {
					if err := m.Install(true); err != nil {
						t.Fatal(err)
					}
				}
				a.state.Settings.Language = variant.locale
				a.page, a.settingsState.Category = "settings", "cli"
				a.cli.loaded, a.cli.directory, a.cli.helpExpanded = true, m.Directory, true
				var err error
				a.cli.status, err = m.Status()
				if err != nil {
					t.Fatal(err)
				}
				if operation == "retry" {
					a.cli.err = &commandlink.ConflictError{Path: m.Directory + "/pass"}
				}
				started, release := make(chan struct{}), make(chan struct{})
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
				a.cli.manager = func() (commandlink.Manager, error) {
					close(started)
					<-release
					return m, nil
				}
				view := ui.NewTester(testView(a), variant.width, 760)
				view.SetDark(variant.dark)
				view.Advance(time.Second)
				labels := []string{a.t("安装位置"), a.t("使用说明")}
				bounds := make(map[string]ui.Rect)
				pixels := make(map[string]*image.RGBA)
				for _, label := range labels {
					r := visualBounds(t, view, label)
					bounds[label] = r
					area := image.Rect(int(r.X), int(r.Y), int(r.X+r.W), int(r.Y+r.H))
					pixels[label] = image.NewRGBA(area)
					draw.Draw(pixels[label], area, view.Image(), area.Min, draw.Src)
				}
				button := a.t("安装命令")
				if operation == "uninstall" {
					button = a.t("卸载")
				}
				if operation == "status" {
					button = a.t("刷新状态")
				}
				if err := view.Click(button); err != nil {
					t.Fatal(err)
				}
				<-started
				for _, elapsed := range []time.Duration{0, 16 * time.Millisecond, 300 * time.Millisecond} {
					view.Advance(elapsed)
					after := view.Image()
					for _, label := range labels {
						if got := visualBounds(t, view, label); got != bounds[label] {
							t.Fatalf("%s moved while %s was pending: before=%v after=%v", label, operation, bounds[label], got)
						}
						before := pixels[label]
						for y := before.Rect.Min.Y; y < before.Rect.Max.Y; y++ {
							for x := before.Rect.Min.X; x < before.Rect.Max.X; x++ {
								if after.RGBAAt(x, y) != before.RGBAAt(x, y) {
									t.Fatalf("%s flickered during %s", label, operation)
								}
							}
						}
					}
				}
				close(release)
				awaitMain(t, callbacks)
				view.Frame()
				if operation != "retry" {
					for _, label := range labels {
						if got := visualBounds(t, view, label); got != bounds[label] {
							t.Fatalf("%s moved after %s: %v -> %v", label, operation, bounds[label], got)
						}
					}
				}
				if !a.cli.helpExpanded || a.cli.operation != "" || a.cli.err != nil {
					t.Fatal("completion lost the details state or operation result")
				}
			})
		}
	}
}

func TestCLIStatusChangesRenderWithoutIntermediateAnimation(t *testing.T) {
	a := fixture()
	a.cli.loaded = true
	entry := commandlink.Entry{Name: "passpine-dev"}
	view := ui.NewTester(func(c *ui.Context) { a.cliStatusBadge(c, entry) }, 320, 100)
	view.Advance(time.Second)
	entry.Exists, entry.Owned, entry.Removable = true, true, true
	view.Frame()
	complete := append([]byte(nil), view.Image().Pix...)
	for _, elapsed := range []time.Duration{16 * time.Millisecond, 80 * time.Millisecond, 300 * time.Millisecond} {
		view.Advance(elapsed)
		if !bytes.Equal(complete, view.Image().Pix) {
			t.Fatal("the completed CLI status changed pixels after its result was rendered")
		}
	}
}

func TestCLIShortOperationKeepsEntirePagePixels(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not start command-link operations")
	}
	for _, hover := range []string{"", "安装命令", "卸载", "刷新状态", "同时安装 pass 别名"} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("hover=%s/dark=%t", hover, dark), func(t *testing.T) {
				a := fixture()
				a.state.Preview = false
				a.page, a.settingsState.Category = "settings", "cli"
				a.cli.loaded, a.cli.directory = true, "/Users/demo/.local/bin"
				a.cli.status = commandlink.Status{
					Main:  commandlink.Entry{Name: "passpine-dev", Exists: true, Owned: true, Removable: true},
					Alias: commandlink.Entry{Name: "pass", Exists: true, Owned: true, Removable: true},
				}
				view := ui.NewTester(testView(a), 1120, 760)
				view.SetDark(dark)
				if hover != "" {
					r := visualBounds(t, view, a.t(hover))
					view.Move(r.X+r.W/2, r.Y+r.H/2)
				}
				view.Advance(time.Second)
				before := append([]byte(nil), view.Image().Pix...)
				a.cli.operation = "install"
				a.cli.started = view.Now()
				for _, elapsed := range []time.Duration{0, 16 * time.Millisecond, 80 * time.Millisecond} {
					view.Advance(elapsed)
					view.Frame()
					if !bytes.Equal(before, view.Image().Pix) {
						t.Fatal("starting a short operation changed the card's controls or icon")
					}
				}
				a.cli.operation = ""
				for _, elapsed := range []time.Duration{0, 16 * time.Millisecond, 80 * time.Millisecond, 300 * time.Millisecond} {
					view.Advance(elapsed)
					view.Frame()
					if !bytes.Equal(before, view.Image().Pix) {
						t.Fatal("a no-op completion changed page pixels")
					}
				}
			})
		}
	}
}

func TestCLISlowOperationOnlyChangesRefreshIndicator(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no command-link worker")
	}
	a := fixture()
	a.state.Preview = false
	a.cli.loaded = true
	a.cli.status = commandlink.Status{Main: commandlink.Entry{Name: "passpine-dev"}, Alias: commandlink.Entry{Name: "pass"}}
	view := ui.NewTester(a.cliSettingsView, 700, 600)
	refresh := visualBounds(t, view, a.t("刷新状态"))
	before := append([]byte(nil), view.Image().Pix...)
	a.cli.operation, a.cli.started = "install", view.Now()
	view.Frame()
	view.Advance(cliProgressDelay + time.Millisecond)
	if _, ok := view.Find(a.t("正在安装命令…")); !ok {
		t.Fatal("a slow operation did not show local progress")
	}
	after := view.Image()
	for y := 0; y < after.Rect.Dy(); y++ {
		for x := 0; x < after.Rect.Dx(); x++ {
			if float32(x) >= refresh.X && float32(x) < refresh.X+refresh.W && float32(y) >= refresh.Y && float32(y) < refresh.Y+refresh.H {
				continue
			}
			i := after.PixOffset(x, y)
			if !bytes.Equal(before[i:i+4], after.Pix[i:i+4]) {
				t.Fatalf("slow progress changed another control at %d,%d", x, y)
			}
		}
	}
	a.cli.operation = ""
	view.Frame()
	if !bytes.Equal(before, view.Image().Pix) {
		t.Fatal("completed progress did not restore the same controls")
	}
}
