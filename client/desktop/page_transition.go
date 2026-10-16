package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Only the current page is built. An exit snapshot could retain revealed
// credentials after locking, so navigation animates the incoming surface.
type pagePresentationKey struct {
	page, category string
	setup          bool
	setupMode      int
	locked         bool
	editorID       *editorIdentity
}

func (a *application) pagePresentationKey() pagePresentationKey {
	key := pagePresentationKey{page: a.page,
		category: a.settingsState.Category, setup: a.setup || !a.state.Configured,
		setupMode: a.setupMode, locked: a.state.Locked}
	if a.editor.open && a.page == "entries" {
		key.editorID = a.editor.identity
	}
	return key
}

func (a *application) pageTransition(c *ui.Context) ui.ElementTransition {
	direction := float32(1)
	if a.history.direction < 0 {
		direction = -1
	}
	motion := &ui.Motion{X: direction * 12, Opacity: .65}
	if c.Preferences().ReduceMotion {
		motion = &ui.Motion{Opacity: 1}
	}
	return ui.ElementTransition{Duration: 180 * time.Millisecond, Enter: motion}
}
