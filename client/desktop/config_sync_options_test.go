package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/cloudsync"
	"github.com/oneo-me/onevra/source/components"
)

func configurationSyncOptionsFixture(locale string) (*application, chan func()) {
	a, callbacks := configurationSyncFixture()
	a.settings.Language, a.state.Settings.Language = locale, locale
	a.settingsSaved.Language, a.settingsObserved.Language = locale, locale
	a.configSync.options = app.DefaultConfigSyncOptions()
	a.configSync.optionsSaved = a.configSync.options
	a.configSync.optionsLoaded = true
	a.configSync.status.Options = a.configSync.options
	return a, callbacks
}

func configurationSyncOptionsView(a *application) func(*ui.Context) {
	return func(c *ui.Context) {
		c.SetTheme(a.clientTheme(c))
		a.configSyncOptions(c)
	}
}

func syncIntervalLabel(a *application, minutes int) string {
	switch minutes {
	case 1:
		return a.t("1 分钟")
	case 60:
		return a.t("1 小时")
	default:
		return a.t("%d 分钟", minutes)
	}
}

func chooseConfigurationSyncBoolean(t *testing.T, view *ui.Tester, label string, enabled bool) {
	t.Helper()
	clickBooleanSettingSegment(t, view, label, enabled)
}

func TestConfigurationSyncOptionsChooseIntervalsAndNamespacesInBothLanguages(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a, _ := configurationSyncOptionsFixture(locale)
			view := ui.NewTester(configurationSyncOptionsView(a), 620, 1800)
			if a.configSync.options != a.configSync.optionsSaved || !view.HasText(a.t("已保存")) {
				t.Fatal("rendering changed saved synchronization options")
			}
			for _, minutes := range []int{1, 5, 15, 30, 60} {
				if err := view.Click(syncIntervalLabel(a, a.configSync.options.IntervalMinutes)); err != nil {
					t.Fatal(err)
				}
				if err := view.Click(syncIntervalLabel(a, minutes)); err != nil {
					t.Fatal(err)
				}
				if a.configSync.options.IntervalMinutes != minutes {
					t.Fatalf("interval choice did not update the draft: got %d, want %d", a.configSync.options.IntervalMinutes, minutes)
				}
			}
			for _, option := range []struct {
				label string
				value *bool
			}{{"修改后自动同步", &a.configSync.options.SyncOnChange}, {"客户端偏好", &a.configSync.options.Desktop}, {"扩展偏好", &a.configSync.options.Browser}} {
				label := a.t("%s：开启或关闭", a.t(option.label))
				chooseConfigurationSyncBoolean(t, view, label, false)
				if *option.value {
					t.Fatalf("could not disable %s", option.label)
				}
				chooseConfigurationSyncBoolean(t, view, label, true)
				if !*option.value {
					t.Fatalf("could not reenable %s", option.label)
				}
			}
			if !view.HasText(a.t("有未保存的同步选项。")) || a.configSync.optionsSaved.IntervalMinutes != 5 {
				t.Fatal("editing options did not retain their saved baseline")
			}
		})
	}
}

func TestConfigurationSyncManualModeRetainsAutomaticChoices(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a, _ := configurationSyncOptionsFixture(locale)
			view := ui.NewTester(configurationSyncOptionsView(a), 620, 1800)
			if err := view.Click(a.t("仅手动同步")); err != nil {
				t.Fatal(err)
			}
			if a.configSync.options.Mode != "manual" {
				t.Fatal("manual mode did not update the draft")
			}
			if err := view.Click(syncIntervalLabel(a, 5)); err != nil {
				t.Fatal(err)
			}
			if view.HasText(syncIntervalLabel(a, 15)) {
				t.Fatal("manual mode opened an automatic interval selector")
			}
			for _, label := range []string{"修改后自动同步", "启动时同步"} {
				chooseConfigurationSyncBoolean(t, view, a.t("%s：开启或关闭", a.t(label)), false)
			}
			if !a.configSync.options.SyncOnChange || !a.configSync.options.SyncOnStart || a.configSync.options.IntervalMinutes != 5 {
				t.Fatal("manual mode changed the retained automatic schedule")
			}
			chooseConfigurationSyncBoolean(t, view, a.t("%s：开启或关闭", a.t("同步异常提醒")), false)
			if a.configSync.options.NotifyErrors {
				t.Fatal("manual mode disabled the independent notification preference")
			}
			if err := view.Click(a.t("自动同步")); err != nil {
				t.Fatal(err)
			}
			for _, label := range []string{"修改后自动同步", "启动时同步"} {
				chooseConfigurationSyncBoolean(t, view, a.t("%s：开启或关闭", a.t(label)), false)
			}
			if a.configSync.options.Mode != "automatic" || a.configSync.options.SyncOnChange || a.configSync.options.SyncOnStart {
				t.Fatal("automatic mode did not restore its editable schedule controls")
			}
		})
	}
}

