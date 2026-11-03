package main

import (
	"fmt"
	"testing"

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
				allowed := !test.enabled || test.preview && !test.seed && directory == ""
				if (err == nil) != allowed {
					t.Fatalf("validation error = %v, allowed = %v", err, allowed)
				}
				if test.enabled && test.seed && err.Error() != i18n.Text(i18n.SystemLocale(), i18n.LayoutAuditNoSeed) {
					t.Fatalf("seed conflict did not explain the non-isolated operation: %v", err)
				}
			})
		}
	}

	for _, locale := range []string{i18n.SimplifiedChinese, i18n.English} {
		message := i18n.Text(locale, i18n.LayoutAuditNoSeed)
		if message == "" || locale == i18n.English && message == i18n.LayoutAuditNoSeed {
			t.Fatalf("missing seed conflict translation for %s", locale)
		}
	}
}
