package desktop

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/i18n"
)

func paletteCommandIDs(commands []paletteCommand) []string {
	ids := make([]string, len(commands))
	for index, command := range commands {
		ids[index] = command.id
	}
	return ids
}

func TestPaletteSearchIsBilingualRankedAndDoesNotIndexSecrets(t *testing.T) {
	for _, locale := range []string{i18n.English, i18n.SimplifiedChinese} {
		t.Run(locale, func(t *testing.T) {
			a := fixture()
			a.state.Settings.Language = locale
			for query, expected := range map[string]string{
				"复制 密码": "copy-password", " COPY password ": "copy-password",
				"复制 username": "copy-username", "appearance language": "settings",
				"外观 语言": "settings", ">new entry": "new", "trash": "trash",
				"配置 同步": "config-sync", "Firefox": "browser", "open vault": "open-vault",
			} {
				results := a.matchingPaletteCommands(query)
				if len(results) == 0 || results[0].id != expected {
					t.Errorf("query %q: got %v, want %q first", query, paletteCommandIDs(results), expected)
				}
			}
			for _, query := range []string{"no-such-command", a.detail.Password, a.detail.Username, a.detail.Entry, a.state.VaultName} {
				if results := a.matchingPaletteCommands(query); len(results) != 0 {
					t.Errorf("query %q unexpectedly matched %v", query, paletteCommandIDs(results))
				}
			}
		})
	}
}

func TestPaletteCommandsKeepOrderAndLocalizedMetadataAcrossStateChanges(t *testing.T) {
	a := fixture()
	before := paletteCommandIDs(a.paletteCommands())
	if len(before) != 18 {
		t.Fatalf("got %d commands", len(before))
	}
	a.busy, a.state.Locked = true, true
	if after := paletteCommandIDs(a.paletteCommands()); !slices.Equal(before, after) {
		t.Fatal("availability changes reordered the default command list")
	}
	for _, source := range paletteCommandSources() {
		for _, message := range []string{source.label, source.description, source.group} {
			if !i18n.HasEnglish(message) {
				t.Errorf("missing English command text: %q", message)
			}
		}
	}
	a.state.Settings.Language = i18n.English
	for _, command := range a.paletteCommands() {
		if command.disabled != "" && strings.ContainsAny(command.disabled, "请密码") {
			t.Errorf("untranslated disabled reason: %q", command.disabled)
		}
	}
}

func TestPaletteCommandsRecheckCurrentContextBeforeDispatch(t *testing.T) {
	cases := []struct {
		name string
		edit func(*application)
	}{
		{"locked", func(a *application) { a.state.Locked = true }},
		{"busy", func(a *application) { a.busy = true }},
		{"unconfigured", func(a *application) { a.state.Configured = false }},
		{"different-selection", func(a *application) { a.selected = "other/entry" }},
		{"no-selection", func(a *application) { a.selected = "" }},
		{"settings-page", func(a *application) { a.page = "settings" }},
		{"setup", func(a *application) { a.setup = true }},
		{"batch", func(a *application) { a.batch.active = true }},
		{"editor", func(a *application) { a.editor.open = true }},
		{"dialog", func(a *application) { a.deleteOpen = true }},
		{"recording", func(a *application) { a.shortcut.recording = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := fixture()
			_ = a.paletteCommands() // Capture the earlier eligible presentation.
			tc.edit(a)
			a.service = backendFunc(func(request app.Request) app.Response {
				t.Errorf("unavailable command dispatched %q", request.Action)
				return app.Response{}
			})
			for _, id := range []string{"copy-password", "copy-username", "edit"} {
				if a.paletteCommandDisabled(id) == "" || a.executePaletteCommand(id) {
					t.Errorf("%s accepted an invalid current context", id)
				}
			}
		})
	}
	a := fixture()
	if a.executePaletteCommand("unknown") {
		t.Fatal("unknown command was accepted")
	}
	a.detail.Username = ""
	if a.executePaletteCommand("copy-username") || a.paletteCommandDisabled("copy-username") == "" {
		t.Fatal("empty field remained copyable")
	}
}

func TestPaletteCopyUsesExistingRevisionCheckedBackend(t *testing.T) {
	for _, field := range []string{"password", "username"} {
		t.Run(field, func(t *testing.T) {
			a := fixture()
			callbacks, requests := make(chan func(), 1), make(chan app.Request, 1)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			state, entry, revision := a.state, a.detail.Entry, a.detail.Revision
			a.service = backendFunc(func(request app.Request) app.Response {
				requests <- request
				return app.Response{Status: "ok", Data: state}
			})
			if !a.executePaletteCommand("copy-" + field) {
				t.Fatal("eligible copy was not dispatched")
			}
			awaitMain(t, callbacks)
			request := <-requests
			if request.Action != "copy" || request.Field != field || request.Entry != entry || request.Revision != revision || request.Contents != "" || request.Passphrase != "" {
				t.Fatalf("copy bypassed the existing backend contract: %+v", request)
			}
			if a.notice == "" {
				t.Fatal("copy completion did not use existing feedback")
			}
		})
	}
}

