package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const DefaultGlobalShortcut = "Ctrl+Alt+Space"

// NormalizeGlobalShortcut accepts MyGo accelerator names and produces a stable
// value for preferences. Require a modifier for typing keys so a global binding
// cannot consume ordinary typing in another application.
func NormalizeGlobalShortcut(value string) (string, error) {
	if strings.ContainsAny(value, "\r\n\t") {
		return "", errors.New("快捷键不能包含换行或制表符")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultGlobalShortcut, nil
	}
	parts := strings.Split(value, "+")
	if strings.HasSuffix(value, "++") {
		parts = append(parts[:len(parts)-2], "Plus")
	}
	modifiers := map[string]bool{}
	for _, part := range parts[:len(parts)-1] {
		modifier := ""
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "cmd", "command", "super", "meta", "win":
			modifier = "Cmd"
		case "ctrl", "control":
			modifier = "Ctrl"
		case "alt", "option", "altgr":
			modifier = "Alt"
		case "shift":
			modifier = "Shift"
		case "cmdorctrl", "commandorcontrol":
			modifier = "CmdOrCtrl"
		default:
			return "", errors.New("请使用 Ctrl、Alt、Shift、Command 与一个按键组合")
		}
		if modifiers[modifier] {
			return "", errors.New("组合键中不能重复使用同一个修饰键")
		}
		modifiers[modifier] = true
	}
	if modifiers["CmdOrCtrl"] && (modifiers["Cmd"] || modifiers["Ctrl"]) {
		return "", errors.New("CmdOrCtrl 不能与 Ctrl 或 Command 重复组合")
	}
	key, functionKey := shortcutKey(strings.TrimSpace(parts[len(parts)-1]))
	if key == "" {
		return "", errors.New("不支持这个按键，请使用字母、数字、空格或功能键")
	}
	if !modifiers["Cmd"] && !modifiers["Ctrl"] && !modifiers["Alt"] && !modifiers["CmdOrCtrl"] && !functionKey {
		return "", errors.New("请同时按住 Ctrl、Alt 或 Command，避免影响正常输入")
	}
	result := make([]string, 0, 5)
	for _, modifier := range []string{"CmdOrCtrl", "Cmd", "Ctrl", "Alt", "Shift"} {
		if modifiers[modifier] {
			result = append(result, modifier)
		}
	}
	return strings.Join(append(result, key), "+"), nil
}

func shortcutKey(value string) (string, bool) {
	if len(value) == 1 && (value[0] >= 'a' && value[0] <= 'z' || value[0] >= 'A' && value[0] <= 'Z' || value[0] >= '0' && value[0] <= '9' || strings.ContainsAny(value, "-=[]\\;',./`")) {
		return strings.ToUpper(value), false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "f") {
		if n, err := strconv.Atoi(lower[1:]); err == nil && n >= 1 && n <= 24 && strconv.Itoa(n) == lower[1:] {
			return fmt.Sprintf("F%d", n), true
		}
	}
	if len(lower) == 4 && strings.HasPrefix(lower, "num") && lower[3] >= '0' && lower[3] <= '9' {
		return "Num" + lower[3:], false
	}
	for _, key := range []string{"Space", "Tab", "Enter", "Backspace", "Delete", "Insert", "Escape", "Up", "Down", "Left", "Right", "Home", "End", "PageUp", "PageDown", "Plus", "CapsLock", "NumLock", "ScrollLock", "NumDec", "NumAdd", "NumSub", "NumMult", "NumDiv"} {
		if strings.EqualFold(value, key) {
			return key, false
		}
	}
	for _, key := range []string{"VolumeUp", "VolumeDown", "VolumeMute", "MediaNextTrack", "MediaPreviousTrack", "MediaStop", "MediaPlayPause", "PrintScreen"} {
		if strings.EqualFold(value, key) {
			return key, true
		}
	}
	switch lower {
	case "return":
		return "Enter", false
	case "esc":
		return "Escape", false
	case "del":
		return "Delete", false
	case "ins":
		return "Insert", false
	}
	return "", false
}
