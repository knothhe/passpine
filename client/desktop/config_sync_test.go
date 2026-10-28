package desktop

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/cloudsync"
	"github.com/knothhe/passpine/client/i18n"
	"github.com/knothhe/r2connect"
)

func TestConfigurationSyncJurisdictionLabelsFollowSDKValidation(t *testing.T) {
	values := supportedSyncJurisdictions()
	if len(values) == 0 || values[0] != "default" {
		t.Fatal("default R2 jurisdiction is not selectable")
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !r2connect.ValidJurisdiction(value) || seen[value] {
			t.Fatalf("unsupported or repeated jurisdiction: %q", value)
		}
		seen[value] = true
	}
}

func TestConfigurationSyncSDKFailuresHaveSpecificLocalizedRecovery(t *testing.T) {
	cases := map[string]string{
		"oauth_invalid_scope":        "Cloudflare 拒绝了请求的权限。请联系发行者检查应用权限和 Refresh Token grant 配置。",
		"oauth_denied":               "Cloudflare 授权未获允许。请重试，并在 Edit Permissions 中开启 Account API Tokens Write。",
		"oauth_callback_stopped":     "Cloudflare 回调监听已停止，请重新连接。",
		"oauth_missing_scopes":       "Cloudflare 授权缺少必需权限，请重新连接并授予所需权限。",
		"oauth_client_changed":       "OAuth Client ID 已更改。请恢复原 Client ID 并断开旧连接，再使用新的应用身份。",
		"oauth_accounts_missing":     "重新授权时请包含所有已连接的 Cloudflare 账号，或先断开对应存储桶。",
		"oauth_credentials_invalid":  "已保存的 R2 桶凭据无效，请重新连接存储桶。",
		"oauth_unknown_connection":   "请重新选择 Cloudflare 存储桶后连接。",
		"oauth_too_many_connections": "待处理的 Cloudflare 连接过多，请先断开并完成清理后重试。",
		"oauth_cleanup_pending":      "仍有 R2 凭据待清理，请重新连接同一 Cloudflare 账号后重试断开。",
		"oauth_invalid_state":        "无法读取 Cloudflare 授权状态，原资料已保留。请检查配置目录并恢复有效状态后重新读取。",
		"oauth_rate_limited":         "Cloudflare 请求受限，请稍后重试。",
		"credentials_invalid":        "本机保存的同步凭据缺失或不完整，请重新连接。",
	}
	for _, locale := range []string{"en", "zh-CN"} {
		a, _ := configurationSyncFixture()
		a.settings.Language = locale
		for code, key := range cases {
			if got, want := a.configSyncError(code), i18n.Text(locale, key); got != want {
				t.Errorf("%s/%s: %q != %q", locale, code, got, want)
			}
			if locale == "en" && !i18n.HasEnglish(key) {
				t.Errorf("missing English message for %s", code)
			}
		}
	}
}

