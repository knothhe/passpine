// Adapted from Onevra v0.2026.9 shells/sidebar; see LICENSE.
package clientshell

// Labels supplies application-localized names for Shell's own controls.
// Empty fields restore English defaults. It does not affect native OS menus.
type Labels struct {
	LeftToggle, LeftResize             string
	Minimize, Maximize, Restore, Close string
}

// SetLabels changes control names without recreating the window or its content.
// Call on the UI thread. Sidebar geometry, page, and focus identities are retained.
func (s *Shell) SetLabels(labels Labels) {
	s.options.LeftToggleLabel = captionLabel(labels.LeftToggle, "Toggle sidebar")
	s.options.LeftResizeLabel = captionLabel(labels.LeftResize, "Resize sidebar")
	s.options.MinimizeLabel = labels.Minimize
	s.options.MaximizeLabel = labels.Maximize
	s.options.RestoreLabel = labels.Restore
	s.options.CloseLabel = labels.Close
	s.refresh()
}
