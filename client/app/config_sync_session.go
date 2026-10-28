package app

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/knothhe/r2connect"
)

// A healthy Manager retains refresh rotations which have not yet been saved.
// Do not replace it when another process changes its persisted state: stop this
// session instead, so it cannot serve cached secrets or overwrite newer state.
var errSyncSessionChanged = fmt.Errorf("saved R2 session changed; restart before continuing: %w", r2connect.ErrInvalidState)

type syncSessionState struct {
	mu           sync.Mutex
	file         r2connect.FileStateStore
	clearSession func()
	loaded       bool
	exists       bool
	fingerprint  [sha256.Size]byte
	changed      bool
	clearPending bool
}

func newSyncSessionState(path string, clearSession func()) *syncSessionState {
	return &syncSessionState{file: r2connect.FileStateStore{Path: path}, clearSession: clearSession}
}

// finish releases this state-store lock before clearing the secret store. The
// callback must not call back into Manager, which may still hold its own lock.
func (s *syncSessionState) finish() {
	clearSession := s.clearPending
	s.clearPending = false
	s.mu.Unlock()
	if clearSession && s.clearSession != nil {
		s.clearSession()
	}
}

// syncStateSnapshot lets the SDK validate the exact snapshot read from disk,
// without reading the file twice and accidentally accepting a different copy.
type syncStateSnapshot struct {
	state  r2connect.State
	exists bool
}

func (s syncStateSnapshot) Load() (r2connect.State, bool, error) {
	return s.state, s.exists, nil
}

func (syncStateSnapshot) Save(r2connect.State) error { return r2connect.ErrInvalidState }

func syncStateFingerprint(state r2connect.State, exists bool) (r2connect.State, [sha256.Size]byte, error) {
	state, err := r2connect.LoadState(syncStateSnapshot{state: state, exists: exists})
	if err != nil {
		return state, [sha256.Size]byte{}, err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return state, [sha256.Size]byte{}, err
	}
	return state, sha256.Sum256(data), nil
}

func (s *syncSessionState) read() (r2connect.State, bool, [sha256.Size]byte, error) {
	state, exists, err := s.file.Load()
	if err != nil {
		return state, exists, [sha256.Size]byte{}, err
	}
	state, fingerprint, err := syncStateFingerprint(state, exists)
	return state, exists, fingerprint, err
}

func (s *syncSessionState) checkLocked() (r2connect.State, bool, error) {
	if s.changed {
		return r2connect.State{}, false, errSyncSessionChanged
	}
	state, exists, fingerprint, err := s.read()
	if !s.loaded {
		// Keep initial read/validation failures intact for Manager.LoadError and
		// its existing explicit reload path. There is no loaded session to clear.
		if err == nil {
			s.loaded, s.exists, s.fingerprint = true, exists, fingerprint
		}
		return state, exists, err
	}
	if err != nil || exists != s.exists || fingerprint != s.fingerprint {
		s.changed, s.clearPending = true, true
		return r2connect.State{}, false, errSyncSessionChanged
	}
	return state, exists, nil
}

func (s *syncSessionState) Load() (r2connect.State, bool, error) {
	s.mu.Lock()
	defer s.finish()
	return s.checkLocked()
}

// Check reads public SDK metadata only. It neither opens the keyring nor
// resets its refusal latch. Once changed, this session stays stopped.
func (s *syncSessionState) Check() error {
	s.mu.Lock()
	defer s.finish()
	_, _, err := s.checkLocked()
	return err
}

func (s *syncSessionState) Save(state r2connect.State) error {
	s.mu.Lock()
	defer s.finish()
	if _, _, err := s.checkLocked(); err != nil {
		return err
	}
	_, fingerprint, err := syncStateFingerprint(state, true)
	if err != nil {
		return err
	}
	if err = s.file.Save(state); err != nil {
		return err
	}
	// Trust only the state supplied by this Manager. A post-write reread could
	// silently adopt a concurrent process's state as if we had written it.
	s.exists, s.fingerprint = true, fingerprint
	return nil
}
