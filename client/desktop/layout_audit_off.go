//go:build mygo_noinspector

package desktop

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// LayoutAuditAvailable is false in release builds: the layout audit module is
// a development dependency and is not compiled into them.
const LayoutAuditAvailable = false

type auditState struct{ Enabled bool }

func (a *application) enableAudit(bool) {}

func (a *application) decorateAudit(view func(*ui.Context)) func(*ui.Context) { return view }

func (a *application) decorateQuickAudit() func(*ui.Context) { return a.quickSearchView }

func (a *application) auditMenu() []*mygo.MenuItem { return nil }
