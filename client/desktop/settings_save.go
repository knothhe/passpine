package desktop

import (
	"time"

	"github.com/knothhe/passpine-neo/client/app"
)

// Coalesce changes while a save is in flight; keep the latest local values.
func (a *application) saveSettingsAutomatically() {
	if a.settingsSaving || !a.settingsDirty || a.closing || a.service == nil {
		return
	}
	settings := a.settings
	finishShortcut := func(bool) {}
	shortcutChanged := settings.GlobalShortcut != a.settingsSaved.GlobalShortcut
	if shortcutChanged {
		var err error
		finishShortcut, err = a.prepareGlobalShortcut(settings.GlobalShortcut)
		if err != nil {
			a.shortcut.errorText = err.Error()
			a.settings.GlobalShortcut = a.settingsSaved.GlobalShortcut
			a.settingsObserved = a.settings
			a.settingsDirty = a.settings != a.settingsSaved
			if a.settingsDirty {
				a.saveSettingsAutomatically()
			}
			return
		}
	}
	a.settingsSaving = true
	a.settingsDirty = false
	go func() {
		result := a.service.Execute(app.Request{Action: "settings", Settings: &settings})
		a.runOnMain(func() {
			if a.closing {
				return
			}
			a.settingsSaving = false
			finishShortcut(result.Status == "ok")
			if result.Status != "ok" {
				a.errorText = result.Message
				if a.errorText == "" {
					a.errorText = "设置保存失败，请重试"
				}
				if shortcutChanged && a.settings.GlobalShortcut == settings.GlobalShortcut {
					a.settings.GlobalShortcut = a.settingsSaved.GlobalShortcut
					a.settingsObserved = a.settings
					a.shortcut.errorText = "快捷键保存失败，请重试。"
					if a.shortcut.registered != "" {
						a.shortcut.errorText = "快捷键保存失败，原快捷键保持有效。请重试。"
					}
				}
			} else {
				a.settingsSaved = result.Data.Settings
				a.state.Settings = result.Data.Settings
				if a.settings == settings {
					a.settings = result.Data.Settings
					a.settingsObserved = a.settings
					a.settingsDirty = false
					a.settingsSavedUntil = time.Now().Add(2 * time.Second)
				} else {
					a.settingsDirty = true
				}
				a.applyTheme()
			}
			a.saveSettingsAutomatically()
			a.invalidate()
		})
	}()
}

func (a *application) saveGitRemoteAutomatically() {
	if a.gitRemoteSaving || a.closing || a.service == nil || a.busy {
		return
	}
	remote, vaultID := a.gitRemote, a.gitRemoteVaultID
	a.gitRemoteSaveAt = time.Time{}
	if a.state.VaultID != vaultID {
		a.gitRemote = a.state.GitRemote
		a.gitRemoteObserved = a.gitRemote
		return
	}
	a.gitRemoteSaving = true
	go func() {
		result := a.service.Execute(app.Request{Action: "git-remote", URL: remote, VaultID: vaultID})
		a.runOnMain(func() {
			a.gitRemoteSaving = false
			if a.closing || a.state.VaultID != vaultID {
				return
			}
			if result.Status != "ok" {
				a.errorText = result.Message
				if a.errorText == "" {
					a.errorText = "Git 远端地址保存失败，请重新编辑"
				}
			} else {
				a.state.GitRemote = result.Data.GitRemote
				if a.gitRemote == remote {
					a.gitRemote = result.Data.GitRemote
					a.gitRemoteObserved = a.gitRemote
					a.settingsSavedUntil = time.Now().Add(2 * time.Second)
				}
			}
			a.invalidate()
		})
	}()
}
