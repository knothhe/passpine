//go:build !windows

package vault

import (
	"os/exec"
	"syscall"
)

func configureGitProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
}

func renameCheckout(source, destination string) error { return syscall.Rename(source, destination) }
