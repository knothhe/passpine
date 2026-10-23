//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// openTerminal returns the controlling terminal so the private-key passphrase
// is never read from a pipe meant for entry contents.
func openTerminal() (in, out *os.File, err error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	return f, f, err
}

func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), ioctlGetTermios)
	return err == nil
}

// withoutEcho disables echo while keeping line editing and Ctrl+C available.
func withoutEcho(f *os.File) (func(), error) {
	fd := int(f.Fd())
	previous, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return nil, err
	}
	hidden := *previous
	hidden.Lflag &^= unix.ECHO
	hidden.Lflag |= unix.ICANON | unix.ISIG
	hidden.Iflag |= unix.ICRNL
	if err := unix.IoctlSetTermios(fd, ioctlSetTermios, &hidden); err != nil {
		return nil, err
	}
	return func() { _ = unix.IoctlSetTermios(fd, ioctlSetTermios, previous) }, nil
}

// detach lets the clipboard-clearing helper outlive the command's terminal.
func detach(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// editorCommand runs $EDITOR through the shell like pass, so editor arguments
// such as "code --wait" keep working. The file path is passed separately.
func editorCommand(editor, file string) *exec.Cmd {
	return exec.Command("/bin/sh", "-c", editor+` "$1"`, "sh", file)
}

// plaintextDirectory prefers a memory-backed filesystem for decrypted edits.
func plaintextDirectory() (string, bool) {
	if info, err := os.Stat("/dev/shm"); err == nil && info.IsDir() && unix.Access("/dev/shm", unix.W_OK|unix.X_OK) == nil {
		return "/dev/shm", true
	}
	return os.TempDir(), false
}
