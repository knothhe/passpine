//go:build !windows

package update

import (
	"os/exec"
	"syscall"
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	e := syscall.Kill(pid, 0)
	return e == nil || e == syscall.EPERM
}
func detachHelper(command *exec.Cmd) { command.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
