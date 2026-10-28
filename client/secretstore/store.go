// Package secretstore keeps Passpine sync secrets in an encrypted local store
// backed by a system master key and a process-local read session. Native
// credential access and value chunking belong entirely to the SDK.
package secretstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"

	"github.com/knothhe/r2connect/keyring"
)

var (
	ErrNotFound       = keyring.ErrNotFound
	ErrDenied         = keyring.ErrAccessDenied
	ErrUnavailable    = keyring.ErrUnavailable
	ErrInvalid        = errors.New("secret_store_invalid")
	ErrSessionChanged = fmt.Errorf("secret_store_session_changed: %w", ErrInvalid)
)

const MaxSecretBytes = keyring.MaxValueBytes

var validChannel = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type Backend = keyring.Backend

type Store struct {
	mu         sync.Mutex
	service    string
	sdk        *keyring.Store
	denied     atomic.Bool
	sessionMu  sync.Mutex
	generation uint64
	loaded     map[string][]byte
	encrypted  *encryptedBackend
}

func New(channel, dataDir string, labels ...string) (*Store, error) {
	if len(labels) > 1 {
		return nil, ErrInvalid
	}
	label := "Passpine"
	if len(labels) == 1 {
		label = labels[0]
	}
	return NewWithEncryptedBackend(channel, dataDir, keyring.Native(label))
}

func NewWithBackend(channel, dataDir string, backend Backend) (*Store, error) {
	if backend == nil {
		return nil, ErrInvalid
	}
	service, err := Namespace(channel, dataDir)
	if err != nil {
		return nil, err
	}
	s := &Store{service: service, sdk: keyring.NewWithBackend(service, backend)}
	s.sdk.OnAccessDenied = func() { s.denied.Store(true) }
	return s, nil
}

// Namespace only resolves application identity; it never opens a keyring.
func Namespace(channel, dataDir string) (string, error) {
	if !validChannel.MatchString(channel) || dataDir == "" {
		return "", ErrInvalid
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return "", ErrInvalid
	}
	// Resolve existing ancestors as well, so first-run aliases stay isolated by
	// the canonical data directory rather than the spelling of its path.
	current, suffix := filepath.Clean(abs), []string{}
	for {
		resolved, e := filepath.EvalSymlinks(current)
		if e == nil {
			abs = filepath.Join(append([]string{resolved}, suffix...)...)
			break
		}
		if !errors.Is(e, os.ErrNotExist) || filepath.Dir(current) == current {
			return "", ErrInvalid
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = filepath.Dir(current)
	}
	digest := sha256.Sum256([]byte(abs))
	return "com.passpine.sync." + channel + "." + hex.EncodeToString(digest[:16]), nil
}

// RetryAccess is only for an explicit connection or disconnect action.
func (s *Store) RetryAccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retryAccessLocked(true)
}

// RetrySession explicitly retries access and reports whether the encrypted
// file changed. Callers can retain pending writes when only authorization was
// denied. A metadata read failure does not reset access or cached reads.
func (s *Store) RetrySession() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	if s.encrypted != nil {
		// Bypass the outer denial latch: this check reads only local metadata,
		// and an unchanged file must remain distinguishable after a refusal.
		err := s.encrypted.CheckSession()
		if err != nil && !errors.Is(err, ErrSessionChanged) {
			return false, err
		}
		changed = errors.Is(err, ErrSessionChanged)
	}
	// An unchanged snapshot still owns pending writes after this call. Keep
	// observing it so a restore before the caller's next read is detected.
	s.retryAccessLocked(changed)
	return changed, nil
}

// retryAccessLocked requires mu and performs no native credential operations.
func (s *Store) retryAccessLocked(resetSnapshot bool) {
	s.ClearSession()
	s.sdk.RetryAccess()
	if s.encrypted != nil {
		s.encrypted.retryAccess(resetSnapshot)
	}
	s.denied.Store(false)
}
func (s *Store) Denied() bool { return s.denied.Load() }

