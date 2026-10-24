package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ProtonMail/go-crypto/openpgp"
)

type Vault struct {
	config          Config
	operations      sync.Mutex
	gitOperations   sync.Mutex
	gitLifecycle    sync.Mutex
	gitClosed       bool
	gitNext         uint64
	gitContexts     map[uint64]context.CancelFunc
	session         sync.Mutex
	keys            openpgp.EntityList
	generation      uint64
	opened, touched time.Time
	timer           *time.Timer
	clock           func() time.Time
}

func New(config Config) (*Vault, error) {
	if config.Directory == "" {
		return nil, ErrPath
	}
	root, e := privateDirectory(config.Directory)
	if e != nil {
		return nil, e
	}
	config.Directory = root
	if config.KeyFile == "" {
		digest := sha256.Sum256([]byte(root))
		config.KeyFile = filepath.Join(filepath.Dir(root), ".passpine-keys", hex.EncodeToString(digest[:16]), "secret.asc")
	}
	keyDir, e := privateDirectory(filepath.Dir(config.KeyFile))
	if e != nil {
		return nil, e
	}
	config.KeyFile = filepath.Join(keyDir, filepath.Base(config.KeyFile))
	if config.RecoveryDirectory == "" {
		config.RecoveryDirectory = filepath.Join(keyDir, "recovery")
	}
	config.RecoveryDirectory, e = privateDirectory(config.RecoveryDirectory)
	if e != nil {
		return nil, e
	}
	for _, privatePath := range []string{config.KeyFile, config.RecoveryDirectory} {
		relative, e := filepath.Rel(root, privatePath)
		if e != nil {
			return nil, e
		}
		if relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return nil, errors.New("private keys and recovery records must be outside the synchronized password store")
		}
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = time.Minute
	}
	if config.MaximumSession <= 0 {
		config.MaximumSession = time.Hour
	}
	return &Vault{config: config, clock: time.Now}, nil
}

func (v *Vault) Config() Config { v.session.Lock(); defer v.session.Unlock(); return v.config }

func (v *Vault) SetTimeouts(idle, maximum time.Duration) {
	v.session.Lock()
	defer v.session.Unlock()
	v.config.IdleTimeout = max(time.Second, idle)
	v.config.MaximumSession = max(time.Second, maximum)
	if v.checkSession() {
		v.scheduleExpiry()
	}
}

func (v *Vault) lockSession() {
	v.keys = nil
	v.generation++
	if v.timer != nil {
		v.timer.Stop()
		v.timer = nil
	}
}

func (v *Vault) Lock() { v.session.Lock(); defer v.session.Unlock(); v.lockSession() }

// Close ends this checkout's lifecycle. Lock only clears decrypted keys;
// encrypted Git work stays available until the vault is switched or closed.
func (v *Vault) Close() {
	v.gitLifecycle.Lock()
	v.gitClosed = true
	for _, cancel := range v.gitContexts {
		cancel()
	}
	v.gitLifecycle.Unlock()
	v.Lock()
}

func (v *Vault) beginGit(parent context.Context) (context.Context, func(), error) {
	v.gitLifecycle.Lock()
	defer v.gitLifecycle.Unlock()
	if v.gitClosed {
		return nil, nil, ErrCancelled
	}
	ctx, cancel := context.WithCancel(parent)
	if v.gitContexts == nil {
		v.gitContexts = make(map[uint64]context.CancelFunc)
	}
	v.gitNext++
	id := v.gitNext
	v.gitContexts[id] = cancel
	return ctx, func() {
		cancel()
		v.gitLifecycle.Lock()
		delete(v.gitContexts, id)
		v.gitLifecycle.Unlock()
	}, nil
}

func (v *Vault) checkSession() bool {
	if len(v.keys) == 0 {
		return false
	}
	now := v.clock()
	if now.Sub(v.touched) >= v.config.IdleTimeout || now.Sub(v.opened) >= v.config.MaximumSession {
		v.lockSession()
		return false
	}
	return true
}

