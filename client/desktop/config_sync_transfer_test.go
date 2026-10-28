package desktop

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/cloudsync"
)

func configurationSyncTransferFixture(locale string) (*application, chan func(), *atomic.Int32) {
	a, callbacks := configurationSyncOptionsFixture(locale)
	a.page, a.settingsState.Category = "settings", "config-sync"
	var requests atomic.Int32
	a.service = backendFunc(func(app.Request) app.Response {
		requests.Add(1)
		return app.Response{Status: "ok"}
	})
	return a, callbacks, &requests
}

func configurationSyncTransferDocument(t *testing.T) (app.ConfigSyncOptions, cloudsync.Config, []byte) {
	t.Helper()
	options := app.DefaultConfigSyncOptions()
	options.Mode, options.IntervalMinutes, options.SyncOnChange = "manual", 30, false
	options.NotifyErrors, options.DesktopFields.Language, options.BrowserFields.Appearance = false, false, false
	config := cloudsync.Config{Endpoint: "https://storage.example.test", Region: "auto", Bucket: "portable-bucket", Prefix: "portable/偏好", PathStyle: true}
	data, err := app.EncodeConfigSyncTransfer(options, &config)
	if err != nil {
		t.Fatal(err)
	}
	return options, config, data
}

func configurationSyncTransferFile(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "同步 config with spaces.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func finishConfigurationSyncTransfer(t *testing.T, a *application, callbacks <-chan func()) {
	t.Helper()
	deadline := time.NewTimer(4 * time.Second)
	defer deadline.Stop()
	for a.configSync.transfer != nil {
		select {
		case callback := <-callbacks:
			callback()
		case <-deadline.C:
			t.Fatal("configuration transfer did not complete its main-thread callbacks")
		}
	}
}

func TestConfigurationSyncTransferImportStagesOptionsAndConnectionWithoutSaving(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a, callbacks, requests := configurationSyncTransferFixture(locale)
			options, config, data := configurationSyncTransferDocument(t)
			path := configurationSyncTransferFile(t, data)
			a.configSync.manual.secrets = cloudsync.Secrets{AccessKeyID: "old-key", SecretAccessKey: "old-secret", Passphrase: "old-sync-passphrase"}
			a.configSync.r2.secrets.Passphrase = "unchanged-r2-passphrase"
			a.configSync.mode, a.configSync.editing = 0, false
			saved, status, r2 := a.configSync.optionsSaved, a.configSync.status, a.configSync.r2
			a.preferences.openDialog = func(dialog mygo.OpenDialogOptions) ([]string, error) {
				if dialog.Title != a.t("导入同步配置") || len(dialog.Filters) == 0 {
					t.Error("import dialog has no localized title or JSON filter")
				}
				return []string{path}, nil
			}
			a.importConfigSync()
			finishConfigurationSyncTransfer(t, a, callbacks)
			if a.configSync.options != options || a.configSync.optionsSaved != saved || a.configSync.manual.config != config || a.configSync.manual.secrets != (cloudsync.Secrets{}) || a.configSync.mode != 1 || !a.configSync.editing {
				t.Fatal("import did not stage portable options and a credential-free manual connection")
			}
			if a.configSync.r2 != r2 || !reflect.DeepEqual(a.configSync.status, status) || requests.Load() != 0 {
				t.Fatal("import changed saved connection state, R2 authorization draft or contacted the backend")
			}
		})
	}
}

