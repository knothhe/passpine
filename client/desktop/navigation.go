package desktop

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// History stores destinations and metadata only, never decrypted entry contents.
type navigationLocation struct {
	page, filter, query, selected, vaultID string
	category, settingsQuery                string
	setup                                  bool
	recoveryKind                           int
	editorID                               *editorIdentity
}

type navigationState struct {
	back, forward []navigationLocation
	revision      uint64
	direction     int
}

func (a *application) currentLocation() navigationLocation {
	return navigationLocation{page: a.page, filter: a.filter, query: a.query, selected: a.selected,
		vaultID: a.state.VaultID, category: a.settingsState.Category, settingsQuery: a.settingsState.Query,
		setup: a.setup, recoveryKind: a.recoveryKind, editorID: a.editor.identity}
}

func (a *application) rememberNavigation() {
	location := a.currentLocation()
	h := &a.history
	if len(h.back) == 0 || h.back[len(h.back)-1] != location {
		h.back = append(h.back, location)
		if len(h.back) > 100 {
			h.back = h.back[len(h.back)-100:]
		}
	}
	h.forward = nil
	h.revision++
	h.direction = 1
}

func (a *application) historyControls(c *ui.Context) {
	blocked := a.discardOpen || a.generator.open || a.deleteOpen || a.backupOpen || a.purgeOpen || a.updates.confirmOpen
	if iconButton(c, lucide.ArrowLeft(), a.t("后退"), components.ButtonGhost).Disabled(blocked || len(a.history.back) == 0).Tooltip(a.t("后退")).Clicked() {
		a.moveHistory(-1)
	}
	if iconButton(c, lucide.ArrowRight(), a.t("前进"), components.ButtonGhost).Disabled(blocked || len(a.history.forward) == 0).Tooltip(a.t("前进")).Clicked() {
		a.moveHistory(1)
	}
}

func (a *application) moveHistory(delta int) {
	if a.closing || a.discardOpen || a.generator.open || a.deleteOpen || a.backupOpen || a.purgeOpen || a.updates.confirmOpen {
		return
	}
	h := &a.history
	source := h.back
	if delta > 0 {
		source = h.forward
	}
	if len(source) == 0 {
		return
	}
	target := source[len(source)-1]
	retainedEditor := target.editorID != nil && target.editorID == a.editor.identity
	if a.editor.open && (target.setup || target.page == "entries" && !retainedEditor && target.selected != a.selected) {
		revision := h.revision
		a.requestEditorClose(func() {
			if a.history.revision == revision {
				a.moveHistory(delta)
			}
		})
		return
	}
	current := a.currentLocation()
	if delta > 0 {
		h.forward = h.forward[:len(h.forward)-1]
		h.back = append(h.back, current)
	} else {
		h.back = h.back[:len(h.back)-1]
		h.forward = append(h.forward, current)
	}
	h.revision++
	h.direction = delta
	a.restoreLocation(target)
}

func (a *application) restoreLocation(target navigationLocation) {
	a.cancelUnlockAction()
	previousTransition := a.detailTransition
	a.clearDetailTransition()
	a.pendingCopyEntry, a.pendingCopyVaultID = "", ""
	a.pendingSelection, a.pendingSelectionVaultID = "", ""
	a.hidePreferences()
	a.closeGenerator()
	if a.setup && !target.setup {
		a.passphrase, a.confirmPassphrase = "", ""
	}
	a.page, a.setup, a.filter, a.query = target.page, target.setup, target.filter, target.query
	a.settingsState.Category, a.settingsState.Query = target.category, target.settingsQuery
	a.recoveryKind = target.recoveryKind
	a.clearNotifications()
	if isPreferencesPage(a.page) && !a.setup {
		a.loadPreferencesPage(a.page)
	} else if !a.setup && !a.editor.open {
		path := target.selected
		if target.vaultID != a.state.VaultID {
			path = ""
		}
		if a.selected != path || a.detail.Entry != path {
			if path != "" && !a.state.Locked && a.service != nil {
				a.restoreDetailTransitionStructure(previousTransition)
				a.selected = path
				// Navigation restoration must not append a second history entry.
				a.callEntryForHistory(path)
			} else {
				a.dropDetail()
				a.selected = path
			}
		}
	}
	if a.page == "entries" && !a.setup {
		a.syncSelectedIndex()
	}
	a.closeNavigationOverlay()
	a.invalidate()
}

func (a *application) syncSelectedIndex() {
	a.filtered = a.entries()
	a.selectedIndex = -1
	for i, entry := range a.filtered {
		if entry.Path == a.selected {
			a.selectedIndex = i
			a.list.ScrollIntoView(i)
			break
		}
	}
}

func isPreferencesPage(page string) bool {
	return page == "settings" || page == "recovery" || page == "activity"
}

func (a *application) shouldRememberNavigation(page, filter string) bool {
	// Preferences are one destination. Categories, recovery and activity replace
	// its contents so Back returns to the manager in a single step.
	if !a.setup && isPreferencesPage(a.page) && isPreferencesPage(page) {
		return false
	}
	return a.page != page || a.setup || filter != "" && filter != a.filter
}

func (a *application) navigateSettingsCategory(category string) {
	if a.page != "settings" || a.setup {
		a.navigate("settings", "")
	} else if a.settingsState.Category != category || a.settingsState.Query != "" {
		a.hidePreferences()
	}
	a.settingsState.Category, a.settingsState.Query = category, ""
	a.closeNavigationOverlay()
	a.invalidate()
}

func (a *application) callEntryForHistory(path string) {
	if a.busy {
		a.beginDetailTransition(path)
		a.pendingSelection, a.pendingSelectionVaultID = path, a.state.VaultID
		a.pendingSelectionGeneration = a.generation
		return
	}
	a.fetchSelectedDetail(path)
}

func (a *application) handleMainWindowClose(event *mygo.CloseEvent) {
	a.setShortcutRecording(false)
	if a.closing {
		return
	}
	if a.generator.open {
		event.PreventDefault()
		a.closeGenerator()
		a.invalidate()
		return
	}
	if a.editor.open {
		event.PreventDefault()
		a.requestEditorClose(nil)
		return
	}
	if a.backupOpen || a.purgeOpen || a.deleteOpen {
		event.PreventDefault()
		a.backupOpen, a.purgeOpen, a.deleteOpen = false, false, false
		a.backupPassphrase = ""
		a.invalidate()
		return
	}
	if a.tray != nil && a.state.Settings.CloseToTray {
		event.PreventDefault()
		if a.window != nil {
			a.window.Hide()
		}
		// Hide emits OnHide only when the backend still reports visible. Keep
		// the lifecycle accurate even if the OS has already hidden the window.
		a.mainWindowVisibilityChanged(false)
	} else {
		event.PreventDefault()
		a.requestQuit()
	}
}
