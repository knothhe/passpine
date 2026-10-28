package desktop

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/cloudsync"
	"github.com/knothhe/r2connect"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

type configurationSyncDraft struct {
	config                        cloudsync.Config
	secrets                       cloudsync.Secrets
	account, jurisdiction, bucket string
}

type configurationSyncUI struct {
	status                                                       app.ConfigSyncStatus
	options, optionsSaved                                        app.ConfigSyncOptions
	optionsLoaded                                                bool
	optionsErrorCode                                             string
	manual, r2                                                   configurationSyncDraft
	mode                                                         int
	loaded, editing, advanced, disconnectConfirm, refreshPending bool
	operation, errorCode, message                                string
	started                                                      time.Time
	serial                                                       uint64
	notificationSequence, notificationSerial                     uint64
	notificationCode, notificationSpace                          string
	nativeNotification                                           *mygo.Notification
	notifyIssue                                                  func(title, body string, onClick func()) bool
	transfer                                                     *configurationSyncTransfer
	readTransfer                                                 func(string) (app.ConfigSyncOptions, *cloudsync.Config, error)
	writeTransfer                                                func(string, []byte) error
}

func newConfigurationSyncUI() configurationSyncUI {
	defaults := cloudsync.Config{Region: "auto", Prefix: "passpine", PathStyle: true}
	options := app.DefaultConfigSyncOptions()
	return configurationSyncUI{editing: true, options: options, optionsSaved: options, manual: configurationSyncDraft{config: defaults}, r2: configurationSyncDraft{config: defaults, jurisdiction: "default"}}
}

// These are presentation labels; the SDK remains the authority on which
// jurisdictions are accepted and how their endpoints and resources are formed.
func supportedSyncJurisdictions() []string {
	var supported []string
	for _, value := range []string{"default", "eu", "us", "fedramp", "fedramp-high"} {
		if r2connect.ValidJurisdiction(value) {
			supported = append(supported, value)
		}
	}
	return supported
}

// Configuration work has its own lane: a slow authorization or object request
// cannot block vault reads, editors, ordinary preferences or language changes.
func (a *application) manageConfigSync(action string, connection *app.SyncConnectionRequest) {
	a.manageConfigSyncRequest(app.Request{Action: action, SyncConnection: connection})
}

func (a *application) manageConfigSyncRequest(request app.Request) {
	action, connection := request.Action, request.SyncConnection
	if a.service == nil || a.closing || (a.quit.requested || a.updates.preparing) && action != "config-sync-cancel" {
		return
	}
	if action == "config-sync-cancel" && a.configSync.operation == action {
		return
	}
	if a.configSync.operation != "" && action != "config-sync-cancel" {
		if action == "config-sync-status" {
			a.configSync.refreshPending = true
		}
		return
	}
	a.configSync.serial++
	serial := a.configSync.serial
	settingsRevision := a.settingsSaveRevision
	a.configSync.operation = action
	a.configSync.started = time.Now()
	var captured *app.SyncConnectionRequest
	if connection != nil {
		copy := *connection
		captured = &copy
	}
	request.SyncConnection = captured
	if request.SyncOptions != nil {
		options := *request.SyncOptions
		request.SyncOptions = &options
	}
	go func() {
		result := a.service.Execute(request)
		a.runOnMain(func() {
			if serial != a.configSync.serial {
				return
			}
			a.configSync.operation = ""
			a.configSync.started = time.Time{}
			if a.closing {
				return
			}
			if action == "config-sync-oauth-buckets" && captured != nil && result.Data.ConfigSync != nil && (captured.AccountID != a.configSync.r2.account || captured.Jurisdiction != a.configSync.r2.jurisdiction) {
				result.Data.ConfigSync.Buckets = nil
			}
			if settingsRevision != a.settingsSaveRevision {
				// This snapshot may predate a preference write committed while
				// the request or its main-thread callback was pending. Keep that
				// newer baseline and read current preferences in the next turn.
				result.Data.Settings = app.Settings{}
				a.configSync.refreshPending = true
			}
			options := a.configSync.options
			keepOptionsDraft := action == "config-sync-options" && request.SyncOptions != nil && options != *request.SyncOptions
			a.acceptConfigSyncState(result.Data)
			if keepOptionsDraft {
				a.configSync.options = options
			}
			if action == "config-sync-options" {
				a.configSync.optionsErrorCode = result.Code
				if result.Status != "ok" && result.Code == "" {
					a.configSync.optionsErrorCode = "local_storage"
				}
			} else {
				a.configSync.errorCode = result.Code
			}
			if result.Status == "ok" && action != "config-sync-options" {
				a.configSync.errorCode = a.configSync.status.ErrorCode
				if action == "config-sync-test" {
					a.configSync.message = "连接测试通过。"
				} else if action != "config-sync-status" {
					a.configSync.message = ""
				}
				if action == "config-sync-save" {
					current := a.syncConnectionRequest()
					unchanged := captured != nil && current.Provider == captured.Provider && current.Config == captured.Config && current.Secrets == captured.Secrets && current.AccountID == captured.AccountID && current.Jurisdiction == captured.Jurisdiction && current.Bucket == captured.Bucket
					a.configSync.editing = !unchanged
					if !unchanged {
						a.configSync.message = "连接已保存；当前修改尚未保存。"
					}
					// A field edited during Save belongs to the next draft.
					if unchanged && captured.Provider == "s3" {
						a.configSync.manual.secrets = cloudsync.Secrets{}
					}
					if unchanged && captured.Provider == "r2" {
						a.configSync.r2.secrets = cloudsync.Secrets{}
					}
				}
				if action == "config-sync-disconnect" {
					a.configSync.editing = true
					a.configSync.disconnectConfirm = false
				}
			}
			pending := a.configSync.refreshPending
			a.configSync.refreshPending = false
			a.invalidate()
			a.continueQuit()
			if pending {
				a.refreshConfigSync()
			}
		})
	}()
	a.invalidate()
}

