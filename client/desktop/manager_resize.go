package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	managerListMinWidth   float32 = 240
	managerListMaxWidth   float32 = 480
	managerDetailMinWidth float32 = 360
)

func managerListDefaultWidth(c *ui.Context) float32 {
	width, _ := c.Size()
	if width <= 960 {
		return 280
	}
	return 300
}

func (a *application) managerListWidthLimits(c *ui.Context) (float32, float32) {
	maximum := max(float32(0), min(managerListMaxWidth, a.mainContentWidth(c)-managerDetailMinWidth))
	return min(managerListMinWidth, maximum), maximum
}

// A smaller viewport temporarily constrains the split without overwriting the
// user's width. Flex shrink also protects the detail during sidebar animation.
func (a *application) managerListWidth(c *ui.Context) float32 {
	width := a.managerListPreferredWidth
	if width <= 0 {
		width = managerListDefaultWidth(c)
	}
	minimum, maximum := a.managerListWidthLimits(c)
	return max(minimum, min(width, maximum))
}

// The handle is painted last, over the inspector's left edge. Its parent stays
// unclipped while the inspector body owns clipping, so all six DIPs receive input.
func (a *application) managerResizeHandle(c *ui.Context, list ui.Element) {
	minimum, maximum := a.managerListWidthLimits(c)
	width := list.Bounds().W
	if width <= 0 {
		width = a.managerListWidth(c)
	}
	handle := ui.Box(c).Key("manager-list-resize").Absolute().Left(-3).Top(0).Bottom(0).Width(6).
		Cursor(ui.CursorResizeEW).Focusable().Role(ui.RoleSplitter).Label(a.t("调整条目列表宽度")).
		Tooltip(a.t("拖动以调整列表和详情宽度；双击恢复默认宽度。"))
	next := width
	changed := false
	if dx, _, held := handle.Dragged(); held {
		handle.Focus()
		next += dx
		changed = dx != 0
	}
	if handle.Shortcut(0, ui.KeyLeft) {
		next -= 10
		changed = true
	}
	if handle.Shortcut(0, ui.KeyRight) {
		next += 10
		changed = true
	}
	if handle.Shortcut(ui.Shift, ui.KeyLeft) {
		next -= 40
		changed = true
	}
	if handle.Shortcut(ui.Shift, ui.KeyRight) {
		next += 40
		changed = true
	}
	if handle.Shortcut(0, ui.KeyHome) {
		next = minimum
		changed = true
	}
	if handle.Shortcut(0, ui.KeyEnd) {
		next = maximum
		changed = true
	}
	next = max(minimum, min(next, maximum))
	if changed || handle.Clicked() {
		a.lastInput = time.Now()
	}
	if handle.DoubleClicked() {
		a.managerListPreferredWidth = 0
		a.queueManagerLayoutSave()
		a.lastInput = time.Now()
		c.Invalidate()
	} else if changed && next != width {
		a.managerListPreferredWidth = next
		a.queueManagerLayoutSave()
		c.Invalidate()
	}
	if handle.Hovered() || handle.Dragging() || handle.FocusVisible() {
		handle.Background(c.Theme().Text.Alpha(.05)).Children(func() {
			ui.Box(c).Absolute().Left(2).Top(0).Bottom(0).Width(2).Background(c.Theme().TextMuted.Alpha(.5))
		})
	}
	handle.FocusRing(handle.FocusVisible())
}
