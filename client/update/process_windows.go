//go:build windows

package update

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if e != nil {
		return e != windows.ERROR_INVALID_PARAMETER
	}
	defer windows.CloseHandle(handle)
	status, e := windows.WaitForSingleObject(handle, 0)
	return e != nil || status == uint32(windows.WAIT_TIMEOUT)
}
func detachHelper(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200 | 0x08000000, HideWindow: true}
}