func (v *Vault) scheduleExpiry() {
	if v.timer != nil {
		v.timer.Stop()
	}
	remaining := min(v.config.IdleTimeout-v.clock().Sub(v.touched), v.config.MaximumSession-v.clock().Sub(v.opened))
	generation := v.generation
	v.timer = time.AfterFunc(remaining, func() {
		v.session.Lock()
		defer v.session.Unlock()
		if generation == v.generation {
			v.checkSession()
		}
	})
}

func (v *Vault) Status() Status {
	v.session.Lock()
	defer v.session.Unlock()
	unlocked := v.checkSession()
	remaining := 0
	if unlocked {
		remaining = int(min(v.config.IdleTimeout-v.clock().Sub(v.touched), v.config.MaximumSession-v.clock().Sub(v.opened)).Seconds())
	}
	_, e := safeStat(filepath.Dir(v.config.KeyFile), filepath.Base(v.config.KeyFile))
	return Status{Configured: e == nil, Unlocked: unlocked, Directory: v.config.Directory, RemainingSeconds: remaining}
}

func (v *Vault) Activity() {
	v.session.Lock()
	defer v.session.Unlock()
	if v.checkSession() {
		v.touched = v.clock()
		v.scheduleExpiry()
	}
}

func (v *Vault) token() (openpgp.EntityList, uint64, error) {
	v.session.Lock()
	defer v.session.Unlock()
	if !v.checkSession() {
		return nil, 0, ErrLocked
	}
	return v.keys, v.generation, nil
}

// SessionGeneration captures a capability for the current unlock generation.
// Pair it with WithSession for short native side effects such as clipboard writes.
func (v *Vault) SessionGeneration() (uint64, error) {
	_, generation, e := v.token()
	return generation, e
}

// WithSession authorizes a short side effect against the exact session that
// read its secret. The callback must not call another method on this Vault.
func (v *Vault) WithSession(generation uint64, action func() error) error {
	return v.commit(generation, action)
}

func (v *Vault) generationToken() uint64 {
	v.session.Lock()
	defer v.session.Unlock()
	return v.generation
}

func (v *Vault) validate(token uint64) error {
	v.session.Lock()
	defer v.session.Unlock()
	if !v.checkSession() || token != v.generation {
		return ErrCancelled
	}
	return nil
}

// commit holds only the session mutex, so Lock cancels expensive crypto work but
// cannot race with the short, already-encrypted filesystem commit.
func (v *Vault) commit(token uint64, action func() error) error {
	v.session.Lock()
	defer v.session.Unlock()
	if !v.checkSession() || token != v.generation {
		return ErrCancelled
	}
	return action()
}

func (v *Vault) install(keys openpgp.EntityList, expected uint64, write func() error) error {
	v.session.Lock()
	defer v.session.Unlock()
	if v.generation != expected {
		return ErrCancelled
	}
	if write != nil {
		if e := write(); e != nil {
			return e
		}
	}
	v.lockSession()
	v.keys = keys
	v.opened = v.clock()
	v.touched = v.opened
	v.scheduleExpiry()
	return nil
}

func (v *Vault) Create(name, email, passphrase string) (Identity, error) {
	token := v.generationToken()
	v.operations.Lock()
	defer v.operations.Unlock()
	unlock, e := lockStore(v.config.Directory)
	if e != nil {
		return Identity{}, e
	}
	defer unlock()
	if utf8.RuneCountInString(passphrase) < 12 {
		return Identity{}, errors.New("use a passphrase of at least 12 characters")
	}
	if _, e = safeRead(filepath.Dir(v.config.KeyFile), filepath.Base(v.config.KeyFile)); e == nil {
		return Identity{}, ErrExists
	} else if !errors.Is(e, os.ErrNotExist) {
		return Identity{}, e
	}
	existingStore := errors.New("existing password store: import its private key")
	if _, e = safeRead(v.config.Directory, ".gpg-id"); e == nil {
		return Identity{}, existingStore
	} else if !errors.Is(e, os.ErrNotExist) {
		return Identity{}, e
	}
	// A missing root policy does not make an existing store new. Do not install
	// an unrelated identity over its ciphertext or nested recipient policies.
	if e = filepath.WalkDir(v.config.Directory, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == v.config.Directory {
			return nil
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == ".gpg-id" || strings.HasSuffix(entry.Name(), ".gpg") {
			return existingStore
		}
		return nil
	}); e != nil {
		return Identity{}, e
	}
	if name == "" {
		name = "Passpine"
	}
	entity, e := openpgp.NewEntity(name, "", email, cryptoConfig())
	if e != nil {
		return Identity{}, e
	}
	keys := openpgp.EntityList{entity}
	encrypted, e := protectedKeyData(keys, passphrase)
	if e != nil {
		return Identity{}, e
	}
	if e = decryptKeys(keys, passphrase, true); e != nil {
		return Identity{}, e
	}
	id := identities(keys)[0]
	e = v.install(keys, token, func() error {
		if e := safeWrite(filepath.Dir(v.config.KeyFile), filepath.Base(v.config.KeyFile), encrypted, false); e != nil {
			return e
		}
		if e := safeWrite(v.config.Directory, ".gpg-id", []byte(id.Fingerprint+"\n"), false); e != nil {
			_ = safeRemove(filepath.Dir(v.config.KeyFile), filepath.Base(v.config.KeyFile))
			return e
		}
		return nil
	})
	return id, e
}

