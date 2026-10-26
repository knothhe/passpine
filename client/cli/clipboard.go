package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// clipboardHelperArgument starts the detached process that clears a copied
// secret. The digest arrives on stdin so it never appears in process arguments.
const clipboardHelperArgument = "--passpine-clipboard-clear"

var errNoClipboard = errors.New("no clipboard tool")

type clipboardTool struct {
	copy, paste, clear []string
}

type clipboardClear struct {
	Seconds   int    `json:"seconds"`
	Digest    []byte `json:"digest"`
	Selection string `json:"selection"`
}

// clipboardFor follows pass: Wayland first, then X11 tools, honouring
// PASSWORD_STORE_X_SELECTION for the X11 and Wayland primary selection.
func clipboardFor(selection string, lookPath func(string) (string, error), getenv func(string) string) (clipboardTool, error) {
	has := func(name string) bool { _, err := lookPath(name); return err == nil }
	switch runtime.GOOS {
	case "darwin":
		return clipboardTool{copy: []string{"pbcopy"}, paste: []string{"pbpaste"}, clear: []string{"pbcopy"}}, nil
	case "windows":
		shell := []string{"powershell", "-NoProfile", "-NonInteractive", "-Command"}
		return clipboardTool{
			copy:  append(shell, "[Console]::InputEncoding=[Text.Encoding]::UTF8; Set-Clipboard -Value ([Console]::In.ReadToEnd())"),
			paste: append(shell, "[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-Clipboard -Raw"),
			clear: append(shell, "Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Clipboard]::Clear()"),
		}, nil
	}
	if selection == "" {
		selection = "clipboard"
	}
	if getenv("WAYLAND_DISPLAY") != "" && has("wl-copy") {
		flags := []string{}
		if selection == "primary" {
			flags = append(flags, "--primary")
		}
		return clipboardTool{
			copy:  append([]string{"wl-copy"}, flags...),
			paste: append([]string{"wl-paste", "--no-newline"}, flags...),
			clear: append([]string{"wl-copy", "--clear"}, flags...),
		}, nil
	}
	if getenv("DISPLAY") != "" && has("xclip") {
		return clipboardTool{
			copy:  []string{"xclip", "-selection", selection},
			paste: []string{"xclip", "-o", "-selection", selection},
			clear: []string{"xclip", "-selection", selection},
		}, nil
	}
	if getenv("DISPLAY") != "" && has("xsel") {
		return clipboardTool{
			copy:  []string{"xsel", "--" + selection, "--input"},
			paste: []string{"xsel", "--" + selection, "--output"},
			clear: []string{"xsel", "--" + selection, "--clear"},
		}, nil
	}
	return clipboardTool{}, errNoClipboard
}

// runTool captures output only when reading. Clipboard owners such as xclip
// and wl-copy keep serving the selection in the background, so writes must not
// leave them holding a pipe this process waits on.
func runTool(ctx context.Context, args []string, input []byte, capture bool) ([]byte, error) {
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.Stdin = bytes.NewReader(input)
	var output bytes.Buffer
	if capture {
		command.Stdout = &output
	}
	command.WaitDelay = time.Second
	err := command.Run()
	return output.Bytes(), err
}

// copySecret writes text and schedules a detached helper that clears it after
// seconds, unless the user has copied something else in the meantime.
func copySecret(text []byte, seconds int, selection string) error {
	tool, err := clipboardFor(selection, exec.LookPath, os.Getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err = runTool(ctx, tool.copy, text, false); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	digest := sha256.Sum256(text)
	request, err := json.Marshal(clipboardClear{Seconds: seconds, Digest: digest[:], Selection: selection})
	if err != nil {
		return err
	}
	helper := exec.Command(executable, clipboardHelperArgument)
	helper.Stdin = bytes.NewReader(request)
	detach(helper)
	if err = helper.Start(); err != nil {
		return err
	}
	return helper.Process.Release()
}

// RunHelper handles the detached clipboard-clearing process. It reports false
// for every other invocation so the caller continues normally.
func RunHelper() bool {
	if len(os.Args) != 2 || os.Args[1] != clipboardHelperArgument {
		return false
	}
	var request clipboardClear
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 4096))
	if err != nil || json.Unmarshal(data, &request) != nil || request.Seconds < 1 || request.Seconds > 24*60*60 || len(request.Digest) != sha256.Size {
		return true
	}
	tool, err := clipboardFor(request.Selection, exec.LookPath, os.Getenv)
	if err != nil {
		return true
	}
	time.Sleep(time.Duration(request.Seconds) * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	current, err := runTool(ctx, tool.paste, nil, true)
	digest := sha256.Sum256(current)
	clear(current)
	// Clear when the clipboard cannot be read back: a stale secret is worse
	// than discarding something else that cannot be verified.
	if err != nil || subtle.ConstantTimeCompare(digest[:], request.Digest) == 1 {
		_, _ = runTool(ctx, tool.clear, nil, false)
	}
	return true
}
