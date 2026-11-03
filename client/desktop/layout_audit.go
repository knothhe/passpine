//go:build !mygo_noinspector

package desktop

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	layoutaudit "github.com/knothhe/mygo-layout-audit"
	"github.com/knothhe/passpine/client/i18n"
)

// LayoutAuditAvailable reports whether this build contains the layout audit
// tools. Release builds use the mygo_noinspector tag, which also removes the
// mygo-layout-audit module from the binary.
const LayoutAuditAvailable = true

type auditState = layoutaudit.State

func (a *application) enableAudit(enabled bool) {
	a.audit.Enabled, a.audit.Outlines = enabled, enabled
}

func (a *application) auditLabels() layoutaudit.Labels {
	return layoutaudit.Labels{
		Title: a.t(i18n.LayoutAuditTitle), Outlines: a.t(i18n.LayoutAuditOutlines), Grid: a.t(i18n.LayoutAuditGrid), Ruler: a.t(i18n.LayoutAuditRuler),
		Clear: a.t(i18n.LayoutAuditClear), Collapse: a.t(i18n.LayoutAuditCollapse), Expand: a.t(i18n.LayoutAuditExpand),
		PickFirst: a.t(i18n.LayoutAuditPickFirst), PickSecond: a.t(i18n.LayoutAuditPickSecond), Complete: a.t(i18n.LayoutAuditComplete),
		Legend: a.t(i18n.LayoutAuditLegend), Inspector: a.t(i18n.LayoutAuditInspector),
		Viewport: func(w, h float32) string { return a.t(i18n.LayoutAuditViewport, w, h) },
		Distance: func(x, y float32) string { return a.t(i18n.LayoutAuditDistance, x, y) },
	}
}

func (a *application) decorateAudit(view func(*ui.Context)) func(*ui.Context) {
	return a.audit.Wrap(view, layoutaudit.Options{Labels: a.auditLabels, GridStep: 8, TopInset: 56})
}

func (a *application) decorateQuickAudit() func(*ui.Context) {
	// The panel has different dimensions and must never share ruler points or
	// viewport state with the manager window. Start collapsed to keep its search
	// results accessible in the much smaller panel.
	a.quick.audit.Enabled = a.audit.Enabled
	a.quick.audit.Outlines = a.audit.Enabled
	a.quick.audit.Collapsed = true
	return a.quick.audit.Wrap(a.quickSearchView, layoutaudit.Options{Labels: a.auditLabels, GridStep: 8, TopInset: 64})
}

func (a *application) auditMenu() []*mygo.MenuItem {
	if !a.audit.Enabled {
		return nil
	}
	return []*mygo.MenuItem{mygo.Separator(), {Label: a.t(i18n.LayoutAuditMenu), Role: mygo.RoleToggleDevTools, Accelerator: "F12"}}
}
