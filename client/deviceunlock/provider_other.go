//go:build !windows && !linux

package deviceunlock

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

func configureProvider(p *systemProvider, executable string) {
	if runtime.GOOS == "darwin" {
		p.binary = filepath.Join(filepath.Dir(executable), "passpine-device-unlock")
	}
}

func configureCommand(_ *exec.Cmd) {}

// RunHelper handles the platform-specific authentication subprocess entry point.
func RunHelper() bool { return false }
