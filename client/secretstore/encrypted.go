package secretstore

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/knothhe/r2connect/keyring"
)

const (
	masterKeyName       = "passpine-master-v1"
	encryptedFileName   = "sync-secrets-v1.enc"
	lockFileName        = "sync-secrets-v1.lock"
	initFileName        = "sync-secrets-v1.init"
	maxEncryptedBytes   = 16 << 20
	maxEncryptedRecords = 4096
	encryptedMagic      = "PPSYNCSECRETS\x01"
)

type encryptedRecord struct {
	Value         []byte `json:"value,omitempty"`
	LegacyPending bool   `json:"legacyPending,omitempty"`
}

type encryptedDocument struct {
	Version int                        `json:"version"`
	Records map[string]encryptedRecord `json:"records"`
}

func (d encryptedDocument) clear() {
	for key, record := range d.Records {
		clear(record.Value)
		delete(d.Records, key)
	}
}

// encryptedBackend keeps only a random master key in the native store. Logical
// keys are opaque: SDK candidates, rotations and cleanup retain their exact
// independent Save/Load/Delete semantics inside the encrypted document.
type encryptedBackend struct {
	service, directory, path string
	native                   *keyring.Store
	cacheMu                  sync.Mutex
	master                   []byte
	generation               uint64
	metaMu                   sync.Mutex
	known, exists, changed   bool
	fingerprint              [sha256.Size]byte
}

// NewWithEncryptedBackend uses native solely for the master key and lazy
// migration of this namespace's old native items. It never enumerates secrets.
func NewWithEncryptedBackend(channel, dataDir string, native Backend) (*Store, error) {
	if native == nil {
		return nil, ErrInvalid
	}
	service, err := Namespace(channel, dataDir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dataDir, 0700); err != nil {
		return nil, ErrUnavailable
	}
	directory, err := filepath.EvalSymlinks(dataDir)
	if err != nil {
		return nil, ErrInvalid
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		return nil, ErrInvalid
	}
	if err = privateDirectory(directory); err != nil {
		return nil, err
	}
	b := &encryptedBackend{service: service, directory: directory, path: filepath.Join(directory, encryptedFileName), native: keyring.NewWithBackend(service, native)}
	s, err := NewWithBackend(channel, directory, b)
	if err != nil {
		return nil, err
	}
	s.encrypted = b
	return s, nil
}

func (b *encryptedBackend) ClearSession() {
	b.cacheMu.Lock()
	b.generation++
	clear(b.master)
	b.master = nil
	b.cacheMu.Unlock()
}

func (b *encryptedBackend) retryAccess(resetSnapshot bool) {
	b.ClearSession()
	b.native.RetryAccess()
	if resetSnapshot {
		b.metaMu.Lock()
		b.known, b.changed = false, false
		b.metaMu.Unlock()
	}
}

func (b *encryptedBackend) header() []byte {
	digest := sha256.Sum256([]byte(b.service))
	return append([]byte(encryptedMagic), digest[:]...)
}

func (b *encryptedBackend) snapshot() ([]byte, bool, error) {
	data, exists, err := readPrivateFile(b.path, maxEncryptedBytes)
	if err != nil {
		return nil, exists, err
	}
	if exists && (len(data) < len(b.header())+12+16 || !bytes.Equal(data[:len(b.header())], b.header())) {
		return nil, true, ErrInvalid
	}
	return data, exists, nil
}

func (b *encryptedBackend) checkSnapshot(data []byte, exists bool, readErr error) error {
	b.metaMu.Lock()
	defer b.metaMu.Unlock()
	if b.changed {
		return ErrSessionChanged
	}
	digest := sha256.Sum256(data)
	if b.known && (readErr != nil || exists != b.exists || digest != b.fingerprint) {
		b.changed = true
		return ErrSessionChanged
	}
	if readErr != nil {
		if errors.Is(readErr, ErrInvalid) {
			b.changed = true
		}
		return readErr
	}
	b.known, b.exists, b.fingerprint = true, exists, digest
	return nil
}

