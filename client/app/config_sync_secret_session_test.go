package app

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/knothhe/passpine/client/cloudsync"
	"github.com/knothhe/passpine/client/secretstore"
	"github.com/knothhe/r2connect"
)

// This backend counts actual credential-store operations without opening a
// system keyring. The real Store supplies its normal read-session behavior.
type syncSessionBackend struct {
	mu           sync.Mutex
	values       map[string][]byte
	loads        int
	saves        int
	deletes      int
	failNextSave error
}

func (b *syncSessionBackend) Load(service, key string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.loads++
	value, ok := b.values[service+"/"+key]
	if !ok {
		return nil, secretstore.ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (b *syncSessionBackend) Save(service, key string, value []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.saves++
	if err := b.failNextSave; err != nil {
		b.failNextSave = nil
		return err
	}
	b.values[service+"/"+key] = append([]byte(nil), value...)
	return nil
}

func (b *syncSessionBackend) Delete(service, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deletes++
	delete(b.values, service+"/"+key)
	return nil
}

func (b *syncSessionBackend) counts() (loads, saves, deletes int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.loads, b.saves, b.deletes
}

func newSyncSecretSessionFixture(t *testing.T) (*Service, *secretstore.Store, *syncSessionBackend, cloudsync.Config, cloudsync.Secrets) {
	t.Helper()
	s, _, _, config, secrets, _ := newSyncAppSpace(t)
	backend := &syncSessionBackend{values: make(map[string][]byte)}
	store, err := secretstore.NewWithBackend("dev", s.directory, backend)
	if err != nil {
		t.Fatal(err)
	}
	s.configSync.store = store
	return s, store, backend, config, secrets
}

func TestConfigSyncUnchangedBaselineReusesNativeSecretRead(t *testing.T) {
	s, _, backend, config, secrets := newSyncSecretSessionFixture(t)
	data := saveSyncFixture(t, s, config, secrets)
	loads, saves, deletes := backend.counts()
	must(t, s, Request{Action: "config-sync-now"})
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads+1 || gotSaves != saves || gotDeletes != deletes {
		t.Fatalf("first sync after save = %d reads/%d writes/%d deletes; want %d/%d/%d", gotLoads, gotSaves, gotDeletes, loads+1, saves, deletes)
	}
	loads++
	for range 3 {
		must(t, s, Request{Action: "config-sync-now"})
	}
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads || gotSaves != saves || gotDeletes != deletes {
		t.Fatal("unchanged sync reread or rewrote the native credentials")
	}

	settings := data.Settings
	settings.Theme = "dark"
	must(t, s, Request{Action: "settings", Settings: &settings})
	must(t, s, Request{Action: "config-sync-now"})
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads || gotSaves != saves+1 || gotDeletes != deletes {
		t.Fatal("changed baseline was not persisted using the existing read session")
	}
	saves++
	// Real writes must still invalidate the affected read; the next operation
	// must verify the value through the native backend before reusing it.
	must(t, s, Request{Action: "config-sync-now"})
	loads++
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads || gotSaves != saves || gotDeletes != deletes {
		t.Fatal("baseline update skipped native readback or rewrote an unchanged value")
	}
	must(t, s, Request{Action: "config-sync-now"})
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads || gotSaves != saves || gotDeletes != deletes {
		t.Fatal("verified baseline did not remain reusable")
	}
}

func TestConfigSyncPendingBaselineStillRetriesNativeSave(t *testing.T) {
	s, _, backend, config, secrets := newSyncSecretSessionFixture(t)
	data := saveSyncFixture(t, s, config, secrets)
	must(t, s, Request{Action: "config-sync-now"})
	settings := data.Settings
	settings.Theme = "dark"
	must(t, s, Request{Action: "settings", Settings: &settings})
	backend.mu.Lock()
	backend.failNextSave = secretstore.ErrUnavailable
	backend.mu.Unlock()
	loads, saves, deletes := backend.counts()
	response := s.Execute(Request{Action: "config-sync-now"})
	if response.Status != "error" || response.Code != "credentials_unavailable" || len(s.configSync.pendingSecrets) != 1 {
		t.Fatalf("failed baseline save did not remain pending: %s %s", response.Status, response.Code)
	}
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads || gotSaves != saves+1 || gotDeletes != deletes {
		t.Fatal("baseline failure did not attempt exactly one native save")
	}
	// loadSyncSecret now returns the pending baseline. It equals the result of
	// the next sync, but that must not suppress retrying its failed write.
	must(t, s, Request{Action: "config-sync-now"})
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads || gotSaves != saves+2 || gotDeletes != deletes || len(s.configSync.pendingSecrets) != 0 {
		t.Fatal("unchanged pending baseline bypassed native persistence retry")
	}
	must(t, s, Request{Action: "config-sync-now"})
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads+1 || gotSaves != saves+2 || gotDeletes != deletes {
		t.Fatal("successful pending write was not read back from native storage")
	}
}

func TestConfigSyncCloseClearsNativeReadSessionAndPreservesSavedValue(t *testing.T) {
	s, store, backend, config, secrets := newSyncSecretSessionFixture(t)
	saveSyncFixture(t, s, config, secrets)
	must(t, s, Request{Action: "config-sync-now"})
	profile := s.configSync.meta.Profiles["s3"]
	loads, saves, deletes := backend.counts()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(profile.SecretRef); err != nil {
		t.Fatal("closing removed saved credentials", err)
	}
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads+1 || gotSaves != saves || gotDeletes != deletes {
		t.Fatal("closing retained a reusable secret read or changed saved credentials")
	}
}

func newSyncOAuthSecretSessionFixture(t *testing.T) (*Service, *secretstore.Store, *syncSessionBackend, *oauthAPI) {
	t.Helper()
	s, _, api, _ := oauthFixture(t)
	backend := &syncSessionBackend{values: make(map[string][]byte)}
	store, err := secretstore.NewWithBackend("dev", s.directory, backend)
	if err != nil {
		t.Fatal(err)
	}
	s.configSync.store = store
	s.configSync.manager, s.configSync.client = nil, nil
	if err := s.initSyncSDK(); err != nil {
		t.Fatal(err)
	}
	return s, store, backend, api
}

func TestConfigSyncR2AccountBucketAndCredentialReadsShareNativeSession(t *testing.T) {
	s, _, backend, _ := newSyncOAuthSecretSessionFixture(t)
	authorizeOAuthCandidate(t, s)
	loads, saves, deletes := backend.counts()
	if err := s.resumeSyncOAuth(t.Context()); err != nil {
		t.Fatal(err)
	}
	loads++
	request := &SyncConnectionRequest{AccountID: oauthTestAccount, Jurisdiction: "default"}
	for range 3 {
		if err := s.resumeSyncOAuth(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := s.listSyncBuckets(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads || gotSaves != saves || gotDeletes != deletes {
		t.Fatal("account and bucket requests repeatedly read the OAuth grant")
	}
	profile, value := prepareOAuthCandidate(t, s, "chosen-bucket")
	// Connect writes one bucket credential; the app's subsequent Credentials
	// lookup verifies that native value rather than seeding a cache from Save.
	loads++
	saves++
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads || gotSaves != saves || gotDeletes != deletes {
		t.Fatal("new bucket credential was not read back exactly once")
	}
	for range 3 {
		if err := s.renewR2Credentials(t.Context(), &profile, &value); err != nil {
			t.Fatal(err)
		}
		if err := s.listSyncBuckets(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads || gotSaves != saves || gotDeletes != deletes {
		t.Fatal("valid bucket credentials did not share the successful read session")
	}
}

func TestConfigSyncCachedR2SecretsStillRefreshAndRenewThroughSDK(t *testing.T) {
	s, store, backend, api := newSyncOAuthSecretSessionFixture(t)
	authorizeOAuthCandidate(t, s)
	profile, value := prepareOAuthCandidate(t, s, "chosen-bucket")
	oldAccessKey := value.Secrets.AccessKeyID
	connectionKey := s.configSync.manager.State().Connections[0].SecretKey()
	data, err := store.Load(connectionKey)
	if err != nil {
		t.Fatal(err)
	}
	var credentials r2connect.Credentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		t.Fatal(err)
	}
	clear(data)
	credentials.ExpiresAt = time.Now().Add(-time.Hour)
	data, err = json.Marshal(credentials)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(connectionKey, data); err != nil {
		t.Fatal(err)
	}
	clear(data)
	// Warm both expired values through genuine native reads. Their presence
	// in the process cache must not bypass SDK expiry or refresh checks.
	data, err = store.Load(connectionKey)
	if err != nil {
		t.Fatal(err)
	}
	clear(data)
	tokens := oauthTokens()
	tokens.ExpiresAt = time.Now().Add(-time.Hour)
	data, err = json.Marshal(tokens)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(s.configSync.manager.OAuthSecretKey(), data); err != nil {
		t.Fatal(err)
	}
	clear(data)
	data, err = store.Load(s.configSync.manager.OAuthSecretKey())
	if err != nil {
		t.Fatal(err)
	}
	clear(data)
	loads, saves, deletes := backend.counts()
	creates, remoteDeletes, revokes, refreshes := api.counts()
	if err := s.renewR2Credentials(context.Background(), &profile, &value); err != nil {
		t.Fatal(err)
	}
	if value.Secrets.AccessKeyID == oldAccessKey {
		t.Fatal("expired cached credential was returned instead of renewed")
	}
	if gotCreates, gotDeletes, gotRevokes, gotRefreshes := api.counts(); gotCreates != creates+1 || gotDeletes != remoteDeletes+1 || gotRevokes != revokes || gotRefreshes != refreshes+1 {
		t.Fatal("cached expiry did not trigger SDK refresh, renewal, and old-token cleanup")
	}
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads || gotSaves != saves+2 || gotDeletes != deletes+1 {
		t.Fatal("SDK renewal bypassed the cached reads or failed to persist rotated secrets")
	}
	// Rotation must invalidate both entries. Each new value is read once,
	// then later requests reuse it while still going through the SDK.
	for range 2 {
		if err := s.renewR2Credentials(t.Context(), &profile, &value); err != nil {
			t.Fatal(err)
		}
		if err := s.resumeSyncOAuth(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads+2 || gotSaves != saves+2 || gotDeletes != deletes+1 {
		t.Fatal("rotated secrets were not natively verified once and then reused")
	}
}
