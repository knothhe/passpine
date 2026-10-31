// Package clisession defines the authenticated local CLI bridge. Private keys
// stay in the desktop process. A terminal-entered passphrase may be sent only
// in an explicit unlock request; it is never returned or persisted.
package clisession

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/knothhe/passpine/client/vault"
)

const Protocol = 1

// PayloadVersion distinguishes the byte-preserving format from older CLI
// payloads that transported entry contents and passphrases as JSON text.
const PayloadVersion = 1

// Complete CLI entries use a bounded private transport, independently of the
// browser/native-messaging frame limit.
const MaxFrame = 64 << 20

func Directory(dataDir string) string { return filepath.Join(dataDir, "cli") }

type Request struct {
	Protocol       int    `json:"protocol"`
	PayloadVersion int    `json:"payloadVersion"`
	Action         string `json:"action"`
	Directory      string `json:"directory"`
	Session        string `json:"session,omitempty"`
	Path           string `json:"path,omitempty"`
	Destination    string `json:"destination,omitempty"`
	Contents       []byte `json:"contents,omitempty"`
	Revision       string `json:"revision,omitempty"`
	Mode           string `json:"mode,omitempty"`
	Passphrase     []byte `json:"passphrase,omitempty"`
}

// Document sends the original text once; the CLI parses fields locally.
type Document struct {
	Path     string `json:"path"`
	Raw      []byte `json:"raw"`
	Revision string `json:"revision"`
}

type Response struct {
	Protocol     int           `json:"protocol"`
	Status       string        `json:"status"`
	Code         string        `json:"code,omitempty"`
	Session      string        `json:"session,omitempty"`
	Document     *Document     `json:"document,omitempty"`
	Entries      []vault.Entry `json:"entries,omitempty"`
	RecoveryID   string        `json:"recoveryId,omitempty"`
	CanUnlock    bool          `json:"canUnlock,omitempty"`
	DeviceUnlock bool          `json:"deviceUnlock,omitempty"`
}

var (
	ErrUnavailable    = errors.New("desktop session is unavailable")
	ErrOperation      = errors.New("desktop vault operation failed")
	ErrAuthentication = errors.New("desktop authentication did not complete")
)

// ErrorCode preserves errors that command logic uses, without sending private
// paths or arbitrary filesystem diagnostics to the other process.
func ErrorCode(err error) string {
	for _, item := range errorCodes {
		if errors.Is(err, item.err) {
			return item.code
		}
	}
	return "operation_failed"
}

func Error(code string) error {
	for _, item := range errorCodes {
		if item.code == code {
			return item.err
		}
	}
	return ErrOperation
}

var errorCodes = []struct {
	code string
	err  error
}{
	{"cancelled", vault.ErrCancelled},
	{"locked", vault.ErrLocked},
	{"not_configured", vault.ErrNotConfigured},
	{"invalid_passphrase", vault.ErrPassphrase},
	{"authentication_failed", ErrAuthentication},
	{"vault_busy", vault.ErrBusy},
	{"revision_conflict", vault.ErrConflict},
	{"already_exists", vault.ErrExists},
	{"source_remains", vault.ErrSourceRemains},
	// A missing parent can wrap both ErrPath and fs.ErrNotExist. Commands use
	// the latter to create new nested entries, so preserve that classification.
	{"not_found", fs.ErrNotExist},
	{"invalid_path", vault.ErrPath},
	{"permission_denied", fs.ErrPermission},
	{"app_unavailable", ErrUnavailable},
}
