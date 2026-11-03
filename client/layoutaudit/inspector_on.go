//go:build !mygo_noinspector

package layoutaudit

// InspectorAvailable reports whether this build includes MyGo's native tree
// inspector. The host must also set Page.DevTools to DevToolsEnabled.
func InspectorAvailable() bool { return true }