func (b *encryptedBackend) remember(data []byte) {
	b.metaMu.Lock()
	b.known, b.exists, b.fingerprint = true, true, sha256.Sum256(data)
	b.metaMu.Unlock()
}

func (b *encryptedBackend) CheckSession() error {
	unlock, err := acquireFileLock(filepath.Join(b.directory, lockFileName))
	if err != nil {
		return err
	}
	defer unlock()
	data, exists, err := b.snapshot()
	return b.checkSnapshot(data, exists, err)
}

func (b *encryptedBackend) masterKey(force bool, generation uint64) ([]byte, error) {
	b.cacheMu.Lock()
	if !force && generation == b.generation && len(b.master) == 32 {
		key := append([]byte(nil), b.master...)
		b.cacheMu.Unlock()
		return key, nil
	}
	b.cacheMu.Unlock()
	key, err := b.native.Load(masterKeyName)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		clear(key)
		return nil, ErrInvalid
	}
	b.cacheMu.Lock()
	if generation == b.generation {
		clear(b.master)
		b.master = append([]byte(nil), key...)
	}
	b.cacheMu.Unlock()
	return key, nil
}

func (b *encryptedBackend) aead(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalid
	}
	return cipher.NewGCM(block)
}

func (b *encryptedBackend) decode(data, key []byte) (encryptedDocument, error) {
	var document encryptedDocument
	aead, err := b.aead(key)
	if err != nil {
		return document, err
	}
	header := b.header()
	if len(data) < len(header)+aead.NonceSize()+aead.Overhead() || !bytes.Equal(data[:len(header)], header) {
		return document, ErrInvalid
	}
	nonce := data[len(header) : len(header)+aead.NonceSize()]
	plain, err := aead.Open(nil, nonce, data[len(header)+len(nonce):], header)
	if err != nil {
		return document, ErrInvalid
	}
	defer clear(plain)
	decoder := json.NewDecoder(bytes.NewReader(plain))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&document) != nil || decoder.Decode(new(any)) != io.EOF || document.Version != 1 || document.Records == nil || len(document.Records) > maxEncryptedRecords {
		document.clear()
		return encryptedDocument{}, ErrInvalid
	}
	for key, record := range document.Records {
		if !keyring.ValidKey(key) || key == masterKeyName || len(record.Value) > MaxSecretBytes {
			document.clear()
			return encryptedDocument{}, ErrInvalid
		}
	}
	return document, nil
}

func (b *encryptedBackend) persist(document encryptedDocument, key []byte) error {
	if len(document.Records) > maxEncryptedRecords {
		return ErrInvalid
	}
	plain, err := json.Marshal(document)
	if err != nil {
		return ErrInvalid
	}
	defer clear(plain)
	aead, err := b.aead(key)
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return ErrUnavailable
	}
	header := b.header()
	data := append(append([]byte(nil), header...), nonce...)
	data = aead.Seal(data, nonce, plain, header)
	if len(data) > maxEncryptedBytes {
		return ErrInvalid
	}
	// Native authorization can pause an operation long enough for a backup
	// restore or another non-locking writer to replace the file. Recheck the
	// exact known snapshot immediately before replacing it.
	current, exists, readErr := b.snapshot()
	if err = b.checkSnapshot(current, exists, readErr); err != nil {
		return err
	}
	if err = atomicPrivateFile(b.path, data); err != nil {
		return err
	}
	b.remember(data)
	// Verify the actual persisted ciphertext. Supplying Save's value never
	// seeds either the logical read cache or the native master cache.
	read, exists, err := b.snapshot()
	if err != nil || !exists || !bytes.Equal(read, data) {
		return ErrInvalid
	}
	verified, err := b.decode(read, key)
	verified.clear()
	return err
}

