package vault

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureGitProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		// Kill SSH and credential helper descendants as well as Git on timeout.
		kill := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "taskkill.exe"), "/T", "/F", "/PID", strconv.Itoa(command.Process.Pid))
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := kill.Run(); err != nil {
			return command.Process.Kill()
		}
		return nil
	}
}

func renameCheckout(source, destination string) error {
	// Windows cannot rename over an empty directory. Remove only that empty
	// directory; the subsequent non-replacing rename preserves concurrent files.
	directory, err := windowsOpen(destination, windows.GENERIC_READ|windows.DELETE, windows.OPEN_EXISTING, true)
	if err == nil {
		// Delete the verified directory by handle; a concurrent regular file
		// must never be removed by the empty-directory compatibility step.
		disposition := byte(1)
		err = windows.SetFileInformationByHandle(windows.Handle(directory.Fd()), windows.FileDispositionInfo, &disposition, 1)
		directory.Close()
		if err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(source, destination)
}
