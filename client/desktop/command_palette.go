package desktop

import (
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

type commandPaletteState struct {
	open, focusPending             bool
	query, previousQuery, selected string
	results                        []paletteCommand
	searchRef                      ui.Ref
	list                           components.ListState
}

type commandPaletteOwnerKey struct{}

func commandPaletteOwner(c *ui.Context) **application {
	return ui.Local(c.Root(), commandPaletteOwnerKey{}, func() *application { return nil })
}

func (a *application) commandPaletteShortcuts(c *ui.Context) {
	*commandPaletteOwner(c) = a
	if a.shortcut.recording {
		return
	}
	c.OnShortcut(ui.Ctrl|ui.Shift, ui.KeyP, a.toggleCommandPalette)
	if runtime.GOOS == "darwin" {
		c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyP, a.toggleCommandPalette)
	}
}

func (a *application) commandPaletteTextKey(key ui.TextKey) bool {
	// AppKit-style text navigation consumes Control-Shift-P before normal
	// shortcuts. Claim this exact chord while preserving all other editing.
	if key.Key != ui.KeyP || key.Mods != ui.Ctrl|ui.Shift || a.commandPaletteBlocked() {
		return false
	}
	a.toggleCommandPalette()
	return true
}

func (a *application) commandPaletteBlocked() bool {
	return a.closing || a.initializing || a.quit.requested || a.updates.preparing ||
		a.shortcut.recording || a.discardOpen || a.deleteOpen || a.purgeOpen ||
		a.backupOpen || a.generator.open || a.updates.confirmOpen || a.batch.trashConfirmation != nil && a.batch.trashConfirmation.open
}

func (a *application) toggleCommandPalette() {
	if a.palette.open {
		a.closeCommandPalette()
		return
	}
	if a.commandPaletteBlocked() {
		return
	}
	a.hideQuickSearch()
	a.show()
	p := &a.palette
	p.open, p.focusPending = true, true
	p.query, p.previousQuery, p.selected = "", "", ""
	p.list.ScrollTo(0, ui.Start)
	a.refreshPaletteResults()
	a.lastInput = time.Now()
	a.invalidate()
}

func (a *application) closeCommandPalette() {
	p := &a.palette
	p.open, p.focusPending = false, false
	p.query, p.previousQuery, p.selected = "", "", ""
	p.results = nil
	p.searchRef.CancelFocus()
	a.invalidate()
}

func (a *application) refreshPaletteResults() {
	p := &a.palette
	p.results = a.matchingPaletteCommands(p.query)
	if p.previousQuery != p.query {
		p.selected = ""
		p.previousQuery = p.query
		p.list.ScrollTo(0, ui.Start)
	}
	for _, command := range p.results {
		if command.id == p.selected && command.disabled == "" {
			return
		}
	}
	p.selected = ""
	for _, command := range p.results {
		if command.disabled == "" {
			p.selected = command.id
			break
		}
	}
}

func (a *application) movePaletteSelection(delta int) {
	a.refreshPaletteResults()
	p := &a.palette
	index := -1
	for i, command := range p.results {
		if command.id == p.selected {
			index = i
			break
		}
	}
	for step := 0; step < len(p.results); step++ {
		index = (index + delta + len(p.results)) % len(p.results)
		if p.results[index].disabled == "" {
			p.selected = p.results[index].id
			p.list.ScrollIntoView(index)
			break
		}
	}
	a.lastInput = time.Now()
	a.invalidate()
}

func (a *application) submitPaletteCommand(id string) {
	if !a.palette.open || a.commandPaletteBlocked() {
		return
	}
	// Re-resolve availability at activation time. A lock or refresh may have
	// invalidated the command after the last painted frame.
	for _, command := range a.matchingPaletteCommands(a.palette.query) {
		if command.id == id && command.disabled == "" {
			a.closeCommandPalette()
			a.executePaletteCommand(id)
			return
		}
	}
}