func (b *encryptedBackend) initializeMaster(document encryptedDocument, generation uint64) ([]byte, error) {
	// A legacy read can wait on native authorization after open found no
	// master or file. Recheck before creating either initialization resource.
	current, exists, err := b.snapshot()
	if err = b.checkSnapshot(current, exists, err); err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrInvalid
	}
	marker := filepath.Join(b.directory, initFileName)
	if err := atomicPrivateFile(marker, b.header()); err != nil {
		return nil, err
	}
	generated := make([]byte, 32)
	defer clear(generated)
	if _, err := rand.Read(generated); err != nil {
		return nil, ErrUnavailable
	}
	if err := b.native.Save(masterKeyName, generated); err != nil {
		return nil, err
	}
	actual, err := b.masterKey(true, generation)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(actual, generated) {
		clear(actual)
		return nil, ErrInvalid
	}
	// Commit an empty database and remove the initialization marker before
	// committing any logical secret. A leftover marker can only recover this
	// empty first initialization, never a previously populated lost database.
	if err = b.persist(document, actual); err != nil {
		clear(actual)
		return nil, err
	}
	if err = os.Remove(marker); err != nil {
		clear(actual)
		return nil, ErrUnavailable
	}
	return actual, nil
}

func (b *encryptedBackend) open(data []byte, exists bool, generation uint64) (encryptedDocument, []byte, error) {
	empty := encryptedDocument{Version: 1, Records: make(map[string]encryptedRecord)}
	key, err := b.masterKey(false, generation)
	if errors.Is(err, ErrNotFound) {
		if exists {
			return empty, nil, ErrInvalid
		}
		marker, present, markerErr := readPrivateFile(filepath.Join(b.directory, initFileName), 1024)
		if markerErr != nil {
			return empty, nil, markerErr
		}
		if present && !bytes.Equal(marker, b.header()) {
			return empty, nil, ErrInvalid
		}
		return empty, nil, nil
	}
	if err != nil {
		return empty, nil, err
	}
	if exists {
		doc, err := b.decode(data, key)
		if err != nil {
			clear(key)
			return empty, nil, err
		}
		// A marker is only written before the first empty document.
		if marker, present, markerErr := readPrivateFile(filepath.Join(b.directory, initFileName), 1024); markerErr != nil {
			doc.clear()
			clear(key)
			return empty, nil, markerErr
		} else if present {
			if !bytes.Equal(marker, b.header()) || len(doc.Records) != 0 {
				doc.clear()
				clear(key)
				return empty, nil, ErrInvalid
			}
			if err := os.Remove(filepath.Join(b.directory, initFileName)); err != nil {
				doc.clear()
				clear(key)
				return empty, nil, ErrUnavailable
			}
		}
		return doc, key, nil
	}
	marker, present, err := readPrivateFile(filepath.Join(b.directory, initFileName), 1024)
	if err != nil || !present || !bytes.Equal(marker, b.header()) {
		clear(key)
		return empty, nil, ErrInvalid
	}
	if err = b.persist(empty, key); err != nil {
		clear(key)
		return empty, nil, err
	}
	if err = os.Remove(filepath.Join(b.directory, initFileName)); err != nil {
		clear(key)
		return empty, nil, ErrUnavailable
	}
	return empty, key, nil
}

func (b *encryptedBackend) finishLegacy(document encryptedDocument, key string, master []byte, generation uint64) error {
	record := document.Records[key]
	if !record.LegacyPending {
		return nil
	}
	// Migration deletion requires an actual native master reread and actual
	// disk decrypt, independently of both in-process caches.
	actual, err := b.masterKey(true, generation)
	if err != nil {
		return err
	}
	defer clear(actual)
	if !bytes.Equal(actual, master) {
		return ErrInvalid
	}
	data, exists, err := b.snapshot()
	if err = b.checkSnapshot(data, exists, err); err != nil {
		return err
	}
	if !exists {
		return ErrInvalid
	}
	verified, err := b.decode(data, actual)
	if err != nil {
		return err
	}
	value, ok := verified.Records[key]
	valid := ok && value.LegacyPending && bytes.Equal(value.Value, record.Value)
	verified.clear()
	if !valid {
		return ErrInvalid
	}
	if err = b.native.Delete(key); err != nil {
		return err
	}
	record.LegacyPending = false
	if len(record.Value) == 0 {
		delete(document.Records, key)
	} else {
		document.Records[key] = record
	}
	return b.persist(document, master)
}

