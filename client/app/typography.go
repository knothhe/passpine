package app

const DefaultFontScale = 100

// FontScale is a percentage of the client's default typography. Missing
// values in older preferences retain the default or the user's saved choice.
func NormalizeFontScale(scale int) int {
	if scale == 0 {
		return DefaultFontScale
	}
	return scale
}

func ValidFontScale(scale int) bool {
	return scale >= 80 && scale <= 120 && scale%10 == 0
}

func normalizeTypographySettings(settings *Settings, previous Settings) {
	if settings.FontScale == 0 {
		settings.FontScale = NormalizeFontScale(previous.FontScale)
	}
}
