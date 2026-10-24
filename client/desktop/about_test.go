package desktop

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/about"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/buildinfo"
	"github.com/knothhe/passpine/client/i18n"
)

func TestAboutCopiesBilingualBuildMetadataWithoutVaultData(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		t.Run(language, func(t *testing.T) {
			a := fixture()
			secret, vaultName := a.detail.Password, a.state.VaultName
			a.page, a.settingsState.Category, a.buildVersion = "settings", "about", "1.2.3-dev.abcdef"
			a.settings = a.state.Settings
			a.settings.Language = language
			a.settings.FontScale = 120
			a.settingsSaved, a.settingsObserved, a.settingsLoaded = a.settings, a.settings, true
			priorCommit, priorModified, priorExecutor := buildinfo.Commit, buildinfo.Modified, buildinfo.Executor
			buildinfo.Commit, buildinfo.Modified, buildinfo.Executor = strings.Repeat("a", 40), "true", "local"
			defer func() {
				buildinfo.Commit, buildinfo.Modified, buildinfo.Executor = priorCommit, priorModified, priorExecutor
			}()
			var copied string
			a.clipboardWriter = func(value string, sensitive bool) error {
				if sensitive {
					t.Fatal("build metadata marked as credential")
				}
				copied = value
				return nil
			}
			view := ui.NewTester(testView(a), 860, 620)
			if err := view.Click(a.t("复制构建信息")); err != nil {
				t.Fatal(err)
			}
			var metadata buildinfo.Metadata
			if json.Unmarshal([]byte(copied), &metadata) != nil || metadata.Commit != buildinfo.Commit || metadata.Version != a.buildVersion || metadata.Channel != "dev" || metadata.Modified != "true" {
				t.Fatalf("wrong copied metadata: %s", copied)
			}
			if strings.Contains(copied, secret) || strings.Contains(copied, vaultName) || !view.HasText(a.t("构建信息已复制")) {
				t.Fatal("copy included vault data or omitted feedback")
			}
			managerResizeScreenshot(t, view, "about-build-"+language)
			a.clipboardWriter = func(string, bool) error { return errors.New("clipboard unavailable") }
			if err := view.Click(a.t("复制构建信息")); err != nil {
				t.Fatal(err)
			}
			if !view.HasText(a.t(i18n.ClipboardWriteFailed)) {
				t.Fatal("clipboard failure was not reported")
			}
		})
	}
}

func TestAboutNavigationBeforeSetupDoesNotRequestGitStatus(t *testing.T) {
	a := fixture()
	a.state.Configured, a.setup, a.buildVersion = false, true, "0.1.0"
	requests := make(chan app.Request, 1)
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "error", Code: "not_configured", Message: "请先打开密码库"}
	})
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.navigateSettingsCategory("about")
	view := ui.NewTester(testView(a), 1120, 760)
	if a.setup || !view.HasText("版本 0.1.0") {
		t.Fatal("About navigation did not leave setup and show the project information")
	}
	select {
	case request := <-requests:
		t.Fatalf("About requested vault data before setup: %#v", request)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestAboutShowsVersionAndOpensDependencyLinksWithoutBackendRequests(t *testing.T) {
	a := fixture()
	a.page, a.settingsState.Category, a.buildVersion = "settings", "about", "0.1.0"
	a.state.Configured = false // Project information is also available before setup.
	view := ui.NewTester(testView(a), 1120, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	if !view.HasText("版本 0.1.0") {
		t.Fatal("project version is missing")
	}
	if err := view.Click("go-crypto"); err != nil {
		t.Fatal(err)
	}
	dep := about.Dependencies()[0]
	if !view.HasText(dep.Module) {
		t.Fatal("expanded dependency does not show its full module name")
	}
	a.settings.Language = "en"
	view.Frame()
	if !view.HasText("Version 0.1.0") || !view.HasText(dep.Module) || !view.HasText(a.t(i18n.DependencyDescriptionKey(dep.ID))) {
		t.Fatal("language switch lost dependency details or did not translate the interface")
	}
	if err := view.Click("Documentation and homepage"); err != nil {
		t.Fatal(err)
	}
	if got := view.OpenedURLs(); len(got) != 1 || got[0] != dep.URL {
		t.Fatalf("wrong dependency destination: %v", got)
	}
	if err := view.Click("go-crypto"); err != nil {
		t.Fatal(err)
	}
	if view.HasText(dep.Module) {
		t.Fatal("dependency disclosure did not close")
	}
}

func TestEveryDependencyHasBilingualPurpose(t *testing.T) {
	for _, dep := range about.Dependencies() {
		key := i18n.DependencyDescriptionKey(dep.ID)
		if key == "" || !i18n.HasEnglish(key) {
			t.Errorf("dependency %s lacks a translated purpose", dep.Module)
		}
	}
}
