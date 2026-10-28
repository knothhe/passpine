package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knothhe/passpine/client/cloudsync"
	"github.com/knothhe/passpine/client/vault"
)

func associationFixtureVault(t *testing.T, s *Service, directory string) string {
	t.Helper()
	if directory == "" {
		directory = filepath.Join(t.TempDir(), "association vault")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".gpg-id"), []byte("fixture-recipient\n"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err := vault.New(vault.Config{Directory: directory, KeyFile: filepath.Join(t.TempDir(), "fixture key.asc")})
	if err != nil {
		t.Fatal(err)
	}
	s.active = v
	s.prefs.Current = idFor(directory)
	s.prefs.Vaults = []VaultInfo{{ID: s.prefs.Current, Name: "Fixture", Directory: directory}}
	return directory
}

func TestBrowserAssociationsLegacyImportSharedAcrossVaultScopes(t *testing.T) {
	s := newSyncServiceFixture(t, "", &syncMemorySecrets{}, cloudsync.New(nil))
	legacy := map[string]bool{}
	for i := 0; i < maxAssociationValues; i++ {
		legacy[fmt.Sprintf("%064x", i)] = i%2 == 0
	}
	for i := 0; i < 3; i++ {
		if s.active != nil {
			s.active.Close()
		}
		associationFixtureVault(t, s, "")
		initial := associationRead(t, s)
		r := associationNative(t, s, Request{Action: "browser-associations", VaultID: s.prefs.Current, AssociationScope: initial.AssociationScope, AssociationImport: &legacy})
		if r.Status != "ok" || len(*r.Data.Associations) != maxAssociationValues {
			t.Fatal("legacy import duplicated across scopes", r.Code)
		}
	}
	if len(s.prefs.BrowserAssociations.Legacy) != maxAssociationValues {
		t.Fatal("legacy records were not stored once")
	}
	for _, scope := range s.prefs.BrowserAssociations.Scopes {
		if len(scope.Associations) != 0 {
			t.Fatal("legacy records copied into canonical scope")
		}
	}
}

func associationNative(t *testing.T, s *Service, r Request) Response {
	t.Helper()
	r.Protocol = 1
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var result Response
	if err := json.Unmarshal(s.handleBrowser(encoded), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func associationRead(t *testing.T, s *Service) Data {
	t.Helper()
	r := associationNative(t, s, Request{Action: "browser-associations", VaultID: s.prefs.Current})
	if r.Status != "ok" {
		t.Fatalf("association read: %s %s", r.Code, r.Message)
	}
	return r.Data
}

func TestBrowserAssociationsPersistAndImportWithoutReplacingExplicitValues(t *testing.T) {
	s := newSyncServiceFixture(t, "", &syncMemorySecrets{}, cloudsync.New(nil))
	directory := associationFixtureVault(t, s, "")
	initial := associationRead(t, s)
	if !associationHex(initial.AssociationStoreID) || !associationHex(initial.AssociationScope) || initial.Associations == nil || len(*initial.Associations) != 0 || len(initial.AssociationVaultIDs) != 1 || initial.AssociationVaultIDs[0] != s.prefs.Current {
		t.Fatal("missing empty store identity", initial)
	}
	hash := strings.Repeat("a", 64)
	patch := map[string]bool{hash: false}
	result := associationNative(t, s, Request{Action: "browser-associations", VaultID: s.prefs.Current, AssociationScope: initial.AssociationScope, AssociationPatch: &patch})
	if result.Status != "ok" || (*result.Data.Associations)[hash] {
		t.Fatal("explicit false was not saved", result)
	}
	legacy := map[string]bool{hash: true, strings.Repeat("b", 64): true}
	result = associationNative(t, s, Request{Action: "browser-associations", VaultID: s.prefs.Current, AssociationScope: initial.AssociationScope, AssociationImport: &legacy})
	if result.Status != "ok" || (*result.Data.Associations)[hash] || !(*result.Data.Associations)[strings.Repeat("b", 64)] {
		t.Fatal("legacy import replaced a current decision", result)
	}
	if s.entriesVersion != 0 {
		t.Fatal("association changes reset credential editing state")
	}
	_ = s.Close()
	reopened := newSyncServiceFixture(t, s.directory, &syncMemorySecrets{}, cloudsync.New(nil))
	associationFixtureVault(t, reopened, directory)
	again := associationRead(t, reopened)
	if again.AssociationStoreID != initial.AssociationStoreID || again.AssociationScope != initial.AssociationScope || (*again.Associations)[hash] || len(*again.Associations) != 2 {
		t.Fatal("association persistence lost state")
	}
	status := associationNative(t, reopened, Request{Action: "status"})
	if status.Data.Associations != nil || status.Data.AssociationScope != "" || status.Data.AssociationStoreID != "" || status.Data.AssociationVaultIDs != nil {
		t.Fatal("ordinary status exposed association state")
	}
}

func TestBrowserAssociationsRejectMalformedAndStaleNativeRequests(t *testing.T) {
	s := newSyncServiceFixture(t, "", &syncMemorySecrets{}, cloudsync.New(nil))
	associationFixtureVault(t, s, "")
	initial := associationRead(t, s)
	hash := strings.Repeat("c", 64)
	valid := map[string]any{"protocol": 1, "action": "browser-associations", "vaultId": s.prefs.Current, "associationScope": initial.AssociationScope}
	for _, field := range []string{"associationPatch", "associationImport"} {
		for _, value := range []any{nil, map[string]any{hash: nil}, map[string]any{hash: "true"}, map[string]any{"bad": true}} {
			raw := map[string]any{}
			for k, v := range valid {
				raw[k] = v
			}
			raw[field] = value
			encoded, _ := json.Marshal(raw)
			var result Response
			_ = json.Unmarshal(s.handleBrowser(encoded), &result)
			if result.Code != "invalid_request" {
				t.Fatalf("accepted invalid %s: %s", field, encoded)
			}
		}
	}
	for _, raw := range []string{
		`{"protocol":1,"action":"status","associationPatch":null}`,
		`{"protocol":1,"action":"status","associationScope":null}`,
		`{"protocol":1,"action":"status","associationStoreId":null}`,
		`{"protocol":1,"action":"status","associationOperationId":null}`,
		`{"protocol":1,"action":"browser-associations","vaultId":"` + s.prefs.Current + `","entry":"injected"}`,
	} {
		var r Response
		_ = json.Unmarshal(s.handleBrowser([]byte(raw)), &r)
		if r.Code != "invalid_request" {
			t.Fatal("accepted unrelated native arguments", raw, r.Code)
		}
	}
	patch := map[string]bool{hash: true}
	for _, r := range []Request{
		{Action: "browser-associations", VaultID: s.prefs.Current, AssociationPatch: &patch},
		{Action: "browser-associations", VaultID: s.prefs.Current, AssociationScope: strings.Repeat("0", 64), AssociationPatch: &patch},
		{Action: "browser-associations", VaultID: strings.Repeat("0", 32), AssociationScope: initial.AssociationScope, AssociationPatch: &patch},
	} {
		if associationNative(t, s, r).Status != "error" {
			t.Fatal("accepted missing or stale identity")
		}
	}
	if len(*associationRead(t, s).Associations) != 0 {
		t.Fatal("rejected request changed persisted values")
	}
}

func TestBrowserAssociationOperationsAreStoreBoundAndIdempotent(t *testing.T) {
	s := newSyncServiceFixture(t, "", &syncMemorySecrets{}, cloudsync.New(nil))
	directory := associationFixtureVault(t, s, "")
	initial := associationRead(t, s)
	hash, id := strings.Repeat("a", 64), strings.Repeat("b", 64)
	values := map[string]bool{hash: false}
	request := Request{Action: "browser-associations", VaultID: s.prefs.Current, AssociationScope: initial.AssociationScope, AssociationStoreID: initial.AssociationStoreID, AssociationOperationID: id, AssociationPatch: &values}
	wrong := request
	wrong.AssociationStoreID = strings.Repeat("c", 64)
	if result := associationNative(t, s, wrong); result.Code != "vault_changed" {
		t.Fatal("accepted another store's operation", result.Code)
	}
	if len(*associationRead(t, s).Associations) != 0 || len(s.prefs.BrowserAssociations.Operations) != 0 {
		t.Fatal("store mismatch changed associations or receipts")
	}
	associationFile := filepath.Join(s.directory, browserAssociationFile)
	if err := os.Rename(associationFile, associationFile+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(associationFile, 0700); err != nil {
		t.Fatal(err)
	}
	if result := associationNative(t, s, request); result.Code != "local_storage" || len(s.prefs.BrowserAssociations.Operations) != 0 || len(s.prefs.BrowserAssociations.Scopes[initial.AssociationScope].Associations) != 0 {
		t.Fatal("failed atomic receipt save changed memory or reported success", result.Code)
	}
	if err := os.Remove(associationFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(associationFile+".saved", associationFile); err != nil {
		t.Fatal(err)
	}
	if result := associationNative(t, s, request); result.Status != "ok" {
		t.Fatal("operation failed", result.Code)
	}
	// A later browser's decision must survive replay of an acknowledged batch.
	associationPatchFixture(t, s, initial, map[string]bool{hash: true})
	if result := associationNative(t, s, request); result.Status != "ok" || !(*result.Data.Associations)[hash] {
		t.Fatal("replayed batch overwrote a newer browser decision", result.Code)
	}
	changed := map[string]bool{hash: true}
	reused := request
	reused.AssociationPatch = &changed
	if result := associationNative(t, s, reused); result.Code != "invalid_request" {
		t.Fatal("accepted one operation id with different contents", result.Code)
	}
	if len(s.prefs.BrowserAssociations.Operations) != 1 {
		t.Fatal("operation receipt was not recorded exactly once")
	}
	_ = s.Close()
	reopened := newSyncServiceFixture(t, s.directory, &syncMemorySecrets{}, cloudsync.New(nil))
	associationFixtureVault(t, reopened, directory)
	if result := associationNative(t, reopened, request); result.Status != "ok" || !(*result.Data.Associations)[hash] {
		t.Fatal("restart lost persistent replay protection", result.Code)
	}
	for key := range associationSyncValues(reopened.prefs.BrowserAssociations) {
		if strings.Contains(key, id) {
			t.Fatal("operation receipts were uploaded to cloud")
		}
	}
}

func TestBrowserAssociationOperationValidationAndReceiptCapacity(t *testing.T) {
	s := newSyncServiceFixture(t, "", &syncMemorySecrets{}, cloudsync.New(nil))
	associationFixtureVault(t, s, "")
	initial := associationRead(t, s)
	for _, extra := range []string{
		`"associationStoreId":null`,
		`"associationOperationId":null`,
		`"associationStoreId":"bad"`,
		`"associationStoreId":"` + initial.AssociationStoreID + `","associationOperationId":"` + strings.Repeat("d", 64) + `"`,
		`"associationScope":"` + initial.AssociationScope + `","associationOperationId":"` + strings.Repeat("d", 64) + `","associationPatch":{}`,
	} {
		raw := []byte(`{"protocol":1,"action":"browser-associations","vaultId":"` + s.prefs.Current + `",` + extra + `}`)
		var response Response
		_ = json.Unmarshal(s.handleBrowser(raw), &response)
		if response.Code != "invalid_request" {
			t.Fatal("accepted malformed operation request", string(raw), response.Code)
		}
	}
	s.prefs.BrowserAssociations.Operations = map[string]string{}
	for i := 0; i < maxAssociationOperations; i++ {
		s.prefs.BrowserAssociations.Operations[fmt.Sprintf("%064x", i)] = strings.Repeat("a", 64)
	}
	if err := s.saveBrowserAssociations(); err != nil {
		t.Fatal(err)
	}
	patch := map[string]bool{strings.Repeat("c", 64): true}
	response := associationNative(t, s, Request{Action: "browser-associations", VaultID: s.prefs.Current, AssociationScope: initial.AssociationScope, AssociationStoreID: initial.AssociationStoreID, AssociationOperationID: strings.Repeat("f", 64), AssociationPatch: &patch})
	if response.Code != "invalid_request" || len(s.prefs.BrowserAssociations.Operations) != maxAssociationOperations || len(*associationRead(t, s).Associations) != 0 {
		t.Fatal("receipt capacity evicted old replay protection or applied a rejected operation", response.Code)
	}
}

func TestBrowserAssociationsWriteFailureAndMalformedFilePreserveState(t *testing.T) {
	s := newSyncServiceFixture(t, "", &syncMemorySecrets{}, cloudsync.New(nil))
	associationFixtureVault(t, s, "")
	initial := associationRead(t, s)
	file := filepath.Join(s.directory, browserAssociationFile)
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(file, file+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	patch := map[string]bool{strings.Repeat("a", 64): true}
	r := associationNative(t, s, Request{Action: "browser-associations", VaultID: s.prefs.Current, AssociationScope: initial.AssociationScope, AssociationPatch: &patch})
	if r.Code != "local_storage" || len(s.prefs.BrowserAssociations.Scopes[initial.AssociationScope].Associations) != 0 {
		t.Fatal("failed write mutated current state", r)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(before), `"associations": {}`, `"associations": {"`+strings.Repeat("a", 64)+`":null}`, 1)
	if broken == string(before) {
		t.Fatal("fixture did not inject malformed value")
	}
	if err := os.WriteFile(file, []byte(broken), 0600); err != nil {
		t.Fatal(err)
	}
	if reopened, err := New(Options{DataDir: s.directory}); err == nil {
		_ = reopened.Close()
		t.Fatal("malformed association file was accepted")
	}
	after, _ := os.ReadFile(file)
	if string(after) != broken {
		t.Fatal("malformed association file was overwritten")
	}
}
