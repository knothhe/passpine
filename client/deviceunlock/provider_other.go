//go:build !windows && !linux

package deviceunlock

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

func configureProvider(p *systemProvider, executable string) {
	if runtime.GOOS == "darwin" {
		p.binary = helperBeside(executable)
	}
}

// helperBeside locates the authentication helper shipped next to the real
// executable. The command line usually starts through a symlink such as
// ~/.local/bin/pass, and os.Executable reports that link on macOS, so the
// link is resolved to find the helper inside the application bundle.
func helperBeside(executable string) string {
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	return filepath.Join(filepath.Dir(executable), "passpine-device-unlock")
}

func configureCommand(_ *exec.Cmd) {}

// RunHelper handles the platform-specific authentication subprocess entry point.
func RunHelper() bool { return false }
