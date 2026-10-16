package desktop

import (
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestDurationChoicesPreserveSavedValuesWithinThreeOptions(t *testing.T) {
	for _, test := range []struct {
		name    string
		current int
		presets []int
		want    []int
	}{
		{"idle default", 1, []int{1, 10, 30}, []int{1, 10, 30}},
		{"idle previous preset", 5, []int{1, 10, 30}, []int{5, 10, 30}},
		{"idle saved", 15, []int{1, 10, 30}, []int{1, 15, 30}},
		{"session saved", 120, []int{30, 60, 480}, []int{30, 120, 480}},
		{"clipboard saved", 120, []int{15, 30, 60}, []int{15, 30, 120}},
		{"sync disabled", 0, []int{0, 15, 30}, []int{0, 15, 30}},
		{"sync saved", 5, []int{0, 15, 30}, []int{0, 5, 30}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := append([]int(nil), test.presets...)
			if got := durationOptions(test.current, test.presets); !reflect.DeepEqual(got, test.want) || len(got) != 3 {
				t.Fatalf("saved duration was lost or added a fourth option: %v", got)
			}
			if !reflect.DeepEqual(test.presets, before) {
				t.Fatal("rendering mutated the shared presets")
			}
		})
	}
	value := 120
	view := ui.NewTester(func(c *ui.Context) {
		fixture().durationSegments(c, &value, "清理剪贴板", []int{15, 30, 60}, "秒")
	}, 400, 100)
	if value != 120 || !view.HasText("120 秒") {
		t.Fatal("rendering replaced or hid the previously saved duration")
	}
	if err := view.Click("15 秒"); err != nil {
		t.Fatal(err)
	}
	if value != 15 {
		t.Fatal("an explicit duration selection did not update the setting")
	}
}

func TestIdleLockKeepsPresetsAlongsideSavedDuration(t *testing.T) {
	a := fixture()
	a.page = "settings"
	a.settingsState.Category = "security"
	a.state.Settings.IdleMinutes = 5
	view := ui.NewTester(testView(a), 1120, 760)
	if a.settings.IdleMinutes != 5 {
		t.Fatal("rendering changed the previously saved idle duration")
	}
	for _, label := range []string{"1 分钟", "5 分钟", "10 分钟", "30 分钟"} {
		if !view.HasText(label) {
			t.Fatalf("idle duration hid a preset or the saved value: %s", label)
		}
	}
	if err := view.Click("1 分钟"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if a.settings.IdleMinutes != 1 || view.HasText("5 分钟") {
		t.Fatal("choosing one minute did not replace the saved custom option")
	}
	if got := idleDurationPresets(a.settings.IdleMinutes); !reflect.DeepEqual(got, []int{1, 10, 30}) {
		t.Fatalf("choosing a preset did not restore the three idle choices: %v", got)
	}
}

func TestSettingsReuseSidebarAndSearchAcrossCategories(t *testing.T) {
	for _, width := range []int{860, 1120} {
		a := fixture()
		a.page = "settings"
		view := ui.NewTester(testView(a), width, 760)
		category, ok := view.Find("安全")
		if !ok || category.X+category.W > defaultSidebarWidth {
			t.Fatalf("settings navigation created another sidebar at width %d: %v", width, category)
		}
		if _, ok := view.Find("搜索条目"); ok {
			t.Fatal("settings retained a separate entry sidebar")
		}
		if err := view.Click("安全"); err != nil {
			t.Fatal(err)
		}
		control, ok := view.Find("空闲自动锁定")
		if a.settingsState.Category != "security" || !ok || control.X < defaultSidebarWidth {
			t.Fatal("settings category did not render beside the existing sidebar")
		}
		if err := view.Click("搜索设置"); err != nil {
			t.Fatal(err)
		}
		view.Type("恢复中心")
		if !view.HasText("打开恢复中心") || a.settingsState.Category != "security" {
			t.Fatal("settings search did not find another category or lost the selected category")
		}
		a.settingsState.Query = ""
		view.Frame()
		if !view.HasText("空闲自动锁定") {
			t.Fatal("clearing search did not restore the selected category")
		}
	}
}

func TestSettingsKeepRecoveryAndActivityDestinations(t *testing.T) {
	for _, test := range []struct{ category, button, page string }{
		{"vault", "打开恢复中心", "recovery"},
		{"general", "打开操作记录", "activity"},
	} {
		a := fixture()
		a.page = "settings"
		a.settingsState.Category = test.category
		callbacks := make(chan func(), 1)
		a.runOnMain = func(callback func()) { callbacks <- callback }
		state := a.state
		a.service = backendFunc(func(app.Request) app.Response { return app.Response{Status: "ok", Data: state} })
		view := ui.NewTester(testView(a), 1120, 760)
		if err := view.Click(test.button); err != nil {
			t.Fatal(err)
		}
		if a.preferencesPage() != test.page {
			t.Fatalf("settings entry opened %q instead of %q", a.preferencesPage(), test.page)
		}
		awaitMain(t, callbacks)
		view.Frame()
		category, ok := view.Find("安全")
		if !ok || category.X+category.W > defaultSidebarWidth {
			t.Fatal("settings destination did not retain the shared sidebar")
		}
	}
}

func TestGitSettingsExplainStatusWithoutDuplicateRefresh(t *testing.T) {
	a := fixture()
	a.page = "settings"
	a.settingsState.Query = "Git 同步状态"
	a.state.GitAvailable, a.state.GitInitialized = true, true
	a.state.GitBranch, a.state.GitRemote, a.state.GitUpstream = "main", "git@example.test:vault.git", "origin/main"
	a.state.GitLastSync = "2026-10-15T08:00:00Z"
	view := ui.NewTester(testView(a), 860, 760)
	if !view.HasText("本机与远端内容一致。") || !view.HasText("立即同步") {
		t.Fatal("Git settings lost its status explanation or primary action")
	}
	if _, ok := view.Find("刷新 Git 状态"); ok {
		t.Fatal("the redundant refresh icon remains beside sync")
	}
	if strings.Contains(a.gitStatusDescription(), "0 项变更") {
		t.Fatal("clean Git status still contains zero-value counters")
	}
	a.state.GitChanges, a.state.GitAhead = 2, 1
	view.Frame()
	if !view.HasText("2 项变更 · 1 个提交待推送") {
		t.Fatal("pending work is missing from the Git status")
	}
	a.state.GitError = "无法访问远端仓库"
	view.Frame()
	if !view.HasText(a.state.GitError) || !view.HasText("检查网络连接与远端配置后重试。") {
		t.Fatal("Git failure is missing its details or recovery guidance")
	}
}
