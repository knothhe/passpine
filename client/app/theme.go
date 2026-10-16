package app

const (
	ThemeStyleOnevra     = "onevra"
	ThemeStyleMonochrome = "monochrome"
)

// NormalizeThemeStyle keeps legacy or stale preferences on the default style.
// Explicit settings updates are validated before they reach persisted storage.
func NormalizeThemeStyle(style string) string {
	if style == ThemeStyleMonochrome {
		return style
	}
	return ThemeStyleOnevra
}