func (v *Vault) Unlock(passphrase string) error {
	return v.unlock(passphrase, v.generationToken())
}

// UnlockWithProvider binds system authentication to the generation before its prompt.
func (v *Vault) UnlockWithProvider(load func() ([]byte, error)) error {
	token := v.generationToken()
	secret, err := load()
	defer wipe(secret)
	if err != nil {
		return err
	}
	return v.unlock(string(secret), token)
}
func (v *Vault) unlock(passphrase string, token uint64) error {
	v.operations.Lock()
	defer v.operations.Unlock()
	data, e := safeRead(filepath.Dir(v.config.KeyFile), filepath.Base(v.config.KeyFile))
	if errors.Is(e, os.ErrNotExist) {
		return ErrNotConfigured
	}
	if e != nil {
		return e
	}
	keys, e := parseKeys(data)
	if e != nil {
		return e
	}
	if e = decryptKeys(keys, passphrase, true); e != nil {
		return e
	}
	return v.install(keys, token, nil)
}

// ImportKeys accepts binary or ASCII armored OpenPGP exports, including legacy
// combined public/private exports. Imported private keys are protected on disk.
func (v *Vault) ImportKeys(data []byte, passphrase string) ([]Identity, error) {
	return v.importKeys(data, passphrase, v.generationToken())
}

func (v *Vault) importKeys(data []byte, passphrase string, token uint64) ([]Identity, error) {
	keys, e := parseKeys(data)
	if e != nil {
		return nil, e
	}
	return v.importKeyRing(keys, passphrase, token)
}

// ImportKeyFiles reads separately exported OpenPGP keys without using GnuPG.
// The private export may already include every required public key.
func (v *Vault) ImportKeyFiles(privateData, publicData []byte, passphrase string) ([]Identity, error) {
	token := v.generationToken()
	keys, e := parseKeys(privateData)
	if e != nil {
		return nil, errors.New("私钥文件不是有效的 OpenPGP 密钥文件。")
	}
	hasPrivate := false
	for _, key := range keys {
		hasPrivate = hasPrivate || hasSecret(key)
	}
	if !hasPrivate {
		return nil, errors.New("私钥文件中没有可用的私钥。")
	}
	if len(publicData) > 0 {
		publicKeys, e := parseKeys(publicData)
		if e != nil {
			return nil, errors.New("公钥文件不是有效的 OpenPGP 密钥文件。")
		}
		for _, key := range publicKeys {
			if hasSecret(key) {
				return nil, errors.New("公钥文件包含私钥，请在私钥文件中导入。")
			}
		}
		keys = mergeKeyRings(keys, publicKeys)
	}
	return v.importKeyRing(keys, passphrase, token)
}

