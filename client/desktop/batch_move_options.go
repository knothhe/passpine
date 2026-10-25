package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

func (a *application) setBatchMoveFlatten(flatten bool) {
	if a.busy || a.batch.progress != nil || a.closing || a.updates.preparing {
		return
	}
	a.batch.flatten = flatten
	a.batch.result = nil
	a.lastInput = time.Now()
}

func (a *application) batchMoveOptionsView(c *ui.Context) {
	selected := 0
	if a.batch.flatten {
		selected = 1
	}
	ui.Column(c).Key("batch-move-prefix-options").FillWidth().Gap(6).Margin(0, 0, 14, 0).Children(func() {
		if ui.Segmented(c, &selected, a.t("保留前缀"), a.t("不保留前缀")).Label(a.t("移动时的文件夹前缀")).AlignSelf(ui.Start).Disabled(a.busy).Changed() {
			a.setBatchMoveFlatten(selected == 1)
		}
		message := a.t("将原始文件夹路径完整保留在目标文件夹下。")
		if a.batch.flatten {
			message = a.t("直接移入目标文件夹，只保留条目名称。")
		}
		components.Text(c, message).FillWidth().Wrap().FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
	})
}
