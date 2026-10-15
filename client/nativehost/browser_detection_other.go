//go:build !darwin

package nativehost

import (
	"os/exec"
	"runtime"
)

func browserInstalled(target registrationTarget) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	commands := map[string][]string{
		"chrome":   {"google-chrome", "google-chrome-stable"},
		"chromium": {"chromium", "chromium-browser"},
		"edge":     {"microsoft-edge", "microsoft-edge-stable"},
		"brave":    {"brave", "brave-browser"},
		"firefox":  {"firefox"},
	}
	for _, command := range commands[target.id] {
		if _, err := exec.LookPath(command); err == nil {
			return true
		}
	}
	return false
}
