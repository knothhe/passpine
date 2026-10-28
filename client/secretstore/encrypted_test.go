package secretstore

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type encryptedNative struct {
	mu                    sync.Mutex
	items                 map[string][]byte
	loads, saves, deletes int
	masterReads           int
	fault                 func(string, string) error
}

func (b *encryptedNative) failure(op, key string) error {
	b.mu.Lock()
	fault := b.fault
	b.mu.Unlock()
	if fault != nil {
		return fault(op, key)
	}
	return nil
}

func (b *encryptedNative) Load(service, key string) ([]byte, error) {
	if err := b.failure("load", key); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.loads++
	value, ok := b.items[service+"/"+key]
	if !ok {
		return nil, ErrNotFound
	}
	if key == masterKeyName {
		b.masterReads++
	}
	return append([]byte(nil), value...), nil
}

func (b *encryptedNative) Save(service, key string, value []byte) error {
	if err := b.failure("save", key); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.saves++
	b.items[service+"/"+key] = append([]byte(nil), value...)
	return nil
}

func (b *encryptedNative) Delete(service, key string) error {
	if err := b.failure("delete", key); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deletes++
	if _, ok := b.items[service+"/"+key]; !ok {
		return ErrNotFound
	}
	delete(b.items, service+"/"+key)
	return nil
}

func (b *encryptedNative) counts() (int, int, int, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.loads, b.saves, b.deletes, b.masterReads
}

func encryptedFixture(t *testing.T) (*Store, *encryptedNative, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	native := &encryptedNative{items: make(map[string][]byte)}
	s, err := NewWithEncryptedBackend("dev", dir, native)
	if err != nil {
		t.Fatal(err)
	}
	return s, native, dir
}

