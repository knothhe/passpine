package desktop

// A continuation belongs to the vault and destination that requested it.
// Explicit navigation or a new lock discards it before a late unlock can run.
type unlockAction struct {
	action, vaultID, filter string
	generation              uint64
}

func (pending unlockAction) valid(a *application) bool {
	return pending.action != "" && pending.vaultID == a.state.VaultID && pending.generation == a.generation && !a.setup
}

func (a *application) cancelUnlockAction() {
	a.pendingUnlock = unlockAction{}
	a.pendingNewEntry = unlockAction{}
}

func (a *application) beginUnlock(action string) {
	if !a.state.Locked || a.busy || a.closing {
		return
	}
	if action != "" {
		a.pendingUnlock = unlockAction{action: action, vaultID: a.state.VaultID, filter: a.filter, generation: a.generation}
	}
	a.clearNotifications()
	a.show()
	if a.state.Settings.DeviceUnlock && a.state.DeviceUnlockEnrolled && a.state.DeviceUnlockAvailable {
		a.unlock("device")
	}
	a.invalidate()
}

func (a *application) resumeUnlockAction() bool {
	pending := a.pendingUnlock
	a.pendingUnlock = unlockAction{}
	if a.state.Locked || !pending.valid(a) {
		return false
	}
	switch pending.action {
	case "new-entry":
		if a.page == "entries" {
			a.filter = pending.filter
			a.newEntry()
			return true
		}
	case "export":
		if isPreferencesPage(a.page) {
			a.exportKey()
			return true
		}
	}
	return false
}

func (a *application) flushPendingNewEntry() {
	if a.pendingNewEntry.action != "" && !a.busy && !a.closing {
		pending := a.pendingNewEntry
		a.pendingNewEntry = unlockAction{}
		if pending.valid(a) {
			a.filter = pending.filter
			a.newEntry()
		}
	}
}