func TestPaletteLockAndNavigationProtectUnsavedEditor(t *testing.T) {
	for _, id := range []string{"lock", "all-entries", "drafts", "trash", "open-vault"} {
		t.Run(id, func(t *testing.T) {
			a := fixture()
			a.editEntry()
			a.editor.password = "unsaved-fixture-password"
			a.editor.compose()
			before := a.editor
			requests := make(chan app.Request, 1)
			a.service = backendFunc(func(request app.Request) app.Response {
				requests <- request
				return app.Response{}
			})
			if !a.executePaletteCommand(id) || !a.discardOpen || a.editor.exit == nil || a.editor.exit.done == nil {
				t.Fatal("command did not request confirmation before leaving the dirty editor")
			}
			if !a.editor.open || a.editor.contents != before.contents || a.state.Locked || a.setup || a.page != "entries" || len(requests) != 0 {
				t.Fatal("command changed the editor or vault before confirmation")
			}
			if id == "lock" {
				done := a.editor.exit.done
				a.closeEditor()
				done()
				select {
				case request := <-requests:
					if request.Action != "lock" || !a.state.Locked || a.editor.open {
						t.Fatal("confirmed lock did not use the existing lock action")
					}
				case <-time.After(time.Second):
					t.Fatal("confirmed lock was not dispatched")
				}
			}
		})
	}
}

func TestPaletteSearchAndNewPreserveActiveEditor(t *testing.T) {
	for _, id := range []string{"search", "new"} {
		a := fixture()
		a.editEntry()
		a.editor.password = "unsaved-fixture-password"
		a.editor.compose()
		a.page = "settings"
		before := a.editor
		if !a.executePaletteCommand(id) || a.page != "entries" || a.editor != before || a.discardOpen {
			t.Fatalf("%s replaced or discarded the active editor", id)
		}
	}
}

func TestPaletteAllEntriesClearsSearchAndSettingsUseExistingDestinations(t *testing.T) {
	a := fixture()
	a.query = "github"
	if !a.executePaletteCommand("all-entries") || a.query != "" || a.filter != "all" || a.page != "entries" {
		t.Fatal("all entries retained a search filter")
	}
	for id, category := range map[string]string{"settings": "general", "security": "security", "browser": "browser", "vault-settings": "vault", "updates": "updates", "about": "about"} {
		a := fixture()
		if !a.executePaletteCommand(id) || a.page != "settings" || a.settingsState.Category != category || a.settingsState.Query != "" {
			t.Errorf("%s did not open existing settings category %s", id, category)
		}
	}
}

func TestPaletteUnconfiguredAndLockedAvailabilityMatchesExistingFlows(t *testing.T) {
	a := fixture()
	a.state.Configured = false
	for _, command := range a.paletteCommands() {
		allowed := slices.Contains([]string{"new", "open-vault", "about", "config-sync"}, command.id)
		if (command.disabled == "") != allowed {
			t.Errorf("unconfigured availability mismatch for %s", command.id)
		}
	}
	if !a.executePaletteCommand("new") || !a.setup || a.editor.open {
		t.Fatal("new entry without a vault did not open setup")
	}
	a = fixture()
	a.state.Locked = true
	if a.paletteCommandDisabled("new") != "" || !a.executePaletteCommand("new") || a.pendingUnlock.action != "new-entry" || a.editor.open {
		t.Fatal("locked new-entry command bypassed the unlock continuation")
	}
}

func TestPaletteNavigationLeavesSetupForThePreviousDestination(t *testing.T) {
	for _, tc := range []struct {
		id, page string
		kind     int
	}{
		{"all-entries", "entries", 0},
		{"drafts", "recovery", 1},
		{"trash", "recovery", 0},
	} {
		t.Run(tc.id, func(t *testing.T) {
			a := fixture()
			a.page, a.recoveryKind = tc.page, tc.kind
			a.openSetup()
			a.passphrase, a.confirmPassphrase = "setup-secret", "setup-secret"
			if !a.executePaletteCommand(tc.id) || a.setup || a.page != tc.page || a.recoveryKind != tc.kind {
				t.Fatal("navigation stayed in setup instead of returning to the requested destination")
			}
			if a.passphrase != "" || a.confirmPassphrase != "" {
				t.Fatal("navigation retained setup secrets")
			}
		})
	}
}
