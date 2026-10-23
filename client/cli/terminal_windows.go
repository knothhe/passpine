package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

var attachConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")

// attachParentConsole connects the GUI-subsystem client to the console it was
// started from. Without one, the arguments came from a shortcut or file
// manager, so the desktop client opens instead.
func attachParentConsole() bool {
	const attachParentProcess = ^uintptr(0)
	if r, _, err := attachConsole.Call(attachParentProcess); r == 0 && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return false
	}
	return true
}

// unsupportedConsole reports that the command line is not available yet. The
// GUI-subsystem client has no standard streams, so it writes to the console.
func (c *command) unsupportedConsole() bool {
	if out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		defer out.Close()
		c.stderr = out
	}
	fmt.Fprintln(c.stderr, c.t("Windows 版暂不支持命令行，请在 macOS 或 Linux 上使用 %s。", c.program))
	return true
}

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
