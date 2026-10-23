package cli

import (
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func openTerminal() (in, out *os.File, err error) {
	in, err = os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	out, err = os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		in.Close()
		return nil, nil, err
	}
	return in, out, nil
}

func isTerminal(f *os.File) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil
}

func withoutEcho(f *os.File) (func(), error) {
	handle := windows.Handle(f.Fd())
	var previous uint32
	if err := windows.GetConsoleMode(handle, &previous); err != nil {
		return nil, err
	}
	hidden := previous&^windows.ENABLE_ECHO_INPUT | windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT
	if err := windows.SetConsoleMode(handle, hidden); err != nil {
		return nil, err
	}
	return func() { _ = windows.SetConsoleMode(handle, previous) }, nil
}

func detach(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
}

func editorCommand(editor, file string) *exec.Cmd {
	fields := strings.Fields(editor)
	return exec.Command(fields[0], append(fields[1:], file)...)
}

func plaintextDirectory() (string, bool) { return os.TempDir(), false }