func TestConfigurationSyncFieldChoicesAreIndependentAndSurviveMasterToggle(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a, _ := configurationSyncOptionsFixture(locale)
			view := ui.NewTester(configurationSyncOptionsView(a), 620, 1800)
			for _, group := range []struct {
				label  string
				master *bool
				fields *app.SyncFieldSelection
			}{{"客户端偏好", &a.configSync.options.Desktop, &a.configSync.options.DesktopFields}, {"扩展偏好", &a.configSync.options.Browser, &a.configSync.options.BrowserFields}} {
				for _, field := range []struct {
					label string
					value *bool
				}{{"主题与外观", &group.fields.Appearance}, {"字号", &group.fields.FontScale}, {"语言", &group.fields.Language}} {
					label := a.t("%s：%s", a.t(group.label), a.t(field.label))
					if err := view.Click(label); err != nil {
						t.Fatal(err)
					}
					if *field.value {
						t.Fatalf("could not exclude %s/%s", group.label, field.label)
					}
					if err := view.Click(label); err != nil {
						t.Fatal(err)
					}
					if !*field.value {
						t.Fatalf("could not reenable %s/%s", group.label, field.label)
					}
				}
				fontLabel := a.t("%s：%s", a.t(group.label), a.t("字号"))
				if err := view.Click(fontLabel); err != nil {
					t.Fatal(err)
				}
				chooseConfigurationSyncBoolean(t, view, a.t("%s：开启或关闭", a.t(group.label)), false)
				if err := view.Click(fontLabel); err != nil {
					t.Fatal(err)
				}
				if *group.master || group.fields.FontScale || !group.fields.Appearance || !group.fields.Language {
					t.Fatal("disabling a namespace changed its saved field choices or left disabled choices editable")
				}
				chooseConfigurationSyncBoolean(t, view, a.t("%s：开启或关闭", a.t(group.label)), true)
				if !*group.master || group.fields.FontScale || !group.fields.Appearance || !group.fields.Language {
					t.Fatal("reenabling a namespace discarded its field selection")
				}
			}
			if a.configSync.optionsSaved != app.DefaultConfigSyncOptions() {
				t.Fatal("editing individual field choices rewrote the saved baseline")
			}
		})
	}
}

func TestConfigurationSyncOptionsSaveKeepsConnectionDraftAndSyncError(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a, callbacks := configurationSyncOptionsFixture(locale)
			a.configSync.manual.config.Endpoint = "https://unsaved.example.test"
			a.configSync.manual.secrets.Passphrase = "fake-unsaved-sync-phrase"
			before := a.configSync.manual
			a.configSync.errorCode = "conflict"
			a.configSync.status.ErrorCode = "conflict"
			requests := make(chan app.Request, 2)
			a.service = backendFunc(func(r app.Request) app.Response {
				requests <- r
				return syncResponse(a, app.ConfigSyncStatus{Options: *r.SyncOptions, ErrorCode: "conflict"})
			})
			view := ui.NewTester(configurationSyncOptionsView(a), 620, 1800)
			chooseConfigurationSyncBoolean(t, view, a.t("%s：开启或关闭", a.t("扩展偏好")), false)
			if err := view.Click(a.t("保存同步选项")); err != nil {
				t.Fatal(err)
			}
			awaitMain(t, callbacks)
			r := <-requests
			if r.Action != "config-sync-options" || r.SyncOptions == nil || r.SyncOptions.Browser || r.SyncConnection != nil || r.Settings != nil {
				t.Fatal("option save mixed connection or ordinary settings into its request")
			}
			if a.configSync.manual != before || !a.configSync.editing || a.configSync.errorCode != "conflict" || a.configSync.optionsSaved != a.configSync.options {
				t.Fatal("option save changed the connection draft, sync error or saved baseline")
			}
			view.Frame()
			if !view.HasText(a.t("已保存")) || view.HasText(a.t("有未保存的同步选项。")) {
				t.Fatal("successful save did not show the saved state")
			}
			if err := view.Click(a.t("保存同步选项")); err != nil {
				t.Fatal(err)
			}
			select {
			case <-requests:
				t.Fatal("an unchanged option draft sent another save")
			default:
			}
		})
	}
}

