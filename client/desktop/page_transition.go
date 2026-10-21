package desktop

// Page identity follows navigation. Lock and editor changes keep their shared
// parent, preserving any visible list while replacing the current detail view.
type pagePresentationKey struct {
	page, category string
	setup          bool
	setupMode      int
}

func (a *application) pagePresentationKey() pagePresentationKey {
	if a.page == "settings" {
		// Resolve the default before building keyed descendants. Without an
		// animation scheduling another frame, lazy resolution inside the page
		// would replace its subtree on the first user interaction.
		a.selectedSettingsCategory(a.settingsCategories())
	}
	return pagePresentationKey{page: a.page,
		category: a.settingsState.Category, setup: a.setup || !a.state.Configured,
		setupMode: a.setupMode}
}
