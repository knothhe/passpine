package main

import (
	"fmt"
	"testing"

	"github.com/knothhe/passpine/client/desktop"
	"github.com/knothhe/passpine/client/i18n"
)

func TestLayoutAuditRequiresIsolatedPreview(t *testing.T) {
	for _, test := range []struct{ enabled, preview, seed bool }{
		{false, false, false}, {false, true, false}, {true, false, false}, {true, true, false},
		{false, false, true}, {false, true, true}, {true, false, true}, {true, true, true},
	} {
		for _, directory := range []string{"", t.TempDir()} {
			t.Run(fmt.Sprintf("audit=%v/preview=%v/seed=%v/custom-data=%v", test.enabled, test.preview, test.seed, directory != ""), func(t *testing.T) {
				err := validateLayoutAudit(test.enabled, test.preview, test.seed, directory)
				allowed := !test.enabled || desktop.LayoutAuditAvailable && test.preview && !test.seed && directory == ""
				if (err == nil) != allowed {
					t.Fatalf("validation error = %v, allowed = %v", err, allowed)
				}
				if !desktop.LayoutAuditAvailable && test.enabled && err.Error() != i18n.Text(i18n.SystemLocale(), i18n.LayoutAuditUnavailable) {
					t.Fatalf("release build did not explain that layout audit is unavailable: %v", err)
				}
				if desktop.LayoutAuditAvailable && test.enabled && test.seed && err.Error() != i18n.Text(i18n.SystemLocale(), i18n.LayoutAuditNoSeed) {
					t.Fatalf("seed conflict did not explain the non-isolated operation: %v", err)
				}
			})
		}
	}

	for _, locale := range []string{i18n.SimplifiedChinese, i18n.English} {
		for _, key := range []string{i18n.LayoutAuditNoSeed, i18n.LayoutAuditUnavailable} {
			message := i18n.Text(locale, key)
			if message == "" || locale == i18n.English && message == key {
				t.Fatalf("missing layout audit translation for %s: %s", locale, key)
			}
		}
	}
}
