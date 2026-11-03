//go:build mygo_noinspector

package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/i18n"
)

func TestReleaseBuildOmitsLayoutAudit(t *testing.T) {
	a := fixture()
	a.enableAudit(true)
	view := ui.NewTester(testView(a), 1120, 760)
	if LayoutAuditAvailable || a.audit.Enabled || a.mainShellOptions().DevTools || len(a.auditMenu()) != 0 || view.HasText(a.t(i18n.LayoutAuditTitle)) {
		t.Fatal("release build exposed layout audit tools")
	}
}