func TestConfigurationSyncOptionsFailurePreservesEditsForRetry(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a, callbacks := configurationSyncOptionsFixture(locale)
			saved := a.configSync.optionsSaved
			var attempts atomic.Int32
			a.service = backendFunc(func(r app.Request) app.Response {
				if attempts.Add(1) == 1 {
					return app.Response{Status: "error", Code: "local_storage", Data: app.Data{ConfigSync: &app.ConfigSyncStatus{Options: saved}}}
				}
				return syncResponse(a, app.ConfigSyncStatus{Options: *r.SyncOptions})
			})
			view := ui.NewTester(configurationSyncOptionsView(a), 620, 1800)
			chooseConfigurationSyncBoolean(t, view, a.t("%s：开启或关闭", a.t("修改后自动同步")), false)
			for attempt := 0; attempt < 2; attempt++ {
				if err := view.Click(a.t("保存同步选项")); err != nil {
					t.Fatal(err)
				}
				awaitMain(t, callbacks)
				view.Frame()
				if a.configSync.options.SyncOnChange {
					t.Fatal("failed save or retry restored the old draft")
				}
				if attempt == 0 && (a.configSync.optionsSaved != saved || !view.HasText(a.configSyncError("local_storage"))) {
					t.Fatal("failed option save hid its error or replaced the saved baseline")
				}
			}
			if attempts.Load() != 2 || a.configSync.optionsSaved != a.configSync.options || a.configSync.optionsErrorCode != "" {
				t.Fatal("retry did not persist the retained draft or clear the save error")
			}
		})
	}
}

func TestConfigurationSyncOptionsKeepLaterEditsDuringSaveAndStatusRefresh(t *testing.T) {
	a, callbacks := configurationSyncOptionsFixture("zh-CN")
	started, release := make(chan app.Request, 1), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	a.service = backendFunc(func(r app.Request) app.Response {
		started <- r
		<-release
		return syncResponse(a, app.ConfigSyncStatus{Options: *r.SyncOptions})
	})
	view := ui.NewTester(configurationSyncOptionsView(a), 620, 1800)
	chooseConfigurationSyncBoolean(t, view, a.t("%s：开启或关闭", a.t("客户端偏好")), false)
	if err := view.Click(a.t("保存同步选项")); err != nil {
		t.Fatal(err)
	}
	var r app.Request
	select {
	case r = <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("option save did not start")
	}
	captured := *r.SyncOptions
	chooseConfigurationSyncBoolean(t, view, a.t("%s：开启或关闭", a.t("扩展偏好")), false)
	if *r.SyncOptions != captured {
		t.Fatal("an edit during save mutated the captured request")
	}
	close(release)
	awaitMain(t, callbacks)
	if a.configSync.options.Desktop || a.configSync.options.Browser || a.configSync.optionsSaved != captured {
		t.Fatal("a completed save replaced newer edits or recorded them as already saved")
	}
	a.acceptConfigSyncState(app.Data{ConfigSync: &app.ConfigSyncStatus{Options: captured}})
	view.Frame()
	if a.configSync.options.Browser || !view.HasText(a.t("有未保存的同步选项。")) {
		t.Fatal("a status refresh discarded an unsaved option edit")
	}
	// A refresh does replace a clean model, including changes loaded on restart.
	a.configSync.options = captured
	updated := captured
	updated.IntervalMinutes = 30
	a.acceptConfigSyncState(app.Data{ConfigSync: &app.ConfigSyncStatus{Options: updated}})
	if a.configSync.options != updated || a.configSync.optionsSaved != updated {
		t.Fatal("a clean model did not accept newly saved options")
	}
}