func (a *application) refreshConfigSync() { a.manageConfigSync("config-sync-status", nil) }

func (a *application) settingsAvailableWithoutVault() bool {
	return !a.setup && a.page == "settings" && (a.settingsState.Category == "about" || a.settingsState.Category == "config-sync")
}

// Apply only the fields owned by synchronization, and only when this UI has
// not changed that field since its saved baseline. Detail/editor state remains
// untouched; a late network result never replaces the full app snapshot.
func (a *application) acceptConfigSyncState(data app.Data) {
	if data.ConfigSync != nil {
		status := *data.ConfigSync
		defer func() { a.acceptConfigSyncNotification(status) }()
		if status.Options.IntervalMinutes > 0 {
			if !a.configSync.optionsLoaded || a.configSync.options == a.configSync.optionsSaved {
				a.configSync.options = status.Options
			}
			a.configSync.optionsSaved = status.Options
			a.configSync.optionsLoaded = true
		}
		first := !a.configSync.loaded
		a.configSync.status = status
		a.configSync.loaded = true
		if first {
			if status.ManualConfig != nil {
				a.configSync.manual.config = *status.ManualConfig
			}
			if status.R2Config != nil {
				a.configSync.r2.config = *status.R2Config
			}
			a.configSync.r2.account = status.AccountID
			if status.Jurisdiction != "" {
				a.configSync.r2.jurisdiction = status.Jurisdiction
			}
			a.configSync.r2.bucket = a.configSync.r2.config.Bucket
			a.configSync.editing = status.Provider == ""
			if status.Provider == "s3" || status.Provider == "" && !status.OAuthAvailable {
				a.configSync.mode = 1
			}
		}
		if status.OAuthStage == "bucket" && a.configSync.r2.account == "" && len(status.Accounts) > 0 {
			a.configSync.r2.account = status.Accounts[0].ID
		}
		jurisdiction, draftJurisdiction := status.Jurisdiction, a.configSync.r2.jurisdiction
		if jurisdiction == "" {
			jurisdiction = "default"
		}
		if draftJurisdiction == "" {
			draftJurisdiction = "default"
		}
		if status.AccountID != a.configSync.r2.account || jurisdiction != draftJurisdiction {
			// Status refreshes carry the last server-side bucket query too. Its
			// options belong to that selection, including after a late callback.
			a.configSync.status.Buckets = nil
		}
	}
	if data.Settings.Theme == "" || !a.settingsLoaded {
		return
	}
	before := a.settings
	if a.settings.Theme == a.settingsSaved.Theme {
		a.settings.Theme = data.Settings.Theme
		a.settingsObserved.Theme = a.settings.Theme
	}
	if a.settings.ThemeStyle == a.settingsSaved.ThemeStyle {
		a.settings.ThemeStyle = data.Settings.ThemeStyle
		a.settingsObserved.ThemeStyle = a.settings.ThemeStyle
	}
	if a.settings.FontScale == a.settingsSaved.FontScale {
		a.settings.FontScale = data.Settings.FontScale
		a.settingsObserved.FontScale = a.settings.FontScale
	}
	if a.settings.Language == a.settingsSaved.Language {
		a.settings.Language = data.Settings.Language
		a.settingsObserved.Language = a.settings.Language
	}
	a.settingsSaved.Theme, a.settingsSaved.ThemeStyle = data.Settings.Theme, data.Settings.ThemeStyle
	a.settingsSaved.FontScale, a.settingsSaved.Language = data.Settings.FontScale, data.Settings.Language
	a.state.Settings.Theme, a.state.Settings.ThemeStyle = data.Settings.Theme, data.Settings.ThemeStyle
	a.state.Settings.FontScale, a.state.Settings.Language = data.Settings.FontScale, data.Settings.Language
	if before.Theme != a.settings.Theme || before.ThemeStyle != a.settings.ThemeStyle || before.FontScale != a.settings.FontScale {
		a.applyTheme()
	}
	if before.Language != a.settings.Language {
		a.applyLanguage()
	}
}

