package desktop

import (
	"time"

	"github.com/knothhe/passpine/client/app"
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
			a.cancelQuitOnSaveFailure()
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
	finishLogin, err := a.prepareLaunchAtLogin(settings.LaunchAtLogin)
	if err != nil {
		a.cancelQuitOnSaveFailure()
		finishShortcut(false)
		a.errorText = a.t("无法设置开机自启：%s", err)
		a.settings.LaunchAtLogin = a.settingsSaved.LaunchAtLogin
		a.settingsObserved = a.settings
		a.settingsDirty = a.settings != a.settingsSaved
		if a.settingsDirty {
			a.saveSettingsAutomatically()
		}
		return
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
			finishLogin(result.Status == "ok")
			if result.Status != "ok" {
				a.cancelQuitOnSaveFailure()
				a.errorText = result.Message
				if a.errorText == "" {
					a.errorText = a.t("设置保存失败，请重试")
				}
				if shortcutChanged && a.settings.GlobalShortcut == settings.GlobalShortcut {
					a.settings.GlobalShortcut = a.settingsSaved.GlobalShortcut
					a.settingsObserved = a.settings
					a.shortcut.errorText = a.t("快捷键保存失败，请重试。")
					if a.shortcut.registered != "" {
						a.shortcut.errorText = a.t("快捷键保存失败，原快捷键保持有效。请重试。")
					}
				}
				if a.settings.LaunchAtLogin == settings.LaunchAtLogin {
					a.settings.LaunchAtLogin = a.settingsSaved.LaunchAtLogin
					a.settingsObserved = a.settings
				}
			} else {
				updateDownloadChanged := a.settingsSaved.AutoDownloadUpdates != result.Data.Settings.AutoDownloadUpdates
				a.settingsSaved = result.Data.Settings
				a.state.Settings = result.Data.Settings
				if a.settings == settings {
					a.settings = result.Data.Settings
					a.settingsObserved = a.settings
					a.settingsDirty = false
					a.notice = a.t("已保存")
				} else {
					a.settingsDirty = true
				}
				a.applyTheme()
				a.applyLanguage()
				a.applyNativeSettings()
				if updateDownloadChanged && a.updates.manager != nil {
					a.updates.manager.SetAutoDownload(result.Data.Settings.AutoDownloadUpdates)
				}
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
				a.cancelQuitOnSaveFailure()
				a.errorText = result.Message
				if a.errorText == "" {
					a.errorText = a.t("Git 远端地址保存失败，请重新编辑")
				}
			} else {
				a.state.GitRemote = result.Data.GitRemote
				if a.gitRemote == remote {
					a.gitRemote = result.Data.GitRemote
					a.gitRemoteObserved = a.gitRemote
					a.notice = a.t("已保存")
				}
			}
			a.invalidate()
		})
	}()
}
