package desktop

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
)

// What a move keeps from each original path beneath the destination.
const (
	batchKeepStrip = "strip"
	batchKeepFull  = "full"
	batchKeepName  = "name"
)

// batchCommonFolders lists the folders every selected entry lies within,
// shallowest first. Removing any of them keeps the structure beneath it, so
// no selected entry can fail to match the removed prefix.
func batchCommonFolders(entries []app.Entry) []string {
	if len(entries) == 0 {
		return nil
	}
	var shared []string
	for i, entry := range entries {
		if entry.Folder == "" {
			return nil
		}
		parts := strings.Split(entry.Folder, "/")
		if i == 0 {
			shared = parts
			continue
		}
		n := 0
		for n < len(shared) && n < len(parts) && shared[n] == parts[n] {
			n++
		}
		shared = shared[:n]
	}
	folders := make([]string, len(shared))
	for i := range shared {
		folders[i] = strings.Join(shared[:i+1], "/")
	}
	return folders
}

// batchKeep resolves the chosen option against the current selection. Until
// the user picks, removing the deepest shared folder is the default; without a
// shared folder that option is unavailable and the full path is kept instead.
// A chosen folder stays in effect while every selected entry still shares it.
func (a *application) batchKeep(entries []app.Entry) (mode, strip string, common []string) {
	common = batchCommonFolders(entries)
	mode = a.batch.keep
	if mode == "" {
		mode = batchKeepStrip
	}
	if mode == batchKeepStrip && len(common) == 0 {
		return batchKeepFull, "", common
	}
	strip = a.batch.stripFrom
	if !containsFolder(common, strip) && len(common) > 0 {
		strip = common[len(common)-1]
	}
	return mode, strip, common
}

// batchMoveTarget resolves the option once, so planning a large selection does
// not recompute the shared folders for every entry.
func (a *application) batchMoveTarget(entries []app.Entry) func(app.Entry) string {
	mode, strip, _ := a.batchKeep(entries)
	destination := a.batch.destination
	return func(entry app.Entry) string {
		relative := entry.Path
		switch {
		case mode == batchKeepName:
			relative = entry.Name
		case mode == batchKeepStrip && strings.HasPrefix(entry.Path, strip+"/"):
			relative = strings.TrimPrefix(entry.Path, strip+"/")
		}
		return batchDestinationPath(destination, relative)
	}
}

func (a *application) batchEntryDestination(entry app.Entry) string {
	return a.batchMoveTarget(a.selectedBatchEntries())(entry)
}

func (a *application) batchKeepEditable() bool {
	return !a.busy && a.batch.progress == nil && !a.closing && !a.updates.preparing
}

// setBatchKeep only changes the draft and its preview; nothing moves until the
// move button is used.
func (a *application) setBatchKeep(mode string) {
	if !a.batchKeepEditable() || mode != batchKeepStrip && mode != batchKeepFull && mode != batchKeepName {
		return
	}
	if mode == batchKeepStrip && len(batchCommonFolders(a.selectedBatchEntries())) == 0 {
		return
	}
	a.batch.keep = mode
	a.batch.result = nil
	a.lastInput = time.Now()
}

// Choosing the prefix to remove also chooses that option.
func (a *application) chooseBatchStripFolder(folder string) {
	if !a.batchKeepEditable() || !containsFolder(batchCommonFolders(a.selectedBatchEntries()), folder) {
		return
	}
	a.batch.keep, a.batch.stripFrom = batchKeepStrip, folder
	a.batch.result = nil
	a.lastInput = time.Now()
}