func (a *application) syncConnectionRequest() *app.SyncConnectionRequest {
	d := a.configSync.manual
	provider := "s3"
	if a.configSync.mode == 0 {
		d = a.configSync.r2
		provider = "r2"
	}
	return &app.SyncConnectionRequest{Provider: provider, Config: d.config, Secrets: d.secrets, AccountID: d.account, Jurisdiction: d.jurisdiction, Bucket: d.bucket}
}

func (a *application) beginConfigSyncOAuth(services ui.Services) {
	if a.configSync.status.OAuthLoadErrorCode != "" {
		return
	}
	request := a.syncConnectionRequest()
	request.OpenURL = func(url string) error {
		done := make(chan error, 1)
		a.runOnMain(func() {
			if a.closing || a.quit.requested {
				done <- errors.New("cancelled")
				return
			}
			services.OpenURL(url)
			done <- nil
		})
		select {
		case err := <-done:
			return err
		case <-time.After(10 * time.Second):
			return errors.New("browser_unavailable")
		}
	}
	a.manageConfigSync("config-sync-oauth-start", request)
}

func (a *application) configSyncError(code string) string {
	switch code {
	case "":
		return ""
	case "conflict":
		return a.t("此设备与云端修改了相同配置。请选择要保留的配置后重试。")
	case "invalid_config", "invalid_credentials":
		return a.t("请检查服务端点、存储桶、区域和访问密钥。")
	case "invalid_sync_options":
		return a.t("请选择有效的同步间隔后重试。")
	case "invalid_passphrase":
		return a.t("同步加密口令至少需要 12 个字符。")
	case "decrypt_failed", "invalid_ciphertext", "authentication_failed":
		return a.t("无法解密云端配置。请确认使用相同的加密口令。")
	case "credentials_denied":
		return a.t("系统拒绝访问同步凭据。请解除系统限制后主动重新连接。")
	case "credentials_unavailable":
		return a.t("系统凭据存储不可用。请启用系统凭据存储后重试。")
	case "credentials_invalid":
		return a.t("本机保存的同步凭据缺失或不完整，请重新连接。")
	case "oauth_unavailable", "oauth_invalid_config", "unavailable":
		return a.t("此版本尚未配置 Cloudflare 授权。请使用手动 S3 配置。")
	case "cancelled", "oauth_cancelled":
		return a.t("操作已取消，可以修改配置后重试。")
	case "invalid_document":
		return a.t("同步数据格式无效或来自更新版本，原有数据未被重置。")
	case "local_storage":
		return a.t("无法保存同步配置。请检查本机存储后重试。")
	case "access_denied", "missing_scope", "permission_denied", "oauth_permission_required":
		return a.t("Cloudflare 授权权限不足。请检查同意页权限后重新连接。")
	case "oauth_authorization_required":
		return a.t("Cloudflare 授权已失效。请重新连接以继续同步。")
	case "oauth_credentials_invalid":
		return a.t("已保存的 R2 桶凭据无效，请重新连接存储桶。")
	case "oauth_invalid_scope":
		return a.t("Cloudflare 拒绝了请求的权限。请联系发行者检查应用权限和 Refresh Token grant 配置。")
	case "oauth_denied":
		return a.t("Cloudflare 授权未获允许。请重试，并在 Edit Permissions 中开启 Account API Tokens Write。")
	case "oauth_callback_stopped":
		return a.t("Cloudflare 回调监听已停止，请重新连接。")
	case "oauth_missing_scopes":
		return a.t("Cloudflare 授权缺少必需权限，请重新连接并授予所需权限。")
	case "oauth_client_changed":
		return a.t("OAuth Client ID 已更改。请恢复原 Client ID 并断开旧连接，再使用新的应用身份。")
	case "oauth_accounts_missing":
		return a.t("重新授权时请包含所有已连接的 Cloudflare 账号，或先断开对应存储桶。")
	case "oauth_listen_failed":
		return a.t("无法启动本机授权回调。请检查回调端口是否被占用后重试。")
	case "oauth_invalid_selection":
		return a.t("所选账号、辖区或存储桶不可用。请重新获取存储桶列表。")
	case "oauth_unknown_connection":
		return a.t("请重新选择 Cloudflare 存储桶后连接。")
	case "oauth_too_many_connections":
		return a.t("待处理的 Cloudflare 连接过多，请先断开并完成清理后重试。")
	case "oauth_cleanup_pending":
		return a.t("仍有 R2 凭据待清理，请重新连接同一 Cloudflare 账号后重试断开。")
	case "oauth_invalid_state":
		return a.t("无法读取 Cloudflare 授权状态，原资料已保留。请检查配置目录并恢复有效状态后重新读取。")
	case "oauth_rate_limited":
		return a.t("Cloudflare 请求受限，请稍后重试。")
	default:
		return a.t("无法完成配置同步。请检查连接和访问权限后重试，输入会保留。")
	}
}

