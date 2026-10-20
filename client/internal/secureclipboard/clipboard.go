// Package secureclipboard writes text with platform clipboard privacy hints.
// Hints are advisory: receiving applications and clipboard managers must honor
// them. Callers should retain their clipboard expiration and ownership checks.
package secureclipboard

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

// WriteText replaces the clipboard with text. Sensitive writes offer the text
// and its privacy hints together, without first publishing unmarked text.
// Like the MyGo clipboard, it requires a running MyGo application.
func WriteText(value string, sensitive bool) error {
	if sensitive {
		return writeSensitiveText(value)
	}
	// WriteText discards native errors; Write lets the caller report failure.
	return mygo.Clipboard.Write(transfer.TextData(value))
}