func (s *Store) sessionGeneration() uint64 {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	return s.generation
}

func (s *Store) finishGeneration(generation uint64) {
	if generation != s.sessionGeneration() && s.encrypted != nil {
		s.encrypted.ClearSession()
	}
}

// ClearSession forgets successful reads without reopening denied access. Call
// it when the connection session ends or its saved SDK state changes outside
// this process. Direct native-store edits cannot be detected by this cache;
// restarting or explicitly reconnecting starts a fresh read session.
// Clearing never waits for a native prompt: a read already in progress can
// finish for its caller, but cannot repopulate the cleared session.
func (s *Store) ClearSession() {
	s.sessionMu.Lock()
	s.generation++
	for key := range s.loaded {
		s.forget(key)
	}
	s.sessionMu.Unlock()
	if s.encrypted != nil {
		s.encrypted.ClearSession()
	}
}

// CheckSession inspects only the encrypted file, without opening the native
// credential store. It also protects callers holding a pending failed write.
func (s *Store) CheckSession() error {
	if s.denied.Load() {
		return ErrDenied
	}
	if s.encrypted == nil {
		return nil
	}
	err := s.encrypted.CheckSession()
	if err != nil {
		s.ClearSession()
	}
	return err
}

// forget requires sessionMu. Native calls always take mu before sessionMu;
// clearing the session only takes sessionMu, so it cannot wait on native I/O.
func (s *Store) forget(key string) {
	clear(s.loaded[key])
	delete(s.loaded, key)
}

func (s *Store) check(err error) error {
	if errors.Is(err, ErrDenied) {
		s.denied.Store(true)
		s.ClearSession()
		return ErrDenied
	}
	if errors.Is(err, ErrInvalid) {
		s.ClearSession()
	}
	if errors.Is(err, keyring.ErrStoreMissing) {
		return ErrUnavailable
	}
	return err
}

func (s *Store) Load(key string) ([]byte, error) {
	generation := s.sessionGeneration()
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.finishGeneration(generation)
	if !keyring.ValidKey(key) {
		return nil, ErrInvalid
	}
	if s.denied.Load() {
		return nil, ErrDenied
	}
	if err := s.CheckSession(); err != nil {
		return nil, err
	}
	s.sessionMu.Lock()
	if value, ok := s.loaded[key]; ok {
		value = append([]byte(nil), value...)
		s.sessionMu.Unlock()
		return value, nil
	}
	s.sessionMu.Unlock()
	value, err := s.sdk.Load(key)
	if err = s.check(err); err != nil {
		return nil, err
	}
	s.sessionMu.Lock()
	if s.generation == generation {
		if s.loaded == nil {
			s.loaded = make(map[string][]byte)
		}
		s.loaded[key] = append([]byte(nil), value...)
	}
	s.sessionMu.Unlock()
	return value, nil
}
func (s *Store) Save(key string, value []byte) error {
	generation := s.sessionGeneration()
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.finishGeneration(generation)
	if !keyring.ValidKey(key) || len(value) < 1 || len(value) > MaxSecretBytes {
		return ErrInvalid
	}
	if s.denied.Load() {
		return ErrDenied
	}
	// Always read back from the persistent backend after a write, even if it fails.
	// A supplied value must not stand in for a verified successful read.
	s.sessionMu.Lock()
	s.forget(key)
	s.sessionMu.Unlock()
	return s.check(s.sdk.Save(key, value))
}
func (s *Store) Delete(key string) error {
	generation := s.sessionGeneration()
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.finishGeneration(generation)
	if !keyring.ValidKey(key) {
		return ErrInvalid
	}
	if s.denied.Load() {
		return ErrDenied
	}
	s.sessionMu.Lock()
	s.forget(key)
	s.sessionMu.Unlock()
	return s.check(s.sdk.Delete(key))
}