func TestConfigurationSyncTransferOptionsOnlyImportPreservesConnectionDrafts(t *testing.T) {
	a, callbacks, requests := configurationSyncTransferFixture("en")
	options := app.DefaultConfigSyncOptions()
	options.IntervalMinutes = 60
	data, err := app.EncodeConfigSyncTransfer(options, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := configurationSyncTransferFile(t, data)
	a.configSync.manual.secrets.Passphrase = "existing-manual-passphrase"
	a.configSync.r2.account, a.configSync.r2.secrets.Passphrase = "draft-account", "existing-r2-passphrase"
	a.configSync.mode, a.configSync.editing = 0, false
	manual, r2, saved := a.configSync.manual, a.configSync.r2, a.configSync.optionsSaved
	a.preferences.openDialog = func(mygo.OpenDialogOptions) ([]string, error) { return []string{path}, nil }
	a.importConfigSync()
	finishConfigurationSyncTransfer(t, a, callbacks)
	if a.configSync.options != options || a.configSync.optionsSaved != saved || a.configSync.manual != manual || a.configSync.r2 != r2 || a.configSync.mode != 0 || a.configSync.editing || requests.Load() != 0 {
		t.Fatal("options-only import changed a connection draft or saved state")
	}
}

func TestConfigurationSyncTransferExportCapturesDraftAndExcludesSecrets(t *testing.T) {
	a, callbacks, requests := configurationSyncTransferFixture("en")
	options, config, _ := configurationSyncTransferDocument(t)
	a.configSync.options, a.configSync.manual.config = options, config
	a.configSync.manual.secrets = cloudsync.Secrets{AccessKeyID: "never-export-access-key", SecretAccessKey: "never-export-secret-key", SessionToken: "never-export-token", Passphrase: "never-export-passphrase"}
	a.configSync.r2.account, a.configSync.r2.bucket = "never-export-account", "never-export-sdk-bucket"
	saved := a.configSync.optionsSaved
	path := filepath.Join(t.TempDir(), "portable config.json")
	started, release := make(chan mygo.SaveDialogOptions, 1), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	a.preferences.saveDialog = func(dialog mygo.SaveDialogOptions) (string, error) {
		started <- dialog
		<-release
		return path, nil
	}
	a.exportConfigSync()
	select {
	case dialog := <-started:
		if dialog.Title != a.t("导出同步配置") || dialog.DefaultPath != "passpine-sync-config.json" || len(dialog.Filters) == 0 {
			t.Fatal("export dialog has no portable filename, localized title or JSON filter")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("export did not open its native save dialog")
	}
	a.configSync.options.IntervalMinutes = 15
	a.configSync.manual.config.Prefix = "later-edit"
	once.Do(func() { close(release) })
	finishConfigurationSyncTransfer(t, a, callbacks)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, target, err := app.DecodeConfigSyncTransfer(data)
	if err != nil || decoded != options || target == nil || *target != config {
		t.Fatal("export did not retain the options and connection snapshot captured before the dialog", err)
	}
	if a.configSync.options.IntervalMinutes != 15 || a.configSync.manual.config.Prefix != "later-edit" || a.configSync.optionsSaved != saved || requests.Load() != 0 {
		t.Fatal("export replaced later edits, saved options or contacted the backend")
	}
	for _, private := range []string{"never-export-", "secretRef", "baseline", "connectionId", "accountId", "lastSyncedAt"} {
		if bytes.Contains(data, []byte(private)) {
			t.Fatalf("portable export includes private state: %s", private)
		}
	}
}

func TestConfigurationSyncTransferExportUsesSavedTargetOutsideConnectionEditing(t *testing.T) {
	a, callbacks, requests := configurationSyncTransferFixture("zh-CN")
	_, config, _ := configurationSyncTransferDocument(t)
	a.configSync.editing, a.configSync.mode = false, 0
	a.configSync.status.Provider, a.configSync.status.Config = "r2", config
	a.configSync.r2.config.Bucket, a.configSync.manual.config.Bucket = "unsaved-r2", "unsaved-manual"
	path := filepath.Join(t.TempDir(), "saved-r2.json")
	a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) { return path, nil }
	a.exportConfigSync()
	finishConfigurationSyncTransfer(t, a, callbacks)
	_, target, err := app.ReadConfigSyncTransferFile(path)
	if err != nil || target == nil || *target != config || requests.Load() != 0 {
		t.Fatal("export outside connection editing did not use the saved target without SDK access", err)
	}
}

func TestConfigurationSyncTransferDialogCancellationAndFailureCanRetry(t *testing.T) {
	for _, action := range []string{"import", "export"} {
		for _, outcome := range []string{"cancel", "error"} {
			t.Run(action+"/"+outcome, func(t *testing.T) {
				a, callbacks, requests := configurationSyncTransferFixture("en")
				_, _, data := configurationSyncTransferDocument(t)
				path := configurationSyncTransferFile(t, data)
				var dialogs atomic.Int32
				failure := error(nil)
				if outcome == "error" {
					failure = errors.New("private provider path must not be exposed")
				}
				a.preferences.openDialog = func(mygo.OpenDialogOptions) ([]string, error) { dialogs.Add(1); return nil, failure }
				a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) { dialogs.Add(1); return "", failure }
				start := a.importConfigSync
				if action == "export" {
					start = a.exportConfigSync
				}
				start()
				start()
				finishConfigurationSyncTransfer(t, a, callbacks)
				if dialogs.Load() != 1 || requests.Load() != 0 {
					t.Fatal("duplicate clicks opened multiple dialogs or contacted the backend")
				}
				if strings.Contains(a.errorText, "private provider") {
					t.Fatal("dialog error exposed uncontrolled provider diagnostics")
				}
				a.preferences.openDialog = func(mygo.OpenDialogOptions) ([]string, error) { dialogs.Add(1); return []string{path}, nil }
				a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) {
					dialogs.Add(1)
					return filepath.Join(t.TempDir(), "retry.json"), nil
				}
				start()
				finishConfigurationSyncTransfer(t, a, callbacks)
				if dialogs.Load() != 2 || requests.Load() != 0 || a.errorText != "" || a.notice == "" {
					t.Fatal("cancelled or failed native dialog prevented retry or left stale failure feedback")
				}
			})
		}
	}
}