func (a *application) configSyncView(c *ui.Context) {
	ui.Column(c).Key("config-sync-settings").FillWidth().MaxWidth(720).MinWidth(0).Gap(16).Children(func() {
		ui.Column(c).Key("config-sync-connection").FillWidth().MinWidth(0).Padding(20).Gap(14).Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(10).Children(func() {
			a.configSyncSectionTitle(c, a.t("同步连接"), lucide.Cloud)
			if a.state.Preview {
				muted(c, a.t("预览模式不会连接对象存储或读取系统凭据。"))
				return
			}
			if a.configSync.status.Provider != "" {
				a.configSyncConnected(c)
			}
			if a.configSync.status.Provider == "" || a.configSync.editing {
				if a.configSync.status.Provider != "" {
					ui.Box(c).FillWidth().Height(1).Background(c.Theme().Border)
					components.Text(c, a.t("编辑同步连接")).Bold()
				}
				a.configSyncEditor(c)
			}
			a.configSyncOAuthDiagnostics(c)
			code := a.configSync.errorCode
			if code == "" {
				code = a.configSync.status.ErrorCode
			}
			if code != "" && code != a.configSync.status.OAuthLoadErrorCode {
				components.Text(c, a.configSyncError(code)).Key("sync-error").TextColor(c.Theme().Danger).Wrap()
				if code == "conflict" {
					a.configSyncConflict(c)
				}
			}
			if a.configSync.message != "" {
				components.Text(c, a.t(a.configSync.message)).TextColor(c.Theme().Success).Wrap()
			}
			if a.configSync.operation != "" {
				ui.Box(c).Key("sync-progress").FillWidth().Height(22).Children(func() {
					if remaining := 180*time.Millisecond - c.Now().Sub(a.configSync.started); remaining > 0 {
						c.After(remaining)
					} else {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							components.ProgressRing(c, -1).Size(15, 15).Label(a.t("正在处理配置同步…"))
							muted(c, a.t("正在处理配置同步…"))
						})
					}
				})
			}
		})
		a.configSyncTransferView(c)
		a.configSyncOptions(c)
	})
}

func (a *application) configSyncOptions(c *ui.Context) {
	ui.Column(c).Key("sync-options").FillWidth().MinWidth(0).Gap(16).Children(func() {
		ui.Column(c).Key("sync-content-options").FillWidth().MinWidth(0).Padding(20).Gap(16).Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(10).Children(func() {
			a.configSyncSectionTitle(c, a.t("同步内容"), lucide.ListChecks)
			muted(c, a.t("选择在设备间同步的偏好，两端可分别设置。"))
			a.configSyncFieldOptions(c, "sync-desktop", a.t("客户端偏好"), &a.configSync.options.Desktop, &a.configSync.options.DesktopFields)
			ui.Box(c).FillWidth().Height(1).Background(c.Theme().Border)
			a.configSyncFieldOptions(c, "sync-browser", a.t("扩展偏好"), &a.configSync.options.Browser, &a.configSync.options.BrowserFields)
			muted(c, a.t("扩展偏好开启时，也会同步扩展保存的网站关联与取消关联。"))
			muted(c, a.t("未选中的内容保留在本机，云端已有配置也会保留。密码库继续使用 Git 同步。"))
		})
		ui.Column(c).Key("sync-behavior-options").FillWidth().MinWidth(0).Padding(20).Gap(16).Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(10).Children(func() {
			a.configSyncSectionTitle(c, a.t("同步行为"), lucide.Clock)
			ui.Column(c).FillWidth().MinWidth(0).Gap(8).Children(func() {
				components.Text(c, a.t("同步模式")).Wrap()
				choiceSettingSegments(c, &a.configSync.options.Mode, a.t("同步模式"), []string{"automatic", "manual"}, []string{a.t("自动同步"), a.t("仅手动同步")}).FillWidth()
				if a.configSync.options.Mode == "manual" {
					muted(c, a.t("仅在点击“立即同步”时同步云端配置，自动设置会保留。"))
				}
			})
			ui.Column(c).Key("sync-automatic-options").FillWidth().MinWidth(0).Gap(16).Disabled(a.configSync.options.Mode == "manual").Children(func() {
				ui.Column(c).Key("sync-interval").FillWidth().MinWidth(0).Gap(6).Children(func() {
					components.Text(c, a.t("同步时间间隔")).Wrap()
					value := a.configSync.options.IntervalMinutes
					values := []int{1, 5, 15, 30, 60}
					choices := []string{"1", "5", "15", "30", "60"}
					labels := []string{a.t("1 分钟"), a.t("%d 分钟", 5), a.t("%d 分钟", 15), a.t("%d 分钟", 30), a.t("1 小时")}
					selected := "5"
					for i, minutes := range values {
						if minutes == value {
							selected = choices[i]
						}
					}
					if a.syncChoice(c, "sync-interval-choice", &selected, a.t("同步时间间隔"), choices, labels).Changed() {
						for i, choice := range choices {
							if selected == choice {
								a.configSync.options.IntervalMinutes = values[i]
							}
						}
					}
				})
				a.configSyncOption(c, "sync-on-start", a.t("启动时同步"), a.t("启动应用后自动检查一次云端配置。"), &a.configSync.options.SyncOnStart)
				a.configSyncOption(c, "sync-on-change", a.t("修改后自动同步"), a.t("本地偏好修改后约 2 秒安排同步。"), &a.configSync.options.SyncOnChange)
			})
			ui.Box(c).FillWidth().Height(1).Background(c.Theme().Border)
			a.configSyncOption(c, "sync-notify-errors", a.t("同步异常提醒"), a.t("同步失败或出现冲突时提醒；窗口隐藏时使用系统通知。"), &a.configSync.options.NotifyErrors)
		})
		ui.Column(c).Key("sync-options-footer").FillWidth().MinWidth(0).Gap(10).Children(func() {
			if a.page != "settings" || a.settingsState.Category != "config-sync" || strings.TrimSpace(a.settingsState.Query) != "" {
				a.configSyncSaveControls(c)
			} else {
				a.configSyncRevertButton(c)
			}
			if a.configSync.optionsErrorCode != "" {
				components.Text(c, a.configSyncError(a.configSync.optionsErrorCode)).Key("sync-options-error").TextColor(c.Theme().Danger).Wrap()
			}
			muted(c, a.t("这些选项仅在此设备生效，保存后用于下一次同步。"))
			muted(c, a.t("本机路径、私钥、系统解锁、开机自启和浏览器权限保留在本机。"))
		})
	})
}

