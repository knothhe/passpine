package desktop

import (
	"strings"
	"testing"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/cloudsync"
)

func TestConfigurationSyncNotificationsDeduplicateRetriesAndResetAfterRecovery(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a, _ := configurationSyncOptionsFixture(locale)
			a.lifecycle.mainVisible = false
			var titles, bodies []string
			a.configSync.notifyIssue = func(title, body string, onClick func()) bool {
				if onClick == nil {
					t.Error("a sync notification cannot open its settings")
				}
				titles, bodies = append(titles, title), append(bodies, body)
				return true
			}
			status := app.ConfigSyncStatus{
				Options: app.DefaultConfigSyncOptions(), Provider: "s3", Enabled: true, Ready: true,
				SpaceID: strings.Repeat("a", 64), AttemptSequence: 1, AttemptErrorCode: "conflict", ErrorCode: "conflict",
				Config: cloudsync.Config{Endpoint: "https://private-endpoint.example.test", Bucket: "private-bucket", Prefix: "private-prefix"},
			}
			a.acceptConfigSyncNotification(status)
			a.acceptConfigSyncNotification(status)
			status.AttemptSequence++
			a.acceptConfigSyncNotification(status)
			if len(titles) != 1 {
				t.Fatal("status polling or a repeated failure raised another sync reminder")
			}
			status.AttemptSequence++
			status.AttemptErrorCode, status.ErrorCode = "connection_failed", "connection_failed"
			a.acceptConfigSyncNotification(status)
			if len(titles) != 2 {
				t.Fatal("a different sync failure did not raise a new reminder")
			}
			status.AttemptSequence++
			status.AttemptErrorCode, status.ErrorCode = "", ""
			a.acceptConfigSyncNotification(status)
			status.AttemptSequence++
			status.AttemptErrorCode, status.ErrorCode = "connection_failed", "connection_failed"
			a.acceptConfigSyncNotification(status)
			if len(titles) != 3 {
				t.Fatal("recovery did not permit a later failure reminder")
			}
			status.AttemptSequence++
			status.SpaceID = strings.Repeat("b", 64)
			a.acceptConfigSyncNotification(status)
			if len(titles) != 4 {
				t.Fatal("a different sync space inherited another space's failure suppression")
			}
			for i := range titles {
				if titles[i] == "" || bodies[i] == "" {
					t.Fatal("a sync reminder has no readable title or recovery information")
				}
				for _, private := range []string{"private-endpoint", "private-bucket", "private-prefix", "connection_failed"} {
					if strings.Contains(titles[i], private) || strings.Contains(bodies[i], private) {
						t.Fatalf("sync reminder exposes private connection data or a technical code: %q", private)
					}
				}
			}
		})
	}
}

func TestConfigurationSyncNotificationsUseSavedOptionAndIgnoreNonAttempts(t *testing.T) {
	a, _ := configurationSyncOptionsFixture("en")
	a.lifecycle.mainVisible = false
	calls := 0
	a.configSync.notifyIssue = func(_, _ string, _ func()) bool { calls++; return true }
	status := app.ConfigSyncStatus{Options: app.DefaultConfigSyncOptions(), Provider: "s3", SpaceID: strings.Repeat("a", 64), ErrorCode: "conflict"}
	a.acceptConfigSyncNotification(status)
	if calls != 0 {
		t.Fatal("a status-only error raised a completed-operation reminder")
	}
	status.Options.NotifyErrors = false
	status.AttemptSequence, status.AttemptErrorCode = 1, "conflict"
	a.configSync.options.NotifyErrors = true // Unsaved UI edits do not control reminders.
	a.acceptConfigSyncNotification(status)
	if calls != 0 {
		t.Fatal("an unsaved notification preference overrode the saved option")
	}
	status.Options.NotifyErrors = true
	status.AttemptSequence, status.AttemptErrorCode = 2, "connection_failed"
	a.configSync.options.NotifyErrors = false
	a.acceptConfigSyncNotification(status)
	if calls != 1 {
		t.Fatal("an unsaved notification draft suppressed the saved notification behavior")
	}
	status.AttemptSequence, status.AttemptErrorCode = 3, "cancelled"
	a.acceptConfigSyncNotification(status)
	if calls != 1 {
		t.Fatal("an intentional cancellation raised an abnormal-sync reminder")
	}
}

