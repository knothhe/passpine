package app

const (
	DockIconShow         = "show"
	DockIconHide         = "hide"
	DockIconFollowWindow = "follow-window"
	TrayClickQuickPanel  = "quick-panel"
	TrayClickMainWindow  = "main-window"
)

// Empty enum values come from clients and preferences written before these
// options existed. Keep the previous values when an older client saves them.
func normalizeLifecycleSettings(settings *Settings, previous Settings) {
	if settings.DockIcon == "" {
		settings.DockIcon = previous.DockIcon
		if settings.DockIcon == "" {
			settings.DockIcon = DockIconShow
		}
	}
	if settings.TrayClickAction == "" {
		settings.TrayClickAction = previous.TrayClickAction
		if settings.TrayClickAction == "" {
			settings.TrayClickAction = TrayClickQuickPanel
		}
	}
}
