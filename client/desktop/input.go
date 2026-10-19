package desktop

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
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