func reopenEncrypted(t *testing.T, dir string, native Backend) *Store {
	t.Helper()
	s, err := NewWithEncryptedBackend("dev", dir, native)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func encryptedDiskDocument(t *testing.T, s *Store, native *encryptedNative) encryptedDocument {
	t.Helper()
	data, err := os.ReadFile(s.encrypted.path)
	if err != nil {
		t.Fatal(err)
	}
	native.mu.Lock()
	master := append([]byte(nil), native.items[s.service+"/"+masterKeyName]...)
	native.mu.Unlock()
	defer clear(master)
	doc, err := s.encrypted.decode(data, master)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestEncryptedLogicalRecordsUseOneMasterAndExceedNativeValueLimit(t *testing.T) {
	s, native, dir := encryptedFixture(t)
	values := map[string][]byte{
		"app-profile":    bytes.Repeat([]byte("private-app-phrase-and-baseline!"), 2000),
		"connection-sdk": bytes.Repeat([]byte("private-sdk-credentials!"), 2400),
		"oauth-sdk":      []byte("private-refresh-and-access-token"),
	}
	for key, value := range values {
		if err := s.Save(key, value); err != nil {
			t.Fatal(err)
		}
	}
	_, _, _, initialReads := native.counts()
	if initialReads != 1 {
		t.Fatalf("fresh logical writes unlocked master %d times", initialReads)
	}
	native.mu.Lock()
	if len(native.items) != 1 || len(native.items[s.service+"/"+masterKeyName]) != 32 {
		t.Fatal("native store contains logical plaintext records")
	}
	native.mu.Unlock()
	data, err := os.ReadFile(s.encrypted.path)
	if err != nil || len(data) <= MaxSecretBytes {
		t.Fatal("aggregate remained bound to one native record", err)
	}
	for _, value := range values {
		if bytes.Contains(data, value[:16]) {
			t.Fatal("plaintext persisted")
		}
	}
	s = reopenEncrypted(t, dir, native)
	for range 3 {
		for key, expected := range values {
			value, err := s.Load(key)
			if err != nil || !bytes.Equal(value, expected) {
				t.Fatal("logical record changed", key, err)
			}
			value[0] ^= 1
		}
	}
	_, _, _, reads := native.counts()
	if reads != initialReads+1 {
		t.Fatal("reopened logical records did not share one master read", reads)
	}
	if err := s.Save("connection-rotated", []byte("new sdk credential")); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("connection-sdk"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("app-profile", []byte("updated baseline")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("app-profile"); err != nil {
		t.Fatal(err)
	}
	_, _, _, after := native.counts()
	if after != reads {
		t.Fatal("normal logical write or rotation reread the master")
	}
	if runtime.GOOS != "windows" {
		for _, name := range []string{encryptedFileName, lockFileName} {
			info, err := os.Stat(filepath.Join(dir, name))
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("unsafe file permissions", name, err)
			}
		}
	}
}

func TestEncryptedMigrationKeepsLegacyUntilVerifiedAndRecoversRefusal(t *testing.T) {
	for _, action := range []string{"load", "save", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, native, dir := encryptedFixture(t)
			if err := native.Save(s.service, "legacy", []byte("old secret")); err != nil {
				t.Fatal(err)
			}
			var refused atomic.Bool
			native.fault = func(op, key string) error {
				if op == "delete" && key == "legacy" && refused.CompareAndSwap(false, true) {
					doc := encryptedDiskDocument(t, s, native)
					defer doc.clear()
					if !doc.Records[key].LegacyPending {
						t.Error("legacy deletion had no durable cleanup owner")
					}
					return ErrDenied
				}
				return nil
			}
			var err error
			switch action {
			case "load":
				_, err = s.Load("legacy")
			case "save":
				err = s.Save("legacy", []byte("replacement"))
			case "delete":
				err = s.Delete("legacy")
			}
			if !errors.Is(err, ErrDenied) || !s.Denied() {
				t.Fatal("migration refusal was hidden", err)
			}
			loads, saves, deletes, reads := native.counts()
			_, _ = s.Load("legacy")
			_ = s.Save("other", []byte("must not persist"))
			_ = s.Delete("legacy")
			_ = s.CheckSession()
			if l, w, d, r := native.counts(); l != loads || w != saves || d != deletes || r != reads {
				t.Fatal("refusal retriggered native access")
			}
			native.mu.Lock()
			if string(native.items[s.service+"/legacy"]) != "old secret" {
				t.Fatal("legacy source lost after refusal")
			}
			native.mu.Unlock()
			s.RetryAccess()
			value, err := s.Load("legacy")
			if action == "delete" {
				if !errors.Is(err, ErrNotFound) {
					t.Fatal("deleted legacy record resurrected", err)
				}
			} else {
				expected := "old secret"
				if action == "save" {
					expected = "replacement"
				}
				if err != nil || string(value) != expected {
					t.Fatal("migration did not resume committed value", err)
				}
			}
			s = reopenEncrypted(t, dir, native)
			if action != "delete" {
				if _, err := s.Load("legacy"); err != nil {
					t.Fatal(err)
				}
			}
			native.mu.Lock()
			if _, ok := native.items[s.service+"/legacy"]; ok {
				t.Fatal("migration retained legacy plaintext")
			}
			native.mu.Unlock()
		})
	}
}

func TestEncryptedFailedMasterReadbackRetainsLegacyAndRecoversInitialization(t *testing.T) {
	s, native, dir := encryptedFixture(t)
	_ = native.Save(s.service, "legacy", []byte("preserve me"))
	var masterLoads atomic.Int32
	native.fault = func(op, key string) error {
		if op == "load" && key == masterKeyName && masterLoads.Add(1) == 2 {
			return ErrUnavailable
		}
		return nil
	}
	if _, err := s.Load("legacy"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("failed master readback was ignored", err)
	}
	native.mu.Lock()
	if string(native.items[s.service+"/legacy"]) != "preserve me" {
		t.Fatal("unverified migration deleted legacy secret")
	}
	native.mu.Unlock()
	if _, err := os.Stat(filepath.Join(dir, initFileName)); err != nil {
		t.Fatal("interrupted initialization has no recovery owner", err)
	}
	s = reopenEncrypted(t, dir, native)
	if value, err := s.Load("legacy"); err != nil || string(value) != "preserve me" {
		t.Fatal("initialization could not recover", err)
	}
	if _, err := os.Stat(filepath.Join(dir, initFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("completed initialization marker retained", err)
	}
}

func TestEncryptedMigrationNativeReadbackFailureKeepsDurablePendingAndLegacy(t *testing.T) {
	s, native, _ := encryptedFixture(t)
	if err := s.Save("already-encrypted", []byte("existing value")); err != nil {
		t.Fatal(err)
	}
	if err := native.Save(s.service, "legacy", []byte("must remain recoverable")); err != nil {
		t.Fatal(err)
	}
	var failed atomic.Bool
	native.fault = func(op, key string) error {
		if op == "load" && key == masterKeyName && failed.CompareAndSwap(false, true) {
			return ErrUnavailable
		}
		return nil
	}
	if _, err := s.Load("legacy"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("failed migration master reread was ignored", err)
	}
	doc := encryptedDiskDocument(t, s, native)
	if !doc.Records["legacy"].LegacyPending || string(doc.Records["legacy"].Value) != "must remain recoverable" {
		t.Fatal("unverified migration has no durable recovery record")
	}
	doc.clear()
	native.mu.Lock()
	legacyExists := string(native.items[s.service+"/legacy"]) == "must remain recoverable"
	native.mu.Unlock()
	if !legacyExists {
		t.Fatal("failed migration verification deleted original")
	}
	if value, err := s.Load("legacy"); err != nil || string(value) != "must remain recoverable" {
		t.Fatal("migration did not resume after temporary verification failure", err)
	}
}

func TestEncryptedCorruptionInvalidatesWarmCacheAndNeverFallsBack(t *testing.T) {
	for _, mutation := range []string{"truncate", "ciphertext", "version", "namespace", "delete"} {
		t.Run(mutation, func(t *testing.T) {
			s, native, dir := encryptedFixture(t)
			if err := s.Save("profile", []byte("persisted secret")); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Load("profile"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(s.encrypted.path)
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "truncate":
				data = data[:10]
			case "ciphertext":
				data[len(data)-1] ^= 1
			case "version":
				data[len(encryptedMagic)-1]++
			case "namespace":
				data[len(encryptedMagic)] ^= 1
			case "delete":
				err = os.Remove(s.encrypted.path)
			}
			if mutation != "delete" {
				err = os.WriteFile(s.encrypted.path, data, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Load("profile"); !errors.Is(err, ErrSessionChanged) {
				t.Fatal("warm cache hid external mutation", err)
			}
			if len(s.loaded) != 0 || len(s.encrypted.master) != 0 {
				t.Fatal("invalid session retained secrets")
			}
			fresh := reopenEncrypted(t, dir, native)
			_, saves, deletes, _ := native.counts()
			if _, err := fresh.Load("profile"); !errors.Is(err, ErrInvalid) {
				t.Fatal("corruption fell back to legacy or regenerated state", err)
			}
			if _, w, d, _ := native.counts(); w != saves || d != deletes {
				t.Fatal("corrupt store mutated native secrets")
			}
		})
	}
}

func TestEncryptedMissingMasterAndMalformedInitializationFailClosed(t *testing.T) {
	t.Run("missing master", func(t *testing.T) {
		s, native, dir := encryptedFixture(t)
		if err := s.Save("profile", []byte("secret")); err != nil {
			t.Fatal(err)
		}
		before, _ := os.ReadFile(s.encrypted.path)
		_ = native.Delete(s.service, masterKeyName)
		s = reopenEncrypted(t, dir, native)
		_, saves, _, _ := native.counts()
		if err := s.Save("profile", []byte("replacement")); !errors.Is(err, ErrInvalid) {
			t.Fatal("missing master was replaced", err)
		}
		after, _ := os.ReadFile(s.encrypted.path)
		if !bytes.Equal(before, after) {
			t.Fatal("unreadable encrypted database replaced")
		}
		if _, got, _, _ := native.counts(); got != saves {
			t.Fatal("replacement master generated")
		}
	})
	t.Run("malformed marker", func(t *testing.T) {
		s, native, dir := encryptedFixture(t)
		if err := os.WriteFile(filepath.Join(dir, initFileName), []byte("unknown marker"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := s.Save("profile", []byte("secret")); !errors.Is(err, ErrInvalid) {
			t.Fatal("malformed marker overwritten", err)
		}
		_, saves, _, _ := native.counts()
		if saves != 0 {
			t.Fatal("unknown initialization created a master")
		}
	})
}

func TestEncryptedAuthenticatedCorruptionCannotRepeatedlyPromptForMaster(t *testing.T) {
	s, native, dir := encryptedFixture(t)
	if err := s.Save("profile", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.encrypted.path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	if err = os.WriteFile(s.encrypted.path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s = reopenEncrypted(t, dir, native)
	_, _, _, before := native.counts()
	if _, err := s.Load("profile"); !errors.Is(err, ErrInvalid) {
		t.Fatal("corrupt authentication was accepted", err)
	}
	_, _, _, after := native.counts()
	if after != before+1 {
		t.Fatal("initial authentication did not use one native master read")
	}
	for range 3 {
		if _, err := s.Load("profile"); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		if err := s.Save("other", []byte("new")); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		if err := s.CheckSession(); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if _, _, _, got := native.counts(); got != after {
		t.Fatal("permanent corruption reopened the native master")
	}
}

func TestEncryptedRetrySessionClassifiesFileBeforeResettingAccess(t *testing.T) {
	for _, denied := range []bool{false, true} {
		for _, changed := range []bool{false, true} {
			t.Run(fmt.Sprintf("denied=%t/changed=%t", denied, changed), func(t *testing.T) {
				s, native, _ := encryptedFixture(t)
				if err := s.Save("profile", []byte("original")); err != nil {
					t.Fatal(err)
				}
				restored, err := os.ReadFile(s.encrypted.path)
				if err != nil {
					t.Fatal(err)
				}
				if err := s.Save("profile", []byte("current")); err != nil {
					t.Fatal(err)
				}
				if _, err := s.Load("profile"); err != nil {
					t.Fatal(err)
				}
				if denied {
					s.ClearSession()
					var refused atomic.Bool
					native.fault = func(op, key string) error {
						if op == "load" && key == masterKeyName && refused.CompareAndSwap(false, true) {
							return ErrDenied
						}
						return nil
					}
					if _, err := s.Load("profile"); !errors.Is(err, ErrDenied) || !s.Denied() {
						t.Fatal("native refusal did not close access", err)
					}
				}
				if changed {
					if err := os.WriteFile(s.encrypted.path, restored, 0600); err != nil {
						t.Fatal(err)
					}
				}
				loads, saves, deletes, reads := native.counts()
				gotChanged, err := s.RetrySession()
				if err != nil || gotChanged != changed {
					t.Fatal("retry misclassified the encrypted file", gotChanged, err)
				}
				if s.Denied() || len(s.loaded) != 0 || len(s.encrypted.master) != 0 {
					t.Fatal("successful retry retained an old session")
				}
				if l, w, d, r := native.counts(); l != loads || w != saves || d != deletes || r != reads {
					t.Fatal("retry classification accessed native credentials")
				}
				expected := "current"
				if changed {
					expected = "original"
				}
				if value, err := s.Load("profile"); err != nil || string(value) != expected {
					t.Fatal("retry did not reopen the current file", err)
				}
				if _, _, _, after := native.counts(); after != reads+1 {
					t.Fatal("retry reused the previous native master cache")
				}
			})
		}
	}
}

func TestEncryptedRetrySessionPreservesStateOnMetadataFailure(t *testing.T) {
	t.Run("unknown corrupt file", func(t *testing.T) {
		s, native, _ := encryptedFixture(t)
		if err := os.WriteFile(s.encrypted.path, []byte("corrupt envelope"), 0600); err != nil {
			t.Fatal(err)
		}
		if changed, err := s.RetrySession(); changed || !errors.Is(err, ErrInvalid) {
			t.Fatal("retry reset access before classifying corruption", changed, err)
		}
		if !s.encrypted.changed {
			t.Fatal("retry removed the corruption latch")
		}
		if l, w, d, r := native.counts(); l != 0 || w != 0 || d != 0 || r != 0 {
			t.Fatal("corrupt file reached native credentials")
		}
	})
	t.Run("unsafe lock after denial", func(t *testing.T) {
		s, native, dir := encryptedFixture(t)
		native.fault = func(op, key string) error { return ErrDenied }
		if _, err := s.Load("profile"); !errors.Is(err, ErrDenied) {
			t.Fatal(err)
		}
		lockPath := filepath.Join(dir, lockFileName)
		if err := os.Remove(lockPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(lockPath, 0700); err != nil {
			t.Fatal(err)
		}
		if changed, err := s.RetrySession(); changed || !errors.Is(err, ErrInvalid) || !s.Denied() {
			t.Fatal("unsafe metadata reset denied access", changed, err)
		}
		if _, err := s.Load("profile"); !errors.Is(err, ErrDenied) {
			t.Fatal("failed retry reopened native access", err)
		}
		if l, w, d, r := native.counts(); l != 0 || w != 0 || d != 0 || r != 0 {
			t.Fatal("metadata failure accessed native credentials")
		}
	})
	t.Run("unavailable lock retains warm reads", func(t *testing.T) {
		s, native, dir := encryptedFixture(t)
		if err := s.Save("profile", []byte("current")); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Load("profile"); err != nil {
			t.Fatal(err)
		}
		unlock, err := acquireFileLock(filepath.Join(dir, lockFileName))
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
		loads, saves, deletes, reads := native.counts()
		if changed, err := s.RetrySession(); changed || !errors.Is(err, ErrUnavailable) {
			t.Fatal("lock timeout reset access", changed, err)
		}
		if s.Denied() || string(s.loaded["profile"]) != "current" || len(s.encrypted.master) != 32 {
			t.Fatal("failed metadata check discarded the existing session")
		}
		if l, w, d, r := native.counts(); l != loads || w != saves || d != deletes || r != reads {
			t.Fatal("lock timeout accessed native credentials")
		}
	})
}

func TestEncryptedUnchangedRetrySessionStillDetectsLaterRestore(t *testing.T) {
	s, native, _ := encryptedFixture(t)
	if err := s.Save("profile", []byte("original")); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(s.encrypted.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save("profile", []byte("current")); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.RetrySession(); err != nil || changed {
		t.Fatal("unchanged retry failed", changed, err)
	}
	loads, saves, deletes, reads := native.counts()
	if err := os.WriteFile(s.encrypted.path, restored, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckSession(); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("retry forgot the snapshot that owns pending writes", err)
	}
	if _, err := s.Load("profile"); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("later restore did not close access", err)
	}
	if l, w, d, r := native.counts(); l != loads || w != saves || d != deletes || r != reads {
		t.Fatal("later restore opened native credentials")
	}
}

func TestEncryptedConcurrentStoresNeverLoseCommittedRecords(t *testing.T) {
	first, native, dir := encryptedFixture(t)
	second := reopenEncrypted(t, dir, native)
	errorsOut := make(chan error, 2)
	go func() { errorsOut <- first.Save("first", []byte("first secret")) }()
	go func() { errorsOut <- second.Save("second", []byte("second secret")) }()
	for range 2 {
		if err := <-errorsOut; err != nil {
			t.Fatal(err)
		}
	}
	_, _, _, reads := native.counts()
	if reads != 2 {
		t.Fatal("concurrent initialization replaced or reinitialized master", reads)
	}
	reader := reopenEncrypted(t, dir, native)
	for _, key := range []string{"first", "second"} {
		if value, err := reader.Load(key); err != nil || string(value) != key+" secret" {
			t.Fatal("concurrent write was lost", key, err)
		}
	}
	// Once a process has a known snapshot, it fails closed on another
	// process's later writes; explicit retry reopens against the latest file.
	if err := first.Save("later", []byte("must preserve both")); err != nil && !errors.Is(err, ErrSessionChanged) {
		t.Fatal(err)
	}
	first.RetryAccess()
	if err := first.Save("later", []byte("must preserve both")); err != nil {
		t.Fatal(err)
	}
}

func TestEncryptedReservedMasterKeyAndRecordLimits(t *testing.T) {
	s, native, _ := encryptedFixture(t)
	if _, err := s.Load(masterKeyName); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err := s.Save(masterKeyName, []byte("bad")); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err := s.Delete(masterKeyName); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if l, w, d, _ := native.counts(); l != 0 || w != 0 || d != 0 {
		t.Fatal("reserved key reached native store")
	}
	if err := s.Save("too-large", make([]byte, MaxSecretBytes+1)); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err := s.Save("maximum", bytes.Repeat([]byte("x"), MaxSecretBytes)); err != nil {
		t.Fatal(err)
	}
	value, err := s.Load("maximum")
	if err != nil || len(value) != MaxSecretBytes {
		t.Fatal("per-record maximum regressed", err)
	}
}

func TestEncryptedDeleteChurnDoesNotAccumulateTombstones(t *testing.T) {
	s, native, _ := encryptedFixture(t)
	for i := range 64 {
		key := fmt.Sprintf("fresh-key-%d", i)
		if err := s.Save(key, []byte("temporary secret")); err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(key); err != nil {
			t.Fatal(err)
		}
	}
	doc := encryptedDiskDocument(t, s, native)
	defer doc.clear()
	if len(doc.Records) != 0 {
		t.Fatal("completed logical deletions accumulated tombstones")
	}
}

func TestEncryptedClearSessionDoesNotWaitOrCacheLateNativePhases(t *testing.T) {
	s, native, _ := encryptedFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	native.fault = func(op, key string) error {
		if op == "load" && key == masterKeyName {
			once.Do(func() { close(started); <-release })
		}
		return nil
	}
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	finished := make(chan error, 1)
	go func() { finished <- s.Save("profile", []byte("late secret")) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("master access did not start")
	}
	cleared := make(chan struct{})
	go func() { s.ClearSession(); close(cleared) }()
	select {
	case <-cleared:
	case <-time.After(3 * time.Second):
		t.Fatal("clear waited on native access")
	}
	unblock.Do(func() { close(release) })
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if len(s.encrypted.master) != 0 || len(s.loaded) != 0 {
		t.Fatal("late multi-stage operation repopulated session")
	}
	_, _, _, reads := native.counts()
	if _, err := s.Load("profile"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, got := native.counts(); got != reads+1 {
		t.Fatal("new session did not reread native master")
	}
}

func TestEncryptedClearDuringFileLockWaitCannotRepopulateSession(t *testing.T) {
	s, native, dir := encryptedFixture(t)
	if err := s.Save("profile", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	unlock, err := acquireFileLock(filepath.Join(dir, lockFileName))
	if err != nil {
		t.Fatal(err)
	}
	var release sync.Once
	defer release.Do(unlock)
	finished := make(chan error, 1)
	go func() { _, err := s.Load("profile"); finished <- err }()
	deadline := time.Now().Add(3 * time.Second)
	for s.mu.TryLock() {
		s.mu.Unlock()
		if time.Now().After(deadline) {
			t.Fatal("reader did not begin waiting for file lock")
		}
		time.Sleep(time.Millisecond)
	}
	s.ClearSession()
	release.Do(unlock)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if len(s.loaded) != 0 || len(s.encrypted.master) != 0 {
		t.Fatal("file-lock waiter repopulated a cleared session")
	}
	_, _, _, before := native.counts()
	if _, err := s.Load("profile"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, after := native.counts(); after != before+1 {
		t.Fatal("new session skipped native read after late waiter")
	}
}

func TestEncryptedSymlinkEnvelopeAndLockAreRejected(t *testing.T) {
	for _, name := range []string{encryptedFileName, lockFileName} {
		t.Run(name, func(t *testing.T) {
			s, native, dir := encryptedFixture(t)
			target := filepath.Join(t.TempDir(), "unrelated")
			if err := os.WriteFile(target, []byte("preserve unrelated file"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
				t.Skip("symlinks unavailable on this native platform")
			}
			if err := s.Save("profile", []byte("secret")); !errors.Is(err, ErrInvalid) {
				t.Fatal("symlink was followed", err)
			}
			data, err := os.ReadFile(target)
			if err != nil || string(data) != "preserve unrelated file" {
				t.Fatal("unrelated target changed", err)
			}
			if l, w, d, _ := native.counts(); l != 0 || w != 0 || d != 0 {
				t.Fatal("unsafe path reached native secrets")
			}
		})
	}
}

func TestEncryptedNativePromptCannotOverwriteExternalFileRestore(t *testing.T) {
	s, native, _ := encryptedFixture(t)
	if err := s.Save("first", []byte("first value")); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(s.encrypted.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save("second", []byte("second value")); err != nil {
		t.Fatal(err)
	}
	if err := native.Save(s.service, "legacy", []byte("preserve legacy")); err != nil {
		t.Fatal(err)
	}
	var restoredOnce atomic.Bool
	native.fault = func(op, key string) error {
		if op == "load" && key == "legacy" && restoredOnce.CompareAndSwap(false, true) {
			return os.WriteFile(s.encrypted.path, restored, 0600)
		}
		return nil
	}
	if err := s.Save("legacy", []byte("candidate")); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("native pause overwrote an external restore", err)
	}
	actual, err := os.ReadFile(s.encrypted.path)
	if err != nil || !bytes.Equal(actual, restored) {
		t.Fatal("restored file was replaced", err)
	}
	native.mu.Lock()
	legacyPresent := string(native.items[s.service+"/legacy"]) == "preserve legacy"
	native.mu.Unlock()
	if !legacyPresent {
		t.Fatal("uncommitted migration removed legacy source")
	}
}

func TestEncryptedNativeMasterPromptCannotExposeExternalFileRestore(t *testing.T) {
	s, native, dir := encryptedFixture(t)
	if err := s.Save("profile", []byte("restored baseline")); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(s.encrypted.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save("profile", []byte("current baseline")); err != nil {
		t.Fatal(err)
	}
	s = reopenEncrypted(t, dir, native)
	var restoredOnce atomic.Bool
	native.fault = func(op, key string) error {
		if op == "load" && key == masterKeyName && restoredOnce.CompareAndSwap(false, true) {
			return os.WriteFile(s.encrypted.path, restored, 0600)
		}
		return nil
	}
	if value, err := s.Load("profile"); !errors.Is(err, ErrSessionChanged) || value != nil {
		t.Fatal("native prompt exposed a snapshot replaced during authorization", err)
	}
	if len(s.loaded) != 0 || len(s.encrypted.master) != 0 {
		t.Fatal("replaced snapshot retained cached secrets")
	}
	loads, saves, deletes, reads := native.counts()
	if _, err := s.Load("profile"); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("external restore did not latch the session change", err)
	}
	if l, w, d, r := native.counts(); l != loads || w != saves || d != deletes || r != reads {
		t.Fatal("invalidated snapshot retriggered native access")
	}
	actual, err := os.ReadFile(s.encrypted.path)
	if err != nil || !bytes.Equal(actual, restored) {
		t.Fatal("external restore was changed", err)
	}
}

func TestEncryptedLegacyPromptCannotCreateMasterForRestoredFile(t *testing.T) {
	s, native, dir := encryptedFixture(t)
	if err := s.Save("profile", []byte("original")); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(s.encrypted.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.encrypted.path); err != nil {
		t.Fatal(err)
	}
	if err := native.Delete(s.service, masterKeyName); err != nil {
		t.Fatal(err)
	}
	if err := native.Save(s.service, "legacy", []byte("preserve legacy")); err != nil {
		t.Fatal(err)
	}
	s = reopenEncrypted(t, dir, native)
	var restoredOnce atomic.Bool
	native.fault = func(op, key string) error {
		if op == "load" && key == "legacy" && restoredOnce.CompareAndSwap(false, true) {
			return os.WriteFile(s.encrypted.path, restored, 0600)
		}
		return nil
	}
	_, saves, _, _ := native.counts()
	if _, err := s.Load("legacy"); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("restore did not invalidate initialization", err)
	}
	if _, after, _, _ := native.counts(); after != saves {
		t.Fatal("restored ciphertext triggered a replacement native master")
	}
	native.mu.Lock()
	_, masterExists := native.items[s.service+"/"+masterKeyName]
	legacyPresent := string(native.items[s.service+"/legacy"]) == "preserve legacy"
	native.mu.Unlock()
	if masterExists || !legacyPresent {
		t.Fatal("restored ciphertext changed native credential sources")
	}
	if _, err := os.Stat(filepath.Join(dir, initFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("restore created an initialization marker", err)
	}
	actual, err := os.ReadFile(s.encrypted.path)
	if err != nil || !bytes.Equal(actual, restored) {
		t.Fatal("restored ciphertext was changed", err)
	}
}

func TestEncryptedPathWithSpacesAndUnicode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "资料 with spaces")
	native := &encryptedNative{items: make(map[string][]byte)}
	s := reopenEncrypted(t, dir, native)
	if err := s.Save("profile", []byte("private value")); err != nil {
		t.Fatal(err)
	}
	s = reopenEncrypted(t, dir, native)
	if value, err := s.Load("profile"); err != nil || string(value) != "private value" {
		t.Fatal("special path failed", err)
	}
}

func TestEncryptedFileLockSubprocessHelper(t *testing.T) {
	path := os.Getenv("PASS_PINE_SECRET_LOCK_TEST_PATH")
	if path == "" {
		return
	}
	unlock, err := acquireFileLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	fmt.Println("locked")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func TestEncryptedLockCoordinatesNativeGoProcesses(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, lockFileName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEncryptedFileLockSubprocessHelper$")
	command.Env = append(os.Environ(), "PASS_PINE_SECRET_LOCK_TEST_PATH="+path)
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			input.Close()
			_ = command.Wait()
		}
	}()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatal("isolated helper failed to acquire lock", err, stderr.String())
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	locked, err := tryFileLock(file)
	if locked {
		releaseFileLock(file)
	}
	file.Close()
	if err != nil || locked {
		t.Fatal("native processes did not share lock exclusion", err)
	}
	if err = input.Close(); err != nil {
		t.Fatal(err)
	}
	if err = command.Wait(); err != nil {
		t.Fatal("lock helper failed to exit", err, stderr.String())
	}
	waited = true
	unlock, err := acquireFileLock(path)
	if err != nil {
		t.Fatal("completed process retained lock", err)
	}
	unlock()
}
