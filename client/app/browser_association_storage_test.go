package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knothhe/passpine/client/cloudsync"
)

func TestBrowserAssociationsSurviveOlderPreferenceSerializer(t *testing.T) {
	s := newSyncServiceFixture(t, "", &syncMemorySecrets{}, cloudsync.New(nil))
	directory := associationFixtureVault(t, s, "")
	initial := associationRead(t, s)
	hash := strings.Repeat("a", 64)
	associationPatchFixture(t, s, initial, map[string]bool{hash: false})
	if err := s.savePreferences(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.directory, "preferences.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(before, []byte("browserAssociations")) {
		t.Fatal("authority still serialized into legacy preferences")
	}
	// This is the exact field set in the prior release. Unknown future fields
	// are discarded when that release saves any setting.
	var old struct {
		Current            string                  `json:"current"`
		Vaults             []VaultInfo             `json:"vaults"`
		Settings           Settings                `json:"settings"`
		BrowserPreferences *BrowserSyncPreferences `json:"browserPreferences,omitempty"`
	}
	if err := json.Unmarshal(before, &old); err != nil {
		t.Fatal(err)
	}
	old.Settings.Theme = "light"
	rewritten, _ := json.Marshal(old)
	if err := os.WriteFile(path, rewritten, 0600); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	reopened := newSyncServiceFixture(t, s.directory, &syncMemorySecrets{}, cloudsync.New(nil))
	associationFixtureVault(t, reopened, directory)
	result := associationRead(t, reopened)
	if result.AssociationStoreID != initial.AssociationStoreID {
		t.Fatal("old serializer changed client store identity")
	}
	if value, present := (*result.Associations)[hash]; !present || value {
		t.Fatal("old serializer erased explicit cancellation")
	}
}

func TestBrowserAssociationsMigrateEmbeddedFieldAndPreferIndependentStore(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "migrate", true: "independent-wins"}[existing], func(t *testing.T) {
			s := newSyncServiceFixture(t, "", &syncMemorySecrets{}, cloudsync.New(nil))
			associationFixtureVault(t, s, "")
			initial := associationRead(t, s)
			hash := strings.Repeat("b", 64)
			associationPatchFixture(t, s, initial, map[string]bool{hash: false})
			if err := s.savePreferences(); err != nil {
				t.Fatal(err)
			}
			store := jsonValue(s.prefs.BrowserAssociations)
			path := filepath.Join(s.directory, "preferences.json")
			encoded, _ := os.ReadFile(path)
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if existing {
				fields["browserAssociations"] = json.RawMessage(`{"version":999,"scopes":"invalid old field"}`)
			} else {
				fields["browserAssociations"] = store
				if err := os.Remove(filepath.Join(s.directory, browserAssociationFile)); err != nil {
					t.Fatal(err)
				}
			}
			encoded, _ = json.Marshal(fields)
			if err := os.WriteFile(path, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			_ = s.Close()
			reopened, err := New(Options{DataDir: s.directory})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			if reopened.prefs.BrowserAssociations.StoreID != initial.AssociationStoreID || reopened.prefs.BrowserAssociations.Scopes[initial.AssociationScope].Associations[hash] {
				t.Fatal("migration lost identity or unlinked state")
			}
			if _, err := os.Stat(filepath.Join(s.directory, browserAssociationFile)); err != nil {
				t.Fatal("migration was not persisted", err)
			}
			if err := reopened.savePreferences(); err != nil {
				t.Fatal(err)
			}
			cleaned, _ := os.ReadFile(path)
			if bytes.Contains(cleaned, []byte("browserAssociations")) {
				t.Fatal("migrated legacy field was retained in preferences")
			}
		})
	}
}

func TestBrowserAssociationFileFailureRollsBackCloudPreferenceApplication(t *testing.T) {
	s := newSyncServiceFixture(t, "", &syncMemorySecrets{}, cloudsync.New(nil))
	associationFixtureVault(t, s, "")
	initial := associationRead(t, s)
	hash := strings.Repeat("c", 64)
	associationPatchFixture(t, s, initial, map[string]bool{hash: true})
	if err := s.savePreferences(); err != nil {
		t.Fatal(err)
	}
	prefsFile := filepath.Join(s.directory, "preferences.json")
	before, _ := os.ReadFile(prefsFile)
	associationPath := filepath.Join(s.directory, browserAssociationFile)
	if err := os.Rename(associationPath, associationPath+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(associationPath, 0700); err != nil {
		t.Fatal(err)
	}
	started := s.localSyncValues(nil)
	values := copySyncValues(started)
	values["desktop.theme"] = jsonValue("dark")
	values[associationValuePrefix+initial.AssociationScope+"."+hash] = jsonValue(false)
	if err := s.applySyncValues(values, started); err == nil {
		t.Fatal("association storage failure reported successful cloud application")
	}
	after, _ := os.ReadFile(prefsFile)
	if !bytes.Equal(before, after) {
		t.Fatal("association failure left updated preferences on disk")
	}
	if s.prefs.Settings.Theme == "dark" || !s.prefs.BrowserAssociations.Scopes[initial.AssociationScope].Associations[hash] {
		t.Fatal("association failure left updated in-memory state")
	}
	if err := os.Remove(associationPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(associationPath+".saved", associationPath); err != nil {
		t.Fatal(err)
	}
	if err := s.applySyncValues(values, started); err != nil {
		t.Fatal("subsequent cloud application failed", err)
	}
}

func TestBrowserAssociationMigrationFailurePreservesEmbeddedAndIndependentBytes(t *testing.T) {
	s := newSyncServiceFixture(t, "", &syncMemorySecrets{}, cloudsync.New(nil))
	associationFixtureVault(t, s, "")
	initial := associationRead(t, s)
	associationPatchFixture(t, s, initial, map[string]bool{strings.Repeat("d", 64): false})
	if err := s.savePreferences(); err != nil {
		t.Fatal(err)
	}
	prefsPath := filepath.Join(s.directory, "preferences.json")
	encoded, _ := os.ReadFile(prefsPath)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	fields["browserAssociations"] = jsonValue(s.prefs.BrowserAssociations)
	embedded, _ := json.Marshal(fields)
	if err := os.WriteFile(prefsPath, embedded, 0600); err != nil {
		t.Fatal(err)
	}
	associationPath := filepath.Join(s.directory, browserAssociationFile)
	broken := []byte(`{"version":999,"storeId":"future"}`)
	if err := os.WriteFile(associationPath, broken, 0600); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if reopened, err := New(Options{DataDir: s.directory}); err == nil {
		_ = reopened.Close()
		t.Fatal("invalid independent store was replaced by embedded data")
	}
	actualPrefs, _ := os.ReadFile(prefsPath)
	actualStore, _ := os.ReadFile(associationPath)
	if !bytes.Equal(actualPrefs, embedded) || !bytes.Equal(actualStore, broken) {
		t.Fatal("failed migration overwrote recoverable original data")
	}
}