func (a *application) configSyncSaveDisabled() bool {
	return a.state.Preview || !a.configSync.optionsLoaded || a.configSync.operation != "" || a.configSync.status.Busy || a.configSync.transfer != nil || a.quit.requested || a.closing || a.updates.preparing
}

func (a *application) configSyncSaveButton(c *ui.Context) {
	dirty := a.configSync.options != a.configSync.optionsSaved
	if components.Button(c, a.t("保存同步选项"), components.ButtonPrimary).Disabled(a.configSyncSaveDisabled() || !dirty).Clicked() {
		a.manageConfigSyncRequest(app.Request{Action: "config-sync-options", SyncOptions: &a.configSync.options})
	}
}

func (a *application) configSyncRevertButton(c *ui.Context) {
	dirty := a.configSync.options != a.configSync.optionsSaved
	if components.Button(c, a.t("还原更改"), components.ButtonGhost).Disabled(a.configSyncSaveDisabled() || !dirty).AlignSelf(ui.Start).Clicked() {
		a.configSync.options = a.configSync.optionsSaved
		a.configSync.optionsErrorCode = ""
	}
}

func (a *application) configSyncSaveControls(c *ui.Context) {
	ui.Row(c).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
		dirty := a.configSync.options != a.configSync.optionsSaved
		a.configSyncSaveButton(c)
		a.configSyncRevertButton(c)
		if dirty {
			muted(c, a.t("有未保存的同步选项。"))
		} else if a.configSync.optionsLoaded {
			muted(c, a.t("已保存"))
		}
	})
}

func (a *application) configSyncSectionTitle(c *ui.Context, title string, icon func() *ui.SVG) {
	ui.Row(c).FillWidth().MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
		components.Icon(c, icon).Size(17, 17).TextColor(c.Theme().TextMuted)
		components.Text(c, title).Bold().Wrap()
	})
}

func (a *application) configSyncFieldOptions(c *ui.Context, key, title string, enabled *bool, fields *app.SyncFieldSelection) {
	ui.Column(c).Key(key).FillWidth().MinWidth(0).Gap(10).Children(func() {
		a.configSyncOption(c, key+"-master", title, "", enabled)
		ui.Row(c).FillWidth().MinWidth(0).Wrap().Gap(16).Disabled(!*enabled).Children(func() {
			for _, field := range []struct {
				title string
				value *bool
			}{{a.t("主题与外观"), &fields.Appearance}, {a.t("字号"), &fields.FontScale}, {a.t("语言"), &fields.Language}} {
				components.Checkbox(c, field.value, field.title).Label(a.t("%s：%s", title, field.title))
			}
		})
	})
}

func (a *application) configSyncOption(c *ui.Context, key, title, description string, value *bool) {
	var row ui.Element
	if a.mainContentWidth(c) < 540 {
		row = ui.Column(c)
	} else {
		row = ui.Row(c)
	}
	row.Key(key).FillWidth().MinWidth(0).Gap(12).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			components.Text(c, title).Wrap()
			if description != "" {
				muted(c, description)
			}
		})
		a.booleanSettingSegments(c, value, a.t("%s：开启或关闭", title))
	})
}