func TestConfigurationSyncOptionsRevertIntervalDuringSaveRemainsUnsaved(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a, callbacks := configurationSyncOptionsFixture(locale)
			started, release := make(chan app.Request, 1), make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			a.service = backendFunc(func(r app.Request) app.Response {
				started <- r
				<-release
				return syncResponse(a, app.ConfigSyncStatus{Options: *r.SyncOptions})
			})
			view := ui.NewTester(configurationSyncOptionsView(a), 620, 1800)
			if err := view.Click(syncIntervalLabel(a, 5)); err != nil {
				t.Fatal(err)
			}
			if err := view.Click(syncIntervalLabel(a, 15)); err != nil {
				t.Fatal(err)
			}
			if err := view.Click(a.t("保存同步选项")); err != nil {
				t.Fatal(err)
			}
			var request app.Request
			select {
			case request = <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("interval save did not start")
			}
			if err := view.Click(syncIntervalLabel(a, 15)); err != nil {
				t.Fatal(err)
			}
			if err := view.Click(syncIntervalLabel(a, 5)); err != nil {
				t.Fatal(err)
			}
			if request.SyncOptions.IntervalMinutes != 15 || a.configSync.options.IntervalMinutes != 5 {
				t.Fatal("reverting the draft changed the interval already being saved")
			}
			close(release)
			awaitMain(t, callbacks)
			view.Frame()
			if a.configSync.options.IntervalMinutes != 5 || a.configSync.optionsSaved.IntervalMinutes != 15 || !view.HasText(a.t("有未保存的同步选项。")) {
				t.Fatal("save completion discarded the interval reverted to the previous saved value")
			}
		})
	}
}

func TestConfigurationSyncOptionsCannotSaveBeforeLoadInPreviewOrDuringWork(t *testing.T) {
	for _, state := range []string{"unloaded", "preview", "operation", "busy", "quit", "closing"} {
		t.Run(state, func(t *testing.T) {
			a, _ := configurationSyncOptionsFixture("en")
			a.configSync.options.IntervalMinutes = 30
			switch state {
			case "unloaded":
				a.configSync.optionsLoaded = false
			case "preview":
				a.state.Preview = true
			case "operation":
				a.configSync.operation = "config-sync-now"
			case "busy":
				a.configSync.status.Busy = true
			case "quit":
				a.quit.requested = true
			case "closing":
				a.closing = true
			}
			var calls atomic.Int32
			a.service = backendFunc(func(app.Request) app.Response { calls.Add(1); return app.Response{} })
			view := ui.NewTester(configurationSyncOptionsView(a), 620, 1800)
			if err := view.Click(a.t("保存同步选项")); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 0 || a.configSync.operation == "config-sync-options" {
				t.Fatal("an unavailable save started backend work")
			}
		})
	}
}

func TestConfigurationSyncOptionsAreSearchableByLocalizedTitles(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		a, _ := configurationSyncOptionsFixture(locale)
		categories := a.settingsCategories()
		for _, source := range []string{"同步选项", "同步时间间隔", "修改后自动同步", "同步内容", "客户端偏好", "扩展偏好", "同步模式", "仅手动同步", "启动时同步", "同步异常提醒", "主题与外观"} {
			found := false
			for _, result := range components.SearchSettings(categories, a.t(source)) {
				category := categories[result.Category]
				if category.ID == "config-sync" && category.Groups[result.Group].Items[result.Item].ID == "config-sync-content" {
					found = true
				}
			}
			if !found {
				t.Errorf("%s search did not find synchronization options for %q", locale, a.t(source))
			}
		}
	}
}

func TestConfigurationSyncOptionsNarrowLayoutAndScreenshots(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, scale := range []int{80, 120} {
			for _, width := range []int{360, 664} {
				for _, dark := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%d/%d/%t", locale, scale, width, dark), func(t *testing.T) {
						a, _ := configurationSyncOptionsFixture(locale)
						a.settings.FontScale, a.state.Settings.FontScale = scale, scale
						view := ui.NewTester(configurationSyncOptionsView(a), width, 1800)
						view.SetPreferences(ui.Preferences{ReduceMotion: true})
						view.SetDark(dark)
						assertTypographyBounds(t, view, width, 1800, a.t("同步内容"), a.t("同步行为"), a.t("同步模式"), a.t("同步时间间隔"), a.t("修改后自动同步"), a.t("启动时同步"), a.t("同步异常提醒"), a.t("客户端偏好"), a.t("扩展偏好"), a.t("保存同步选项"))
						if dir := os.Getenv("PASSPINE_SYNC_OPTIONS_SCREENSHOT_DIR"); dir != "" {
							if err := os.MkdirAll(dir, 0700); err != nil {
								t.Fatal(err)
							}
							file, err := os.Create(filepath.Join(dir, fmt.Sprintf("sync-options-%s-%d-%d-dark-%t.png", locale, scale, width, dark)))
							if err != nil {
								t.Fatal(err)
							}
							err = png.Encode(file, view.Image())
							closeErr := file.Close()
							if err != nil || closeErr != nil {
								t.Fatalf("save screenshot: encode=%v close=%v", err, closeErr)
							}
						}
					})
				}
			}
		}
	}
}

