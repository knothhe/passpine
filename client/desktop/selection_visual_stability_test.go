package desktop

import (
	"fmt"
	"image"
	"image/draw"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func selectionPixels(view *ui.Tester, bounds ui.Rect) *image.RGBA {
	r := image.Rect(int(bounds.X), int(bounds.Y), int(bounds.X+bounds.W), int(bounds.Y+bounds.H))
	copy := image.NewRGBA(r)
	draw.Draw(copy, r, view.Image(), r.Min, draw.Src)
	return copy
}

func assertSelectionPixels(t *testing.T, view *ui.Tester, bounds ui.Rect, before *image.RGBA, label string) {
	t.Helper()
	after := view.Image()
	for y := before.Rect.Min.Y; y < before.Rect.Max.Y; y++ {
		for x := before.Rect.Min.X; x < before.Rect.Max.X; x++ {
			if got, want := after.RGBAAt(x, y), before.RGBAAt(x, y); got != want {
				t.Fatalf("%s changed pixels during detail read at (%d,%d): got %v, want %v (bounds %v)", label, x, y, got, want, bounds)
			}
		}
	}
}

func TestSelectionKeepsControlAndBodyPixelsUntilReplacement(t *testing.T) {
	for _, language := range []string{"en", "zh-CN"} {
		for _, dark := range []bool{false, true} {
			for _, width := range []int{860, 1120} {
				t.Run(fmt.Sprintf("%s/dark=%t/width=%d", language, dark, width), func(t *testing.T) {
					a := fixture()
					a.state.Settings.Language = language
					a.detail.Notes = strings.Repeat("Visible note stays in place. ", 6)
					original, state := a.detail, a.state
					callbacks := make(chan func(), 1)
					a.runOnMain = func(callback func()) { callbacks <- callback }
					a.service = backendFunc(func(request app.Request) app.Response {
						data := state
						data.Entry, data.Username, data.URL, data.Notes = request.Entry, "new-account@example.test", "https://figma.com", "Replacement note."
						return app.Response{Status: "ok", Data: data}
					})
					view := ui.NewTester(testView(a), width, 760)
					view.SetDark(dark)
					view.Advance(time.Second)
					labels := []string{"切换密码库", "选择", "条目详情"}
					if width > 960 {
						labels = append(labels, "清理空文件夹")
					}
					bounds := make(map[string]ui.Rect)
					pixels := make(map[string]*image.RGBA)
					for _, key := range labels {
						label := a.t(key)
						r, found := view.Find(label)
						if !found || r.W <= 0 || r.H <= 0 {
							t.Fatalf("missing region %q: %v", label, r)
						}
						// Local slow-read feedback sits below the retained detail.
						if key == "条目详情" {
							r.H -= 64
						}
						bounds[key], pixels[key] = r, selectionPixels(view, r)
					}
					managerResizeScreenshot(t, view, fmt.Sprintf("selection-before-%s-%t-%d", language, dark, width))
					a.selectEntry("工作/Figma")
					for _, delayed := range []bool{false, true} {
						if delayed {
							a.detailLoadingStarted = view.Now().Add(-detailLoadingDelay - time.Millisecond)
						}
						view.Frame()
						for _, label := range labels {
							assertSelectionPixels(t, view, bounds[label], pixels[label], a.t(label))
						}
						managerResizeScreenshot(t, view, fmt.Sprintf("selection-pending-%s-%t-%d-%t", language, dark, width, delayed))
					}
					awaitMain(t, callbacks)
					view.Frame()
					for _, label := range labels {
						if label != "条目详情" {
							assertSelectionPixels(t, view, bounds[label], pixels[label], a.t(label))
						}
					}
					if !view.HasText("Replacement note.") || view.HasText(original.Notes) || view.HasText(a.t("正在读取账号…")) {
						t.Fatal("completed read did not replace the retained body in one frame")
					}
					body := bounds["条目详情"]
					complete := selectionPixels(view, body)
					for _, elapsed := range []time.Duration{16 * time.Millisecond, 60 * time.Millisecond, 220 * time.Millisecond} {
						view.Advance(elapsed)
						assertSelectionPixels(t, view, body, complete, "completed detail")
					}
				})
			}
		}
	}
}

func TestSelectionProtectsActionsWithoutDimming(t *testing.T) {
	a := fixture()
	a.state.VaultID = "current"
	a.state.Vaults = []app.VaultInfo{{ID: "current", Name: "Current"}, {ID: "other", Name: "Other"}}
	state, original := a.state, a.detail
	requests := make(chan app.Request, 8)
	callbacks := make(chan func(), 8)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		data := state
		data.Entry, data.Password, data.Username, data.URL = request.Entry, "replacement-secret", original.Username, original.URL
		return app.Response{Status: "ok", Data: data}
	})
	view := ui.NewTester(testView(a), 1120, 760)
	a.selectEntry("工作/Figma")
	view.Frame()
	for _, label := range []string{"清理空文件夹", "选择", "编辑条目", "移到回收站", "显示全部字段", "显示或隐藏密码", "复制用户名", "复制密码", "打开网站", "复制密码 Figma"} {
		if err := view.Click(a.t(label)); err != nil {
			t.Fatal(err)
		}
		view.Key(0, ui.KeyEnter)
		view.Key(0, ui.KeySpace)
	}
	if a.editor.open || a.deleteOpen || a.reveal || len(a.revealedFields) != 0 || a.batch.active || len(view.OpenedURLs()) != 0 {
		t.Fatal("pending selection allowed a stale credential or folder action")
	}
	if err := view.Click(a.t("切换密码库")); err != nil {
		t.Fatal(err)
	}
	if view.HasText(a.t("密码库选项")) {
		if err := view.Click(a.t("切换到密码库 %s", "Other")); err != nil {
			t.Fatal(err)
		}
		view.Key(0, ui.KeyDown)
		view.Key(0, ui.KeyEnter)
		view.Key(0, ui.KeyEscape)
	}
	if err := view.Click(a.t("切换密码库")); err != nil {
		t.Fatal(err)
	}
	view.Key(0, ui.KeyEnd)
	view.Key(0, ui.KeyEnter)
	view.Key(0, ui.KeyEscape)
	if a.setup || a.state.VaultID != "current" || a.vaultSelection != "current" {
		t.Fatal("pending selection switched or opened a vault")
	}
	awaitMain(t, callbacks)
	view.Frame()
	if len(requests) != 1 || a.detail.Entry != "工作/Figma" {
		t.Fatal("protected clicks dispatched another request or cancelled the detail read")
	}
	if err := view.Click(a.t("显示或隐藏密码")); err != nil {
		t.Fatal(err)
	}
	if !view.HasText("replacement-secret") {
		t.Fatal("detail action did not become usable after the read")
	}
}
