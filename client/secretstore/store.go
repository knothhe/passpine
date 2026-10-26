// Package secretstore gives the r2connect keyring a Passpine namespace. Native
// credential access and value chunking belong entirely to the SDK.
package secretstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"

	"github.com/knothhe/r2connect/keyring"
)

var (
	ErrNotFound    = keyring.ErrNotFound
	ErrDenied      = keyring.ErrAccessDenied
	ErrUnavailable = keyring.ErrUnavailable
	ErrInvalid     = errors.New("secret_store_invalid")
)

const MaxSecretBytes = keyring.MaxValueBytes

var validChannel = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type Backend = keyring.Backend

type Store struct {
	mu      sync.Mutex
	service string
	sdk     *keyring.Store
	denied  atomic.Bool
}

func New(channel, dataDir string, labels ...string) (*Store, error) {
	if len(labels) > 1 {
		return nil, ErrInvalid
	}
	label := "Passpine"
	if len(labels) == 1 {
		label = labels[0]
	}
	return NewWithBackend(channel, dataDir, keyring.Native(label))
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
	s.sdk.RetryAccess()
	s.denied.Store(false)
}
func (s *Store) Denied() bool { return s.denied.Load() }

func (s *Store) check(err error) error {
	if errors.Is(err, ErrDenied) {
		s.denied.Store(true)
		return ErrDenied
	}
	if errors.Is(err, keyring.ErrStoreMissing) {
		return ErrUnavailable
	}
	return err
}

func (s *Store) Load(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !keyring.ValidKey(key) {
		return nil, ErrInvalid
	}
	if s.denied.Load() {
		return nil, ErrDenied
	}
	value, err := s.sdk.Load(key)
	return value, s.check(err)
}
func (s *Store) Save(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !keyring.ValidKey(key) || len(value) < 1 || len(value) > MaxSecretBytes {
		return ErrInvalid
	}
	if s.denied.Load() {
		return ErrDenied
	}
	return s.check(s.sdk.Save(key, value))
}
func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !keyring.ValidKey(key) {
		return ErrInvalid
	}
	if s.denied.Load() {
		return ErrDenied
	}
	return s.check(s.sdk.Delete(key))
}
