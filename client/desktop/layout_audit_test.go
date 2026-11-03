package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/i18n"
	"github.com/knothhe/passpine/client/layoutaudit"
)

func TestLayoutAuditOptInAndLiveLocalization(t *testing.T) {
	a := fixture()
	view := ui.NewTester(testView(a), 1120, 760)
	if view.HasText(a.t(i18n.LayoutAuditTitle)) || a.mainShellOptions().DevTools || len(a.auditMenu()) != 0 {
		t.Fatal("normal launch included audit tools")
	}
	a.audit.Enabled, a.audit.Outlines = true, true
	view.Frame()
	if !view.HasText(a.t(i18n.LayoutAuditTitle)) || !a.mainShellOptions().DevTools {
		t.Fatal("audit did not cover existing main view")
	}
	if layoutaudit.InspectorAvailable() {
		menu := a.auditMenu()
		if len(menu) != 2 || menu[1].Role != mygo.RoleToggleDevTools {
			t.Fatal("native inspector menu role missing")
		}
	} else if len(a.auditMenu()) != 0 || !view.HasText(a.t(i18n.LayoutAuditInspectorUnavailable)) {
		t.Fatal("build without inspector did not explain its reduced capabilities")
	}
	a.query = "untranslated draft"
	a.state.Settings.Language = "en"
	view.Frame()
	if !view.HasText("Layout audit") || a.query != "untranslated draft" {
		t.Fatal("live localization reset content or did not update audit labels")
	}
}

func TestLayoutAuditQuickSearchUsesIndependentRulerAndEscape(t *testing.T) {
	a, window := quickSearchFixture()
	a.audit.Enabled = true
	view := ui.NewTester(a.decorateQuickAudit(), 660, 460)
	if err := view.Click(a.t(i18n.LayoutAuditExpand)); err != nil {
		t.Fatal(err)
	}
	if err := view.Click(a.t(i18n.LayoutAuditRuler)); err != nil {
		t.Fatal(err)
	}
	view.ClickAt(30, 140)
	view.ClickAt(130, 220)
	m, ok := a.quick.audit.Measurement()
	if !ok || m.Horizontal() != 100 || m.Vertical() != 80 || a.audit.Ruler {
		t.Fatal("quick-search ruler was not independent from the manager")
	}
	view.Key(0, ui.KeyEscape)
	if a.quick.audit.Ruler || !a.quick.visible || window.hidden != 0 {
		t.Fatal("ruler Escape also dismissed the quick-search window")
	}
	view.Key(0, ui.KeyEscape)
	if a.quick.visible || window.hidden != 1 {
		t.Fatal("ruler exit did not restore the panel's own Escape shortcut")
	}
}

func TestLayoutAuditScreensAndScreenshotExport(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en"} {
		for _, page := range []string{"entries", "settings", "recovery", "setup", "locked", "editor", "generator", "dialog", "quick-search"} {
			t.Run(locale+"/"+page, func(t *testing.T) {
				a := fixture()
				a.state.Settings.Language = locale
				a.audit.Enabled, a.audit.Outlines, a.audit.Grid = true, true, true
				switch page {
				case "settings", "recovery":
					a.page = page
				case "setup":
					a.state.Configured = false
				case "locked":
					a.state.Locked = true
				case "editor", "generator":
					a.editEntry()
					if page == "generator" {
						a.generator.open = true
						a.generator.target = a.editor.ensureIdentity()
					}
				case "dialog":
					a.deleteOpen = true
				}
				build, width, height := testView(a), 1120, 760
				if page == "quick-search" {
					a.quick.visible = true
					build, width, height = a.decorateQuickAudit(), 660, 460
				}
				view := ui.NewTester(build, width, height)
				view.SetPreferences(ui.Preferences{ReduceMotion: true})
				view.SetDark(locale == "en")
				if !view.HasText(a.t(i18n.LayoutAuditTitle)) {
					t.Fatalf("audit toolbar missing on %s", page)
				}
				if page == "entries" {
					a.audit.Ruler = true
					view.Frame()
					view.ClickAt(240, 80)
					view.ClickAt(540, 280)
					if _, ok := a.audit.Measurement(); !ok {
						t.Fatal("main view ruler did not record points")
					}
				}
				if directory := os.Getenv("PASSPINE_LAYOUT_AUDIT_SCREENSHOTS"); directory != "" {
					if err := os.MkdirAll(directory, 0755); err != nil {
						t.Fatal(err)
					}
					file, err := os.Create(filepath.Join(directory, fmt.Sprintf("%s-%s.png", page, locale)))
					if err != nil {
						t.Fatal(err)
					}
					defer file.Close()
					if err := png.Encode(file, view.Image()); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}
