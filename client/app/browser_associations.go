package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/knothhe/passpine/client/cloudsync"
)

const associationValuePrefix = "browser.associations."
const associationAliasPrefix = "browser.associationAliases."
const associationLegacyPrefix = "browser.associationLegacy."
const maxAssociationValues = 2048
const maxAssociationAliases = 256
const maxAssociationScopes = 128
const maxAssociationOperations = 4096

type browserAssociationScope struct {
	Associations map[string]bool `json:"associations"`
	VaultIDs     []string        `json:"vaultIds"`
}

type browserAssociationStore struct {
	Version    int                                `json:"version"`
	StoreID    string                             `json:"storeId"`
	Legacy     map[string]bool                    `json:"legacy"`
	Scopes     map[string]browserAssociationScope `json:"scopes"`
	Operations map[string]string                  `json:"operations,omitempty"`
}

// encoding/json accepts null as false for bools. Persisted association values
// must retain the same strict bool contract as native messages and cloud data.
func (scope *browserAssociationScope) UnmarshalJSON(encoded []byte) error {
	var raw struct {
		Associations map[string]json.RawMessage `json:"associations"`
		VaultIDs     []string                   `json:"vaultIds"`
	}
	if json.Unmarshal(encoded, &raw) != nil || raw.Associations == nil {
		return &associationError{"invalid_document"}
	}
	values := map[string]bool{}
	for key, value := range raw.Associations {
		if string(value) != "true" && string(value) != "false" {
			return &associationError{"invalid_document"}
		}
		values[key] = string(value) == "true"
	}
	scope.Associations, scope.VaultIDs = values, raw.VaultIDs
	return nil
}

func (store *browserAssociationStore) UnmarshalJSON(encoded []byte) error {
	type plain browserAssociationStore
	var decoded plain
	if json.Unmarshal(encoded, &decoded) != nil || !validAssociationStore((*browserAssociationStore)(&decoded)) {
		return &associationError{"invalid_document"}
	}
	var raw struct {
		Legacy map[string]json.RawMessage `json:"legacy"`
	}
	if json.Unmarshal(encoded, &raw) != nil {
		return &associationError{"invalid_document"}
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &fields)
	if value, present := fields["legacy"]; present && string(value) == "null" {
		return &associationError{"invalid_document"}
	}
	if value, present := fields["operations"]; present && string(value) == "null" {
		return &associationError{"invalid_document"}
	}
	for _, value := range raw.Legacy {
		if string(value) != "true" && string(value) != "false" {
			return &associationError{"invalid_document"}
		}
	}
	*store = browserAssociationStore(decoded)
	return nil
}

type associationError struct{ code string }

func (e *associationError) Error() string { return e.code }