func (v *Vault) importKeyRing(keys openpgp.EntityList, passphrase string, token uint64) ([]Identity, error) {
	v.operations.Lock()
	defer v.operations.Unlock()
	unlock, e := lockStore(v.config.Directory)
	if e != nil {
		return nil, e
	}
	defer unlock()
	if passphrase == "" {
		return nil, ErrPassphrase
	}
	existing, e := safeRead(filepath.Dir(v.config.KeyFile), filepath.Base(v.config.KeyFile))
	replace := e == nil
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	if replace {
		old, e := parseKeys(existing)
		if e != nil {
			return nil, e
		}
		if e = decryptKeys(old, passphrase, true); e != nil {
			return nil, e
		}
		keys = mergeKeyRings(old, keys)
		sort.Slice(keys, func(i, j int) bool { return fingerprint(keys[i]) < fingerprint(keys[j]) })
	}
	if e = decryptKeys(keys, passphrase, false); e != nil {
		return nil, e
	}
	encoded, e := protectedKeyData(keys, passphrase)
	if e != nil {
		return nil, e
	}
	if e = decryptKeys(keys, passphrase, true); e != nil {
		return nil, e
	}
	ids := identities(keys)
	_, policyError := safeRead(v.config.Directory, ".gpg-id")
	if policyError != nil && !errors.Is(policyError, os.ErrNotExist) {
		return nil, policyError
	}
	if policyError == nil {
		names, e := v.recipients("")
		if e != nil {
			return nil, e
		}
		if _, e = resolveRecipients(names, keys); e != nil {
			return nil, e
		}
	}
	if e = v.verifyKeys(keys); e != nil {
		return nil, e
	}
	e = v.install(keys, token, func() error {
		if e := safeWrite(filepath.Dir(v.config.KeyFile), filepath.Base(v.config.KeyFile), encoded, replace); e != nil {
			return e
		}
		if errors.Is(policyError, os.ErrNotExist) {
			for _, id := range ids {
				if id.HasSecret {
					return safeWrite(v.config.Directory, ".gpg-id", []byte(id.Fingerprint+"\n"), false)
				}
			}
		}
		return nil
	})
	return ids, e
}

func (v *Vault) Identities() ([]Identity, error) {
	v.operations.Lock()
	defer v.operations.Unlock()
	data, e := safeRead(filepath.Dir(v.config.KeyFile), filepath.Base(v.config.KeyFile))
	if e != nil {
		return nil, e
	}
	keys, e := parseKeys(data)
	if e != nil {
		return nil, e
	}
	return identities(keys), nil
}

func (v *Vault) ExportSecret() ([]byte, error) {
	v.operations.Lock()
	defer v.operations.Unlock()
	_, token, e := v.token()
	if e != nil {
		return nil, e
	}
	data, e := safeRead(filepath.Dir(v.config.KeyFile), filepath.Base(v.config.KeyFile))
	if e != nil {
		return nil, e
	}
	if e = v.validate(token); e != nil {
		return nil, e
	}
	return data, nil
}

func (v *Vault) recipients(name string) ([]string, error) {
	dir := path.Dir(name)
	if name == "" {
		dir = "."
	}
	for {
		data, e := safeRead(v.config.Directory, path.Join(dir, ".gpg-id"))
		if e == nil {
			names := []string{}
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
				if line != "" {
					names = append(names, line)
				}
			}
			if len(names) == 0 {
				return nil, errors.New("empty .gpg-id recipient policy")
			}
			return names, nil
		}
		if !errors.Is(e, os.ErrNotExist) {
			return nil, e
		}
		if dir == "." {
			return nil, ErrNotConfigured
		}
		dir = path.Dir(dir)
	}
}

func (v *Vault) Recipients(name string) ([]string, error) {
	if name != "" {
		if e := ValidatePath(name); e != nil {
			return nil, e
		}
	}
	v.operations.Lock()
	defer v.operations.Unlock()
	return v.recipients(name)
}

func (v *Vault) List() ([]Entry, error) {
	v.operations.Lock()
	defer v.operations.Unlock()
	_, token, e := v.token()
	if e != nil {
		return nil, e
	}
	entries, e := v.listMetadata()
	if e != nil {
		return nil, e
	}
	if e = v.validate(token); e != nil {
		return nil, e
	}
	return entries, nil
}

