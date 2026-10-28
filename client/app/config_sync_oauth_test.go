package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
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

const oauthTestAccount = "0123456789abcdef0123456789abcdef"
const oauthTestClientID = "11111111111111111111111111111111"
const oauthTestPermissionID = "22222222222222222222222222222222"

type oauthMemorySecrets struct {
	*syncMemorySecrets
	denied         bool
	failNextSave   error
	failNextDelete error
	operations     int
	onSave         func(string)
}

func (m *oauthMemorySecrets) Denied() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.denied }
func (m *oauthMemorySecrets) RetryAccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.denied = false
	m.retries++
}
func (m *oauthMemorySecrets) Load(key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.denied {
		return nil, secretstore.ErrDenied
	}
	m.operations++
	m.loads++
	data, ok := m.values[key]
	if !ok {
		return nil, secretstore.ErrNotFound
	}
	return append([]byte(nil), data...), nil
}
func (m *oauthMemorySecrets) Save(key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.denied {
		return secretstore.ErrDenied
	}
	m.operations++
	if err := m.failNextSave; err != nil {
		m.failNextSave = nil
		if errors.Is(err, secretstore.ErrDenied) {
			m.denied = true
		}
		return err
	}
	m.values[key] = append([]byte(nil), data...)
	if m.onSave != nil {
		m.onSave(key)
	}
	return nil
}
func (m *oauthMemorySecrets) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.denied {
		return secretstore.ErrDenied
	}
	m.operations++
	if err := m.failNextDelete; err != nil {
		m.failNextDelete = nil
		return err
	}
	delete(m.values, key)
	return nil
}
func oauthTokens() r2connect.TokenSet {
	return r2connect.TokenSet{AccessToken: "test-oauth-access", RefreshToken: "test-oauth-refresh", Scope: r2connect.OAuthScopes, ExpiresAt: time.Now().Add(time.Hour)}
}
func oauthEnvelope(w http.ResponseWriter, result any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": result})
}

type oauthAPI struct {
	mu                                   sync.Mutex
	creates, deletes, revokes, refreshes int
	deleteStatus                         int
	lastDeleted                          string
}