func TestConfigurationSyncVisibleFailureUsesLocalizedSettingsAction(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a, _ := configurationSyncOptionsFixture(locale)
			a.lifecycle.mainVisible = true
			nativeCalls := 0
			a.configSync.notifyIssue = func(_, _ string, _ func()) bool { nativeCalls++; return true }
			status := app.ConfigSyncStatus{Options: app.DefaultConfigSyncOptions(), Provider: "s3", SpaceID: strings.Repeat("a", 64), AttemptSequence: 1, AttemptErrorCode: "conflict"}
			a.acceptConfigSyncNotification(status)
			if nativeCalls != 0 || a.notice == "" || a.notifications.action != a.t("查看同步设置") || a.notifications.onAction == nil {
				t.Fatal("a visible sync failure did not provide a localized in-app settings action")
			}
			if a.notifications.description != a.configSyncError("conflict") {
				t.Fatal("the sync reminder did not use the controlled localized error copy")
			}
		})
	}
}

func TestConfigurationSyncNotificationActionsExpireOnRecoveryOrDisable(t *testing.T) {
	a, _ := configurationSyncOptionsFixture("en")
	a.page, a.settingsState.Category = "entries", "general"
	a.lifecycle.mainVisible = false
	var open func()
	a.configSync.notifyIssue = func(_, _ string, onClick func()) bool { open = onClick; return true }
	status := app.ConfigSyncStatus{Options: app.DefaultConfigSyncOptions(), Provider: "s3", SpaceID: strings.Repeat("a", 64), AttemptSequence: 1, AttemptErrorCode: "conflict"}
	a.acceptConfigSyncNotification(status)
	stale := open
	status.AttemptSequence, status.AttemptErrorCode = 2, ""
	a.acceptConfigSyncNotification(status)
	stale()
	if a.page != "entries" || a.settingsState.Category != "general" {
		t.Fatal("a notification action navigated after its issue recovered")
	}
	status.AttemptSequence, status.AttemptErrorCode = 3, "conflict"
	a.acceptConfigSyncNotification(status)
	stale = open
	status.Options.NotifyErrors = false
	status.AttemptSequence = 4
	a.acceptConfigSyncNotification(status)
	stale()
	if a.page != "entries" || a.settingsState.Category != "general" {
		t.Fatal("a disabled notification retained an active navigation callback")
	}
	status.Options.NotifyErrors = true
	status.AttemptSequence = 5
	a.acceptConfigSyncNotification(status)
	open()
	if a.page != "settings" || a.settingsState.Category != "config-sync" {
		t.Fatal("a current notification did not open configuration sync settings")
	}
}

func TestConfigurationSyncNativeNotificationFallbackAndUnknownErrorsStayControlled(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a, _ := configurationSyncOptionsFixture(locale)
			a.lifecycle.mainVisible = false
			calls := 0
			a.configSync.notifyIssue = func(_, _ string, _ func()) bool { calls++; return false }
			privateCode := "private-response https://private-endpoint.example.test fake-secret-passphrase"
			status := app.ConfigSyncStatus{Options: app.DefaultConfigSyncOptions(), Provider: "r2", SpaceID: strings.Repeat("a", 64), AttemptSequence: 1, AttemptErrorCode: privateCode}
			a.acceptConfigSyncNotification(status)
			if calls != 1 || a.notice != a.t("配置同步需要处理") || a.notifications.action != a.t("查看同步设置") || a.notifications.onAction == nil {
				t.Fatal("unavailable native notifications did not leave an actionable in-app reminder")
			}
			if a.notifications.description != a.configSyncError("connection_failed") || strings.Contains(a.notice+a.notifications.description, "private-") || strings.Contains(a.notice+a.notifications.description, "fake-secret") {
				t.Fatal("an unknown provider error escaped the controlled localized failure copy")
			}
		})
	}
}
