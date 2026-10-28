package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/knothhe/passpine/client/cloudsync"
	"github.com/knothhe/passpine/client/secretstore"
	"github.com/knothhe/r2connect"
)

// Only the native master/legacy boundary is simulated. Encryption, migration,
// SDK state persistence and the application service use their real code paths.
type encryptedSyncNative struct {
	mu            sync.Mutex
	values        map[string][]byte
	loads         int
	saves         int
	deletes       int
	denyDeleteKey string
}

func (b *encryptedSyncNative) Load(service, key string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.loads++
	value, exists := b.values[service+"/"+key]
	if !exists {
		return nil, secretstore.ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (b *encryptedSyncNative) Save(service, key string, value []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.saves++
	b.values[service+"/"+key] = append([]byte(nil), value...)
	return nil
}

func (b *encryptedSyncNative) Delete(service, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deletes++
	if key == b.denyDeleteKey {
		b.denyDeleteKey = ""
		return secretstore.ErrDenied
	}
	if _, exists := b.values[service+"/"+key]; !exists {
		return secretstore.ErrNotFound
	}
	delete(b.values, service+"/"+key)
	return nil
}

func (b *encryptedSyncNative) counts() (int, int, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.loads, b.saves, b.deletes
}

func encryptedSyncEngine(t *testing.T) *cloudsync.Engine {
	engine, _ := encryptedSyncEngineFixture(t)
	return engine
}

func encryptedSyncEngineFixture(t *testing.T) (*cloudsync.Engine, *syncTestBucket) {
	t.Helper()
	bucket := &syncTestBucket{}
	server := httptest.NewServer(http.HandlerFunc(bucket.serve))
	t.Cleanup(server.Close)
	destination, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return cloudsync.New(&http.Client{Transport: syncReviewRoundTripper(func(request *http.Request) (*http.Response, error) {
		forward := request.Clone(request.Context())
		address := *request.URL
		address.Scheme, address.Host = destination.Scheme, destination.Host
		forward.URL = &address
		return http.DefaultTransport.RoundTrip(forward)
	})}), bucket
}

func installEncryptedSyncStore(t *testing.T, s *Service, native *encryptedSyncNative, apiURL string) *secretstore.Store {
	t.Helper()
	if err := os.Chmod(s.directory, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := secretstore.NewWithEncryptedBackend("dev", s.directory, native)
	if err != nil {
		t.Fatal(err)
	}
	s.configSync.store = store
	s.configSync.manager, s.configSync.client = nil, nil
	s.configSync.managerFactory = func(clientID string, state r2connect.StateStore) (*r2connect.Manager, *r2connect.Client, error) {
		client := r2connect.NewClient(clientID)
		client.APIBaseURL, client.OAuthBaseURL = apiURL, apiURL
		manager := r2connect.NewManager(client, r2connect.ManagerOptions{
			Secrets: s.configSync.store, State: state, TokenNamePrefix: "Passpine-",
			OnWarning: func(r2connect.Warning) {
				s.mu.Lock()
				s.configSync.warningCode = "oauth_cleanup_pending"
				s.mu.Unlock()
			},
		})
		return manager, client, manager.LoadError()
	}
	if err := s.initSyncSDK(); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestConfigSyncEncryptedStoreR2FlowAndRestartReadOneNativeMaster(t *testing.T) {
	s, _, api, redirect := oauthFixture(t)
	apiURL := s.configSync.client.APIBaseURL
	engine := encryptedSyncEngine(t)
	s.configSync.engine = engine
	native := &encryptedSyncNative{values: make(map[string][]byte)}
	installEncryptedSyncStore(t, s, native, apiURL)
	response := s.Execute(Request{Action: "config-sync-oauth-start", SyncConnection: &SyncConnectionRequest{OpenURL: func(raw string) error {
		address, err := url.Parse(raw)
		if err != nil {
			return err
		}
		callback, err := http.Get(redirect + "?" + url.Values{"state": {address.Query().Get("state")}, "code": {"isolated-code"}}.Encode())
		if err == nil {
			callback.Body.Close()
		}
		return err
	}}})
	if response.Status != "ok" || response.Data.ConfigSync == nil || !response.Data.ConfigSync.OAuthAuthorized {
		t.Fatalf("encrypted-store authorization failed: %s %s", response.Status, response.Code)
	}
	response = s.Execute(Request{Action: "config-sync-oauth-buckets", SyncConnection: &SyncConnectionRequest{AccountID: oauthTestAccount, Jurisdiction: "default"}})
	if response.Status != "ok" || len(response.Data.ConfigSync.Buckets) == 0 {
		t.Fatalf("encrypted-store bucket selection failed: %s %s", response.Status, response.Code)
	}
	request := &SyncConnectionRequest{
		Provider: "r2", AccountID: oauthTestAccount, Jurisdiction: "default", Bucket: "chosen-bucket",
		Config: cloudsync.Config{Prefix: "isolated-encrypted-store"}, Secrets: cloudsync.Secrets{Passphrase: "isolated encryption passphrase"},
	}
	response = s.Execute(Request{Action: "config-sync-save", SyncConnection: request})
	if response.Status != "ok" || !response.Data.ConfigSync.Ready {
		t.Fatalf("encrypted-store R2 save failed: %s %s", response.Status, response.Code)
	}
	connectionID := s.configSync.meta.Profiles["r2"].Connection.ID
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	beforeLoads, _, _ := native.counts()
	// A fresh Service reads the real app and SDK metadata saved by the first
	// instance. There is no copied in-memory Manager or decrypted secret cache.
	reopened := newSyncServiceFixture(t, s.directory, &syncMemorySecrets{values: make(map[string][]byte)}, engine)
	installEncryptedSyncStore(t, reopened, native, apiURL)
	if loads, _, _ := native.counts(); loads != beforeLoads {
		t.Fatal("loading public app/SDK metadata opened the native secret store")
	}
	for range 3 {
		must(t, reopened, Request{Action: "config-sync-now"})
		must(t, reopened, Request{Action: "config-sync-oauth-accounts"})
		must(t, reopened, Request{Action: "config-sync-oauth-buckets", SyncConnection: &SyncConnectionRequest{AccountID: oauthTestAccount, Jurisdiction: "default"}})
	}
	if loads, _, _ := native.counts(); loads != beforeLoads+1 {
		t.Fatalf("reopened R2 operations read %d native items; want one master", loads-beforeLoads)
	}
	if saved := reopened.configSync.meta.Profiles["r2"]; saved.Connection == nil || saved.Connection.ID != connectionID {
		t.Fatal("encrypted-store restart changed the saved SDK connection identity")
	}
	must(t, reopened, Request{Action: "config-sync-disconnect"})
	creates, deletes, revokes, _ := api.counts()
	if creates != 1 || deletes != 1 || revokes != 1 || reopened.configSync.meta.Active != "" {
		t.Fatal("encrypted-store lifecycle lost remote token cleanup ownership")
	}
}

func TestConfigSyncLegacyR2MigrationDenialKeepsCleanupAndStopsBackgroundRetry(t *testing.T) {
	legacy, memory, api, _ := oauthFixture(t)
	apiURL := legacy.configSync.client.APIBaseURL
	authorizeOAuthCandidate(t, legacy)
	profile, value := prepareOAuthCandidate(t, legacy, "chosen-bucket")
	activateOAuthCandidate(t, legacy, profile, value)
	oauthKey := legacy.configSync.manager.OAuthSecretKey()
	native := &encryptedSyncNative{values: make(map[string][]byte)}
	service, err := secretstore.Namespace("dev", legacy.directory)
	if err != nil {
		t.Fatal(err)
	}
	memory.mu.Lock()
	for key, data := range memory.values {
		native.values[service+"/"+key] = append([]byte(nil), data...)
	}
	memory.mu.Unlock()
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}
	engine := encryptedSyncEngine(t)
	reopened := newSyncServiceFixture(t, legacy.directory, &syncMemorySecrets{values: make(map[string][]byte)}, engine)
	store := installEncryptedSyncStore(t, reopened, native, apiURL)
	// The ordinary sync lazily migrates app passphrase/baseline and bucket
	// credentials; its valid S3 pair does not need the OAuth grant yet.
	must(t, reopened, Request{Action: "config-sync-now"})
	native.mu.Lock()
	_, appStillLegacy := native.values[service+"/"+profile.SecretRef]
	_, bucketStillLegacy := native.values[service+"/"+profile.Connection.SecretKey()]
	_, oauthStillLegacy := native.values[service+"/"+oauthKey]
	native.denyDeleteKey = oauthKey
	native.mu.Unlock()
	if appStillLegacy || bucketStillLegacy || !oauthStillLegacy {
		t.Fatal("legacy values were dropped or migrated outside the requested operation")
	}
	response := reopened.Execute(Request{Action: "config-sync-oauth-accounts"})
	if response.Status != "error" || response.Code != "credentials_denied" || !store.Denied() {
		t.Fatalf("last legacy-delete refusal was not latched: %s %s", response.Status, response.Code)
	}
	if saved := reopened.configSync.meta.Profiles["r2"]; saved.Connection == nil || saved.Connection.ID != profile.Connection.ID || reopened.configSync.manager.OAuthSecretKey() != oauthKey {
		t.Fatal("migration refusal lost app or SDK cleanup references")
	}
	beforeLoads, beforeSaves, beforeDeletes := native.counts()
	response = reopened.Execute(Request{Action: "config-sync-now"})
	if response.Status != "error" || response.Code != "credentials_denied" {
		t.Fatal("background-equivalent sync bypassed migration denial", response.Code)
	}
	must(t, reopened, Request{Action: "config-sync-status"})
	reopened.configSyncMu.Lock()
	reopened.mu.Lock()
	reopened.configSync.next = time.Time{}
	reopened.configSync.status.ErrorCode = ""
	reopened.mu.Unlock()
	reopened.scheduleConfigSync()
	reopened.mu.Lock()
	queued := reopened.configSync.queued
	reopened.mu.Unlock()
	reopened.configSyncMu.Unlock()
	if queued {
		t.Fatal("automatic sync queued another native prompt after migration refusal")
	}
	if loads, saves, deletes := native.counts(); loads != beforeLoads || saves != beforeSaves || deletes != beforeDeletes {
		t.Fatal("status or automatic retry touched the native store after migration refusal")
	}
	// Explicit disconnect is allowed to reopen access and finish both legacy
	// deletion and the SDK's original remote token/revocation cleanup.
	must(t, reopened, Request{Action: "config-sync-disconnect"})
	creates, deletes, revokes, _ := api.counts()
	native.mu.Lock()
	_, oauthStillLegacy = native.values[service+"/"+oauthKey]
	native.mu.Unlock()
	if creates != 1 || deletes != 1 || revokes != 1 || oauthStillLegacy || reopened.configSync.meta.Active != "" {
		t.Fatal("explicit disconnect failed to recover preserved legacy/remote cleanup")
	}
}

func TestConfigSyncEncryptedFileChangeStopsSDKRenewalFallbackBeforeObjectIO(t *testing.T) {
	s, _, api, _ := oauthFixture(t)
	apiURL := s.configSync.client.APIBaseURL
	engine, bucket := encryptedSyncEngineFixture(t)
	s.configSync.engine = engine
	native := &encryptedSyncNative{values: make(map[string][]byte)}
	store := installEncryptedSyncStore(t, s, native, apiURL)
	authorizeOAuthCandidate(t, s)
	profile, value := prepareOAuthCandidate(t, s, "chosen-bucket")
	activateOAuthCandidate(t, s, profile, value)
	must(t, s, Request{Action: "config-sync-now"})

	connection := s.configSync.manager.State().Connections[0]
	data, err := store.Load(connection.SecretKey())
	if err != nil {
		t.Fatal(err)
	}
	var credentials r2connect.Credentials
	if err = json.Unmarshal(data, &credentials); err != nil {
		t.Fatal(err)
	}
	clear(data)
	// Within the SDK renewal window but beyond its expiry margin: a failed
	// early renewal deliberately returns the still-valid cached credentials.
	credentials.ExpiresAt = time.Now().Add(time.Hour)
	data, err = json.Marshal(credentials)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Save(connection.SecretKey(), data); err != nil {
		t.Fatal(err)
	}
	clear(data)
	for _, key := range []string{connection.SecretKey(), s.configSync.manager.OAuthSecretKey()} {
		data, err = store.Load(key)
		if err != nil {
			t.Fatal(err)
		}
		clear(data)
	}
	_, _, beforeObjectRequests := bucket.snapshot()
	beforeCreates, _, _, _ := api.counts()
	var mutate sync.Once
	var mutationErr error
	mutated := false
	s.configSync.client.HTTPClient = &http.Client{Transport: syncReviewRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/tokens") {
			mutate.Do(func() {
				// Change real encrypted storage after Credentials has loaded its
				// valid old pair, while Cloudflare creates the replacement token.
				mutationErr = os.WriteFile(filepath.Join(s.directory, "sync-secrets-v1.enc"), []byte("isolated corrupt ciphertext"), 0600)
				mutated = mutationErr == nil
			})
			if mutationErr != nil {
				return nil, mutationErr
			}
		}
		return http.DefaultTransport.RoundTrip(request)
	})}
	response := s.Execute(Request{Action: "config-sync-now"})
	if !mutated || mutationErr != nil {
		t.Fatal("renewal did not reach the isolated encrypted-file change", mutationErr)
	}
	if response.Status != "error" || response.Code != "credentials_state_changed" {
		t.Fatalf("SDK fallback concealed changed encrypted storage: %s %s", response.Status, response.Code)
	}
	if creates, _, _, _ := api.counts(); creates != beforeCreates+1 {
		t.Fatal("test did not exercise actual SDK early renewal")
	}
	if store.Denied() || s.configSync.manager.LoadError() != nil {
		t.Fatal("test stopped through native denial or failed SDK initialization")
	}
	if err = s.ensureSyncManager(); err != nil {
		t.Fatal("public SDK metadata unexpectedly stopped the fallback", err)
	}
	if _, _, requests := bucket.snapshot(); requests != beforeObjectRequests {
		t.Fatal("SDK early-renew fallback reached object storage after encrypted-file corruption")
	}
}