func TestConfigurationSyncPageConnectionAndOptionsRemainReachable(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, width := range []int{860, 1120} {
			for _, state := range []string{"disconnected", "connected", "editing"} {
				t.Run(fmt.Sprintf("%s/%d/%s", locale, width, state), func(t *testing.T) {
					a, _ := configurationSyncOptionsFixture(locale)
					a.page, a.settingsState.Category = "settings", "config-sync"
					a.settings.FontScale, a.state.Settings.FontScale = 120, 120
					a.configSync.manual.config = cloudsync.Config{Endpoint: "https://storage.example.test", Region: "auto", Bucket: "test-bucket", Prefix: "passpine", PathStyle: true}
					label := a.t("服务端点")
					if state != "disconnected" {
						a.configSync.status.Provider = "s3"
						a.configSync.status.Enabled, a.configSync.status.Ready = true, true
						a.configSync.status.Config = a.configSync.manual.config
					}
					if state == "connected" {
						a.configSync.editing = false
						label = a.t("立即同步")
					}
					view := ui.NewTester(testView(a), width, 760)
					view.SetPreferences(ui.Preferences{ReduceMotion: true})
					view.SetDark(locale == "en")
					view.Advance(time.Second)
					assertTypographyBounds(t, view, width, 760, label)
					saveConfigurationSyncPageScreenshot(t, view, locale, width, state, "top")
					view.Scroll(float32(width-100), 450, 0, 10000)
					view.Advance(time.Second)
					assertTypographyBounds(t, view, width, 760, a.t("保存同步选项"))
					saveConfigurationSyncPageScreenshot(t, view, locale, width, state, "options")
					if a.configSync.operation != "" || a.configSync.options != app.DefaultConfigSyncOptions() || a.configSync.manual.config.Bucket != "test-bucket" {
						t.Fatal("layout or scrolling changed a draft or started synchronization")
					}
				})
			}
		}
	}
}

func TestConfigurationSyncPageSavesFromHeaderWhileScrolled(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		t.Run(locale, func(t *testing.T) {
			a, callbacks := configurationSyncOptionsFixture(locale)
			a.page, a.settingsState.Category = "settings", "config-sync"
			a.configSync.editing = false
			a.configSync.status.Provider = "s3"
			a.configSync.status.Enabled, a.configSync.status.Ready = true, true
			a.configSync.status.Config = cloudsync.Config{Endpoint: "https://storage.example.test", Bucket: "test-bucket", Prefix: "passpine"}
			requests := make(chan app.Request, 1)
			a.service = backendFunc(func(r app.Request) app.Response {
				requests <- r
				return syncResponse(a, app.ConfigSyncStatus{Options: *r.SyncOptions})
			})
			view := ui.NewTester(testView(a), 860, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			view.Scroll(760, 450, 0, 300)
			view.Advance(time.Second)
			if err := view.Click(a.t("%s：%s", a.t("扩展偏好"), a.t("字号"))); err != nil {
				t.Fatal(err)
			}
			if a.configSync.options.BrowserFields.FontScale {
				t.Fatal("the extension font-size choice did not change")
			}
			view.Scroll(760, 450, 0, 10000)
			view.Advance(time.Second)
			assertTypographyBounds(t, view, 860, 760, a.t("保存同步选项"))
			if err := view.Click(a.t("保存同步选项")); err != nil {
				t.Fatal(err)
			}
			awaitMain(t, callbacks)
			r := <-requests
			if r.Action != "config-sync-options" || r.SyncOptions == nil || r.SyncOptions.BrowserFields.FontScale || a.configSync.optionsSaved.BrowserFields.FontScale {
				t.Fatal("the fixed header did not persist the selected preferences while the page was scrolled")
			}
		})
	}
}

func saveConfigurationSyncPageScreenshot(t *testing.T, view *ui.Tester, locale string, width int, state, position string) {
	t.Helper()
	dir := os.Getenv("PASSPINE_SYNC_PAGE_SCREENSHOT_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, fmt.Sprintf("sync-page-%s-%d-%s-%s.png", locale, width, state, position)))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, view.Image())
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("save screenshot: encode=%v close=%v", err, closeErr)
	}
}