func TestConfigurationSyncSDKWarningsAndReadFailureKeepSavedTarget(t *testing.T) {
	for _, locale := range []string{"en", "zh-CN"} {
		a, callbacks := configurationSyncFixture()
		a.settings.Language = locale
		a.configSync.editing = false
		a.configSync.mode = 0
		a.configSync.manual.config.Endpoint = "https://manual.example.test"
		a.configSync.status = app.ConfigSyncStatus{Provider: "r2", Enabled: true, Ready: true, OAuthAvailable: true, OAuthAuthorized: true, OAuthRenewable: false, WarningCode: "retired-token", Config: cloudsync.Config{Endpoint: "https://account.r2.cloudflarestorage.com", Bucket: "saved-bucket", Prefix: "personal"}}
		view := ui.NewTester(a.configSyncView, 700, 1000)
		warning := a.t("部分旧凭据或授权尚未清理。请保留本机资料，断开连接时重试清理。")
		renew := a.t("此授权不支持静默续期，请在桶凭据到期前重新连接。")
		if !view.HasText(warning) || !view.HasText(renew) {
			t.Fatal("SDK cleanup or no-refresh warning was hidden")
		}
		a.configSync.status.OAuthRenewable = true
		view.Frame()
		if view.HasText(renew) {
			t.Fatal("renewable grant still shows the no-refresh warning")
		}
		status := a.configSync.status
		status.OAuthLoadErrorCode = "oauth_invalid_state"
		a.acceptConfigSyncState(app.Data{ConfigSync: &status})
		view.Frame()
		if !view.HasText(a.t("连接状态读取失败")) || !view.HasText("saved-bucket") || !view.HasText(a.configSyncError("oauth_invalid_state")) || !view.HasText(warning) {
			t.Fatal("metadata failure reset the saved target or hid the recovery warning")
		}
		requests := make(chan app.Request, 1)
		a.service = backendFunc(func(r app.Request) app.Response { requests <- r; return syncResponse(a, status) })
		if err := view.Click(a.t("重新读取配置")); err != nil {
			t.Fatal(err)
		}
		awaitMain(t, callbacks)
		if r := <-requests; r.Action != "config-sync-oauth-reload" {
			t.Fatalf("wrong reload action: %s", r.Action)
		}
		if a.configSync.status.Config.Bucket != "saved-bucket" || a.configSync.manual.config.Endpoint != "https://manual.example.test" {
			t.Fatal("failed metadata reload replaced saved or draft configuration")
		}
		a.configSync.editing = true
		a.configSync.mode = 1
		view.Frame()
		if !view.HasText(a.t("服务端点")) || !view.HasText(a.t("保存并连接")) {
			t.Fatal("OAuth metadata failure blocked manual S3 configuration")
		}
	}
}

func TestConfigurationSyncCanOpenBeforeVaultSetup(t *testing.T) {
	a, _ := configurationSyncFixture()
	a.state.Configured = false
	a.page, a.settingsState.Category = "settings", "config-sync"
	view := ui.NewTester(testView(a), 1120, 760)
	if !a.settingsAvailableWithoutVault() || !view.HasText(a.t("将个人偏好加密保存到你自己的对象存储，在其他设备恢复。")) || !view.HasText(a.t("服务端点")) {
		t.Fatal("configuration sync is blocked behind vault setup")
	}
}

func TestConfigurationSyncOperationBlocksSuspensionAndClearsOnlySecretDrafts(t *testing.T) {
	a, _ := configurationSyncFixture()
	now := time.Now()
	a.state.Settings.LightweightMode = true
	a.lifecycle.hiddenAt = now.Add(-lightweightIdleDelay)
	a.configSync.operation = "config-sync-now"
	if a.lightweightSuspensionReady(now) || !a.updateInstallBlocked() {
		t.Fatal("an active sync did not block UI suspension and update installation")
	}
	a.configSync.manual.config.Endpoint = "https://storage.example.test"
	a.configSync.manual.secrets = cloudsync.Secrets{AccessKeyID: "fake", SecretAccessKey: "fake-secret", Passphrase: "fake-long-passphrase"}
	a.configSync.r2.secrets = cloudsync.Secrets{Passphrase: "fake-r2-passphrase"}
	a.clearSecrets()
	if a.configSync.manual.secrets != (cloudsync.Secrets{}) || a.configSync.r2.secrets != (cloudsync.Secrets{}) || a.configSync.manual.config.Endpoint != "https://storage.example.test" {
		t.Fatal("secret cleanup retained a sync secret or removed public input")
	}
	a.configSync.operation = ""
	if !a.lightweightSuspensionReady(now) {
		t.Fatal("completed sync still blocks UI suspension")
	}
}