func (a *application) configSyncEditor(c *ui.Context) {
	if ui.Segmented(c, &a.configSync.mode, a.t("Cloudflare 授权"), a.t("手动配置 S3")).Label(a.t("连接方式")).FillWidth().Changed() && a.configSync.operation == "config-sync-oauth-start" {
		a.manageConfigSync("config-sync-cancel", nil)
	}
	if a.configSync.mode == 1 {
		a.configSyncManual(c)
		return
	}
	if a.configSync.status.OAuthLoadErrorCode != "" {
		return
	}
	if a.configSync.operation == "config-sync-oauth-start" {
		components.Text(c, a.t("等待浏览器授权")).Bold()
		muted(c, a.t("请在 Cloudflare 同意页检查权限。若 Account API Tokens Write 未启用，请点击 Edit Permissions 开启。"))
		if components.Button(c, a.t("取消授权"), components.ButtonOutline).Clicked() {
			a.manageConfigSync("config-sync-cancel", nil)
		}
		return
	}
	if !a.configSync.status.OAuthAvailable {
		muted(c, a.t("此版本尚未配置 Cloudflare 授权。请使用手动 S3 配置。"))
		return
	}
	if a.configSync.status.OAuthStage != "bucket" {
		muted(c, a.t("在系统浏览器中授权，然后选择你自己的 R2 存储桶。"))
		muted(c, a.t("请在 Cloudflare 同意页检查权限。若 Account API Tokens Write 未启用，请点击 Edit Permissions 开启。"))
		if components.Button(c, a.t("连接 Cloudflare"), components.ButtonPrimary).Clicked() {
			a.beginConfigSyncOAuth(c.Services())
		}
		if a.configSync.status.R2Config != nil {
			a.configSyncR2Saved(c)
		}
		return
	}
	a.configSyncBucket(c)
}

func (a *application) syncTextField(c *ui.Context, key string, value *string, label, placeholder string, password bool) {
	ui.Column(c).Key(key).FillWidth().MinWidth(0).Gap(5).Children(func() {
		components.Text(c, label).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
		input := textBox(c, value, components.TextBoxOptions{}).Label(label).Placeholder(placeholder).FillWidth().MinWidth(0)
		if password {
			input.Password()
		}
	})
}

func (a *application) syncFields(c *ui.Context, left, right func(*ui.Context)) {
	var row ui.Element
	if a.mainContentWidth(c) < 540 {
		row = ui.Column(c)
	} else {
		row = ui.Row(c)
	}
	row.FillWidth().MinWidth(0).Gap(12).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() { left(c) })
		ui.Column(c).Grow(1).MinWidth(0).Children(func() { right(c) })
	})
}

func (a *application) configSyncManual(c *ui.Context) {
	d := &a.configSync.manual
	muted(c, a.t("支持 Cloudflare R2 和其他兼容 S3 的对象存储。"))
	a.syncTextField(c, "sync-endpoint", &d.config.Endpoint, a.t("服务端点"), "https://storage.example.com", false)
	a.syncFields(c, func(c *ui.Context) {
		a.syncTextField(c, "sync-bucket", &d.config.Bucket, a.t("存储桶"), "passpine-config", false)
	}, func(c *ui.Context) { a.syncTextField(c, "sync-region", &d.config.Region, a.t("区域"), "auto", false) })
	a.syncTextField(c, "sync-prefix", &d.config.Prefix, a.t("目录前缀"), "passpine", false)
	placeholder := ""
	if saved := a.configSync.status.ManualConfig; saved != nil {
		if normalized, err := cloudsync.NormalizeConfig(d.config); err == nil && normalized == *saved {
			placeholder = a.t("已保存；留空保持不变")
		}
	}
	a.syncFields(c, func(c *ui.Context) {
		a.syncTextField(c, "sync-key", &d.secrets.AccessKeyID, a.t("Access Key ID"), placeholder, true)
	}, func(c *ui.Context) {
		a.syncTextField(c, "sync-secret", &d.secrets.SecretAccessKey, a.t("Secret Access Key"), placeholder, true)
	})
	components.Checkbox(c, &d.config.PathStyle, a.t("使用路径寻址")).Label(a.t("使用路径寻址"))
	parts := components.Collapsible(c.Key("sync-advanced"), a.t("临时凭据选项"), &a.configSync.advanced, components.CollapsibleOptions{ArrowPosition: components.ArrowLeft}, func() {
		a.syncTextField(c, "sync-session", &d.secrets.SessionToken, a.t("Session Token（可选）"), placeholder, true)
	})
	parts.Root.FillWidth().MinWidth(0)
	a.configSyncPhrase(c, &d.secrets.Passphrase, placeholder)
	a.configSyncEditorActions(c, true)
}

func (a *application) configSyncPhrase(c *ui.Context, value *string, placeholder string) {
	a.syncTextField(c, "sync-phrase", value, a.t("同步加密口令"), placeholder, true)
	muted(c, a.t("至少 12 个字符。其他设备需使用相同的存储桶、前缀和口令；凭据与口令仅保存在本机系统凭据存储。"))
}

