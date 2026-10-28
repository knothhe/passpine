package desktop

import (
	"github.com/egoist/mygo"
	"github.com/knothhe/passpine/client/app"
)

// Polling and retries describe the same issue until a successful cloud attempt
// ends it. Keep notification ownership separate from connection editor drafts.
func (a *application) acceptConfigSyncNotification(status app.ConfigSyncStatus) {
	if !status.Options.NotifyErrors || a.state.Preview || a.closing {
		a.clearConfigSyncNativeNotification()
		a.configSync.notificationSequence = status.AttemptSequence
		a.configSync.notificationCode, a.configSync.notificationSpace = "", ""
		return
	}
	if status.AttemptSequence == 0 || status.AttemptSequence <= a.configSync.notificationSequence {
		return
	}
	a.configSync.notificationSequence = status.AttemptSequence
	code := status.AttemptErrorCode
	if code == "" || code == "cancelled" || code == "not_ready" {
		a.clearConfigSyncNativeNotification()
		a.configSync.notificationCode, a.configSync.notificationSpace = "", ""
		return
	}
	if code == a.configSync.notificationCode && status.SpaceID == a.configSync.notificationSpace {
		return
	}
	a.clearConfigSyncNativeNotification()
	a.configSync.notificationCode, a.configSync.notificationSpace = code, status.SpaceID
	serial := a.configSync.notificationSerial
	current := func() bool {
		return !a.closing && !a.quit.requested && a.configSync.notificationSerial == serial && a.configSync.status.Options.NotifyErrors
	}
	open := func() {
		if !current() {
			return
		}
		a.clearConfigSyncNativeNotification()
		a.navigateSettingsCategory("config-sync")
		if a.lifecycle.nativeReady {
			a.show()
		}
	}
	title, body := a.t("配置同步需要处理"), a.configSyncError(code)
	if !a.lifecycle.mainVisible {
		if a.configSync.notifyIssue != nil {
			if a.configSync.notifyIssue(title, body, open) {
				return
			}
		} else if a.lifecycle.nativeReady && mygo.NotificationsSupported() {
			// Each delivery owns a distinct native ID: a late consent response
			// for an obsolete notification must not remove its replacement.
			notification := mygo.NewNotification(mygo.NotificationOptions{Group: "passpine-config-sync", Title: title, Body: body})
			notification.OnClick(open)
			a.configSync.nativeNotification = notification
			// Showing the first macOS notification may await system consent.
			// Keep that wait off the UI thread and fall back to an in-app alert.
			go func() {
				err := notification.Show()
				a.runOnMain(func() {
					if !current() {
						notification.Close()
						return
					}
					if err != nil {
						a.notifyErrorWithAction(a.t("配置同步需要处理"), a.configSyncError(code), a.t("查看同步设置"), open)
						a.invalidate()
					}
				})
			}()
			return
		}
	}
	a.notifyErrorWithAction(title, body, a.t("查看同步设置"), open)
}

func (a *application) clearConfigSyncNativeNotification() {
	a.configSync.notificationSerial++
	if notification := a.configSync.nativeNotification; notification != nil {
		notification.Close()
		a.configSync.nativeNotification = nil
	}
}
