package desktop

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func monochromeInputBackground(dark bool) ui.Color {
	if dark {
		return ui.Hex("#212227")
	}
	return ui.Hex("#f8f8fa")
}

// Onevra owns input interaction and focus styling. Give monochrome fields a
// solid surface so their contrast does not depend on the pane underneath.
func textBox(c *ui.Context, value *string, options components.TextBoxOptions) ui.Element {
	input := components.TextBox(c, value, options)
	if owner := *commandPaletteOwner(c); owner != nil {
		input.OnTextKey(owner.commandPaletteTextKey)
	}
	t := c.Theme()
	if isMonochromePalette(t) {
		background := monochromeInputBackground(t.Dark)
		if options.ReadOnly {
			background = background.Mix(t.Text, .04)
		}
		input.Background(background)
	}
	return input
}

// Keep reveal state with this mounted field, independently of other secrets.
// Returning the input preserves callers' focus, change and submit handling.
func (a *application) passwordInput(c *ui.Context, value *string, label, placeholder string, disabled bool) ui.Element {
	var input ui.Element
	row := ui.Row(c).Key(value).FillWidth().MinWidth(0).Gap(8).AlignItems(ui.Center).Disabled(disabled)
	type visibility struct{ revealed, hadValue bool }
	state := ui.Local(row, "password-visibility", func() visibility { return visibility{} })
	if state.hadValue && *value == "" {
		state.revealed = false
	}
	state.hadValue = *value != ""
	row.Children(func() {
		input = textBox(c, value, components.TextBoxOptions{}).Grow(1).MinWidth(0).Label(label).Placeholder(placeholder)
		if !state.revealed {
			input.Password()
		}
		icon, action := lucide.Eye(), a.t("显示%s", label)
		if state.revealed {
			icon, action = lucide.EyeOff(), a.t("隐藏%s", label)
		}
		if iconButton(c, icon, action, components.ButtonGhost).Shrink(0).Clicked() {
			state.revealed = !state.revealed
			c.Invalidate()
		}
	})
	return input
}

func (a *application) passwordField(c *ui.Context, label string, value *string, placeholder string) ui.Element {
	var input ui.Element
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		components.Text(c, label).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
		input = a.passwordInput(c, value, label, placeholder, false)
	})
	return input
}