func TestConfigurationSyncTransferExportRejectsUnpreparedR2Selection(t *testing.T) {
	for _, bucket := range []string{"", "portable-bucket"} {
		t.Run("bucket-"+bucket, func(t *testing.T) {
			a, _, requests := configurationSyncTransferFixture("en")
			_, config, _ := configurationSyncTransferDocument(t)
			a.configSync.mode, a.configSync.editing = 0, true
			a.configSync.status.Provider, a.configSync.status.Config = "r2", config
			a.configSync.status.OAuthStage, a.configSync.status.AccountID = "bucket", "saved-account"
			a.configSync.r2.config, a.configSync.r2.account, a.configSync.r2.bucket = config, "different-account", bucket
			before := a.configSync.r2
			var dialogs atomic.Int32
			a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) { dialogs.Add(1); return "unreachable.json", nil }
			a.exportConfigSync()
			if dialogs.Load() != 0 || a.configSync.transfer != nil || a.errorText == "" || a.configSync.r2 != before || requests.Load() != 0 {
				t.Fatal("export prepared an endpoint or reused a saved same-named bucket from another OAuth account")
			}
		})
	}
}

func TestConfigurationSyncTransferBlankConnectionExportsOnlyOptionsAndHalfTargetIsRetained(t *testing.T) {
	for _, target := range []string{"blank", "half"} {
		t.Run(target, func(t *testing.T) {
			a, callbacks, requests := configurationSyncTransferFixture("en")
			if target == "half" {
				a.configSync.manual.config.Endpoint = "https://unfinished.example.test"
			}
			before := a.configSync.manual
			path := filepath.Join(t.TempDir(), "options-only.json")
			var dialogs atomic.Int32
			a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) { dialogs.Add(1); return path, nil }
			a.exportConfigSync()
			finishConfigurationSyncTransfer(t, a, callbacks)
			if target == "half" {
				if dialogs.Load() != 0 || a.errorText == "" {
					t.Fatal("a partially entered connection silently disappeared from its export")
				}
			} else {
				options, config, err := app.ReadConfigSyncTransferFile(path)
				if err != nil || config != nil || options != a.configSync.options || dialogs.Load() != 1 {
					t.Fatal("an unconnected device could not export its sync options", err)
				}
			}
			if a.configSync.manual != before || requests.Load() != 0 {
				t.Fatal("export modified an incomplete target or contacted the backend")
			}
		})
	}
}

func TestConfigurationSyncTransferWriteFailureCancelsQuitAndPreservesExistingFile(t *testing.T) {
	a, callbacks, requests := configurationSyncTransferFixture("en")
	path := filepath.Join(t.TempDir(), "existing 同步 config.json")
	const original = "preserved existing file"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) { return path, nil }
	started, release := make(chan struct{}, 1), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	a.configSync.writeTransfer = func(path string, data []byte) error {
		started <- struct{}{}
		<-release
		return app.WriteConfigSyncTransferFile(path, data)
	}
	a.exportConfigSync()
	awaitMain(t, callbacks)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("export did not begin its write")
	}
	a.requestQuit()
	if a.closing || a.quit.draining || !a.quit.requested {
		t.Fatal("quit failed to wait for the owned write")
	}
	once.Do(func() { close(release) })
	finishConfigurationSyncTransfer(t, a, callbacks)
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != original || a.quit.requested || a.quit.draining || a.quit.ready || a.closing || a.errorText != a.t("目标文件已存在，请更换文件名后重试。") || requests.Load() != 0 {
		t.Fatal("failed export replaced user data, exited the application, or hid its recoverable failure", err)
	}
}

func TestConfigurationSyncTransferQuitCancelsNativeDialogWithoutWriting(t *testing.T) {
	a, callbacks, requests := configurationSyncTransferFixture("en")
	var writes atomic.Int32
	a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) { return "unused.json", nil }
	a.configSync.writeTransfer = func(string, []byte) error { writes.Add(1); return nil }
	a.exportConfigSync()
	callback := takeExportCallback(t, callbacks)
	a.requestQuit()
	callback()
	for !a.quit.ready {
		awaitMain(t, callbacks)
	}
	if a.configSync.transfer != nil || writes.Load() != 0 || requests.Load() != 0 {
		t.Fatal("closing an application continued a pending native file dialog")
	}
}