func associationHex(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func associationVaultID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func validAssociationMap(values map[string]bool) bool {
	if values == nil || len(values) > maxAssociationValues {
		return false
	}
	for key := range values {
		if !associationHex(key) {
			return false
		}
	}
	return true
}

func validAssociationStore(store *browserAssociationStore) bool {
	if store == nil {
		return true
	}
	if store.Version != 1 || !associationHex(store.StoreID) || store.Scopes == nil || len(store.Scopes) > maxAssociationScopes {
		return false
	}
	if len(store.Operations) > maxAssociationOperations {
		return false
	}
	for id, digest := range store.Operations {
		if !associationHex(id) || !associationHex(digest) {
			return false
		}
	}
	if store.Legacy != nil && !validAssociationMap(store.Legacy) {
		return false
	}
	values, aliases := len(store.Legacy), 0
	for scope, data := range store.Scopes {
		if !associationHex(scope) || !validAssociationMap(data.Associations) {
			return false
		}
		seen := map[string]bool{}
		for _, id := range data.VaultIDs {
			if !associationVaultID(id) || seen[id] {
				return false
			}
			seen[id] = true
		}
		values += len(data.Associations)
		aliases += len(data.VaultIDs)
	}
	return values <= maxAssociationValues && aliases <= maxAssociationAliases
}

func cloneAssociationStore(store *browserAssociationStore) (*browserAssociationStore, error) {
	if !validAssociationStore(store) {
		return nil, &associationError{"invalid_document"}
	}
	if store == nil {
		var id [32]byte
		if _, err := rand.Read(id[:]); err != nil {
			return nil, &associationError{"local_storage"}
		}
		return &browserAssociationStore{Version: 1, StoreID: hex.EncodeToString(id[:]), Legacy: map[string]bool{}, Scopes: map[string]browserAssociationScope{}}, nil
	}
	next := *store
	next.Operations = maps.Clone(store.Operations)
	next.Legacy = maps.Clone(store.Legacy)
	if next.Legacy == nil {
		next.Legacy = map[string]bool{}
	}
	next.Scopes = map[string]browserAssociationScope{}
	for scope, data := range store.Scopes {
		next.Scopes[scope] = browserAssociationScope{Associations: maps.Clone(data.Associations), VaultIDs: slices.Clone(data.VaultIDs)}
	}
	return &next, nil
}

// Called under the service lock. Imports only fill absent values; explicit
// patches are authoritative, including false tombstones for inferred matches.
func (s *Service) browserAssociations(r Request) (Data, error) {
	if err := s.requireVault(); err != nil {
		return Data{}, err
	}
	if r.VaultID == "" || r.VaultID != s.prefs.Current {
		return Data{}, errors.New("vault_changed")
	}
	scope, err := s.active.BrowserAssociationScope(r.VaultID)
	if err != nil {
		return Data{}, &associationError{"local_storage"}
	}
	if r.AssociationScope != "" && r.AssociationScope != scope {
		return Data{}, errors.New("vault_changed")
	}
	if (r.AssociationPatch != nil || r.AssociationImport != nil) && r.AssociationScope == "" {
		return Data{}, &associationError{"invalid_request"}
	}
	next, err := cloneAssociationStore(s.prefs.BrowserAssociations)
	if err != nil {
		return Data{}, err
	}
	if r.AssociationStoreID != "" && r.AssociationStoreID != next.StoreID {
		return Data{}, errors.New("vault_changed")
	}
	if r.AssociationOperationID != "" {
		if r.AssociationPatch == nil || r.AssociationImport != nil || r.AssociationStoreID == "" || !associationHex(r.AssociationOperationID) {
			return Data{}, &associationError{"invalid_request"}
		}
		encoded := jsonValue(struct {
			Scope string          `json:"scope"`
			Patch map[string]bool `json:"patch"`
		}{scope, *r.AssociationPatch})
		digest := sha256.Sum256(encoded)
		operationDigest := hex.EncodeToString(digest[:])
		if previous, present := next.Operations[r.AssociationOperationID]; present {
			if previous != operationDigest {
				return Data{}, &associationError{"invalid_request"}
			}
			return associationData(next, scope), nil
		}
		if len(next.Operations) >= maxAssociationOperations {
			return Data{}, &associationError{"invalid_request"}
		}
		if next.Operations == nil {
			next.Operations = map[string]string{}
		}
		next.Operations[r.AssociationOperationID] = operationDigest
	}
	data, ok := next.Scopes[scope]
	if !ok {
		data = browserAssociationScope{Associations: map[string]bool{}, VaultIDs: []string{}}
	}
	if !slices.Contains(data.VaultIDs, r.VaultID) {
		data.VaultIDs = append(data.VaultIDs, r.VaultID)
		slices.Sort(data.VaultIDs)
	}
	for _, update := range []struct {
		values     *map[string]bool
		importOnly bool
	}{{r.AssociationImport, true}, {r.AssociationPatch, false}} {
		if update.values == nil {
			continue
		}
		if !validAssociationMap(*update.values) {
			return Data{}, &associationError{"invalid_request"}
		}
		for key, value := range *update.values {
			if update.importOnly {
				if _, exists := next.Legacy[key]; !exists {
					next.Legacy[key] = value
				}
			} else {
				data.Associations[key] = value
			}
		}
	}
	next.Scopes[scope] = data
	if !validAssociationStore(next) {
		return Data{}, &associationError{"invalid_request"}
	}
	if string(jsonValue(next)) != string(jsonValue(s.prefs.BrowserAssociations)) {
		previous := s.prefs.BrowserAssociations
		s.prefs.BrowserAssociations = next
		if err := s.saveBrowserAssociations(); err != nil {
			s.prefs.BrowserAssociations = previous
			return Data{}, &associationError{"local_storage"}
		}
		s.markConfigSyncDirty()
	}
	return associationData(next, scope), nil
}

func associationData(store *browserAssociationStore, scope string) Data {
	data := store.Scopes[scope]
	associations := maps.Clone(store.Legacy)
	if associations == nil {
		associations = map[string]bool{}
	}
	maps.Copy(associations, data.Associations)
	return Data{AssociationStoreID: store.StoreID, AssociationScope: scope, AssociationVaultIDs: slices.Clone(data.VaultIDs), Associations: &associations}
}

func hasBrowserAssociationFields(raw json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return true
	}
	for _, key := range []string{"associationScope", "associationPatch", "associationImport", "associationStoreId", "associationOperationId"} {
		if _, ok := fields[key]; ok {
			return true
		}
	}
	return false
}

func validBrowserAssociationRequest(raw json.RawMessage, r Request) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || !associationVaultID(r.VaultID) {
		return false
	}
	for key := range fields {
		switch key {
		case "protocol", "action", "requestId", "vaultId", "associationScope", "associationPatch", "associationImport", "associationStoreId", "associationOperationId":
		default:
			return false
		}
	}
	if _, present := fields["associationScope"]; present && !associationHex(r.AssociationScope) {
		return false
	}
	if _, present := fields["associationStoreId"]; present && !associationHex(r.AssociationStoreID) {
		return false
	}
	if _, present := fields["associationOperationId"]; present {
		if !associationHex(r.AssociationOperationID) || r.AssociationStoreID == "" || r.AssociationPatch == nil || r.AssociationImport != nil || r.AssociationScope == "" {
			return false
		}
	}
	for _, key := range []string{"associationPatch", "associationImport"} {
		rawMap, present := fields[key]
		if !present {
			continue
		}
		var values map[string]json.RawMessage
		if json.Unmarshal(rawMap, &values) != nil || values == nil || len(values) > maxAssociationValues || r.AssociationScope == "" {
			return false
		}
		for hash, value := range values {
			if !associationHex(hash) || string(value) != "true" && string(value) != "false" {
				return false
			}
		}
	}
	return true
}

