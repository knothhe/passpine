package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

// Group labels distinguish repeated On/Off choices in the full settings page.
func clickBooleanSettingSegment(t *testing.T, view *ui.Tester, label string, enabled bool) {
	t.Helper()
	bounds, ok := view.Find(label)
	if !ok {
		t.Fatalf("boolean setting group is missing: %s", label)
	}
	fraction := float32(.75)
	if enabled {
		fraction = .25
	}
	view.ClickAt(bounds.X+bounds.W*fraction, bounds.Y+bounds.H/2)
}

var booleanSettingsCases = []struct {
	title string
	value func(*app.Settings) *bool
}{
	{"关闭窗口后继续运行", func(s *app.Settings) *bool { return &s.CloseToTray }},
	{"轻量模式", func(s *app.Settings) *bool { return &s.LightweightMode }},
	{"开机自启", func(s *app.Settings) *bool { return &s.LaunchAtLogin }},
	{"使用系统验证解锁", func(s *app.Settings) *bool { return &s.DeviceUnlock }},
	{"锁定时允许列出名称", func(s *app.Settings) *bool { return &s.BrowseWhileLocked }},
	{"自动同步", func(s *app.Settings) *bool { return &s.AutoSync }},
	{"本机操作记录", func(s *app.Settings) *bool { return &s.RecordActivity }},
}

func TestBooleanSettingsSegmentsPreserveSavedValuesAndAutosave(t *testing.T) {
	for _, setting := range booleanSettingsCases {
		if setting.title == "本机操作记录" {
			// This phrase occurs in the category description, so searching it
			// includes every general setting. Its builder is covered separately.
			continue
		}
		for _, initial := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", setting.title, initial), func(t *testing.T) {
				a := fixture()
				a.page, a.settingsState.Query = "settings", setting.title
				if setting.title == "开机自启" {
					a.settingsState.Query = "登录系统后在后台启动 Passpine。"
				}
				a.state.Preview, a.state.DeviceUnlockAvailable = false, true
				*setting.value(&a.state.Settings) = initial
				callbacks, requests := make(chan func(), 1), make(chan app.Request, 1)
				a.runOnMain = func(callback func()) { callbacks <- callback }
				a.service = backendFunc(func(request app.Request) app.Response {
					requests <- request
					return app.Response{Status: "ok", Data: app.Data{Settings: *request.Settings}}
				})
				view := ui.NewTester(testView(a), 860, 620)
				if *setting.value(&a.settings) != initial || !view.HasText("开启") || !view.HasText("关闭") {
					t.Fatal("rendering lost the saved value or a boolean choice")
				}
				label := "开启"
				if initial {
					label = "关闭"
				}
				if err := view.Click(label); err != nil {
					t.Fatal(err)
				}
				awaitMain(t, callbacks)
				request := <-requests
				if request.Action != "settings" || *setting.value(request.Settings) == initial || *setting.value(&a.settingsSaved) == initial {
					t.Fatal("choosing the opposite boolean segment did not persist the setting")
				}
				view.Frame()
				if err := view.Click(label); err != nil {
					t.Fatal(err)
				}
				if a.settingsSaving || a.settingsDirty {
					t.Fatal("choosing the already selected segment started another save")
				}
			})
		}
	}
}