func TestConfigurationSyncQuitCancelsAndWaitsForItsLane(t *testing.T) {
	a, callbacks, count := quitFixture(t)
	a.state.Preview = false
	a.configSync.operation = "config-sync-oauth-start"
	started, release := make(chan struct{}), make(chan struct{})
	a.service = shutdownBackend{backendFunc: func(r app.Request) app.Response {
		if r.Action != "config-sync-cancel" {
			t.Errorf("quit sent %s", r.Action)
		}
		close(started)
		<-release
		return syncResponse(a, app.ConfigSyncStatus{})
	}, shutdown: func(context.Context) error { return nil }}
	a.requestQuit()
	<-started
	if a.closing || a.quit.draining || *count != 0 || a.configSync.operation != "config-sync-cancel" {
		t.Fatal("quit raced past synchronization cancellation")
	}
	close(release)
	for !a.quit.ready {
		awaitMain(t, callbacks)
	}
	if *count != 1 || a.configSync.operation != "" {
		t.Fatal("quit did not finish after cancellation")
	}
}

func configurationSyncFixture() (*application, chan func()) {
	a := fixture()
	a.state.Preview = false
	a.settings, a.settingsSaved, a.settingsObserved = a.state.Settings, a.state.Settings, a.state.Settings
	a.settingsLoaded = true
	callbacks := make(chan func(), 20)
	a.runOnMain = func(f func()) { callbacks <- f }
	a.configSync.loaded = true
	a.configSync.mode = 1
	return a, callbacks
}

func syncResponse(_ *application, status app.ConfigSyncStatus) app.Response {
	// Configuration-only fakes do not read the UI model from worker goroutines.
	// The dedicated remote-preferences test supplies an explicit saved snapshot.
	return app.Response{Status: "ok", Data: app.Data{ConfigSync: &status}}
}

func TestConfigurationSyncHasIndependentCategoryAndRefreshLane(t *testing.T) {
	a, callbacks := configurationSyncFixture()
	a.configSync.loaded = false
	a.page = "settings"
	a.busy = true // Credential work does not block this read.
	requests := make(chan app.Request, 4)
	a.service = backendFunc(func(r app.Request) app.Response { requests <- r; return syncResponse(a, app.ConfigSyncStatus{}) })
	a.navigateSettingsCategory("config-sync")
	awaitMain(t, callbacks)
	if r := <-requests; r.Action != "config-sync-status" {
		t.Fatalf("wrong category action: %s", r.Action)
	}
	if a.settingsState.Category != "config-sync" || !a.configSync.loaded || a.configSync.operation != "" || !a.busy {
		t.Fatal("category did not use its independent read lane")
	}
	view := ui.NewTester(testView(a), 1120, 760)
	if !view.HasText(a.t("将个人偏好加密保存到你自己的对象存储，在其他设备恢复。")) || !view.HasText(a.t("手动配置 S3")) {
		t.Fatal("sync settings are missing")
	}
	view.Frame()
	select {
	case r := <-requests:
		t.Fatalf("repaint sent %s", r.Action)
	default:
	}
}

