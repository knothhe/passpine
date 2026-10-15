// Adapted from Onevra v0.2026.9 shells/sidebar; see LICENSE.
package clientshell

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

type shellCaptionWindow interface {
	Minimize()
	ToggleMaximize()
	IsMaximized() bool
	Close()
}

func captionLabel(label, fallback string) string {
	if label != "" {
		return label
	}
	return fallback
}

func (s *Shell) captionButtons(c *ui.Context) {
	w, _ := s.host.(shellCaptionWindow)
	o := s.options
	// Use a consistent compact glyph size without shrinking the hit targets.
	iconSize := c.Theme().Rem(0.875)
	ui.Row(c).Key("window-controls").Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
		// Asymmetric padding lowers the centered glyph by 2 DIPs at normal scale.
		minimize := iconButton(c, lucide.Minus(), captionLabel(o.MinimizeLabel, "Minimize window"), iconSize).
			Padding(c.Theme().Rem(0.25), 0, 0, 0)
		if minimize.Clicked() && w != nil {
			w.Minimize()
		}
		icon, label := lucide.Square(), captionLabel(o.MaximizeLabel, "Maximize window")
		if w != nil && w.IsMaximized() {
			icon, label = lucide.Copy(), captionLabel(o.RestoreLabel, "Restore window")
		}
		if iconButton(c, icon, label, iconSize).Clicked() && w != nil {
			w.ToggleMaximize()
		}
		closeButton := iconButton(c, lucide.X(), captionLabel(o.CloseLabel, "Close window"), iconSize)
		if closeButton.Hovered() || closeButton.Pressed() {
			closeButton.TextColor(ui.RGB(255, 255, 255))
		}
		closeButton.Draw(func(p *ui.Painter, r ui.Rect) {
			if closeButton.IsDisabled() {
				return
			}
			if closeButton.Pressed() {
				p.Fill(r, ui.RGB(196, 43, 28), c.Theme().Radius)
			} else if closeButton.Hovered() {
				p.Fill(r, ui.RGB(232, 17, 35), c.Theme().Radius)
			}
		})
		if closeButton.Clicked() && w != nil {
			// Close (not Destroy) preserves application close cancellation.
			w.Close()
		}
	})
}