func associationCloudKey(key string) (scope, record string, alias, ok bool) {
	if strings.HasPrefix(key, associationLegacyPrefix) {
		record = strings.TrimPrefix(key, associationLegacyPrefix)
		ok = associationHex(record)
		return
	}
	prefix := associationValuePrefix
	if strings.HasPrefix(key, associationAliasPrefix) {
		prefix, alias = associationAliasPrefix, true
	} else if !strings.HasPrefix(key, prefix) {
		return
	}
	parts := strings.Split(strings.TrimPrefix(key, prefix), ".")
	if len(parts) != 2 || !associationHex(parts[0]) || (!alias && !associationHex(parts[1])) || (alias && !associationVaultID(parts[1])) {
		return
	}
	return parts[0], parts[1], alias, true
}

func associationSyncValues(store *browserAssociationStore) cloudsync.Values {
	out := cloudsync.Values{}
	if store == nil {
		return out
	}
	for hash, value := range store.Legacy {
		out[associationLegacyPrefix+hash] = jsonValue(value)
	}
	for scope, data := range store.Scopes {
		for hash, value := range data.Associations {
			out[associationValuePrefix+scope+"."+hash] = jsonValue(value)
		}
		for _, id := range data.VaultIDs {
			out[associationAliasPrefix+scope+"."+id] = jsonValue(true)
		}
	}
	return out
}

func validateAssociationSyncValues(values cloudsync.Values) error {
	count, aliases := 0, 0
	scopes := map[string]bool{}
	for key, raw := range values {
		if !strings.HasPrefix(key, associationValuePrefix) && !strings.HasPrefix(key, associationAliasPrefix) && !strings.HasPrefix(key, associationLegacyPrefix) {
			continue
		}
		scope, _, alias, ok := associationCloudKey(key)
		var value bool
		if !ok || (string(raw) != "true" && string(raw) != "false") || json.Unmarshal(raw, &value) != nil || alias && !value {
			return &cloudsync.Error{Code: "invalid_document"}
		}
		if scope != "" {
			scopes[scope] = true
		}
		if alias {
			aliases++
		} else {
			count++
		}
	}
	if count > maxAssociationValues || aliases > maxAssociationAliases || len(scopes) > maxAssociationScopes {
		return &cloudsync.Error{Code: "invalid_document"}
	}
	return nil
}

func (s *Service) mergedAssociationSyncValues(values, current, started cloudsync.Values) (*browserAssociationStore, error) {
	if !s.configSyncOptions().Browser {
		return s.prefs.BrowserAssociations, nil
	}
	if !validAssociationStore(s.prefs.BrowserAssociations) {
		return nil, &cloudsync.Error{Code: "invalid_document"}
	}
	var next *browserAssociationStore
	// A missing merged value removes a previously applied record. Keep any
	// record edited locally after this synchronization started.
	for key, raw := range current {
		scope, record, alias, ok := associationCloudKey(key)
		if !ok || values[key] != nil || string(raw) != string(started[key]) {
			continue
		}
		if next == nil {
			var err error
			next, err = cloneAssociationStore(s.prefs.BrowserAssociations)
			if err != nil {
				return nil, err
			}
		}
		data := next.Scopes[scope]
		if scope == "" {
			delete(next.Legacy, record)
			continue
		}
		if alias {
			data.VaultIDs = slices.DeleteFunc(data.VaultIDs, func(id string) bool { return id == record })
		} else {
			delete(data.Associations, record)
		}
		next.Scopes[scope] = data
	}
	for key, raw := range values {
		scope, record, alias, ok := associationCloudKey(key)
		if !ok {
			continue
		}
		if next == nil {
			var err error
			next, err = cloneAssociationStore(s.prefs.BrowserAssociations)
			if err != nil {
				return nil, err
			}
		}
		if local, present := current[key]; present && string(local) != string(started[key]) {
			raw = local
		}
		if scope == "" {
			var value bool
			_ = json.Unmarshal(raw, &value)
			next.Legacy[record] = value
			continue
		}
		data, present := next.Scopes[scope]
		if !present {
			data = browserAssociationScope{Associations: map[string]bool{}, VaultIDs: []string{}}
		}
		if alias {
			if !slices.Contains(data.VaultIDs, record) {
				data.VaultIDs = append(data.VaultIDs, record)
				slices.Sort(data.VaultIDs)
			}
		} else {
			var value bool
			_ = json.Unmarshal(raw, &value)
			data.Associations[record] = value
		}
		next.Scopes[scope] = data
	}
	if next == nil {
		return s.prefs.BrowserAssociations, nil
	}
	if !validAssociationStore(next) {
		return nil, &cloudsync.Error{Code: "invalid_document"}
	}
	return next, nil
}