// ListMetadata deliberately exposes only filenames and modification times. The
// caller must enable its explicit browse-while-locked preference before using it.
func (v *Vault) ListMetadata() ([]Entry, error) {
	v.operations.Lock()
	defer v.operations.Unlock()
	return v.listMetadata()
}

func (v *Vault) listMetadata() ([]Entry, error) {
	entries, _, err := v.listMetadataWithFolders()
	return entries, err
}

// ListMetadataWithFolders also includes empty directories, using the same
// metadata-only traversal and browse-while-locked contract as ListMetadata.
func (v *Vault) ListMetadataWithFolders() ([]Entry, []string, error) {
	v.operations.Lock()
	defer v.operations.Unlock()
	return v.listMetadataWithFolders()
}

func (v *Vault) listMetadataWithFolders() ([]Entry, []string, error) {
	entries := []Entry{}
	folders := []string{}
	e := filepath.WalkDir(v.config.Directory, func(filename string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == v.config.Directory {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, e := filepath.Rel(v.config.Directory, filename)
		if e != nil {
			return e
		}
		if d.IsDir() {
			folder := filepath.ToSlash(rel)
			if ValidatePath(folder) == nil {
				folders = append(folders, folder)
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".gpg") {
			return nil
		}
		name := strings.TrimSuffix(filepath.ToSlash(rel), ".gpg")
		if ValidatePath(name) != nil {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		folder := path.Dir(name)
		if folder == "." {
			folder = ""
		}
		entries = append(entries, Entry{Path: name, Name: path.Base(name), Folder: folder, Modified: info.ModTime()})
		return nil
	})
	if e != nil {
		return nil, nil, e
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, folders, nil
}

// VerifyBackup checks that the backup can decrypt every current entry and
// recovery record, and all recipient policies have usable public keys.
// It does not replace keys, open the session, or write to the password store.
func (v *Vault) VerifyBackup(data []byte, passphrase string) error {
	token := v.generationToken()
	v.operations.Lock()
	defer v.operations.Unlock()
	keys, e := parseKeys(data)
	if e != nil {
		return e
	}
	if e = decryptKeys(keys, passphrase, true); e != nil {
		return e
	}
	if e = v.verifyKeys(keys); e != nil {
		return e
	}
	v.session.Lock()
	defer v.session.Unlock()
	if token != v.generation {
		return ErrCancelled
	}
	return nil
}

func (v *Vault) verifyKeys(keys openpgp.EntityList) error {
	e := filepath.WalkDir(v.config.Directory, func(filename string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == v.config.Directory {
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != ".gpg-id" && !strings.HasSuffix(d.Name(), ".gpg") {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return ErrPath
		}
		relative, e := filepath.Rel(v.config.Directory, filename)
		if e != nil {
			return e
		}
		relative = filepath.ToSlash(relative)
		if d.Name() == ".gpg-id" {
			names, e := v.recipients(path.Join(path.Dir(relative), "entry"))
			if e != nil {
				return e
			}
			_, e = resolveRecipients(names, keys)
			return e
		}
		data, e := safeRead(v.config.Directory, relative)
		if e != nil {
			return e
		}
		plain, e := decrypt(data, keys)
		wipe(plain)
		if e != nil {
			return fmt.Errorf("backup cannot decrypt %s: %w", strings.TrimSuffix(relative, ".gpg"), e)
		}
		return nil
	})
	if e != nil {
		return e
	}
	files, e := os.ReadDir(v.config.RecoveryDirectory)
	if e != nil {
		return e
	}
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".gpg") {
			continue
		}
		if file.Type()&os.ModeSymlink != 0 || file.IsDir() {
			return ErrPath
		}
		data, e := safeRead(v.config.RecoveryDirectory, file.Name())
		if e != nil {
			return e
		}
		plain, e := decrypt(data, keys)
		wipe(plain)
		if e != nil {
			return fmt.Errorf("backup cannot decrypt recovery record %s: %w", strings.TrimSuffix(file.Name(), ".gpg"), e)
		}
	}
	return nil
}

func revision(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (v *Vault) read(name string, keys openpgp.EntityList) (Document, error) {
	if e := ValidatePath(name); e != nil {
		return Document{}, e
	}
	encrypted, e := safeRead(v.config.Directory, name+".gpg")
	if e != nil {
		return Document{}, e
	}
	raw, e := decrypt(encrypted, keys)
	if e != nil {
		return Document{}, e
	}
	defer wipe(raw)
	return ParseDocument(name, string(raw), revision(encrypted)), nil
}

func (v *Vault) Read(name string) (Document, error) {
	v.operations.Lock()
	defer v.operations.Unlock()
	keys, token, e := v.token()
	if e != nil {
		return Document{}, e
	}
	doc, e := v.read(name, keys)
	if e != nil {
		return Document{}, e
	}
	if e = v.validate(token); e != nil {
		return Document{}, e
	}
	return doc, nil
}

func (v *Vault) checkRevision(name, expected string) error {
	data, e := safeRead(v.config.Directory, name+".gpg")
	if errors.Is(e, os.ErrNotExist) {
		if expected != "" {
			return ErrConflict
		}
		return nil
	}
	if e != nil {
		return e
	}
	if expected == "" {
		return ErrExists
	}
	if revision(data) != expected {
		return ErrConflict
	}
	return nil
}

func (v *Vault) encryptedFor(name, raw string, keys openpgp.EntityList) ([]byte, error) {
	names, e := v.recipients(name)
	if e != nil {
		return nil, e
	}
	recipients, e := resolveRecipients(names, keys)
	if e != nil {
		return nil, e
	}
	plain := []byte(raw)
	defer wipe(plain)
	return encryptRecoverable(plain, recipients, keys)
}

func (v *Vault) Save(name, raw, expectedRevision string) (Document, error) {
	return v.SaveRename(name, name, raw, expectedRevision)
}

// SaveRename updates content and destination under one vault operation and file
// lock. The source is deleted only after the destination is durably encrypted.
func (v *Vault) SaveRename(from, to, raw, expectedRevision string) (Document, error) {
	if e := ValidatePath(from); e != nil {
		return Document{}, e
	}
	if e := ValidatePath(to); e != nil {
		return Document{}, e
	}
	v.operations.Lock()
	defer v.operations.Unlock()
	keys, token, e := v.token()
	if e != nil {
		return Document{}, e
	}
	unlock, e := lockStore(v.config.Directory)
	if e != nil {
		return Document{}, e
	}
	defer unlock()
	if e = v.checkRevision(from, expectedRevision); e != nil {
		return Document{}, e
	}
	if from != to {
		if expectedRevision == "" {
			return Document{}, errors.New("moving requires a source revision")
		}
		if e = v.checkRevision(to, ""); e != nil {
			return Document{}, e
		}
	}
	encrypted, e := v.encryptedFor(to, raw, keys)
	if e != nil {
		return Document{}, e
	}
	e = v.commit(token, func() error {
		if e := v.checkRevision(from, expectedRevision); e != nil {
			return e
		}
		if e := safeWrite(v.config.Directory, to+".gpg", encrypted, from == to && expectedRevision != ""); e != nil {
			return e
		}
		if from != to {
			if e := safeRemove(v.config.Directory, from+".gpg"); e != nil {
				return fmt.Errorf("%w: %w", ErrSourceRemains, e)
			}
		}
		return nil
	})
	if e != nil {
		return Document{}, e
	}
	return ParseDocument(to, raw, revision(encrypted)), nil
}

// Revision identifies the current ciphertext of an entry without decrypting it,
// so batch actions can act on the latest version of entries the user selected
// from the list. Move and Trash verify it again under the store lock.
func (v *Vault) Revision(name string) (string, error) {
	if e := ValidatePath(name); e != nil {
		return "", e
	}
	data, e := safeRead(v.config.Directory, name+".gpg")
	if e != nil {
		return "", e
	}
	return revision(data), nil
}

func (v *Vault) Move(from, to, expectedRevision string) error {
	// Read's revision is verified again by SaveRename, so intervening edits are
	// rejected rather than overwritten by the earlier decrypted document.
	doc, e := v.Read(from)
	if e != nil {
		return e
	}
	if doc.Revision != expectedRevision {
		return ErrConflict
	}
	_, e = v.SaveRename(from, to, doc.Raw, expectedRevision)
	return e
}