func TestConfigurationSyncManualFailureRetainsDraftAndEditsMadeDuringSave(t *testing.T) {
	a, callbacks := configurationSyncFixture()
	a.configSync.manual = configurationSyncDraft{config: cloudsync.Config{Endpoint: "https://storage.example.test", Region: "auto", Bucket: "passpine-config", Prefix: "personal", PathStyle: true}, secrets: cloudsync.Secrets{AccessKeyID: "fake-key", SecretAccessKey: "fake-secret", Passphrase: "fake-long-sync-phrase"}}
	started, release := make(chan app.Request, 2), make(chan struct{})
	a.service = backendFunc(func(r app.Request) app.Response {
		started <- r
		<-release
		result := syncResponse(a, app.ConfigSyncStatus{Provider: "s3", Enabled: true})
		result.Status, result.Code = "error", "network"
		return result
	})
	beforeDetail := a.detail
	a.manageConfigSync("config-sync-save", a.syncConnectionRequest())
	r := <-started
	a.configSync.manual.secrets.SecretAccessKey = "fake-edited-secret"
	a.settings.Language = "en"
	a.manageConfigSync("config-sync-save", a.syncConnectionRequest())
	a.hidePreferences() // Navigation preserves the manual draft.
	close(release)
	awaitMain(t, callbacks)
	if r.SyncConnection.Secrets.SecretAccessKey != "fake-secret" || a.configSync.manual.secrets.SecretAccessKey != "fake-edited-secret" || a.configSync.manual.secrets.Passphrase != "fake-long-sync-phrase" {
		t.Fatal("request did not capture its own draft or failure erased newer edits")
	}
	if !a.configSync.editing || a.configSync.errorCode != "network" || !reflect.DeepEqual(a.detail, beforeDetail) {
		t.Fatal("failure discarded editing or credential details")
	}
	select {
	case <-started:
		t.Fatal("duplicate click started another worker")
	default:
	}
	view := ui.NewTester(a.configSyncView, 700, 1000)
	if view.HasText("fake-edited-secret") || view.HasText("fake-long-sync-phrase") {
		t.Fatal("manual secrets were displayed as plain text")
	}
	if !view.HasText(a.t("无法完成配置同步。请检查连接和访问权限后重试，输入会保留。")) {
		t.Fatal("failure feedback did not follow the current language")
	}
}

func TestConfigurationSyncSuccessfulSaveDoesNotClearNewerSecrets(t *testing.T) {
	a, callbacks := configurationSyncFixture()
	started, release := make(chan struct{}), make(chan struct{})
	a.configSync.manual.secrets = cloudsync.Secrets{AccessKeyID: "fake-key", SecretAccessKey: "fake-secret", Passphrase: "fake-long-sync-phrase"}
	a.service = backendFunc(func(r app.Request) app.Response {
		close(started)
		<-release
		return syncResponse(a, app.ConfigSyncStatus{Provider: "s3", Enabled: true, Ready: true})
	})
	a.manageConfigSync("config-sync-save", a.syncConnectionRequest())
	<-started
	a.configSync.manual.secrets.Passphrase = "a-new-unsaved-phrase"
	close(release)
	awaitMain(t, callbacks)
	if a.configSync.manual.secrets.Passphrase != "a-new-unsaved-phrase" || !a.configSync.editing {
		t.Fatal("save cleared a newer draft or did not show saved connection")
	}
}

func TestConfigurationSyncMissingOAuthConfigurationHasManualFallback(t *testing.T) {
	a, _ := configurationSyncFixture()
	a.configSync.mode = 0
	var calls atomic.Int32
	a.service = backendFunc(func(app.Request) app.Response { calls.Add(1); return app.Response{} })
	view := ui.NewTester(a.configSyncView, 700, 600)
	if !view.HasText(a.t("此版本尚未配置 Cloudflare 授权。请使用手动 S3 配置。")) {
		t.Fatal("publisher configuration requirement is not visible")
	}
	if _, ok := view.Find(a.t("连接 Cloudflare")); ok {
		t.Fatal("unconfigured OAuth connection is actionable")
	}
	if err := view.Click(a.t("手动配置 S3")); err != nil {
		t.Fatal(err)
	}
	if a.configSync.mode != 1 || !view.HasText(a.t("服务端点")) || calls.Load() != 0 {
		t.Fatal("manual fallback started OAuth or failed to show its inputs")
	}
}

func TestConfigurationSyncCancelSupersedesLateOAuthResult(t *testing.T) {
	a, callbacks := configurationSyncFixture()
	a.configSync.mode = 0
	started, release := make(chan struct{}), make(chan struct{})
	a.service = backendFunc(func(r app.Request) app.Response {
		if r.Action == "config-sync-oauth-start" {
			close(started)
			<-release
			return syncResponse(a, app.ConfigSyncStatus{OAuthAvailable: true, OAuthStage: "bucket", Accounts: []r2connect.Account{{ID: "old-account", Name: "Old account"}}})
		}
		if r.Action != "config-sync-cancel" {
			t.Errorf("unexpected action: %s", r.Action)
		}
		close(release)
		return syncResponse(a, app.ConfigSyncStatus{OAuthAvailable: true})
	})
	a.manageConfigSync("config-sync-oauth-start", a.syncConnectionRequest())
	<-started
	a.manageConfigSync("config-sync-cancel", nil)
	awaitMain(t, callbacks)
	awaitMain(t, callbacks)
	if a.configSync.operation != "" || a.configSync.status.OAuthStage != "" || len(a.configSync.status.Accounts) != 0 {
		t.Fatal("a late authorization result restored a cancelled candidate")
	}
}