func TestActivityBooleanSegmentsUseTheExistingSettingsField(t *testing.T) {
	a := fixture()
	a.settings, a.settingsSaved = a.state.Settings, a.state.Settings
	a.settings.RecordActivity = false
	var build func(*ui.Context)
	for _, category := range a.settingsCategories() {
		for _, group := range category.Groups {
			for _, item := range group.Items {
				if item.ID == "activity" {
					build = item.Build
				}
			}
		}
	}
	if build == nil {
		t.Fatal("activity setting is missing from the settings catalog")
	}
	view := ui.NewTester(build, 220, 80)
	if a.settings.RecordActivity {
		t.Fatal("rendering replaced the saved activity preference")
	}
	if err := view.Click("开启"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.RecordActivity || a.settings.CloseToTray != a.state.Settings.CloseToTray {
		t.Fatal("activity segment changed a different boolean preference")
	}
	if err := view.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	if a.settings.RecordActivity {
		t.Fatal("activity Off segment did not restore the saved value")
	}
}

func TestBooleanSettingsSegmentsRespectNativeAvailabilityAndKeyboardNavigation(t *testing.T) {
	for _, test := range []struct {
		title   string
		preview bool
		value   func(*app.Settings) *bool
	}{
		{"开机自启", true, func(s *app.Settings) *bool { return &s.LaunchAtLogin }},
		{"使用系统验证解锁", false, func(s *app.Settings) *bool { return &s.DeviceUnlock }},
	} {
		t.Run(test.title, func(t *testing.T) {
			a := fixture()
			a.page, a.settingsState.Query = "settings", test.title
			if test.title == "开机自启" {
				a.settingsState.Query = "登录系统后在后台启动 Passpine。"
			}
			a.state.Preview = test.preview
			a.state.DeviceUnlockAvailable, a.state.DeviceUnlockEnrolled = false, false
			initial := *test.value(&a.state.Settings)
			view := ui.NewTester(testView(a), 860, 620)
			for _, label := range []string{"开启", "关闭"} {
				if err := view.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			if *test.value(&a.settings) != initial || a.settingsDirty {
				t.Fatal("disabled segments changed a protected native setting")
			}
		})
	}
	a := fixture()
	enabled := false
	view := ui.NewTester(func(c *ui.Context) { a.booleanSettingSegments(c, &enabled, "布尔设置") }, 220, 80)
	if err := view.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	view.Key(0, ui.KeyLeft)
	if !enabled {
		t.Fatal("left arrow did not select On")
	}
	view.Key(0, ui.KeyRight)
	if enabled {
		t.Fatal("right arrow did not select Off")
	}
}

func TestDockAndTraySegmentsPreserveAndSaveEveryChoice(t *testing.T) {
	for _, test := range []struct {
		title  string
		values []string
		labels []string
		value  func(*app.Settings) *string
	}{
		{"Dock 图标", []string{app.DockIconShow, app.DockIconHide, app.DockIconFollowWindow}, []string{"展示", "隐藏", "跟随窗口"}, func(s *app.Settings) *string { return &s.DockIcon }},
		{"点击托盘图标", []string{app.TrayClickQuickPanel, app.TrayClickMainWindow}, []string{"打开快捷面板", "打开主窗口"}, func(s *app.Settings) *string { return &s.TrayClickAction }},
	} {
		t.Run(test.title, func(t *testing.T) {
			a := fixture()
			a.page, a.settingsState.Query = "settings", test.title
			*test.value(&a.state.Settings) = test.values[len(test.values)-1]
			callbacks, requests := make(chan func(), 1), make(chan app.Request, 1)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.service = backendFunc(func(request app.Request) app.Response {
				requests <- request
				return app.Response{Status: "ok", Data: app.Data{Settings: *request.Settings}}
			})
			view := ui.NewTester(testView(a), 860, 620)
			if *test.value(&a.settings) != test.values[len(test.values)-1] {
				t.Fatal("rendering replaced an existing enum preference")
			}
			for i, label := range test.labels {
				if err := view.Click(label); err != nil {
					t.Fatal(err)
				}
				awaitMain(t, callbacks)
				if request := <-requests; request.Action != "settings" || *test.value(request.Settings) != test.values[i] || *test.value(&a.settingsSaved) != test.values[i] {
					t.Fatal("a visible segment did not save its existing enum value")
				}
				view.Frame()
			}
		})
	}
}

func TestSettingsSegmentsFitNarrowWindowAtTextSizeExtremes(t *testing.T) {
	queries := []string{"Dock 图标", "点击托盘图标"}
	for _, setting := range booleanSettingsCases {
		queries = append(queries, setting.title)
	}
	for _, language := range []string{"zh-CN", "en"} {
		for _, scale := range []int{80, 120} {
			for _, query := range queries {
				t.Run(fmt.Sprintf("%s/%d/%s", language, scale, query), func(t *testing.T) {
					a := fixture()
					a.state.Settings.Language, a.state.Settings.FontScale = language, scale
					a.page, a.settingsState.Query = "settings", a.t(query)
					view := ui.NewTester(testView(a), 860, 620)
					labels := []string{a.t("开启"), a.t("关闭")}
					if query == "Dock 图标" {
						labels = []string{a.t("展示"), a.t("隐藏"), a.t("跟随窗口")}
					} else if query == "点击托盘图标" {
						labels = []string{a.t("打开快捷面板"), a.t("打开主窗口")}
					}
					assertTypographyBounds(t, view, 860, 620, labels...)
				})
			}
		}
	}
}

func TestSettingsSegmentsScreenshots(t *testing.T) {
	directory := os.Getenv("PASSPINE_SETTINGS_SEGMENTS_SCREENSHOTS")
	if directory == "" {
		t.Skip("set PASSPINE_SETTINGS_SEGMENTS_SCREENSHOTS to render settings")
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for _, category := range []string{"general", "security", "vault"} {
		for _, scale := range []int{80, 120} {
			for _, dark := range []bool{false, true} {
				a := fixture()
				a.page, a.settingsState.Category = "settings", category
				a.state.Settings.FontScale = scale
				view := ui.NewTester(testView(a), 860, 620)
				view.SetPreferences(ui.Preferences{ReduceMotion: true})
				view.SetDark(dark)
				file, err := os.Create(filepath.Join(directory, fmt.Sprintf("%s-%d-dark-%t.png", category, scale, dark)))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(file, view.Image()); err != nil {
					file.Close()
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
