//go:build !linux

package desktop

// AppKit and Win32 own native menu appearance on these platforms.
func (a *application) updateNativeMenuTheme() {}