func (a *application) configSyncEditorActions(c *ui.Context, manual bool) {
	ui.Row(c).FillWidth().MinWidth(0).Wrap().Gap(8).Children(func() {
		if components.Button(c, a.t("保存并连接"), components.ButtonPrimary).Clicked() {
			a.manageConfigSync("config-sync-save", a.syncConnectionRequest())
		}
		if manual && components.Button(c, a.t("测试连接"), components.ButtonOutline).Clicked() {
			a.manageConfigSync("config-sync-test", a.syncConnectionRequest())
		}
		if components.Button(c, a.t("取消"), components.ButtonGhost).Clicked() {
			if a.configSync.mode == 0 {
				a.manageConfigSync("config-sync-cancel", nil)
			}
			if a.configSync.status.Provider != "" {
				a.configSync.editing = false
			}
			a.configSync.manual.secrets, a.configSync.r2.secrets = cloudsync.Secrets{}, cloudsync.Secrets{}
		}
	})
}

func (a *application) syncChoice(c *ui.Context, key string, value *string, label string, values, labels []string) ui.Element {
	box := components.Select(c.Key(key), value)
	display := a.t("请选择")
	for i, v := range values {
		if v == *value {
			display = labels[i]
		}
	}
	box.Trigger.Label(label).FillWidth().MinWidth(0).Children(func() { components.Text(c, display).SingleLine() })
	box.Popup(func(panel ui.Element) {
		panel.MinWidth(200).MaxWidth(400)
		for i, v := range values {
			v, label := v, labels[i]
			box.Item(v).Label(label).Children(func() { components.Text(c, label).SingleLine() })
		}
	})
	return box.Trigger
}

func (a *application) configSyncBucket(c *ui.Context) {
	d := &a.configSync.r2
	accounts, names := []string{}, []string{}
	for _, v := range a.configSync.status.Accounts {
		accounts = append(accounts, v.ID)
		names = append(names, v.Name)
	}
	components.Text(c, a.t("账号")).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
	account := a.syncChoice(c, "sync-account", &d.account, a.t("选择 Cloudflare 账号"), accounts, names)
	if account.Changed() {
		d.bucket = ""
		a.configSync.status.Buckets = nil
	}
	components.Text(c, a.t("辖区")).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
	jurisdictions := supportedSyncJurisdictions()
	jurisdictionNames := make([]string, len(jurisdictions))
	for i, value := range jurisdictions {
		switch value {
		case "default":
			jurisdictionNames[i] = a.t("默认")
		case "eu":
			jurisdictionNames[i] = a.t("欧盟")
		case "us":
			jurisdictionNames[i] = a.t("美国")
		case "fedramp":
			jurisdictionNames[i] = "FedRAMP"
		case "fedramp-high":
			jurisdictionNames[i] = "FedRAMP High"
		default:
			jurisdictionNames[i] = value
		}
	}
	jurisdiction := a.syncChoice(c, "sync-jurisdiction", &d.jurisdiction, a.t("选择 R2 辖区"), jurisdictions, jurisdictionNames)
	if jurisdiction.Changed() {
		d.bucket = ""
		a.configSync.status.Buckets = nil
	}
	ui.Row(c).Wrap().Gap(8).Children(func() {
		if components.Button(c, a.t("获取存储桶"), components.ButtonOutline).Clicked() {
			a.manageConfigSync("config-sync-oauth-buckets", a.syncConnectionRequest())
		}
		if len(accounts) == 0 && components.Button(c, a.t("刷新账号"), components.ButtonGhost).Clicked() {
			a.manageConfigSync("config-sync-oauth-accounts", a.syncConnectionRequest())
		}
	})
	buckets := []string{}
	for _, v := range a.configSync.status.Buckets {
		buckets = append(buckets, v.Name)
	}
	components.Text(c, a.t("存储桶")).FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted)
	a.syncChoice(c, "sync-r2-bucket", &d.bucket, a.t("选择 R2 存储桶"), buckets, buckets)
	a.syncTextField(c, "sync-r2-prefix", &d.config.Prefix, a.t("目录前缀"), "passpine", false)
	a.configSyncPhrase(c, &d.secrets.Passphrase, "")
	a.configSyncEditorActions(c, false)
}

func (a *application) configSyncR2Saved(c *ui.Context) {
	d := &a.configSync.r2
	muted(c, a.t("已有已保存的 R2 连接。可保持原存储桶并更新前缀或加密口令，也可重新授权选择其他存储桶。"))
	a.syncTextField(c, "sync-r2-saved-prefix", &d.config.Prefix, a.t("目录前缀"), "passpine", false)
	a.configSyncPhrase(c, &d.secrets.Passphrase, a.t("已保存；留空保持不变"))
	a.configSyncEditorActions(c, false)
}