func TestConfigurationSyncBucketResultCannotFollowDifferentDraftAccount(t *testing.T) {
	a, callbacks := configurationSyncFixture()
	a.configSync.mode = 0
	a.configSync.r2.account = "first-account"
	started, release := make(chan struct{}), make(chan struct{})
	a.service = backendFunc(func(r app.Request) app.Response {
		close(started)
		<-release
		return syncResponse(a, app.ConfigSyncStatus{OAuthAvailable: true, OAuthStage: "bucket", Buckets: []r2connect.Bucket{{Name: "first-bucket", Jurisdiction: "default"}}})
	})
	a.manageConfigSync("config-sync-oauth-buckets", a.syncConnectionRequest())
	<-started
	a.configSync.r2.account = "second-account"
	close(release)
	awaitMain(t, callbacks)
	if len(a.configSync.status.Buckets) != 0 || a.configSync.r2.account != "second-account" {
		t.Fatal("old account bucket options replaced the new draft")
	}
}

func TestConfigurationSyncRemotePreferencesKeepLocalDraftAndDetail(t *testing.T) {
	a, _ := configurationSyncFixture()
	a.settings.Theme = "light" // User edit not yet saved or observed by a frame.
	a.editor.open = true
	a.editor.password = "fake-unsaved-password"
	before := a.detail
	settings := a.settingsSaved
	settings.Theme, settings.Language, settings.ThemeStyle, settings.FontScale = "dark", "en", "onevra", 110
	a.acceptConfigSyncState(app.Data{Settings: settings, ConfigSync: &app.ConfigSyncStatus{Enabled: true, Ready: true}})
	if a.settings.Theme != "light" || a.settings.Language != "en" || a.settings.ThemeStyle != "onevra" || a.settings.FontScale != 110 {
		t.Fatal("remote settings replaced a local draft or did not apply untouched fields")
	}
	if a.settingsSaved.Theme != "dark" || a.settingsObserved.Theme == "light" {
		t.Fatal("remote update hid the unobserved local draft from autosave")
	}
	if !a.editor.open || a.editor.password != "fake-unsaved-password" || !reflect.DeepEqual(before, a.detail) {
		t.Fatal("remote preference refresh reset the editor or credential details")
	}
}

func TestConfigurationSyncConflictUsesLocalizedFieldLabelsAndExplicitResolution(t *testing.T) {
	a, callbacks := configurationSyncFixture()
	a.configSync.editing = false
	a.configSync.status = app.ConfigSyncStatus{Enabled: true, Ready: true, Provider: "s3", ErrorCode: "conflict", ConflictKeys: []string{"desktop.theme", "browser.language", "browser.associations.scope.one", "browser.associationLegacy.one", "browser.associationAliases.scope.one", "future.collection"}, Config: cloudsync.Config{Bucket: "passpine-config", Endpoint: "https://storage.example.test", Prefix: "personal"}}
	requests := make(chan app.Request, 2)
	a.service = backendFunc(func(r app.Request) app.Response { requests <- r; return syncResponse(a, a.configSync.status) })
	view := ui.NewTester(a.configSyncView, 700, 760)
	if !view.HasText(a.t("客户端主题")+" · "+a.t("扩展语言")+" · "+a.t("扩展网站关联")) || view.HasText("future.collection") || view.HasText("browser.associations.scope") {
		t.Fatal("conflict fields were shown as technical keys")
	}
	if err := view.Click(a.t("保留此设备配置")); err != nil {
		t.Fatal(err)
	}
	awaitMain(t, callbacks)
	if r := <-requests; r.Action != "config-sync-resolve" || r.SyncConnection.Resolution != "local" {
		t.Fatal("conflict action did not use explicit local resolution")
	}
}