func TestConfigurationSyncTransferFailedImportRetainsEveryDraft(t *testing.T) {
	for _, outcome := range []string{"invalid", "oversized", "read-error"} {
		t.Run(outcome, func(t *testing.T) {
			a, callbacks, requests := configurationSyncTransferFixture("en")
			data := []byte(`{"not":"a sync transfer"}`)
			if outcome == "oversized" {
				data = bytes.Repeat([]byte(" "), app.MaxConfigSyncTransferBytes+1)
			}
			path := configurationSyncTransferFile(t, data)
			a.configSync.manual.secrets.Passphrase = "retained-manual-passphrase"
			a.configSync.r2.secrets.Passphrase = "retained-r2-passphrase"
			a.configSync.options.IntervalMinutes = 15
			options, manual, r2, mode, editing := a.configSync.options, a.configSync.manual, a.configSync.r2, a.configSync.mode, a.configSync.editing
			a.preferences.openDialog = func(mygo.OpenDialogOptions) ([]string, error) { return []string{path}, nil }
			if outcome == "read-error" {
				a.configSync.readTransfer = func(string) (app.ConfigSyncOptions, *cloudsync.Config, error) {
					return app.ConfigSyncOptions{}, nil, errors.New("private provider path must not be exposed")
				}
			}
			a.importConfigSync()
			finishConfigurationSyncTransfer(t, a, callbacks)
			if a.configSync.options != options || a.configSync.manual != manual || a.configSync.r2 != r2 || a.configSync.mode != mode || a.configSync.editing != editing || requests.Load() != 0 {
				t.Fatal("a failed import changed existing drafts or contacted the backend")
			}
			if strings.Contains(a.errorText, "private provider") {
				t.Fatal("read failure exposed uncontrolled provider diagnostics")
			}
		})
	}
}

func TestConfigurationSyncTransferImportKeepsEditsMadeDuringDialogOrRead(t *testing.T) {
	for _, stage := range []string{"dialog", "read"} {
		for _, field := range []string{"options", "manual", "r2", "mode", "editing"} {
			t.Run(stage+"/"+field, func(t *testing.T) {
				a, callbacks, requests := configurationSyncTransferFixture("en")
				options, config, _ := configurationSyncTransferDocument(t)
				readStarted, release := make(chan struct{}, 1), make(chan struct{})
				var once sync.Once
				t.Cleanup(func() { once.Do(func() { close(release) }) })
				a.preferences.openDialog = func(mygo.OpenDialogOptions) ([]string, error) { return []string{"fake-selected.json"}, nil }
				a.configSync.readTransfer = func(string) (app.ConfigSyncOptions, *cloudsync.Config, error) {
					readStarted <- struct{}{}
					<-release
					return options, &config, nil
				}
				a.importConfigSync()
				callback := takeExportCallback(t, callbacks)
				if stage == "read" {
					callback()
					select {
					case <-readStarted:
					case <-time.After(3 * time.Second):
						t.Fatal("import did not begin reading the chosen file")
					}
				}
				switch field {
				case "options":
					a.configSync.options.IntervalMinutes = 60
				case "manual":
					a.configSync.manual.secrets.Passphrase = "edited-during-import"
				case "r2":
					a.configSync.r2.bucket = "edited-during-import"
				case "mode":
					a.configSync.mode = 0
				case "editing":
					a.configSync.editing = false
				}
				beforeOptions, beforeManual, beforeR2, beforeMode, beforeEditing := a.configSync.options, a.configSync.manual, a.configSync.r2, a.configSync.mode, a.configSync.editing
				once.Do(func() { close(release) })
				if stage == "dialog" {
					callback()
				}
				finishConfigurationSyncTransfer(t, a, callbacks)
				if a.configSync.options != beforeOptions || a.configSync.manual != beforeManual || a.configSync.r2 != beforeR2 || a.configSync.mode != beforeMode || a.configSync.editing != beforeEditing || requests.Load() != 0 {
					t.Fatal("a late imported document replaced newer editor input")
				}
			})
		}
	}
}

func TestConfigurationSyncTransferLanguageChangeKeepsPendingImport(t *testing.T) {
	a, callbacks, requests := configurationSyncTransferFixture("zh-CN")
	options, _, data := configurationSyncTransferDocument(t)
	path := configurationSyncTransferFile(t, data)
	a.preferences.openDialog = func(mygo.OpenDialogOptions) ([]string, error) { return []string{path}, nil }
	a.importConfigSync()
	callback := takeExportCallback(t, callbacks)
	a.settings.Language = "en"
	callback()
	finishConfigurationSyncTransfer(t, a, callbacks)
	if a.configSync.options != options || a.settings.Language != "en" || requests.Load() != 0 {
		t.Fatal("switching interface language cancelled the import or imported the interface language")
	}
}