func (b *encryptedBackend) operation(service, key string, value []byte, action string) (result []byte, resultErr error) {
	if service != b.service || !keyring.ValidKey(key) || key == masterKeyName {
		return nil, ErrInvalid
	}
	defer func() {
		// Corrupt authenticated data or an unusable master cannot become
		// valid by repeatedly reopening a native authorization prompt.
		if errors.Is(resultErr, ErrInvalid) {
			b.metaMu.Lock()
			b.changed = true
			b.metaMu.Unlock()
		}
	}()
	b.cacheMu.Lock()
	generation := b.generation
	b.cacheMu.Unlock()
	unlock, err := acquireFileLock(filepath.Join(b.directory, lockFileName))
	if err != nil {
		return nil, err
	}
	defer unlock()
	data, exists, err := b.snapshot()
	if err = b.checkSnapshot(data, exists, err); err != nil {
		return nil, err
	}
	document, master, err := b.open(data, exists, generation)
	if err != nil {
		return nil, err
	}
	defer document.clear()
	defer func() { clear(master) }()
	// Opening can wait on native authorization. A backup restore during that
	// prompt must invalidate the decoded snapshot before it is used or exposed.
	current, present, readErr := b.snapshot()
	if err = b.checkSnapshot(current, present, readErr); err != nil {
		return nil, err
	}
	record, known := document.Records[key]
	switch action {
	case "load":
		if !known {
			legacy, err := b.native.Load(key)
			if err != nil {
				return nil, err
			}
			if master == nil {
				master, err = b.initializeMaster(document, generation)
				if err != nil {
					clear(legacy)
					return nil, err
				}
			}
			record = encryptedRecord{Value: legacy, LegacyPending: true}
			document.Records[key] = record
			if err = b.persist(document, master); err != nil {
				return nil, err
			}
		}
	case "save", "delete":
		if !known {
			// A missing legacy item proves this is a fresh logical key. Only
			// actual legacy values require migration readback and cleanup;
			// new SDK rotations must not repeatedly unlock the master item.
			legacy, legacyErr := b.native.Load(key)
			clear(legacy)
			if legacyErr != nil && !errors.Is(legacyErr, ErrNotFound) {
				return nil, legacyErr
			}
			record.LegacyPending = legacyErr == nil
			if action == "delete" && !record.LegacyPending {
				return nil, nil
			}
		}
		if master == nil {
			master, err = b.initializeMaster(document, generation)
			if err != nil {
				return nil, err
			}
		}
		clear(record.Value)
		record.Value = nil
		if action == "save" {
			record.Value = append([]byte(nil), value...)
		}
		if action == "delete" && !record.LegacyPending {
			delete(document.Records, key)
		} else {
			document.Records[key] = record
		}
		if err = b.persist(document, master); err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalid
	}
	if err = b.finishLegacy(document, key, master, generation); err != nil {
		return nil, err
	}
	if action == "load" {
		if len(record.Value) == 0 {
			return nil, ErrNotFound
		}
		return append([]byte(nil), record.Value...), nil
	}
	return nil, nil
}

func (b *encryptedBackend) Load(service, key string) ([]byte, error) {
	return b.operation(service, key, nil, "load")
}
func (b *encryptedBackend) Save(service, key string, value []byte) error {
	_, err := b.operation(service, key, value, "save")
	return err
}
func (b *encryptedBackend) Delete(service, key string) error {
	_, err := b.operation(service, key, nil, "delete")
	return err
}
