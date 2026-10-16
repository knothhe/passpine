package desktop

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/i18n"
)

func TestEnglishInterfaceAndLanguageSwitchPreserveVaultData(t *testing.T) {
	a := fixture()
	a.state.Settings.Language = "en"
	before := a.detail
	view := ui.NewTester(testView(a), 1120, 760)
	for _, label := range []string{"Search entries", "Settings", "Edit entry", "Show all fields", a.t("复制%s", a.t("密码"))} {
		if _, ok := view.Find(label); !ok {
			t.Errorf("English control %q is missing", label)
		}
	}
	if view.HasText(a.detail.Password) {
		t.Fatal("language switch revealed the password")
	}
	if !view.HasText(a.detail.Notes) {
		t.Fatal("Chinese vault content was translated or hidden")
	}
	a.settings.Language = "zh-CN"
	view.Frame()
	if _, ok := view.Find("编辑条目"); !ok {
		t.Fatal("setting a language did not repaint the active interface")
	}
	if a.detail.Password != before.Password || a.detail.Notes != before.Notes || a.detail.Contents != before.Contents {
		t.Fatal("switching language changed vault contents")
	}
}

func TestEnglishPreferencesAndQuickSearch(t *testing.T) {
	a := fixture()
	a.state.Settings.Language = "en"
	a.settings, a.settingsSaved, a.settingsObserved = a.state.Settings, a.state.Settings, a.state.Settings
	a.settingsLoaded = true
	a.page = "settings"
	view := ui.NewTester(testView(a), 1000, 740)
	for _, label := range []string{"General", "Security", "Browsers", "Language", "Maximum activity entries", "Retention days"} {
		if !view.HasText(label) {
			t.Errorf("English preferences text %q is missing", label)
		}
	}
	quick := ui.NewTester(a.quickSearchView, 660, 460)
	for _, label := range []string{"Open main window", "Copy password", "4 entries"} {
		if !quick.HasText(label) {
			t.Errorf("English quick search text %q is missing", label)
		}
	}
	if quick.HasText(a.detail.Password) {
		t.Fatal("quick search exposed decrypted contents")
	}
}

// Every explicit message key must have an English translation: English is also
// the fallback for machines whose language is not supported by this client.
func TestDesktopMessageCatalogCoverage(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "t" {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			key, err := strconv.Unquote(literal.Value)
			if err != nil || !i18n.HasEnglish(key) {
				t.Errorf("%s: missing English translation for %s", path, literal.Value)
			}
			return true
		})
	}
}

func TestActivityAndMenusUseSelectedLanguage(t *testing.T) {
	a := fixture()
	a.state.Settings.Language = "en"
	for kind, values := range activityLabels {
		for value, key := range values {
			if !i18n.HasEnglish(key) {
				t.Errorf("activity %s/%s has no English translation", kind, value)
			}
		}
	}
	if a.activityLabel("action", "save") != "Save entry" || a.activityLabel("outcome", "ok") != "Succeeded" {
		t.Fatal("activity codes were not translated")
	}
	if a.activityLabel("action", "unknown-action") != "unknown-action" {
		t.Fatal("unknown activity code changed")
	}
	menu := a.localizedEditMenu()
	if menu.Label != "Edit" || menu.Submenu[0].Label != "Undo" {
		t.Fatal("native edit menu did not use English")
	}
	a.settings.Language = "zh-CN"
	menu = a.localizedEditMenu()
	if menu.Label != "编辑" || menu.Submenu[0].Label != "撤销" || a.localizedWindowMenu().Label != "窗口" {
		t.Fatal("native menu did not follow the changed app language")
	}
}
