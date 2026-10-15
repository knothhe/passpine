package desktop

import (
	"github.com/egoist/mygo"
	"github.com/knothhe/passpine-neo/client/app"
)

// Preferences use the manager window and share its navigation. The epoch
// cancels pending file panels or recovery reads when the destination changes.
type preferencesState struct {
	epoch       uint64
	pendingRead *preferencesRead
}

type preferencesRead struct {
	page, kind, vaultID string
	epoch, generation   uint64
}

func (read preferencesRead) valid(a *application) bool {
	return !a.closing && read.page == a.page && read.epoch == a.preferences.epoch &&
		read.generation == a.generation && read.vaultID == a.state.VaultID &&
		(read.page != "recovery" || !a.state.Locked && read.kind == a.recoveryType())
}

func (a *application) preferencesPage() string { return a.page }

func (a *application) loadPreferencesPage(page string) {
	if !a.settingsLoaded {
		a.settings, a.settingsSaved, a.settingsObserved = a.state.Settings, a.state.Settings, a.state.Settings
		a.settingsLoaded = true
	}
	if page == "recovery" {
		a.loadRecovery()
	} else if page == "activity" {
		a.requestPreferencesRead(page)
	} else if a.service != nil {
		if a.gitRemoteSaveAt.IsZero() && !a.gitRemoteSaving {
			a.gitRemote, a.gitRemoteObserved = a.state.GitRemote, a.state.GitRemote
		}
		a.call(app.Request{Action: "git-status"}, func(d app.Data) {
			if a.gitRemoteSaveAt.IsZero() && !a.gitRemoteSaving {
				a.gitRemote, a.gitRemoteObserved = d.GitRemote, d.GitRemote
			}
		})
	}
}

func (a *application) hidePreferences() {
	a.preferences.epoch++
	a.preferences.pendingRead = nil
	a.backupOpen, a.backupPassphrase = false, ""
	a.purgeOpen, a.purgeID = false, ""
	a.setShortcutRecording(false)
}

// Navigation remains available during a credential operation. Keep the latest
// requested list read until that lane is free, bound to the current destination.
func (a *application) requestPreferencesRead(page string) {
	if a.service == nil || a.closing || page == "recovery" && a.state.Locked {
		return
	}
	read := preferencesRead{page: page, vaultID: a.state.VaultID, epoch: a.preferences.epoch, generation: a.generation}
	if page == "recovery" {
		read.kind = a.recoveryType()
	}
	a.preferences.pendingRead = &read
	a.flushPreferencesRead()
}

func (a *application) flushPreferencesRead() {
	read := a.preferences.pendingRead
	if read == nil || a.busy {
		return
	}
	a.preferences.pendingRead = nil
	if !read.valid(a) || a.service == nil {
		return
	}
	a.call(app.Request{Action: read.page, Kind: read.kind, VaultID: read.vaultID}, func(d app.Data) {
		if !read.valid(a) {
			return
		}
		if read.page == "activity" {
			a.activity = d.Activity
		} else {
			a.recovery = d.Recovery
		}
	})
}

func (a *application) preferencesLease() func() bool {
	epoch, generation, vaultID := a.preferences.epoch, a.generation, a.state.VaultID
	return func() bool {
		return !a.closing && a.preferences.epoch == epoch && a.generation == generation && a.state.VaultID == vaultID
	}
}

func (a *application) dialogParent() *mygo.Window { return a.window }
