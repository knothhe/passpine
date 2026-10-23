package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func managerResizeFixture(width int) (*application, *ui.Tester) {
	a := fixture()
	view := ui.NewTester(testView(a), width, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	return a, view
}

func managerResizeListBounds(t *testing.T, view *ui.Tester) ui.Rect {
	t.Helper()
	for _, label := range []string{"条目列表", "Entry list", "条目数量", "Entry count"} {
		if bounds, found := view.Find(label); found && bounds.W > 0 && bounds.H > 0 {
			// Recovery's pane has a one-DIP right border outside its footer.
			// The separator center is the actual column edge on both pages.
			for _, resizeLabel := range []string{"调整条目列表宽度", "Resize entry list"} {
				if handle, ok := view.Find(resizeLabel); ok {
					bounds.W = handle.X + handle.W/2 - bounds.X
					break
				}
			}
			return bounds
		}
	}
	t.Fatal("the entry list footer is missing or has no visible area")
	return ui.Rect{}
}

func managerResizeScreenshot(t *testing.T, view *ui.Tester, name string) {
	t.Helper()
	directory := os.Getenv("PASSPINE_RESIZE_SCREENSHOTS")
	if directory == "" {
		return
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(directory, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, view.Image()); err != nil {
		t.Fatal(err)
	}
}

func dragManagerListTo(t *testing.T, a *application, view *ui.Tester, width float32) {
	t.Helper()
	view.Advance(time.Second)
	list := managerResizeListBounds(t, view)
	handle := visualBounds(t, view, a.t("调整条目列表宽度"))
	x, y := handle.X+handle.W/2, handle.Y+handle.H/2
	delta := width - list.W
	view.Press(x, y)
	view.Move(x+delta/2, y)
	view.Move(x+delta, y)
	view.Release(x+delta, y)
}

func assertManagerListWidth(t *testing.T, view *ui.Tester, want float32) {
	t.Helper()
	if got := managerResizeListBounds(t, view).W; !visualNear(got, want) {
		t.Fatalf("list width = %g, want %g", got, want)
	}
}

func TestManagerResizePointerKeyboardLimitsAndResponsiveReset(t *testing.T) {
	a, view := managerResizeFixture(1120)
	assertManagerListWidth(t, view, 300)
	handle := visualBounds(t, view, a.t("调整条目列表宽度"))
	list := managerResizeListBounds(t, view)
	if !visualNear(handle.W, 6) || !visualNear(handle.X+handle.W/2, list.X+list.W) {
		t.Fatalf("resize handle is not centered over the column boundary: handle=%v list=%v", handle, list)
	}
	view.Move(handle.X+handle.W/2, handle.Y+handle.H/2)
	if view.Cursor() != ui.CursorResizeEW {
		t.Fatal("the split boundary does not expose the horizontal resize cursor")
	}
	dragManagerListTo(t, a, view, 380)
	assertManagerListWidth(t, view, 380)
	if a.managerListPreferredWidth != 380 {
		t.Fatal("pointer movement did not retain the user's list width")
	}
	view.Advance(time.Second)
	if err := view.Click(a.t("调整条目列表宽度")); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		mods ui.Modifiers
		key  ui.Key
		want float32
	}{
		{0, ui.KeyRight, 390}, {0, ui.KeyLeft, 380},
		{ui.Shift, ui.KeyRight, 420}, {ui.Shift, ui.KeyLeft, 380},
		{0, ui.KeyHome, 240}, {0, ui.KeyLeft, 240},
		{0, ui.KeyEnd, 480}, {0, ui.KeyRight, 480},
	} {
		view.Key(step.mods, step.key)
		assertManagerListWidth(t, view, step.want)
		if !view.Focused(a.t("调整条目列表宽度")) {
			t.Fatal("keyboard resizing lost splitter focus")
		}
	}
	dragManagerListTo(t, a, view, 100)
	assertManagerListWidth(t, view, 240)
	dragManagerListTo(t, a, view, 700)
	assertManagerListWidth(t, view, 480)
	managerResizeScreenshot(t, view, "manager-list-480")
	view.Advance(time.Second)
	if err := view.Click(a.t("调整条目列表宽度")); err != nil {
		t.Fatal(err)
	}
	view.Advance(100 * time.Millisecond)
	if err := view.Click(a.t("调整条目列表宽度")); err != nil {
		t.Fatal(err)
	}
	assertManagerListWidth(t, view, 300)
	if a.managerListPreferredWidth != 0 {
		t.Fatal("double-click did not restore responsive defaults")
	}
	view.SetSize(860, 760)
	assertManagerListWidth(t, view, 280)
}

func TestManagerResizeWindowAndSidebarClampWithoutLosingPreference(t *testing.T) {
	a, view := managerResizeFixture(1120)
	dragManagerListTo(t, a, view, 480)
	view.SetSize(961, 760)
	want := 961 - a.shell.LeftWidth() - 360 - 2*(managerFrameInset+managerFrameBorder)
	assertManagerListWidth(t, view, want)
	managerResizeScreenshot(t, view, "manager-list-clamped-961")
	if a.managerListPreferredWidth != 480 {
		t.Fatal("a narrower viewport replaced the user's saved width")
	}
	view.Advance(time.Second)
	if err := view.Click(a.t("调整条目列表宽度")); err != nil {
		t.Fatal(err)
	}
	view.Key(0, ui.KeyRight)
	if a.managerListPreferredWidth != 480 {
		t.Fatal("a no-op key at the temporary maximum replaced the user's width")
	}
	assertManagerListWidth(t, view, want)
	a.shell.SetLeftWidth(280)
	view.Frame()
	assertManagerListWidth(t, view, 321-2*(managerFrameInset+managerFrameBorder))
	if a.managerListPreferredWidth != 480 {
		t.Fatal("resizing navigation overwrote the list width preference")
	}
	view.SetSize(860, 760)
	assertManagerListWidth(t, view, 480)
	if err := view.Click(a.t("切换侧栏")); err != nil {
		t.Fatal(err)
	}
	assertManagerListWidth(t, view, 480)
	view.Key(0, ui.KeyEscape)
	view.SetSize(1120, 760)
	assertManagerListWidth(t, view, 480-2*(managerFrameInset+managerFrameBorder))
	if err := view.Click(a.t("切换侧栏")); err != nil {
		t.Fatal(err)
	}
	assertManagerListWidth(t, view, 480)
	if a.managerListPreferredWidth != 480 {
		t.Fatal("sidebar presentation changed the retained preference")
	}
}

func TestManagerResizeKeepsSearchSelectionEditorAndListScroll(t *testing.T) {
	a := fixture()
	a.filter, a.query = "folder:开发", "git"
	for i := range 50 {
		name := fmt.Sprintf("GitHub account %02d", i)
		a.state.Entries = append(a.state.Entries, app.Entry{Path: "开发/" + name, Name: name, Folder: "开发"})
	}
	view := ui.NewTester(testView(a), 1120, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	managerResizeListBounds(t, view)
	a.editEntry()
	view.Frame()
	managerResizeListBounds(t, view)
	if err := view.Click(a.t("账号名称")); err != nil {
		t.Fatal(err)
	}
	view.Key(ui.Cmd, ui.KeyA)
	view.Type("GitHub unsaved name")
	managerResizeScreenshot(t, view, "manager-resize-unsaved-editor")
	identity, path, contents, username := a.editor.identity, a.editor.path, a.editor.contents, a.editor.username
	list := managerResizeListBounds(t, view)
	view.Scroll(list.X+list.W/2, 350, 0, 800)
	first, _ := a.list.Visible()
	if first <= 0 {
		t.Fatal("test needs a scrolled entry list")
	}
	assertRetained := func() {
		t.Helper()
		if !a.editor.open || a.editor.identity != identity || a.editor.path != path || a.editor.contents != contents || a.editor.username != username || a.query != "git" || a.filter != "folder:开发" || a.selected != "开发/GitHub" {
			t.Fatal("resizing changed the current search, selection or unsaved form")
		}
		if next, _ := a.list.Visible(); next != first {
			t.Fatalf("resizing reset list scroll: first=%d want=%d", next, first)
		}
	}
	dragManagerListTo(t, a, view, 430)
	assertRetained()
	for _, width := range []int{961, 860, 1120} {
		view.SetSize(width, 760)
		assertRetained()
	}
	for range 2 {
		if err := view.Click(a.t("切换侧栏")); err != nil {
			t.Fatal(err)
		}
		assertRetained()
	}
	if a.managerListPreferredWidth != 430 {
		t.Fatal("editor or sidebar state reset the user's width")
	}
}

func TestManagerResizeKeepsDetailScrollAndWidthAcrossFolderNavigation(t *testing.T) {
	a, view := managerResizeFixture(1120)
	a.detail.Notes = strings.Repeat("Long notes remain in the same detail scroll area. ", 200)
	view.Frame()
	view.Scroll(900, 400, 0, 500)
	if a.detailScroll.Y <= 0 {
		t.Fatal("test needs a scrolled detail")
	}
	before := a.detailScroll.Y
	dragManagerListTo(t, a, view, 400)
	if a.detailScroll.Y != before {
		t.Fatal("resizing remounted or reset the detail scroll area")
	}
	a.navigateManager("folder:开发")
	view.Frame()
	assertManagerListWidth(t, view, 400)
	a.navigateManager("all")
	view.Frame()
	assertManagerListWidth(t, view, 400)
}

func TestManagerResizeMinimumInspectorKeepsLocalizedControlsVisible(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		t.Run(language, func(t *testing.T) {
			a, view := managerResizeFixture(961)
			a.state.Settings.Language, a.state.Settings.FontScale = language, 120
			view.Frame()
			dragManagerListTo(t, a, view, 480)
			list := managerResizeListBounds(t, view)
			if !visualNear(961-(list.X+list.W), 360+managerFrameInset+managerFrameBorder) {
				t.Fatal("the split did not reserve the inspector's minimum width")
			}
			assertTypographyBounds(t, view, 961, 760,
				a.t("显示全部字段"), a.t("编辑条目"), a.t("移到回收站"), a.t("复制%s", a.t("密码")), a.t("复制%s", a.t("用户名")))
			if err := view.Click(a.t("编辑条目")); err != nil {
				t.Fatal(err)
			}
			managerResizeScreenshot(t, view, "manager-resize-editor-120-"+language)
			assertTypographyBounds(t, view, 961, 760,
				a.t("账号名称"), a.t("用户名"), a.t("网址"), a.t("保存"), a.t("取消"))
		})
	}
}

func TestManagerResizeRecoveryKeepsSelectedRecordAndDestination(t *testing.T) {
	a := recoveryColumnsFixture()
	view := ui.NewTester(testView(a), 1120, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	if err := view.Click(a.t("恢复副本 %s", "工作/Second")); err != nil {
		t.Fatal(err)
	}
	a.restoreDestination = "custom/restored-account"
	view.Frame()
	dragManagerListTo(t, a, view, 420)
	for _, width := range []int{961, 860, 1120} {
		view.SetSize(width, 760)
		if a.recoverySelection != "second-copy" || a.restoreDestination != "custom/restored-account" {
			t.Fatal("resizing recovery columns changed selection or the typed restore destination")
		}
		assertTypographyBounds(t, view, width, 760, a.t("恢复到其他位置（可选）"), a.t("恢复"))
	}
	a.page = "entries"
	view.Frame()
	assertManagerListWidth(t, view, 420)
	a.page = "recovery"
	view.Frame()
	assertManagerListWidth(t, view, 420)
	if a.recoverySelection != "second-copy" || a.restoreDestination != "custom/restored-account" {
		t.Fatal("switching the shared split between entries and recovery reset its form")
	}
}

func TestManagerResizeSidebarAnimationReservesMinimumInspector(t *testing.T) {
	a, view := managerResizeFixture(1120)
	dragManagerListTo(t, a, view, 480)
	view.SetSize(961, 760)
	view.SetPreferences(ui.Preferences{})
	for range 2 {
		if err := view.Click(a.t("切换侧栏")); err != nil {
			t.Fatal(err)
		}
		for range 6 {
			view.Advance(40 * time.Millisecond)
			list := managerResizeListBounds(t, view)
			if detailWidth := 961 - list.X - list.W; detailWidth < 359.5 {
				t.Fatalf("sidebar animation compressed the inspector below its minimum: %g", detailWidth)
			}
			if a.managerListPreferredWidth != 480 {
				t.Fatal("sidebar animation overwrote the retained list width")
			}
		}
	}
}
