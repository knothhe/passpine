package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
)

type RecoveryRecord struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Path        string    `json:"path"`
	Destination string    `json:"destination,omitempty"`
	Revision    string    `json:"revision,omitempty"`
	Raw         string    `json:"raw"`
	CreatedAt   time.Time `json:"createdAt"`
}

func validateRecoveryID(id string) error {
	if len(id) != 32 {
		return ErrPath
	}
	_, e := hex.DecodeString(id)
	if e != nil {
		return ErrPath
	}
	return nil
}

func (v *Vault) recoveryKeys(keys openpgp.EntityList) ([]*openpgp.Entity, error) {
	// Recovery stays readable with the user's private identities even when an
	// entry lives in a folder whose recipients have a different sharing policy.
	out := []*openpgp.Entity{}
	for _, key := range keys {
		selected, ok := key.EncryptionKey(time.Now())
		if ok && selected.PrivateKey != nil && !selected.PrivateKey.Dummy() && !selected.PrivateKey.Encrypted {
			out = append(out, key)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no locally decryptable encryption key available for recovery")
	}
	return out, nil
}

func (v *Vault) encryptRecord(record RecoveryRecord, keys openpgp.EntityList) ([]byte, error) {
	raw, e := json.Marshal(record)
	if e != nil {
		return nil, e
	}
	defer wipe(raw)
	recipients, e := v.recoveryKeys(keys)
	if e != nil {
		return nil, e
	}
	return encryptRecoverable(raw, recipients, keys)
}

func (v *Vault) readRecovery(id string, keys openpgp.EntityList) (RecoveryRecord, error) {
	if e := validateRecoveryID(id); e != nil {
		return RecoveryRecord{}, e
	}
	data, e := safeRead(v.config.RecoveryDirectory, id+".gpg")
	if e != nil {
		return RecoveryRecord{}, e
	}
	raw, e := decrypt(data, keys)
	if e != nil {
		return RecoveryRecord{}, e
	}
	defer wipe(raw)
	var record RecoveryRecord
	if e = json.Unmarshal(raw, &record); e != nil {
		return record, e
	}
	if record.ID != id || (record.Kind != "draft" && record.Kind != "trash") {
		return RecoveryRecord{}, errors.New("invalid recovery record")
	}
	return record, nil
}

func (v *Vault) ReadRecovery(id string) (RecoveryRecord, error) {
	v.operations.Lock()
	defer v.operations.Unlock()
	keys, token, e := v.token()
	if e != nil {
		return RecoveryRecord{}, e
	}
	record, e := v.readRecovery(id, keys)
	if e != nil {
		return RecoveryRecord{}, e
	}
	if e = v.validate(token); e != nil {
		return RecoveryRecord{}, e
	}
	return record, nil
}

func (v *Vault) Trash(name, expectedRevision string) (RecoveryRecord, error) {
	v.operations.Lock()
	defer v.operations.Unlock()
	keys, token, e := v.token()
	if e != nil {
		return RecoveryRecord{}, e
	}
	unlock, e := lockStore(v.config.Directory)
	if e != nil {
		return RecoveryRecord{}, e
	}
	defer unlock()
	doc, e := v.read(name, keys)
	if e != nil {
		return RecoveryRecord{}, e
	}
	if expectedRevision == "" || doc.Revision != expectedRevision {
		return RecoveryRecord{}, ErrConflict
	}
	record := RecoveryRecord{ID: randomID(), Kind: "trash", Path: name, Revision: doc.Revision, Raw: doc.Raw, CreatedAt: v.clock()}
	encrypted, e := v.encryptRecord(record, keys)
	if e != nil {
		return RecoveryRecord{}, e
	}
	e = v.commit(token, func() error {
		if e := v.checkRevision(name, expectedRevision); e != nil {
			return e
		}
		if e := safeWrite(v.config.RecoveryDirectory, record.ID+".gpg", encrypted, false); e != nil {
			return e
		}
		return safeRemove(v.config.Directory, name+".gpg")
	})
	if e != nil {
		return RecoveryRecord{}, e
	}
	return record, nil
}

func (v *Vault) SaveDraft(name, raw string) (RecoveryRecord, error) {
	return v.SaveDraftRecord("", name, name, raw, "")
}

// SaveDraftRecord stores all editor metadata inside the encrypted record. An
// empty id is deterministically assigned per source path, so autosave replaces
// its previous draft rather than creating a new record for each keystroke.
func (v *Vault) SaveDraftRecord(id, name, destination, raw, originalRevision string) (RecoveryRecord, error) {
	if name != "" {
		if e := ValidatePath(name); e != nil {
			return RecoveryRecord{}, e
		}
	}
	if destination != "" {
		if e := ValidatePath(destination); e != nil {
			return RecoveryRecord{}, e
		}
	}
	if id == "" {
		digest := sha256.Sum256([]byte("draft:" + name))
		id = hex.EncodeToString(digest[:16])
	}
	if e := validateRecoveryID(id); e != nil {
		return RecoveryRecord{}, e
	}
	v.operations.Lock()
	defer v.operations.Unlock()
	keys, token, e := v.token()
	if e != nil {
		return RecoveryRecord{}, e
	}
	unlock, e := lockStore(v.config.Directory)
	if e != nil {
		return RecoveryRecord{}, e
	}
	defer unlock()
	record := RecoveryRecord{ID: id, Kind: "draft", Path: name, Destination: destination, Revision: originalRevision, Raw: raw, CreatedAt: v.clock()}
	if prior, e := v.readRecovery(id, keys); e == nil {
		if prior.Kind != "draft" {
			return RecoveryRecord{}, ErrExists
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return RecoveryRecord{}, e
	}
	encrypted, e := v.encryptRecord(record, keys)
	if e != nil {
		return RecoveryRecord{}, e
	}
	e = v.commit(token, func() error { return safeWrite(v.config.RecoveryDirectory, id+".gpg", encrypted, true) })
	if e != nil {
		return RecoveryRecord{}, e
	}
	return record, nil
}

func (v *Vault) Recovery(kind string) ([]RecoveryRecord, error) {
	if kind != "" && kind != "draft" && kind != "trash" {
		return nil, errors.New("invalid recovery kind")
	}
	v.operations.Lock()
	defer v.operations.Unlock()
	keys, token, e := v.token()
	if e != nil {
		return nil, e
	}
	files, e := os.ReadDir(v.config.RecoveryDirectory)
	if e != nil {
		return nil, e
	}
	records := []RecoveryRecord{}
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".gpg") || file.Type()&os.ModeSymlink != 0 || file.IsDir() {
			continue
		}
		id := strings.TrimSuffix(file.Name(), ".gpg")
		if validateRecoveryID(id) != nil {
			continue
		}
		record, e := v.readRecovery(id, keys)
		if e != nil {
			return nil, e
		}
		if kind == "" || record.Kind == kind {
			records = append(records, record)
		}
	}
	if e = v.validate(token); e != nil {
		return nil, e
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.After(records[j].CreatedAt) })
	return records, nil
}

func (v *Vault) RemoveRecovery(id string) error {
	if e := validateRecoveryID(id); e != nil {
		return e
	}
	v.operations.Lock()
	defer v.operations.Unlock()
	_, token, e := v.token()
	if e != nil {
		return e
	}
	unlock, e := lockStore(v.config.Directory)
	if e != nil {
		return e
	}
	defer unlock()
	return v.commit(token, func() error { return safeRemove(v.config.RecoveryDirectory, id+".gpg") })
}

func (v *Vault) Restore(id, name string) (Document, error) {
	if e := ValidatePath(name); e != nil {
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
	record, e := v.readRecovery(id, keys)
	if e != nil {
		return Document{}, e
	}
	if e = v.checkRevision(name, ""); e != nil {
		return Document{}, e
	}
	encrypted, e := v.encryptedFor(name, record.Raw, keys)
	if e != nil {
		return Document{}, e
	}
	e = v.commit(token, func() error {
		if e := safeWrite(v.config.Directory, name+".gpg", encrypted, false); e != nil {
			return e
		}
		return safeRemove(v.config.RecoveryDirectory, id+".gpg")
	})
	if e != nil {
		return Document{}, e
	}
	return ParseDocument(name, record.Raw, revision(encrypted)), nil
}
