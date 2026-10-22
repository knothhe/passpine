package desktop

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// Only data is read from the active theme. Never source shell configuration or
// write hooks into the user's Omarchy installation.
var omarchyColors = struct {
	sync.RWMutex
	loaded bool
	raw    string
	colors map[string]ui.Color
	dark   bool
}{}

var omarchyColorLine = regexp.MustCompile(`^([a-z_]+)\s*=\s*(?:"(#[0-9a-fA-F]{6})"|'(#[0-9a-fA-F]{6})')\s*(?:#.*)?$`)
var omarchyModeLine = regexp.MustCompile(`^(mode|theme_type)\s*=\s*(?:"(light|dark)"|'(light|dark)')\s*(?:#.*)?$`)

func parseOmarchyColors(raw string) map[string]ui.Color {
	colors := make(map[string]ui.Color)
	for _, line := range strings.Split(raw, "\n") {
		if match := omarchyColorLine.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			value := match[2]
			if value == "" {
				value = match[3]
			}
			colors[match[1]] = ui.Hex(value)
		}
	}
	for _, key := range []string{"background", "foreground", "accent"} {
		if _, ok := colors[key]; !ok {
			return nil
		}
	}
	return colors
}

func refreshOmarchyPalette() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return false
	}
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		stateDir = filepath.Join(home, ".local", "state")
	}
	data, err := os.ReadFile(filepath.Join(stateDir, "omarchy", "current", "theme", "colors.toml"))
	if os.IsNotExist(err) {
		data, _ = os.ReadFile(filepath.Join(dir, "omarchy", "current", "theme", "colors.toml"))
	}
	raw := string(data)
	omarchyColors.Lock()
	defer omarchyColors.Unlock()
	if omarchyColors.loaded && raw == omarchyColors.raw {
		return false
	}
	omarchyColors.loaded, omarchyColors.raw = true, raw
	omarchyColors.colors = parseOmarchyColors(raw)
	omarchyColors.dark = parseOmarchyDark(raw, omarchyColors.colors["background"])
	return true
}

func applyOmarchyPalette(t *ui.Theme) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	omarchyColors.RLock()
	loaded := omarchyColors.loaded
	omarchyColors.RUnlock()
	if !loaded {
		refreshOmarchyPalette()
	}
	omarchyColors.RLock()
	defer omarchyColors.RUnlock()
	colors := omarchyColors.colors
	if colors == nil {
		return false
	}
	background, foreground, accent := colors["background"], colors["foreground"], colors["accent"]
	// The accent follows the system even when an explicit light/dark appearance
	// differs from the active Omarchy theme. Adopt surfaces only when they match.
	dark := omarchyColors.dark
	if dark == t.Dark {
		t.Background, t.Text = background, foreground
		t.Surface = background.Mix(foreground, .04)
		if surface, ok := colors["dark_background"]; ok {
			t.Surface = surface
		}
		t.SurfaceHover = t.Surface.Mix(foreground, .08)
		t.SurfacePressed = t.Surface.Mix(foreground, .13)
		t.Border = t.Surface.Mix(foreground, .20)
		t.TextMuted = foreground.Mix(background, .25)
		t.Selection = t.SurfacePressed
		if selection, ok := colors["selection"]; ok {
			t.Selection = selection
		}
	}
	t.Accent = accent
	t.AccentText = ui.Hex("#000000")
	if themeContrastRatio(ui.Hex("#ffffff"), accent) > themeContrastRatio(t.AccentText, accent) {
		t.AccentText = ui.Hex("#ffffff")
	}
	shade := ui.Hex("#ffffff")
	if t.AccentText == shade {
		shade = ui.Hex("#000000")
	}
	t.AccentHover, t.AccentPressed = accent.Mix(shade, .08), accent.Mix(shade, .16)
	t.Focus, t.Scrollbar = accent.Alpha(.65), t.Text.Alpha(.32)
	t.Inverse, t.InverseText = t.Text, t.Background
	return true
}

func themeContrastRatio(a, b ui.Color) float64 {
	luminance := func(c ui.Color) float64 {
		linear := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= .04045 {
				return x / 12.92
			}
			return math.Pow((x+.055)/1.055, 2.4)
		}
		return .2126*linear(c.R) + .7152*linear(c.G) + .0722*linear(c.B)
	}
	x, y := luminance(a), luminance(b)
	return (max(x, y) + .05) / (min(x, y) + .05)
}

// Omarchy's palette also defines the system appearance independently of whether
// the desktop portal has published its color-scheme preference yet.
func omarchyAppearance() string {
	if runtime.GOOS != "linux" {
		return "system"
	}
	omarchyColors.RLock()
	loaded := omarchyColors.loaded
	omarchyColors.RUnlock()
	if !loaded {
		refreshOmarchyPalette()
	}
	omarchyColors.RLock()
	defer omarchyColors.RUnlock()
	if omarchyColors.colors == nil {
		return "system"
	}
	if omarchyColors.dark {
		return "dark"
	}
	return "light"
}

func parseOmarchyDark(raw string, background ui.Color) bool {
	mode, legacy := "", ""
	for _, line := range strings.Split(raw, "\n") {
		match := omarchyModeLine.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		value := match[2]
		if value == "" {
			value = match[3]
		}
		if match[1] == "mode" {
			mode = value
		} else {
			legacy = value
		}
	}
	if mode == "" {
		mode = legacy
	}
	if mode != "" {
		return mode == "dark"
	}
	return int(background.R)*299+int(background.G)*587+int(background.B)*114 < 128000
}
