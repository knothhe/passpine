package app

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/knothhe/passpine/client/internal/clisession"
	"github.com/knothhe/passpine/client/vault"
)

// The CLI endpoint is separate from the browser endpoint. Explicit CLI
// authentication unlocks the desktop's vault; private keys never leave it.
func (s *Service) handleCLI(raw json.RawMessage) json.RawMessage {
	fail := func(code string) json.RawMessage {
		return encode(clisession.Response{Protocol: clisession.Protocol, Status: "error", Code: code})
	}
	var r clisession.Request
	defer func() {
		clear(r.Passphrase)
		clear(r.Contents)
	}()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > clisession.MaxFrame || !utf8.Valid(raw) || decoder.Decode(&r) != nil || decoder.Decode(new(any)) != io.EOF || r.Protocol == 0 {
		return fail("invalid_request")
	}
	if r.Protocol != clisession.Protocol {
		return fail("protocol_mismatch")
	}
	if r.PayloadVersion != clisession.PayloadVersion {
		return fail("cli_protocol_mismatch")
	}
	if !validCLIRequest(r) {
		return fail("invalid_request")
	}
	// Capture admission before waiting for another request. A lock or switch
	// while this request queues must not grant it access to a later session.
	epoch := s.cancellation.Load()
	s.updateRequests.RLock()
	defer s.updateRequests.RUnlock()
	if s.updatePaused.Load() {
		return fail("operation_failed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.preview {
		return fail("app_unavailable")
	}
	current := s.active
	if current == nil {
		return fail("not_configured")
	}
	if epoch != s.cancellation.Load() {
		return fail("cancelled")
	}
	if !sameDirectory(r.Directory, current.Config().Directory) {
		return fail("vault_changed")
	}
	generation, unlocked := current.SessionState()
	session := s.cliSession(epoch, generation)
	if r.Action != "status" && subtle.ConstantTimeCompare([]byte(r.Session), []byte(session)) != 1 {
		return fail("cancelled")
	}
	if epoch != s.cancellation.Load() {
		return fail("cancelled")
	}
	lockedStatus := func() json.RawMessage {
		// A locked challenge authorizes only authentication. It is bound to
		// this exact generation, just like a credential operation's session.
		generation, unlocked := current.SessionState()
		if unlocked || epoch != s.cancellation.Load() || s.cancelVault.Load() != current {
			return fail("cancelled")
		}
		out := clisession.Response{
			Protocol: clisession.Protocol, Status: "error", Code: "locked",
			Session: s.cliSession(epoch, generation), CanUnlock: true,
			DeviceUnlock: s.device != nil && s.device.Available() && s.deviceEnrolled(),
		}
		encoded := encode(out)
		if next, _ := current.SessionState(); next != generation || epoch != s.cancellation.Load() || s.cancelVault.Load() != current {
			clear(encoded)
			return fail("cancelled")
		}
		return encoded
	}
	if r.Action == "status" && !unlocked {
		return lockedStatus()
	}
	if r.Action != "unlock" && !unlocked {
		return fail("locked")
	}
	out := clisession.Response{Protocol: clisession.Protocol, Status: "ok"}
	mutation := false
	var doc vault.Document
	var err error
	switch r.Action {
	case "status":
		out.Session = session
	case "unlock":
		if !unlocked {
			if r.Mode == "device" {
				err = s.unlockDevice(epoch)
				if err != nil && !errors.Is(err, vault.ErrCancelled) {
					err = clisession.ErrAuthentication
				}
			} else {
				err = current.Unlock(string(r.Passphrase))
			}
			// Lock and Close cancel outside s.mu, including a request that
			// races just before Unlock captures its own generation token.
			if epoch != s.cancellation.Load() || s.cancelVault.Load() != current {
				current.Lock()
				err = vault.ErrCancelled
			}
			outcome := "ok"
			if err != nil {
				outcome = "error"
			}
			s.log("unlock", "cli", outcome)
			if err == nil {
				s.cachedAt = time.Time{}
				s.entriesVersion++
				if s.onChange != nil {
					go s.onChange()
				}
			}
		}
		if err == nil {
			generation, err = current.SessionGeneration()
			if err == nil {
				out.Session = s.cliSession(epoch, generation)
			}
		}
	case "list":
		out.Entries, err = current.List()
		if err == nil {
			for _, entry := range out.Entries {
				if !utf8.ValidString(entry.Path) || !utf8.ValidString(entry.Name) || !utf8.ValidString(entry.Folder) {
					err = vault.ErrPath
					break
				}
			}
		}
	case "read":
		doc, err = current.Read(r.Path)
	case "save":
		doc, err = current.Save(r.Path, string(r.Contents), r.Revision)
		mutation = true
	case "trash":
		var record vault.RecoveryRecord
		record, err = current.Trash(r.Path, r.Revision)
		out.RecoveryID = record.ID
		mutation = true
	case "move":
		// The vault also supports moving ciphertext while locked. This bridge
		// deliberately uses the session-gated re-encryption path so a concurrent
		// desktop lock cancels the write, including under unchanged recipients.
		doc, err = current.Read(r.Path)
		if err == nil && doc.Revision != r.Revision {
			err = vault.ErrConflict
		}
		if err == nil {
			_, err = current.SaveRename(r.Path, r.Destination, doc.Raw, r.Revision)
		}
		mutation = true
	}
	if mutation && (err == nil || errors.Is(err, vault.ErrSourceRemains)) {
		s.entriesVersion++
		s.cachedAt = time.Time{}
		s.ensureGitRefreshLocked(true)
		outcome := "ok"
		if err != nil {
			outcome = "error"
		}
		s.log(r.Action, "cli", outcome)
		if s.onChange != nil {
			go s.onChange()
		}
	}
	if err != nil {
		return fail(clisession.ErrorCode(err))
	}
	if r.Action == "read" || r.Action == "save" {
		out.Document = &clisession.Document{Path: doc.Path, Raw: []byte(doc.Raw), Revision: doc.Revision}
		defer clear(out.Document.Raw)
	}
	if r.Action != "status" {
		s.lastActivity.Store(time.Now().UnixNano())
		current.Activity()
	}
	// Vault operations use their own generation checks. Keep serialization in
	// the generation admitted above as well, including status and list responses.
	var encoded json.RawMessage
	err = current.WithSession(generation, func() error {
		if epoch != s.cancellation.Load() || s.cancelVault.Load() != current {
			return vault.ErrCancelled
		}
		encoded = encode(out)
		if epoch != s.cancellation.Load() {
			clear(encoded)
			return vault.ErrCancelled
		}
		return nil
	})
	if err != nil {
		clear(encoded)
		// Passive status may race automatic idle expiry. Return a new locked
		// challenge instead of making the CLI choose an independent session.
		if r.Action == "status" {
			return lockedStatus()
		}
		return fail(clisession.ErrorCode(err))
	}
	return encoded
}

// The random per-service nonce makes an old CLI invocation invalid even if a
// restarted desktop happens to select the same vault and generation numbers.
func (s *Service) cliSession(epoch, generation uint64) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%x:%s:%d:%d", s.cliNonce, s.prefs.Current, epoch, generation)))
	return hex.EncodeToString(digest[:])
}