func TestConfigurationSyncTransferNavigationDiscardsPendingDialog(t *testing.T) {
	for _, action := range []string{"import", "export"} {
		t.Run(action, func(t *testing.T) {
			a, callbacks, requests := configurationSyncTransferFixture("en")
			var reads, writes atomic.Int32
			a.preferences.openDialog = func(mygo.OpenDialogOptions) ([]string, error) { return []string{"unused.json"}, nil }
			a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) { return "unused.json", nil }
			a.configSync.readTransfer = func(string) (app.ConfigSyncOptions, *cloudsync.Config, error) {
				reads.Add(1)
				return app.DefaultConfigSyncOptions(), nil, nil
			}
			a.configSync.writeTransfer = func(string, []byte) error { writes.Add(1); return nil }
			if action == "import" {
				a.importConfigSync()
			} else {
				a.exportConfigSync()
			}
			callback := takeExportCallback(t, callbacks)
			a.hidePreferences()
			callback()
			if a.configSync.transfer != nil || reads.Load() != 0 || writes.Load() != 0 || requests.Load() != 0 {
				t.Fatal("a stale native dialog continued reading or writing after navigation")
			}
		})
	}
}

func TestConfigurationSyncTransferWritingBlocksSuspensionUpdateAndQuitDrain(t *testing.T) {
	a, callbacks, requests := configurationSyncTransferFixture("en")
	a.state.Settings.LightweightMode = true
	a.lifecycle.hiddenAt = time.Now().Add(-lightweightIdleDelay)
	a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) { return "fake-selected.json", nil }
	started, release := make(chan struct{}, 1), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	a.configSync.writeTransfer = func(string, []byte) error { started <- struct{}{}; <-release; return nil }
	a.exportConfigSync()
	awaitMain(t, callbacks)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("export did not begin writing")
	}
	if a.configSync.transfer == nil || !a.configSync.transfer.writing || a.lightweightSuspensionReady(time.Now()) || !a.updateInstallBlocked() {
		t.Fatal("an in-flight configuration write did not block destructive lifecycle transitions")
	}
	a.requestQuit()
	if a.closing || a.quit.draining || a.configSync.transfer == nil || !a.configSync.transfer.writing || requests.Load() != 0 {
		t.Fatal("quit discarded or drained past an in-flight configuration write")
	}
	once.Do(func() { close(release) })
	finishConfigurationSyncTransfer(t, a, callbacks)
	for !a.quit.ready {
		awaitMain(t, callbacks)
	}
	if !a.closing || requests.Load() != 0 {
		t.Fatal("quit did not complete after the configuration write finished")
	}
}

func TestConfigurationSyncTransferControlsRespectAvailabilityAndFitBothLanguages(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, state := range []string{"ready", "preview", "unloaded", "cloud-operation", "transfer"} {
			t.Run(locale+"/"+state, func(t *testing.T) {
				a, callbacks, requests := configurationSyncTransferFixture(locale)
				a.settings.FontScale, a.state.Settings.FontScale = 120, 120
				var dialogs atomic.Int32
				a.preferences.openDialog = func(mygo.OpenDialogOptions) ([]string, error) { dialogs.Add(1); return nil, nil }
				switch state {
				case "preview":
					a.state.Preview = true
				case "unloaded":
					a.configSync.optionsLoaded = false
				case "cloud-operation":
					a.configSync.operation = "config-sync-now"
				case "transfer":
					a.configSync.transfer = &configurationSyncTransfer{}
				}
				view := ui.NewTester(func(c *ui.Context) { c.SetTheme(a.clientTheme(c)); a.configSyncTransferView(c) }, 360, 360)
				assertTypographyBounds(t, view, 360, 360, a.t("导入配置…"), a.t("导出配置…"))
				if err := view.Click(a.t("导入配置…")); err != nil {
					t.Fatal(err)
				}
				if state == "ready" {
					finishConfigurationSyncTransfer(t, a, callbacks)
					if dialogs.Load() != 1 {
						t.Fatal("an available import control did not open the native dialog")
					}
				} else if dialogs.Load() != 0 {
					t.Fatal("a disabled import control opened a native dialog")
				}
				if requests.Load() != 0 {
					t.Fatal("import controls contacted the backend")
				}
			})
		}
	}
}
