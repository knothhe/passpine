package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func quickSearchHeaderButton(c *ui.Context, icon *ui.SVG, label string, refreshing bool) ui.Element {
	button := components.Button(c, "", components.ButtonGhost).Label(label).Tooltip(label).
		Size(25, 25).Padding(5).Shrink(0).TextColor(c.Theme().TextMuted)
	button.Children(func() {
		glyph := components.Icon(c, icon).Size(14, 14).Role(ui.RoleNone)
		if refreshing && !c.Preferences().ReduceMotion {
			glyph.Rotate(glyph.Loop("quick-search-refresh", 800*time.Millisecond, ui.Linear) * 360)
		}
	})
	return button
}

func (a *application) quickSearchHeaderActions(c *ui.Context) {
	ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
		if quickSearchHeaderButton(c, lucide.RefreshCw(), a.t("刷新条目"), a.quick.operation == "configure").
			Disabled(a.busy || !a.state.Configured || a.service == nil).Clicked() {
			a.refreshQuickSearchEntries()
		}
		if quickSearchHeaderButton(c, lucide.ExternalLink(), a.t("打开主窗口"), false).Clicked() {
			a.showQuickSearchManager()
		}
		if quickSearchHeaderButton(c, lucide.Settings(), a.t("打开设置"), false).Clicked() {
			a.openQuickSearchSettings()
		}
	})
}

func (a *application) refreshQuickSearchEntries() {
	if a.closing || a.busy || a.service == nil || !a.quick.visible || !a.state.Configured {
		return
	}
	a.quick.refreshedUntil = time.Time{}
	a.callQuickSearch(app.Request{Action: "configure"})
}

func (a *application) showQuickSearchManager() {
	if a.closing {
		return
	}
	a.hideQuickSearch()
	a.show()
}

func (a *application) openQuickSearchSettings() {
	if a.closing {
		return
	}
	a.hideQuickSearch()
	a.navigate("settings", "")
	a.show()
	a.invalidate()
}