func validCLIRequest(r clisession.Request) bool {
	if r.Protocol == 0 || r.Directory == "" || !filepath.IsAbs(r.Directory) {
		return false
	}
	switch r.Action {
	case "status":
		if r.Session != "" {
			return false
		}
	case "list", "read", "save", "trash", "move":
		if len(r.Session) != 64 {
			return false
		}
	case "unlock":
		if len(r.Session) != 64 || r.Path != "" || r.Destination != "" || len(r.Contents) != 0 || r.Revision != "" {
			return false
		}
		return r.Mode == "passphrase" || r.Mode == "device" && len(r.Passphrase) == 0
	default:
		return false
	}
	if r.Mode != "" || len(r.Passphrase) != 0 {
		return false
	}
	switch r.Action {
	case "status", "list":
		return r.Path == "" && r.Destination == "" && len(r.Contents) == 0 && r.Revision == ""
	case "read":
		return r.Path != "" && r.Destination == "" && len(r.Contents) == 0 && r.Revision == ""
	case "save":
		return r.Path != "" && r.Destination == "" && len(r.Contents) <= 8<<20
	case "trash":
		return r.Path != "" && r.Destination == "" && len(r.Contents) == 0 && r.Revision != ""
	case "move":
		return r.Path != "" && r.Destination != "" && len(r.Contents) == 0 && r.Revision != ""
	}
	return false
}
