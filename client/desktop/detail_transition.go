package desktop

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

const detailLoadingDelay = 180 * time.Millisecond

// A pending read keeps the previous surface read-only until the replacement is
// ready. Secret field values and the raw document never enter this snapshot.
type detailDisplayField struct {
	Key, Label, Value string
	Secret            bool
}

type detailTransitionState struct {
	entry, vaultID       string
	generation, epoch    uint64
	fields               []detailDisplayField
	displayEntry, notes  string
	hasNotes, hasWebsite bool
	unlocking            bool
}

type detailTransitionPresentation struct {
	Pending, Delayed     bool
	Remaining            time.Duration
	Entry, DisplayEntry  string
	Notes                string
	Fields               []detailDisplayField
	HasNotes, HasWebsite bool
	Unlocking            bool
}

func (a *application) beginDetailTransition(path string) {
	previous := a.detailTransition
	if a.detail.Entry != "" && !a.state.Locked {
		previous = detailTransitionState{fields: detailDisplayFields(a.detail), displayEntry: a.detail.Entry,
			notes: a.detail.Notes, hasNotes: a.detail.Notes != "", hasWebsite: a.detail.URL != ""}
	} else if previous.vaultID != a.state.VaultID || previous.generation != a.generation || previous.epoch != a.metadataEpoch {
		previous = detailTransitionState{}
	}
	if a.detailTransition.entry != path || a.detailLoadingStarted.IsZero() {
		a.detailSelectionSerial++
		a.detailLoadingStarted = time.Now()
	}
	previous.entry, previous.vaultID = path, a.state.VaultID
	previous.generation, previous.epoch = a.generation, a.metadataEpoch
	a.detailTransition = previous
	a.detail, a.reveal, a.revealedFields = app.Data{}, false, nil
	a.deleteOpen = false
}

func detailDisplayFields(data app.Data) []detailDisplayField {
	fields := []detailDisplayField{{Key: "password", Label: "密码", Secret: true}}
	if data.Username != "" {
		fields = append(fields, detailDisplayField{Key: "username", Label: "用户名", Value: data.Username})
	}
	if data.URL != "" {
		fields = append(fields, detailDisplayField{Key: "url", Label: "网址", Value: data.URL})
	}
	if data.OTP != "" {
		fields = append(fields, detailDisplayField{Key: "otp", Label: "一次性密码", Secret: true})
	}
	for i, field := range extraFields(data.Contents) {
		fields = append(fields, detailDisplayField{Key: "custom:" + strconv.Itoa(i), Label: field[0], Secret: true})
	}
	return fields
}

func (a *application) clearDetailTransition() {
	a.detailSelectionSerial++
	a.detailTransition = detailTransitionState{}
	a.detailLoadingStarted = time.Time{}
}

func (a *application) restoreDetailTransitionStructure(previous detailTransitionState) {
	if a.detail.Entry != "" || previous.vaultID != a.state.VaultID || previous.generation != a.generation || previous.epoch != a.metadataEpoch {
		return
	}
	// Navigation has already cancelled the old request. Preserve the read-only
	// surface for the next read while giving that read a fresh target lease.
	previous.entry = ""
	a.detailTransition = previous
}

func (a *application) detailTransitionPresentation(now time.Time) detailTransitionPresentation {
	if a.state.Locked || a.setup || a.page != "entries" || a.selected == "" || a.detail.Entry != "" || !a.busy {
		return detailTransitionPresentation{}
	}
	transition := a.detailTransition
	if transition.entry != a.selected || transition.vaultID != a.state.VaultID || transition.generation != a.generation || transition.epoch != a.metadataEpoch {
		return detailTransitionPresentation{}
	}
	remaining := detailLoadingDelay - now.Sub(a.detailLoadingStarted)
	if remaining < 0 {
		remaining = 0
	}
	return detailTransitionPresentation{
		Pending: true, Delayed: remaining == 0, Remaining: remaining, Entry: a.selected,
		Fields: transition.fields, HasNotes: transition.hasNotes, HasWebsite: transition.hasWebsite,
		DisplayEntry: transition.displayEntry, Notes: transition.notes, Unlocking: transition.unlocking,
	}
}

// The serial distinguishes a later visit to the same path from the request
// started before leaving it. Vault, lock and metadata boundaries have their own
// leases so no late read can repopulate an invalidated detail.
func (a *application) fetchSelectedDetail(path string) {
	a.beginDetailTransition(path)
	serial, generation, vaultID, epoch := a.detailSelectionSerial, a.generation, a.state.VaultID, a.metadataEpoch
	a.call(app.Request{Action: "fetch", Entry: path, VaultID: vaultID}, func(data app.Data) {
		if serial != a.detailSelectionSerial || generation != a.generation || vaultID != a.state.VaultID || epoch != a.metadataEpoch || a.selected != path || a.page != "entries" || a.setup || a.editor.open || a.state.Locked {
			return
		}
		// Reset with the new contents, leaving the previous read-only surface
		// at its existing offset while waiting. ScrollTo would start a spring.
		a.detailScroll = ui.ScrollState{Handle: a.detailScroll.Handle}
		a.detail = data
		a.clearDetailTransition()
	})
}
