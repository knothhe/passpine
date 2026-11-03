package desktop

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// Onevra's dialogs, menus and popovers are glass with a nearly opaque tint.
// Under the glass style the client lowers that tint so the panel frosts and
// bends what it covers, and outlines it with a light rim instead of a gray
// border, as Liquid Glass panels do.
func (a *application) glassPanel(c *ui.Context, panel ui.Element) {
	if !a.glassChrome() {
		return
	}
	panel.Material(glass.Glass{Tint: glassPanelTint(c.Theme())}).Border(1, glassRim(c.Theme()))
}

func glassPanelTint(t *ui.Theme) ui.Color {
	if t.Dark {
		return t.Background.Alpha(.6)
	}
	return t.Background.Alpha(.64)
}

// glassRim is the hairline at the edge of glass panels and cards.
func glassRim(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(255, 255, 255, .12)
	}
	return ui.RGBA(255, 255, 255, .6)
}

// dialog is components.Dialog with the client's panel material.
func (a *application) dialog(c *ui.Context, open *bool, options components.DialogOptions, build func(panel ui.Element)) ui.Element {
	return components.Dialog(c, open, options, func(panel ui.Element) {
		a.glassPanel(c, panel)
		if build != nil {
			build(panel)
		}
	})
}

// alertDialog mirrors components.AlertDialog on the client's dialog panel,
// whose material AlertDialog does not expose.
func (a *application) alertDialog(c *ui.Context, open *bool, options components.AlertDialogOptions) components.DialogResult {
	result := components.DialogNone
	wasOpen := *open
	dialogOptions := options.DialogOptions
	dialogOptions.Label = options.Title
	a.dialog(c, open, dialogOptions, func(panel ui.Element) {
		panel.Role(ui.RoleAlertDialog)
		if options.Title != "" {
			components.Text(c, options.Title).Bold()
		}
		if options.Message != "" {
			components.Text(c, options.Message).TextColor(c.Theme().TextMuted)
		}
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(c.Theme().Space(2)).Children(func() {
			if !options.HideCancel && components.Button(c, options.CancelLabel, components.ButtonSecondary).Clicked() {
				result = components.DialogCancelled
				*open = false
			}
			if components.Button(c, options.ConfirmLabel, components.ButtonPrimary).Clicked() {
				result = components.DialogConfirmed
				*open = false
			}
		})
	})
	if wasOpen && !*open && result == components.DialogNone {
		result = components.DialogCancelled
	}
	return result
}