// batchKeepView shows each option with the deepest selected path, where the
// three choices differ most, placed beneath the destination being chosen.
func (a *application) batchKeepView(c *ui.Context, entries []app.Entry) {
	t := c.Theme()
	mode, strip, common := a.batchKeep(entries)
	var sample app.Entry
	for _, entry := range entries {
		if len(entry.Path) > len(sample.Path) {
			sample = entry
		}
	}
	base := "…/"
	if a.batch.chosen {
		base = batchDestinationPath(a.batch.destination, "")
	}
	components.Text(c, a.t("保留原路径")).Margin(16, 0, 6, 0).FontSize(scaledFontSize(c, 11)).TextColor(t.TextMuted)
	selected := mode
	c.Key("batch-keep")
	ui.RadioGroup(c, func() {
		for _, option := range []struct {
			value, label, kept, unavailable string
		}{
			{batchKeepStrip, a.t("去掉公共前缀"), strings.TrimPrefix(sample.Path, strip+"/"), a.t("所选条目没有共同的上级文件夹。")},
			{batchKeepFull, a.t("完整路径"), sample.Path, ""},
			{batchKeepName, a.t("仅条目名"), sample.Name, ""},
		} {
			disabled := option.value == batchKeepStrip && len(common) == 0
			c.Key(option.value)
			row := ui.RadioBase(c, &selected, option.value).Label(option.label).FillWidth().Padding(5, 8).Gap(9).AlignItems(ui.Start).Radius(t.Radius).
				FocusRing(false).Disabled(disabled || !a.batchKeepEditable())
			on, focus := selected == option.value, row.FocusVisible()
			if row.Hovered() && !disabled {
				row.Background(t.Text.Alpha(.04))
			}
			row.Children(func() {
				dot := ui.Box(c).Size(14, 14).Margin(3, 0, 0, 0).Radius(7).Shrink(0)
				if on {
					dot.Background(t.Accent)
				} else {
					dot.Background(t.Surface).Border(1, t.Border.Mix(t.Text, 0.25))
				}
				dot.DrawOver(func(p *ui.Painter, r ui.Rect) {
					if on {
						p.Fill(ui.Rect{X: r.X + r.W/2 - 3, Y: r.Y + r.H/2 - 3, W: 6, H: 6}, t.AccentText, 3)
					}
					if focus {
						p.FocusRing(r, [4]float32{7, 7, 7, 7})
					}
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Row(c).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
						title := components.Text(c, option.label).FontSize(scaledFontSize(c, 12))
						if disabled {
							title.TextColor(t.TextMuted)
						}
						if option.value == batchKeepStrip && strip != "" {
							components.Text(c, strip).MinWidth(0).Wrap().Padding(0, 5).Radius(4).Background(t.Text.Alpha(.06)).FontSize(scaledFontSize(c, 11))
						}
					})
					if disabled {
						components.Text(c, option.unavailable).FillWidth().Wrap().FontSize(scaledFontSize(c, 11)).TextColor(t.TextMuted)
					} else if sample.Path != "" {
						components.Text(c, base+option.kept).FillWidth().Wrap().FontSize(scaledFontSize(c, 11)).TextColor(t.TextMuted)
					}
				})
			})
			if row.Changed() {
				a.setBatchKeep(selected)
			}
		}
	}).Label(a.t("保留原路径")).FillWidth().Margin(0, -8, 0, -8)
	if mode != batchKeepStrip || len(common) < 2 {
		return
	}
	ui.Row(c).Key("batch-strip-from").FillWidth().Margin(6, 0, 0, 0).Gap(8).AlignItems(ui.Center).Children(func() {
		components.Text(c, a.t("要去掉的公共前缀")).Shrink(0).FontSize(scaledFontSize(c, 11)).TextColor(t.TextMuted)
		current := strip
		sel := components.Select(c, &current)
		sel.Trigger.Label(a.t("要去掉的公共前缀")).Grow(1).MinWidth(0).Disabled(!a.batchKeepEditable()).Children(func() {
			components.Text(c, current).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(scaledFontSize(c, 12))
		})
		sel.Popup(func(panel ui.Element) {
			panel.Width(320).MaxWidth(420).Label(a.t("要去掉的公共前缀"))
			components.ScrollView(c, components.ScrollViewOptions{}).FillWidth().MaxHeight(240).Children(func() {
				for _, folder := range common {
					sel.Item(folder).Label(a.t("去掉公共前缀 %s", folder)).Children(func() { components.Text(c, folder).SingleLine() })
				}
			})
		})
		if sel.Trigger.Changed() {
			a.chooseBatchStripFolder(current)
		}
	})
}