func TestConfigurationSyncLayoutsAndDraftSurviveLanguageAndMethodChanges(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, width := range []int{460, 1120} {
			for _, dark := range []bool{false, true} {
				for _, state := range []string{"manual", "oauth", "bucket", "connected", "conflict", "diagnostics"} {
					t.Run(fmt.Sprintf("%s/%d/%t/%s", locale, width, dark, state), func(t *testing.T) {
						a, _ := configurationSyncFixture()
						a.settings.Language, a.settingsSaved.Language, a.state.Settings.Language = locale, locale, locale
						a.page, a.settingsState.Category = "settings", "config-sync"
						a.configSync.manual.config.Endpoint = "https://storage.example.test"
						a.configSync.manual.config.Bucket = "passpine-config"
						a.configSync.status.OAuthAvailable = true
						label := "服务端点"
						if state == "oauth" {
							a.configSync.mode = 0
							a.configSync.status.OAuthAvailable = false
							label = "此版本尚未配置 Cloudflare 授权。请使用手动 S3 配置。"
						}
						if state == "bucket" {
							a.configSync.mode = 0
							a.configSync.status.OAuthStage = "bucket"
							a.configSync.status.Accounts = []r2connect.Account{{ID: "account", Name: "Personal account"}}
							a.configSync.r2.account = "account"
							label = "选择 Cloudflare 账号"
						}
						if state == "connected" || state == "conflict" || state == "diagnostics" {
							a.configSync.editing = false
							a.configSync.status.Provider = "s3"
							a.configSync.status.Enabled = true
							a.configSync.status.Ready = true
							a.configSync.status.Config = a.configSync.manual.config
							label = "编辑连接"
						}
						if state == "conflict" {
							a.configSync.status.ErrorCode = "conflict"
							a.configSync.status.ConflictKeys = []string{"desktop.theme"}
							label = "保留此设备配置"
						}
						if state == "diagnostics" {
							a.configSync.status.Provider = "r2"
							a.configSync.status.OAuthAuthorized = true
							a.configSync.status.OAuthRenewable = false
							a.configSync.status.OAuthLoadErrorCode = "oauth_invalid_state"
							a.configSync.status.WarningCode = "oauth_cleanup_pending"
							label = "重新读取配置"
						}
						view := ui.NewTester(testView(a), width, 760)
						view.SetPreferences(ui.Preferences{ReduceMotion: true})
						view.SetDark(dark)
						view.Advance(time.Second)
						if bounds, ok := view.Find(a.t(label)); !ok || bounds.X < 0 || bounds.X+bounds.W > float32(width)+1 {
							t.Fatalf("%s horizontally clipped: %v", label, bounds)
						}
						if a.configSync.manual.config.Endpoint != "https://storage.example.test" || a.configSync.manual.config.Bucket != "passpine-config" || a.configSync.operation != "" {
							t.Fatal("render/locale changed draft or started work")
						}
						if dir := os.Getenv("PASSPINE_SYNC_SCREENSHOT_DIR"); dir != "" {
							if err := os.MkdirAll(dir, 0700); err != nil {
								t.Fatal(err)
							}
							file, err := os.Create(filepath.Join(dir, fmt.Sprintf("sync-%s-%d-dark-%t-%s.png", locale, width, dark, state)))
							if err != nil {
								t.Fatal(err)
							}
							err = png.Encode(file, view.Image())
							closeErr := file.Close()
							if err != nil || closeErr != nil {
								t.Fatal(err, closeErr)
							}
						}
					})
				}
			}
		}
	}
}