func (a *oauthAPI) handler(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch {
	case r.URL.Path == "/token":
		_ = r.ParseForm()
		if r.Form.Get("grant_type") == "refresh_token" {
			a.refreshes++
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "sdk-oauth-access", "refresh_token": "sdk-oauth-refresh", "token_type": "bearer", "expires_in": 3600, "scope": r2connect.OAuthScopes})
	case r.URL.Path == "/accounts":
		oauthEnvelope(w, []r2connect.Account{{ID: oauthTestAccount, Name: "Isolated account"}})
	case strings.HasSuffix(r.URL.Path, "/r2/buckets"):
		oauthEnvelope(w, map[string]any{"buckets": []r2connect.Bucket{{Name: "chosen-bucket", Jurisdiction: "default"}, {Name: "other-bucket", Jurisdiction: "default"}}})
	case strings.HasSuffix(r.URL.Path, "/permission_groups"):
		oauthEnvelope(w, []any{map[string]any{"id": oauthTestPermissionID, "name": "Workers R2 Storage Bucket Item Write", "scopes": []string{"com.cloudflare.edge.r2.bucket"}}})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/tokens"):
		a.creates++
		var request struct {
			ExpiresOn time.Time `json:"expires_on"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		oauthEnvelope(w, map[string]any{"id": fmt.Sprintf("%032x", a.creates), "value": fmt.Sprintf("test-api-token-%d", a.creates), "expires_on": request.ExpiresOn})
	case r.Method == http.MethodDelete:
		a.deletes++
		if a.deleteStatus != 0 && a.deleteStatus != 200 {
			w.WriteHeader(a.deleteStatus)
			_, _ = w.Write([]byte(`{"success":false}`))
			return
		}
		a.lastDeleted = filepath.Base(r.URL.Path)
		oauthEnvelope(w, map[string]string{"id": a.lastDeleted})
	case r.URL.Path == "/revoke":
		a.revokes++
		w.WriteHeader(200)
	default:
		w.WriteHeader(404)
	}
}
func (a *oauthAPI) counts() (int, int, int, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.creates, a.deletes, a.revokes, a.refreshes
}

func oauthFixture(t *testing.T) (*Service, *oauthMemorySecrets, *oauthAPI, string) {
	t.Helper()
	memory := &oauthMemorySecrets{syncMemorySecrets: &syncMemorySecrets{values: map[string][]byte{}}}
	s := newSyncServiceFixture(t, "", memory.syncMemorySecrets, cloudsync.New(nil))
	s.configSync.store = memory
	s.configSync.manager = nil
	s.configSync.client = nil
	api := &oauthAPI{}
	server := httptest.NewServer(http.HandlerFunc(api.handler))
	t.Cleanup(server.Close)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	redirect := "http://" + listener.Addr().String() + "/oauth/callback"
	_ = listener.Close()
	t.Setenv("PASSPINE_CF_OAUTH_CLIENT_ID", oauthTestClientID)
	t.Setenv("PASSPINE_CF_OAUTH_REDIRECT_URL", redirect)
	s.configSync.managerFactory = func(clientID string, state r2connect.StateStore) (*r2connect.Manager, *r2connect.Client, error) {
		client := r2connect.NewClient(clientID)
		client.APIBaseURL = server.URL
		client.OAuthBaseURL = server.URL
		manager := r2connect.NewManager(client, r2connect.ManagerOptions{Secrets: s.configSync.store, State: state, TokenNamePrefix: "Passpine-", OnWarning: func(r2connect.Warning) {
			s.mu.Lock()
			s.configSync.warningCode = "oauth_cleanup_pending"
			s.mu.Unlock()
		}})
		return manager, client, manager.LoadError()
	}
	if err = s.initSyncSDK(); err != nil {
		t.Fatal(err)
	}
	return s, memory, api, redirect
}
func authorizeOAuthCandidate(t *testing.T, s *Service) syncProfile {
	t.Helper()
	if err := s.ensureSyncManager(); err != nil {
		t.Fatal(err)
	}
	if err := s.configSync.manager.Authorize(context.Background(), oauthTokens()); err != nil {
		t.Fatal(err)
	}
	p := syncProfile{SecretRef: newSyncRef(), ClientID: oauthTestClientID}
	s.configSync.meta.Candidate = &p
	if err := s.saveSyncMetadata(s.configSync.meta); err != nil {
		t.Fatal(err)
	}
	return p
}
func prepareOAuthCandidate(t *testing.T, s *Service, bucket string) (syncProfile, syncSecret) {
	t.Helper()
	p, v, err := s.prepareR2Connection(context.Background(), &SyncConnectionRequest{Provider: "r2", AccountID: oauthTestAccount, Jurisdiction: "default", Bucket: bucket, Config: cloudsync.Config{Prefix: "settings"}, Secrets: cloudsync.Secrets{Passphrase: "test encryption passphrase"}})
	if err != nil {
		t.Fatal(err)
	}
	return p, v
}
func activateOAuthCandidate(t *testing.T, s *Service, p syncProfile, v syncSecret) {
	t.Helper()
	if err := s.storeSyncSecret(p.SecretRef, v); err != nil {
		t.Fatal(err)
	}
	s.configSync.meta.Profiles["r2"] = p
	s.configSync.meta.Active = "r2"
	s.configSync.meta.Enabled = true
	s.configSync.meta.Candidate = nil
	if err := s.saveSyncMetadata(s.configSync.meta); err != nil {
		t.Fatal(err)
	}
}

func TestConfigSyncOAuthSDKLoopbackSavesGrantBeforeBrowserSuccess(t *testing.T) {
	s, _, _, redirect := oauthFixture(t)
	err := s.startSyncOAuth(context.Background(), &SyncConnectionRequest{OpenURL: func(raw string) error {
		u, _ := url.Parse(raw)
		res, err := http.Get(redirect + "?" + url.Values{"state": {u.Query().Get("state")}, "code": {"isolated-code"}}.Encode())
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			t.Errorf("callback status %d", res.StatusCode)
		}
		state := s.configSync.manager.State()
		if !state.Authorized || state.OAuthCredentialKey == "" {
			t.Error("browser completed before SDK saved authorization")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if s.configSync.meta.Candidate == nil || s.configSync.meta.Candidate.ClientID != oauthTestClientID || s.configSync.meta.Active != "" {
		t.Fatal("grant skipped candidate boundary")
	}
	if len(s.configSync.status.Accounts) != 1 || s.configSync.status.OAuthStage != "bucket" {
		t.Fatal("account selection unavailable")
	}
}

func TestConfigSyncOAuthSDKCandidateDiscardPreservesSavedTarget(t *testing.T) {
	s, _, api, _ := oauthFixture(t)
	authorizeOAuthCandidate(t, s)
	saved, value := prepareOAuthCandidate(t, s, "chosen-bucket")
	activateOAuthCandidate(t, s, saved, value)
	authorizeOAuthCandidate(t, s)
	candidate, _ := prepareOAuthCandidate(t, s, "other-bucket")
	if s.configSync.meta.Profiles["r2"].Connection.ID != saved.Connection.ID {
		t.Fatal("candidate replaced saved target")
	}
	if err := s.discardSyncCandidate(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := s.configSync.manager.State()
	_, deleted, revoked, _ := api.counts()
	if len(state.Connections) != 1 || state.Connections[0].ID != saved.Connection.ID || deleted != 1 || revoked != 0 || s.configSync.meta.Candidate != nil {
		t.Fatal("Discard revoked saved grant or lost saved connection")
	}
	if candidate.Connection.ID == saved.Connection.ID {
		t.Fatal("SDK candidate reused saved connection identity")
	}
}

func TestConfigSyncOAuthSDKCredentialsFetchedBeforeUseAndNeverDuplicated(t *testing.T) {
	s, memory, _, _ := oauthFixture(t)
	authorizeOAuthCandidate(t, s)
	p, value := prepareOAuthCandidate(t, s, "chosen-bucket")
	activateOAuthCandidate(t, s, p, value)
	memory.mu.Lock()
	raw := string(memory.values[p.SecretRef])
	before := memory.loads
	memory.mu.Unlock()
	if strings.Contains(raw, value.Secrets.AccessKeyID) || strings.Contains(raw, value.Secrets.SecretAccessKey) || strings.Contains(raw, "test-oauth-") {
		t.Fatal("app sync secret duplicated SDK credentials")
	}
	loaded, err := s.loadSyncSecret(p.SecretRef)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Secrets.AccessKeyID != "" || loaded.ConnectionID != p.Connection.ID {
		t.Fatal("app secret kept S3 key")
	}
	if err = s.renewR2Credentials(context.Background(), &p, &loaded); err != nil || loaded.Secrets.AccessKeyID == "" {
		t.Fatal("Credentials was not resolved for use", err)
	}
	memory.mu.Lock()
	after := memory.loads
	memory.mu.Unlock()
	if after <= before {
		t.Fatal("use bypassed SDK SecretStore")
	}
	p.Config.Endpoint = "https://attacker.invalid"
	memory.mu.Lock()
	before = memory.operations
	memory.mu.Unlock()
	if err = s.renewR2Credentials(context.Background(), &p, &loaded); !errors.Is(err, r2connect.ErrCredentialsInvalid) {
		t.Fatal("tampered endpoint accepted", err)
	}
	memory.mu.Lock()
	after = memory.operations
	memory.mu.Unlock()
	if before != after {
		t.Fatal("binding validation accessed credentials")
	}
}

func TestConfigSyncOAuthSDKDisconnectFailureKeepsProfileAndRetries(t *testing.T) {
	s, _, api, _ := oauthFixture(t)
	authorizeOAuthCandidate(t, s)
	p, value := prepareOAuthCandidate(t, s, "chosen-bucket")
	activateOAuthCandidate(t, s, p, value)
	api.mu.Lock()
	api.deleteStatus = 403
	api.mu.Unlock()
	if err := s.disconnectSync(context.Background()); err == nil {
		t.Fatal("failed remote disconnect reported success")
	}
	if s.configSync.meta.Active != "r2" || s.configSync.meta.Profiles["r2"].Connection.ID != p.Connection.ID {
		t.Fatal("failed disconnect dropped active ownership")
	}
	api.mu.Lock()
	api.deleteStatus = 200
	api.mu.Unlock()
	if err := s.disconnectSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, _, revoked, _ := api.counts()
	if s.configSync.meta.Active != "" || len(s.configSync.meta.PendingCleanup) != 0 || revoked != 1 {
		t.Fatal("disconnect not completed through SDK")
	}
}

func TestConfigSyncOAuthSDKRefusalStopsUseUntilExplicitAction(t *testing.T) {
	s, memory, _, _ := oauthFixture(t)
	authorizeOAuthCandidate(t, s)
	p, value := prepareOAuthCandidate(t, s, "chosen-bucket")
	activateOAuthCandidate(t, s, p, value)
	memory.mu.Lock()
	memory.denied = true
	before := memory.operations
	retries := memory.retries
	memory.mu.Unlock()
	if err := s.renewR2Credentials(context.Background(), &p, &value); !errors.Is(err, secretstore.ErrDenied) {
		t.Fatal(err)
	}
	if err := s.resumeSyncOAuth(context.Background()); !errors.Is(err, secretstore.ErrDenied) {
		t.Fatal(err)
	}
	memory.mu.Lock()
	after := memory.operations
	newRetries := memory.retries
	memory.mu.Unlock()
	if before != after || retries != newRetries {
		t.Fatal("background/status retry reset refusal")
	}
	// Explicit disconnect is one of the SDK integration's allowed RetryAccess actions.
	if err := s.disconnectSync(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConfigSyncOAuthSDKLoadErrorPreservesStateAndReloadsOnlyFailedManager(t *testing.T) {
	s, memory, _, _ := oauthFixture(t)
	healthy := s.configSync.manager
	if err := s.reloadSyncSDK(); err != nil || s.configSync.manager != healthy {
		t.Fatal("reload replaced a healthy manager")
	}
	path := filepath.Join(s.directory, "r2connect-state.json")
	original := []byte(`{"version":99}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	s.configSync.manager = nil
	s.configSync.client = nil
	if err := s.initSyncSDK(); !errors.Is(err, r2connect.ErrInvalidState) {
		t.Fatal(err)
	}
	memory.mu.Lock()
	before := memory.operations
	memory.mu.Unlock()
	if err := s.resumeSyncOAuth(context.Background()); err == nil {
		t.Fatal("invalid SDK state was ignored")
	}
	current, _ := os.ReadFile(path)
	if string(current) != string(original) {
		t.Fatal("invalid SDK state overwritten")
	}
	memory.mu.Lock()
	after := memory.operations
	memory.mu.Unlock()
	if before != after {
		t.Fatal("failed initialization accessed secrets")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := s.reloadSyncSDK(); err != nil {
		t.Fatal("restored state could not be reloaded", err)
	}
}

func TestConfigSyncOAuthSDKRejectsShortPhraseBeforeConnect(t *testing.T) {
	s, _, api, _ := oauthFixture(t)
	authorizeOAuthCandidate(t, s)
	_, _, err := s.prepareR2Connection(context.Background(), &SyncConnectionRequest{Provider: "r2", AccountID: oauthTestAccount, Bucket: "chosen-bucket", Secrets: cloudsync.Secrets{Passphrase: "四个汉字"}})
	var typed *cloudsync.Error
	creates, _, _, _ := api.counts()
	if !errors.As(err, &typed) || typed.Code != "invalid_passphrase" || creates != 0 {
		t.Fatal("short phrase provisioned SDK connection", err)
	}
}

func TestConfigSyncOAuthSDKSavedProfileEditKeepsBucketAndSpaceBoundary(t *testing.T) {
	s, _, _, _ := oauthFixture(t)
	authorizeOAuthCandidate(t, s)
	p, value := prepareOAuthCandidate(t, s, "chosen-bucket")
	activateOAuthCandidate(t, s, p, value)
	request := &SyncConnectionRequest{Provider: "r2", Config: cloudsync.Config{Prefix: p.Config.Prefix}}
	prepared, secret, err := s.prepareR2Connection(context.Background(), request)
	if err != nil || prepared.SecretRef == p.SecretRef || prepared.Connection.ID != p.Connection.ID || secret.Secrets.Passphrase != value.Secrets.Passphrase {
		t.Fatal("saved edit changed SDK connection", err)
	}
	request.Config.Prefix = "other-space"
	if _, _, err = s.prepareR2Connection(context.Background(), request); err == nil {
		t.Fatal("blank phrase copied into other space")
	}
	request.Secrets.Passphrase = "new explicit encryption phrase"
	if _, _, err = s.prepareR2Connection(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Bucket = "other-bucket"
	if _, _, err = s.prepareR2Connection(context.Background(), request); !errors.Is(err, r2connect.ErrBucketUnavailable) {
		t.Fatal("saved credential crossed bucket boundary", err)
	}
}

func TestConfigSyncOAuthSDKMetadataFailureBeforeFlowDoesNotAuthorize(t *testing.T) {
	s, _, api, _ := oauthFixture(t)
	path := filepath.Join(s.directory, "config-sync.json")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	opened := false
	if err := s.startSyncOAuth(context.Background(), &SyncConnectionRequest{OpenURL: func(string) error { opened = true; return nil }}); err == nil {
		t.Fatal("failed ownership commit started authorization")
	}
	creates, _, revokes, _ := api.counts()
	if opened || s.configSync.manager.Status().Authorized || creates != 0 || revokes != 0 {
		t.Fatal("flow ran before public candidate ownership was saved")
	}
}

func TestConfigSyncOAuthSDKMetadataFailureAfterAcceptDoesNotRevokeSavedGrant(t *testing.T) {
	s, memory, api, redirect := oauthFixture(t)
	authorizeOAuthCandidate(t, s)
	saved, value := prepareOAuthCandidate(t, s, "chosen-bucket")
	activateOAuthCandidate(t, s, saved, value)
	metaPath := filepath.Join(s.directory, "config-sync.json")
	memory.onSave = func(key string) {
		if strings.HasPrefix(key, "oauth-") {
			memory.onSave = nil
			if err := os.Remove(metaPath); err != nil {
				t.Error(err)
			}
			if err := os.Mkdir(metaPath, 0700); err != nil {
				t.Error(err)
			}
		}
	}
	err := s.startSyncOAuth(context.Background(), &SyncConnectionRequest{OpenURL: func(raw string) error {
		u, _ := url.Parse(raw)
		response, err := http.Get(redirect + "?" + url.Values{"state": {u.Query().Get("state")}, "code": {"isolated-code"}}.Encode())
		if err == nil {
			response.Body.Close()
		}
		return err
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, revokes, _ := api.counts()
	if revokes != 0 || !s.configSync.manager.Status().Authorized || s.configSync.meta.Profiles["r2"].Connection.ID != saved.Connection.ID {
		t.Fatal("post-Accept app metadata failure revoked the SDK-owned grant or replaced the saved target")
	}
}

func TestConfigSyncOAuthSDKCancelRetainsCleanupOwnership(t *testing.T) {
	s, _, api, _ := oauthFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	err := s.startSyncOAuth(ctx, &SyncConnectionRequest{OpenURL: func(string) error { cancel(); return nil }})
	if !errors.Is(err, context.Canceled) || s.configSync.meta.Candidate == nil {
		t.Fatal("cancel lost pending ownership", err)
	}
	if err = s.discardSyncCandidate(context.Background()); err != nil {
		t.Fatal(err)
	}
	creates, _, revokes, _ := api.counts()
	if s.configSync.meta.Candidate != nil || creates != 0 || revokes != 0 {
		t.Fatal("cancel provisioned a token or retained unused draft")
	}
}

func TestConfigSyncOAuthSDKRetryRecoversMissingDraftConnection(t *testing.T) {
	s, _, _, _ := oauthFixture(t)
	authorizeOAuthCandidate(t, s)
	first, _ := prepareOAuthCandidate(t, s, "chosen-bucket")
	if err := s.configSync.manager.Discard(context.Background(), first.Connection.ID); err != nil {
		t.Fatal(err)
	}
	second, _ := prepareOAuthCandidate(t, s, "chosen-bucket")
	if second.Connection.ID == first.Connection.ID {
		t.Fatal("draft retry did not replace its already-discarded SDK connection")
	}
}
