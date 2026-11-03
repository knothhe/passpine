package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
)

const (
	ThemeStyleOmarchy    = "omarchy"
	ThemeStyleOnevra     = "onevra"
	ThemeStyleMonochrome = "monochrome"
	// ThemeStyleGlass draws the desktop window as Liquid Glass panes over a
	// soft gradient backdrop. It is a desktop-only style.
	ThemeStyleGlass = "glass"
)

const (
	DensityComfortable = "comfortable"
	DensityCompact     = "compact"
)

// NormalizeDensity keeps missing or stale layout density preferences on the
// comfortable default.
func NormalizeDensity(density string) string {
	if density == DensityCompact {
		return density
	}
	return DensityComfortable
}

// themeStyleName is the shape of every theme style name, including ones this
// version does not know yet.
var themeStyleName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// validThemeStyleName accepts any well-formed style, so a style added by a
// newer client passes through configuration sync on older ones instead of
// failing the whole document. Clients show their fallback for styles they do
// not support (NormalizeThemeStyle, NormalizeBrowserThemeStyle).
func validThemeStyleName(style string) bool { return themeStyleName.MatchString(style) }

// NormalizeBrowserThemeStyle maps a synced browser extension style to one the
// extension supports, keeping unknown or newer styles on the default.
func NormalizeBrowserThemeStyle(style string) string {
	if style == ThemeStyleOnevra {
		return style
	}
	return ThemeStyleMonochrome
}

// themeStyleFallbacks records, by sync key, the synced theme styles this
// device cannot show (newer, or for another platform) and displays a fallback
// for. Until the user picks a style here, synchronization keeps the original
// in the shared document instead of uploading the fallback, so other devices
// keep their choice. Recording it, rather than inferring it from the fallback
// value, keeps an explicit choice of the fallback style synchronizable.
//
// Settled keys have a known state: a fallback in Keys, or none. Keys that are
// not settled yet, as in preferences written before this record existed,
// infer it from the sync baseline, the previous rule, so a platform fallback
// such as Omarchy on macOS is not uploaded after an update. Each key settles
// on its own, so clearing one never stops inference for another.
type themeStyleFallbacks struct {
	Keys    map[string]string `json:"keys,omitempty"`
	Settled map[string]bool   `json:"settled,omitempty"`
}

func (f *themeStyleFallbacks) clone() *themeStyleFallbacks {
	out := &themeStyleFallbacks{Keys: map[string]string{}, Settled: map[string]bool{}}
	if f != nil {
		for key, style := range f.Keys {
			out.Keys[key] = style
		}
		for key, settled := range f.Settled {
			out.Settled[key] = settled
		}
	}
	return out
}

// record settles the style a synced key applied: a fallback for an
// unsupported style, or none for one this device shows as is.
func (f *themeStyleFallbacks) record(key, style string, normalize func(string) string) {
	f.Settled[key] = true
	if validThemeStyleName(style) && normalize(style) != style {
		f.Keys[key] = style
	} else {
		delete(f.Keys, key)
	}
}

// clear settles a key without a fallback, after the user picks a style.
func (f *themeStyleFallbacks) clear(key string) {
	f.Settled[key] = true
	delete(f.Keys, key)
}

// keep reports the synced style to upload in place of the local one while
// the local style is still the fallback for it.
func (f *themeStyleFallbacks) keep(key, local string, normalize func(string) string, base map[string]json.RawMessage) (string, bool) {
	var style string
	switch {
	case f != nil && f.Keys[key] != "":
		style = f.Keys[key]
	case f != nil && f.Settled[key]:
		return "", false
	case json.Unmarshal(base[key], &style) != nil || !validThemeStyleName(style):
		return "", false
	}
	return style, normalize(style) != style && local == normalize(style)
}

// clearThemeStyleFallback forgets a fallback after the user picks a style on
// this device, so that choice synchronizes even when it equals the fallback.
func (s *Service) clearThemeStyleFallback(key string) {
	next := s.prefs.ThemeStyleFallbacks.clone()
	next.clear(key)
	s.prefs.ThemeStyleFallbacks = next
}

// NormalizeThemeStyle keeps legacy or stale preferences on the default style.
// Explicit settings updates are validated before they reach persisted storage.
func NormalizeThemeStyle(style string) string {
	if style == ThemeStyleOnevra || style == ThemeStyleGlass || (style == ThemeStyleOmarchy && runtime.GOOS == "linux") {
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