func (a *application) commandPaletteView(c *ui.Context) {
	p := &a.palette
	if p.open && a.commandPaletteBlocked() {
		a.closeCommandPalette()
	}
	wasOpen := p.open
	c.Key("command-palette")
	components.Dialog(c, &p.open, components.DialogOptions{Label: a.t("命令面板")}, func(panel ui.Element) {
		if !p.open {
			return
		}
		t := c.Theme()
		panel.Width(620).Height(510).Padding(0).Gap(0).Radius(12).Background(t.Surface).Border(1, t.Border).Clip()
		c.OnShortcut(ui.Ctrl|ui.Shift, ui.KeyP, a.closeCommandPalette)
		if runtime.GOOS == "darwin" {
			c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyP, a.closeCommandPalette)
		}
		ui.Column(c).FillWidth().Shrink(0).Padding(12, 16, 14, 16).Gap(10).Children(func() {
			ui.Row(c).FillWidth().Height(22).AlignItems(ui.Center).Gap(4).Children(func() {
				components.Text(c, a.t("命令面板")).FontSize(scaledFontSize(c, 12)).FontWeight(500).Grow(1)
				for _, key := range []string{"Ctrl", "Shift", "P"} {
					quickSearchKeycap(c, key, false)
				}
			})
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				c.Key("command-query")
				input := textBox(c, &p.query, components.TextBoxOptions{LeadingIcon: lucide.Search()}).
					Ref(&p.searchRef).Role(ui.RoleComboBox).Expanded(true).Label(a.t("搜索命令")).Placeholder(a.t("搜索命令…")).Grow(1).MinWidth(0).Height(44).FontSize(scaledFontSize(c, 15))
				input.OnShortcut(0, ui.KeyDown, func() { a.movePaletteSelection(1) })
				input.OnShortcut(0, ui.KeyUp, func() { a.movePaletteSelection(-1) })
				input.OnTextKey(func(key ui.TextKey) bool {
					if a.commandPaletteTextKey(key) {
						return true
					}
					if key.Mods == ui.Ctrl && (key.Key == ui.KeyN || key.Key == ui.KeyP) {
						delta := 1
						if key.Key == ui.KeyP {
							delta = -1
						}
						a.movePaletteSelection(delta)
						return true
					}
					return false
				})
				if p.focusPending {
					p.searchRef.Focus()
					p.focusPending = false
				}
				a.refreshPaletteResults()
				if input.Changed() {
					a.lastInput = time.Now()
				}
				if input.Submitted() {
					a.submitPaletteCommand(p.selected)
				}
				if iconButton(c, lucide.X(), a.t("关闭命令面板"), components.ButtonGhost).Clicked() {
					a.closeCommandPalette()
				}
			})
		})
		ui.Box(c).FillWidth().Height(1).Shrink(0).Background(t.Border)
		if len(p.results) == 0 {
			ui.Column(c).FillWidth().Grow(1).MinHeight(0).AlignItems(ui.Center).Justify(ui.Center).Padding(24).Gap(12).Children(func() {
				components.Icon(c, lucide.SearchX).Size(28, 28).TextColor(t.TextMuted)
				components.Text(c, a.t("没有匹配的命令")).FontSize(scaledFontSize(c, 15)).FontWeight(500)
				components.Text(c, a.t("试试“新建”“设置”或其他关键词。")).FontSize(scaledFontSize(c, 12)).TextColor(t.TextMuted).Wrap()
			})
		} else {
			// Capture a frame-local copy: list rows can be built during layout,
			// after clicking a command has already closed and cleared the palette.
			results := append([]paletteCommand(nil), p.results...)
			components.List(c, &p.list, len(results), func(i int) {
				command := results[i]
				ui.Column(c).FillWidth().Padding(0, 8).Children(func() {
					if strings.TrimSpace(p.query) == "" && (i == 0 || results[i-1].group != command.group) {
						components.Text(c, command.group).FontSize(scaledFontSize(c, 11)).TextColor(t.TextMuted).Padding(12, 12, 6, 12)
					}
					a.paletteCommandRow(c, command)
				})
			}).Label(a.t("命令结果")).FillWidth().Grow(1).MinHeight(0)
		}
		ui.Row(c).FillWidth().Shrink(0).MinHeight(40).Padding(10, 16).Gap(5).AlignItems(ui.Center).Border(1, t.Border).BorderWidth(1, 0, 0, 0).Children(func() {
			quickSearchKeycap(c, "↑", false)
			quickSearchKeycap(c, "↓", false)
			paletteHint(c, a.t("选择"))
			quickSearchKeycap(c, "↵", false)
			paletteHint(c, a.t("执行"))
			quickSearchKeycap(c, "Esc", false)
			paletteHint(c, a.t("关闭面板"))
			ui.Spacer(c)
			count := a.t("%d 个命令", len(p.results))
			if len(p.results) == 1 {
				count = a.t("1 个命令")
			}
			components.Text(c, count).FontSize(scaledFontSize(c, 11)).SingleLine().TextColor(t.TextMuted)
		})
	})
	if wasOpen && !p.open {
		a.closeCommandPalette()
	}
}

func paletteHint(c *ui.Context, label string) {
	components.Text(c, label).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted).SingleLine().Margin(0, 8, 0, 0)
}

func (a *application) paletteCommandRow(c *ui.Context, command paletteCommand) {
	t := c.Theme()
	selected := command.id == a.palette.selected
	description := command.description
	if command.disabled != "" {
		description = command.disabled
	}
	row := ui.ButtonBase(c).Key(command.id).Label(command.label).Description(description).FillWidth().MinHeight(50).
		Padding(8, 12).Gap(12).AlignItems(ui.Center).Radius(7).Selected(selected).Disabled(command.disabled != "")
	if selected {
		row.Background(t.Selection)
		c.Resolve(a.palette.searchRef).ActiveDescendant(row)
	} else if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.OnShortcut(0, ui.KeyDown, func() { a.palette.selected = command.id; a.palette.searchRef.Focus(); a.movePaletteSelection(1) })
	row.OnShortcut(0, ui.KeyUp, func() { a.palette.selected = command.id; a.palette.searchRef.Focus(); a.movePaletteSelection(-1) })
	row.Children(func() {
		if selected {
			ui.Box(c).Absolute().Left(1).Top(14).Bottom(14).Width(2).Radius(1).Background(t.Accent)
		}
		components.Icon(c, command.icon).Size(17, 17).Shrink(0).TextColor(t.TextMuted)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			foreground := t.Text
			if command.disabled != "" {
				foreground = t.TextMuted
			}
			components.Text(c, command.label).FontSize(scaledFontSize(c, 13)).FontWeight(500).TextColor(foreground).SingleLine().Ellipsis("…")
			components.Text(c, description).FontSize(scaledFontSize(c, 11)).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
		})
		if command.shortcut != "" {
			quickSearchKeycap(c, command.shortcut, false)
		}
	})
	if row.Clicked() {
		a.submitPaletteCommand(command.id)
	}
}