func (a *application) configSyncConnected(c *ui.Context) {
	s := a.configSync.status
	label := a.t("已连接")
	if !s.Enabled {
		label = a.t("已暂停")
	}
	blocked := s.Provider == "r2" && s.OAuthLoadErrorCode != ""
	if blocked {
		label = a.t("连接状态读取失败")
	}
	ui.Row(c).FillWidth().MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
		components.Text(c, label).Bold().Grow(1)
		components.Text(c, map[string]string{"r2": "Cloudflare R2", "s3": "S3"}[s.Provider]).TextColor(c.Theme().TextMuted)
	})
	components.Text(c, s.Config.Bucket).Wrap()
	muted(c, s.Config.Endpoint)
	muted(c, a.t("目录前缀：%s", s.Config.Prefix))
	if stamp := gitTime(s.LastSyncedAt); stamp != "" {
		muted(c, a.t("上次同步：%s", stamp))
	} else {
		muted(c, a.t("尚未完成首次同步"))
	}
	ui.Row(c).FillWidth().MinWidth(0).Wrap().Gap(8).Children(func() {
		if components.Button(c, a.t("立即同步"), components.ButtonPrimary).Disabled(!s.Enabled || blocked).Clicked() {
			a.manageConfigSync("config-sync-now", nil)
		}
		pauseLabel, pauseAction := a.t("暂停同步"), "config-sync-pause"
		if !s.Enabled {
			pauseLabel, pauseAction = a.t("恢复同步"), "config-sync-resume"
		}
		if components.Button(c, pauseLabel, components.ButtonOutline).Disabled(blocked && !s.Enabled).Clicked() {
			a.manageConfigSync(pauseAction, nil)
		}
		if components.Button(c, a.t("编辑连接"), components.ButtonGhost).Disabled(a.configSync.editing).Clicked() {
			a.configSync.editing = true
			a.configSync.disconnectConfirm = false
		}
		if components.Button(c, a.t("断开连接"), components.ButtonGhost).Disabled(blocked).Clicked() {
			a.configSync.disconnectConfirm = true
		}
	})
	if a.configSync.disconnectConfirm {
		muted(c, a.t("断开会撤销应用创建的 R2 凭据，并保留存储桶中的数据与手动 S3 配置。"))
		ui.Row(c).Gap(8).Children(func() {
			if components.Button(c, a.t("确认断开"), components.ButtonOutline).Clicked() {
				a.manageConfigSync("config-sync-disconnect", nil)
			}
			if components.Button(c, a.t("取消"), components.ButtonGhost).Clicked() {
				a.configSync.disconnectConfirm = false
			}
		})
	}
}

func (a *application) configSyncOAuthDiagnostics(c *ui.Context) {
	s := a.configSync.status
	if s.OAuthLoadErrorCode != "" {
		components.Text(c, a.configSyncError(s.OAuthLoadErrorCode)).Key("oauth-metadata-error").TextColor(c.Theme().Danger).Wrap()
		if components.Button(c, a.t("重新读取配置"), components.ButtonOutline).AlignSelf(ui.Start).Clicked() {
			a.manageConfigSync("config-sync-oauth-reload", nil)
		}
	}
	if s.WarningCode != "" {
		components.Text(c, a.t("部分旧凭据或授权尚未清理。请保留本机资料，断开连接时重试清理。")).Key("oauth-cleanup-warning").TextColor(c.Theme().Warning).Wrap()
	}
	if s.OAuthAuthorized && !s.OAuthRenewable && (s.Provider == "r2" || a.configSync.mode == 0) {
		components.Text(c, a.t("此授权不支持静默续期，请在桶凭据到期前重新连接。")).Key("oauth-renewal-hint").TextColor(c.Theme().Warning).Wrap()
	}
}

func (a *application) configSyncConflict(c *ui.Context) {
	labels := map[string]string{"desktop.theme": "客户端主题", "desktop.themeStyle": "客户端主题风格", "desktop.fontScale": "客户端字号", "desktop.language": "客户端语言", "browser.theme": "扩展主题", "browser.themeStyle": "扩展主题风格", "browser.fontScale": "扩展字号", "browser.language": "扩展语言"}
	shown := []string{}
	for _, key := range a.configSync.status.ConflictKeys {
		if strings.HasPrefix(key, "browser.associations.") || strings.HasPrefix(key, "browser.associationAliases.") || strings.HasPrefix(key, "browser.associationLegacy.") {
			label := a.t("扩展网站关联")
			if !slices.Contains(shown, label) {
				shown = append(shown, label)
			}
			continue
		}
		if label, ok := labels[key]; ok {
			shown = append(shown, a.t(label))
		}
	}
	if len(shown) > 0 {
		components.Text(c, strings.Join(shown, " · ")).Wrap()
	}
	ui.Row(c).Wrap().Gap(8).Children(func() {
		if components.Button(c, a.t("保留此设备配置"), components.ButtonOutline).Clicked() {
			a.manageConfigSync("config-sync-resolve", &app.SyncConnectionRequest{Resolution: "local"})
		}
		if components.Button(c, a.t("使用云端配置"), components.ButtonOutline).Clicked() {
			a.manageConfigSync("config-sync-resolve", &app.SyncConnectionRequest{Resolution: "remote"})
		}
	})
}
