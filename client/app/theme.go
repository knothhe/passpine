package app

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	ThemeStyleOmarchy    = "omarchy"
	ThemeStyleOnevra     = "onevra"
	ThemeStyleMonochrome = "monochrome"
)

// NormalizeThemeStyle keeps legacy or stale preferences on the default style.
// Explicit settings updates are validated before they reach persisted storage.
func NormalizeThemeStyle(style string) string {
	if style == ThemeStyleOnevra || (style == ThemeStyleOmarchy && runtime.GOOS == "linux") {
		return style
	}
	return ThemeStyleMonochrome
}

// New installations use Omarchy colors only when an active Omarchy theme is
// present. Persisted style choices are left intact.
func defaultThemeStyle() string {
	if runtime.GOOS != "linux" {
		return ThemeStyleMonochrome
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		if home, err := os.UserHomeDir(); err == nil {
			state = filepath.Join(home, ".local", "state")
		}
	}
	config, _ := os.UserConfigDir()
	for _, dir := range []string{state, config} {
		if dir == "" {
			continue
		}
		if info, err := os.Stat(filepath.Join(dir, "omarchy", "current", "theme", "colors.toml")); err == nil && info.Mode().IsRegular() {
			return ThemeStyleOmarchy
		}
	}
	return ThemeStyleMonochrome
}
