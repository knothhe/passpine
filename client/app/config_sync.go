package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/knothhe/passpine/client/buildinfo"
	"github.com/knothhe/passpine/client/cloudsync"
	"github.com/knothhe/passpine/client/i18n"
	"github.com/knothhe/passpine/client/r2config"
	"github.com/knothhe/passpine/client/secretstore"
	"github.com/knothhe/r2connect"
	"github.com/knothhe/r2connect/keyring"
	"github.com/knothhe/r2connect/loopback"
)

// Browser preferences are a separate namespace; nil fields mean an omitted
// patch, never a request to reset another device's preference.
type BrowserSyncPreferences struct {
	ThemeStyle *string `json:"themeStyle,omitempty"`
	Theme      *string `json:"theme,omitempty"`
	FontScale  *int    `json:"fontScale,omitempty"`
	Language   *string `json:"language,omitempty"`
}

type SyncConnectionRequest struct {
	Provider     string             `json:"provider"`
	Config       cloudsync.Config   `json:"config"`
	Secrets      cloudsync.Secrets  `json:"secrets"`
	AccountID    string             `json:"accountId,omitempty"`
	Jurisdiction string             `json:"jurisdiction,omitempty"`
	Bucket       string             `json:"bucket,omitempty"`
	Resolution   string             `json:"resolution,omitempty"`
	OpenURL      func(string) error `json:"-"`
}

type ConfigSyncStatus struct {
	Options            ConfigSyncOptions   `json:"options,omitzero"`
	Fields             []string            `json:"fields,omitzero"`
	AttemptSequence    uint64              `json:"attemptSequence,omitempty"`
	AttemptErrorCode   string              `json:"attemptErrorCode,omitempty"`
	Enabled            bool                `json:"enabled"`
	Ready              bool                `json:"ready"`
	Busy               bool                `json:"busy,omitempty"`
	Provider           string              `json:"provider,omitempty"`
	SpaceID            string              `json:"spaceId,omitempty"`
	Config             cloudsync.Config    `json:"config,omitzero"`
	LastSyncedAt       string              `json:"lastSyncedAt,omitempty"`
	ErrorCode          string              `json:"errorCode,omitempty"`
	ConflictKeys       []string            `json:"conflictKeys,omitempty"`
	OAuthAvailable     bool                `json:"oauthAvailable,omitempty"`
	OAuthStage         string              `json:"oauthStage,omitempty"`
	Accounts           []r2connect.Account `json:"accounts,omitempty"`
	Buckets            []r2connect.Bucket  `json:"buckets,omitempty"`
	OAuthAuthorized    bool                `json:"oauthAuthorized,omitempty"`
	OAuthRenewable     bool                `json:"oauthRenewable,omitempty"`
	OAuthLoadErrorCode string              `json:"oauthLoadErrorCode,omitempty"`
	WarningCode        string              `json:"warningCode,omitempty"`
	AccountID          string              `json:"accountId,omitempty"`
	Jurisdiction       string              `json:"jurisdiction,omitempty"`
	ManualConfig       *cloudsync.Config   `json:"manualConfig,omitempty"`
	R2Config           *cloudsync.Config   `json:"r2Config,omitempty"`
}

type syncProfile struct {
	Connection   *r2connect.Connection `json:"connection,omitempty"`
	RemoteClean  bool                  `json:"remoteClean,omitempty"`
	Config       cloudsync.Config      `json:"config"`
	SecretRef    string                `json:"secretRef"`
	AccountID    string                `json:"accountId,omitempty"`
	Jurisdiction string                `json:"jurisdiction,omitempty"`
	ClientID     string                `json:"clientId,omitempty"`
}
type syncMetadata struct {
	Options        *ConfigSyncOptions     `json:"options,omitempty"`
	Version        int                    `json:"version"`
	Active         string                 `json:"active,omitempty"`
	Enabled        bool                   `json:"enabled"`
	Profiles       map[string]syncProfile `json:"profiles"`
	Candidate      *syncProfile           `json:"candidate,omitempty"`
	LastSyncedAt   string                 `json:"lastSyncedAt,omitempty"`
	PendingCleanup []syncProfile          `json:"pendingCleanup,omitempty"`
}
type syncSecret struct {
	ConnectionID            string            `json:"connectionId,omitempty"`
	Secrets                 cloudsync.Secrets `json:"secrets"`
	Baseline                cloudsync.Values  `json:"baseline,omitempty"`
	PendingDesktop          bool              `json:"pendingDesktop,omitempty"`
	PendingBrowser          bool              `json:"pendingBrowser,omitempty"`
	AssociationsInitialized bool              `json:"associationsInitialized,omitempty"`
	PendingKeys             []string          `json:"pendingKeys,omitempty"`
}
type syncSecrets interface {
	Load(string) ([]byte, error)
	Save(string, []byte) error
	Delete(string) error
	RetryAccess()
}
type configSyncState struct {
	pendingSecrets map[string]syncSecret
	managerFactory func(clientID string, state r2connect.StateStore) (*r2connect.Manager, *r2connect.Client, error)
	manager        *r2connect.Manager
	client         *r2connect.Client
	sdkState       *syncSessionState
	warningCode    string
	meta           syncMetadata
	status         ConfigSyncStatus
	store          syncSecrets
	engine         *cloudsync.Engine
	browser        *BrowserSyncPreferences
	dirty          bool
	queued         bool
	invalid        bool
	next           time.Time
	cancel         context.CancelFunc
}

func (s *Service) initConfigSync() {
	s.configSync.meta = syncMetadata{Version: 1, Profiles: map[string]syncProfile{}}
	s.configSync.engine = cloudsync.New(nil)
	locale := i18n.Resolve(s.prefs.Settings.Language, i18n.SystemLocale())
	s.configSync.store, _ = secretstore.New(buildinfo.ChannelName(), s.directory, i18n.Text(locale, "Passpine 同步主密钥"))
	if b, e := readPrivate(filepath.Join(s.directory, "config-sync.json"), 1<<20); e == nil {
		if json.Unmarshal(b, &s.configSync.meta) != nil || s.configSync.meta.Version != 1 || s.configSync.meta.Profiles == nil || s.configSync.meta.Options != nil && !validConfigSyncOptions(*s.configSync.meta.Options) {
			s.configSync.invalid = true
			s.configSync.status.ErrorCode = "invalid_document"
		}
	} else if !isMissing(e) {
		s.configSync.invalid = true
		s.configSync.status.ErrorCode = "local_storage"
	}
	if validBrowserPreferences(s.prefs.BrowserPreferences, false) {
		s.configSync.browser = s.prefs.BrowserPreferences
	}
	if _, exists := s.configSync.meta.Profiles[s.configSync.meta.Active]; exists && !s.configSync.invalid {
		// The local bridge remains available after restart, including manual
		// mode, without loading credentials or contacting object storage.
		s.configSync.status.Ready = s.configSync.meta.LastSyncedAt != ""
	}
	if !s.configSyncOptions().SyncOnStart {
		s.configSync.next = time.Now().Add(time.Duration(s.configSyncOptions().IntervalMinutes) * time.Minute)
	}
	if !s.preview && !s.configSync.invalid {
		_ = s.initSyncSDK()
	}
}

func (s *Service) SetOnSettingsChange(f func()) { s.mu.Lock(); s.onSettingsChange = f; s.mu.Unlock() }

func (s *Service) configSyncSnapshot(d *Data) {
	state := s.configSync.status
	state.Options = s.configSyncOptions()
	state.Enabled = s.configSync.meta.Enabled && !s.configSync.invalid
	state.Provider = s.configSync.meta.Active
	state.LastSyncedAt = s.configSync.meta.LastSyncedAt
	state.OAuthAvailable = r2config.DefaultConfig().Available() && !s.preview
	state.WarningCode = s.configSync.warningCode
	if p, ok := s.configSync.meta.Profiles[state.Provider]; ok {
		state.Config = p.Config
		state.SpaceID = syncSpaceID(p.Config)
	}
	if p, ok := s.configSync.meta.Profiles["s3"]; ok {
		c := p.Config
		state.ManualConfig = &c
	}
	if p, ok := s.configSync.meta.Profiles["r2"]; ok {
		c := p.Config
		state.R2Config = &c
		if state.AccountID == "" {
			state.AccountID = p.AccountID
			state.Jurisdiction = p.Jurisdiction
		}
	}
	state.ConflictKeys = append([]string(nil), state.ConflictKeys...)
	state.Accounts = append([]r2connect.Account(nil), state.Accounts...)
	state.Buckets = append([]r2connect.Bucket(nil), state.Buckets...)
	if s.configSync.meta.Candidate != nil && state.OAuthStage == "" {
		state.OAuthStage = "bucket"
	}
	d.ConfigSync = &state
	if s.configSync.browser != nil {
		d.SyncedBrowserPreferences = cloneBrowserSync(s.configSync.browser)
	}
	d.Capabilities = append(d.Capabilities, "config-sync")
}

func (s *Service) saveSyncMetadata(meta syncMetadata) error {
	b, e := json.MarshalIndent(meta, "", "  ")
	if e != nil {
		return e
	}
	return writePrivate(filepath.Join(s.directory, "config-sync.json"), b, false)
}
func isMissing(e error) bool { return os.IsNotExist(e) }

func (s *Service) loadSyncSecret(ref string) (syncSecret, error) {
	var out syncSecret
	if s.configSync.store == nil {
		return out, secretstore.ErrUnavailable
	}
	// Pending values retain failed saves, but must not conceal a removed or
	// changed encrypted secret file. This check never opens native secrets.
	if err := s.syncSecretSessionError(); err != nil {
		return out, err
	}
	if pending, ok := s.configSync.pendingSecrets[ref]; ok {
		b, e := json.Marshal(pending)
		if e != nil {
			return out, e
		}
		defer clear(b)
		e = json.Unmarshal(b, &out)
		return out, e
	}
	b, e := s.configSync.store.Load(ref)
	if e != nil {
		return out, e
	}
	defer clear(b)
	if json.Unmarshal(b, &out) != nil {
		return out, &cloudsync.Error{Code: "invalid_document"}
	}
	return out, nil
}

func persistedSyncSecret(v syncSecret) syncSecret {
	if v.ConnectionID != "" {
		v.Secrets.AccessKeyID = ""
		v.Secrets.SecretAccessKey = ""
		v.Secrets.SessionToken = ""
	}
	return v
}

func (s *Service) storeSyncSecret(ref string, v syncSecret) error {
	if s.configSync.store == nil {
		return secretstore.ErrUnavailable
	}
	v = persistedSyncSecret(v)
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	defer clear(b)
	e = s.configSync.store.Save(ref, b)
	if e != nil {
		if s.configSync.pendingSecrets == nil {
			s.configSync.pendingSecrets = map[string]syncSecret{}
		}
		s.configSync.pendingSecrets[ref] = v
	} else {
		delete(s.configSync.pendingSecrets, ref)
	}
	return e
}
func newSyncRef() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "sync-" + hex.EncodeToString(b[:])
}

func (s *Service) cancelConfigSync() {
	s.mu.Lock()
	cancel := s.configSync.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func configSyncNeedsUserAction(code string) bool {
	switch code {
	case "conflict", "credentials_denied", "credentials_invalid", "credentials_state_changed", "oauth_state_changed":
		return true
	}
	return false
}

func (s *Service) scheduleConfigSync() {
	allowed := s.syncSecretAccessAllowed()
	s.mu.Lock()
	c := &s.configSync
	if !allowed || s.preview || s.closed || c.invalid || s.configSyncOptions().Mode != "automatic" || !c.meta.Enabled || c.meta.Active == "" || c.queued || c.status.Busy || time.Now().Before(c.next) || configSyncNeedsUserAction(c.status.ErrorCode) {
		s.mu.Unlock()
		return
	}
	c.queued = true
	s.mu.Unlock()
	go func() {
		s.executeConfigSync(Request{Action: "config-sync-now"}, "automatic")
		s.mu.Lock()
		s.configSync.queued = false
		s.mu.Unlock()
	}()
}

func (s *Service) executeConfigSync(r Request, source string) Response {
	if r.Action == "config-sync-options" {
		return s.saveConfigSyncOptions(r, source)
	}
	if r.Action == "config-sync-cancel" {
		s.cancelConfigSync()
	}
	if r.Action == "config-sync-status" || r.Action == "config-sync-browser" {
		s.updateRequests.RLock()
		defer s.updateRequests.RUnlock()
		s.mu.Lock()
		defer s.mu.Unlock()
		var e error
		if r.Action == "config-sync-browser" && r.SyncPreferences != nil {
			if s.closed || s.updatePaused.Load() {
				e = &cloudsync.Error{Code: "cancelled"}
			} else if !s.configSync.meta.Enabled || !s.configSync.status.Ready || !browserSyncPatchSelected(r.SyncPreferences, s.configSyncOptions()) {
				e = &cloudsync.Error{Code: "not_ready"}
			} else if source == "browser" && r.SyncSpaceID != syncSpaceID(s.configSync.meta.Profiles[s.configSync.meta.Active].Config) {
				e = &cloudsync.Error{Code: "not_ready"}
			} else {
				e = s.mergeBrowserPreferences(r.SyncPreferences)
			}
		}
		return s.syncResponseLocked(r, e)
	}
	// All network, profile commits, token cleanup and secret access share one
	// lane, independently of vault locking and normal credential requests.
	s.updateRequests.RLock()
	defer s.updateRequests.RUnlock()
	// A repeated connection click must not queue another RetryAccess behind
	// the active request, especially after its native prompt was refused.
	newAccess := r.Action == "config-sync-oauth-start" || r.Action == "config-sync-disconnect" || r.Action == "config-sync-save" && r.SyncConnection != nil && r.SyncConnection.Provider == "s3"
	if newAccess {
		if !s.configSyncMu.TryLock() {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.syncResponseLocked(r, &cloudsync.Error{Code: "busy"})
		}
	} else {
		s.configSyncMu.Lock()
	}
	defer func() {
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		// A native prompt can outlive the bounded shutdown drain. Discard any
		// successful reads from that late worker before it releases the lane.
		if closed {
			s.clearSyncSecretSession()
			clear(s.configSync.pendingSecrets)
		}
		s.configSyncMu.Unlock()
	}()
	s.mu.Lock()
	if s.closed || s.updatePaused.Load() {
		out := s.syncResponseLocked(r, &cloudsync.Error{Code: "cancelled"})
		s.mu.Unlock()
		return out
	}
	if s.preview || s.configSync.invalid {
		out := s.syncResponseLocked(r, &cloudsync.Error{Code: "unavailable"})
		s.mu.Unlock()
		return out
	}
	// A queued worker may acquire the lane after an options save changed
	// mode or its deadline. Re-check before starting any automatic work.
	if source == "automatic" && (s.configSyncOptions().Mode != "automatic" || !s.configSync.meta.Enabled || time.Now().Before(s.configSync.next) || configSyncNeedsUserAction(s.configSync.status.ErrorCode)) {
		out := s.syncResponseLocked(r, nil)
		s.mu.Unlock()
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	s.configSync.cancel = cancel
	s.configSync.status.Busy = true
	s.mu.Unlock()
	defer cancel()
	var e error
	switch r.Action {
	case "config-sync-test":
		e = s.testSyncConnection(ctx, r.SyncConnection)
	case "config-sync-save":
		e = s.saveSyncConnection(ctx, r.SyncConnection)
	case "config-sync-now":
		e = s.syncConfiguration(ctx, "")
	case "config-sync-resolve":
		if r.SyncConnection == nil || r.SyncConnection.Resolution != "local" && r.SyncConnection.Resolution != "remote" {
			e = &cloudsync.Error{Code: "invalid_config"}
		} else {
			e = s.syncConfiguration(ctx, r.SyncConnection.Resolution)
		}
	case "config-sync-pause", "config-sync-resume":
		s.mu.Lock()
		next := s.configSync.meta
		next.Enabled = r.Action == "config-sync-resume"
		if next.Active == "" {
			e = &cloudsync.Error{Code: "not_ready"}
		} else if e = s.saveSyncMetadata(next); e == nil {
			s.configSync.meta = next
			s.configSync.next = time.Time{}
		}
		s.mu.Unlock()
	case "config-sync-oauth-start":
		e = s.startSyncOAuth(ctx, r.SyncConnection)
	case "config-sync-oauth-buckets":
		e = s.listSyncBuckets(ctx, r.SyncConnection)
	case "config-sync-oauth-accounts":
		e = s.resumeSyncOAuth(ctx)
	case "config-sync-oauth-reload":
		e = s.reloadSyncSDK()
	case "config-sync-cancel":
		e = s.discardSyncCandidate(ctx)
	case "config-sync-disconnect":
		e = s.disconnectSync(ctx)
	default:
		e = &cloudsync.Error{Code: "invalid_config"}
	}
	s.updateSyncSDKStatus()
	if manager := s.configSync.manager; manager != nil && manager.LoadError() == nil {
		state := manager.State()
		s.mu.Lock()
		if len(s.configSync.meta.PendingCleanup) == 0 && len(state.RetiredTokens) == 0 && len(state.RetiredOAuthKeys) == 0 && s.configSync.warningCode == "oauth_cleanup_pending" {
			s.configSync.warningCode = ""
		}
		s.mu.Unlock()
	}
	s.mu.Lock()
	s.configSync.cancel = nil
	s.configSync.status.Busy = false
	s.configSync.status.ErrorCode = syncErrorCode(e)
	if e == nil {
		s.configSync.status.ConflictKeys = nil
	}
	var typed *cloudsync.Error
	if errors.As(e, &typed) && typed.Code == "conflict" {
		s.configSync.status.ConflictKeys = append([]string(nil), typed.Keys...)
	}
	s.configSync.next = time.Now().Add(time.Duration(s.configSyncOptions().IntervalMinutes) * time.Minute)
	if e == nil && s.configSync.dirty && s.configSyncOptions().Mode == "automatic" && s.configSyncOptions().SyncOnChange {
		s.configSync.next = time.Now().Add(2 * time.Second)
	}
	if e != nil {
		s.configSync.next = time.Now().Add(time.Minute)
	}
	switch r.Action {
	case "config-sync-save", "config-sync-now", "config-sync-resolve":
		s.configSync.status.AttemptSequence++
		s.configSync.status.AttemptErrorCode = syncErrorCode(e)
	}
	s.logConfigSync(r.Action, source, e)
	out := s.syncResponseLocked(r, e)
	notify := s.onSettingsChange
	s.mu.Unlock()
	if notify != nil {
		go notify()
	}
	return out
}

func syncErrorCode(e error) string {
	if e == nil {
		return ""
	}
	if errors.Is(e, secretstore.ErrDenied) {
		return "credentials_denied"
	}
	if errors.Is(e, secretstore.ErrUnavailable) || errors.Is(e, secretstore.ErrNotFound) {
		return "credentials_unavailable"
	}
	if errors.Is(e, secretstore.ErrSessionChanged) {
		return "credentials_state_changed"
	}
	if errors.Is(e, secretstore.ErrInvalid) {
		return "credentials_invalid"
	}
	if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return "cancelled"
	}
	if errors.Is(e, keyring.ErrIncomplete) {
		return "credentials_invalid"
	}
	if errors.Is(e, errSyncSessionChanged) {
		return "oauth_state_changed"
	}
	for _, mapped := range []struct {
		err  error
		code string
	}{
		{r2config.ErrUnavailable, "oauth_unavailable"}, {r2config.ErrConfig, "oauth_invalid_config"},
		{loopback.ErrListen, "oauth_listen_failed"}, {loopback.ErrInvalidScope, "oauth_invalid_scope"}, {loopback.ErrDenied, "oauth_denied"}, {loopback.ErrCallbackStopped, "oauth_callback_stopped"}, {loopback.ErrRedirect, "oauth_invalid_config"},
		{r2connect.ErrNotAuthorized, "oauth_authorization_required"}, {r2connect.ErrAuthorizationInvalid, "oauth_authorization_required"}, {r2connect.ErrAuthorizationExpired, "oauth_authorization_required"}, {r2connect.ErrCredentialsInvalid, "oauth_credentials_invalid"},
		{r2connect.ErrMissingScopes, "oauth_missing_scopes"}, {r2connect.ErrClientChanged, "oauth_client_changed"}, {r2connect.ErrAccountsMissing, "oauth_accounts_missing"},
		{r2connect.ErrBucketUnavailable, "oauth_invalid_selection"}, {r2connect.ErrTooManyConnections, "oauth_too_many_connections"}, {r2connect.ErrUnknownConnection, "oauth_unknown_connection"}, {r2connect.ErrCleanupPending, "oauth_cleanup_pending"}, {r2connect.ErrInvalidState, "oauth_invalid_state"},
	} {
		if errors.Is(e, mapped.err) {
			return mapped.code
		}
	}
	var api *r2connect.APIError
	if errors.As(e, &api) {
		switch api.StatusCode {
		case 401:
			return "oauth_authorization_required"
		case 403:
			return "oauth_permission_required"
		case 429:
			return "oauth_rate_limited"
		}
	}
	var ce *cloudsync.Error
	if errors.As(e, &ce) {
		return ce.Code
	}
	return "connection_failed"
}
func (s *Service) syncResponseLocked(r Request, e error) Response {
	out := Response{RequestID: r.RequestID, Protocol: 1, Status: "ok", Data: s.snapshotWithEntries(false)}
	if e != nil {
		out.Status = "error"
		out.Code = syncErrorCode(e)
		out.Message = i18n.Text(i18n.Resolve(s.prefs.Settings.Language, i18n.SystemLocale()), "配置同步未完成，请检查连接设置后重试。")
	}
	return out
}

func (s *Service) manualConnection(r *SyncConnectionRequest) (syncProfile, syncSecret, error) {
	if r == nil {
		return syncProfile{}, syncSecret{}, &cloudsync.Error{Code: "invalid_config"}
	}
	c, e := cloudsync.NormalizeConfig(r.Config)
	if e != nil {
		return syncProfile{}, syncSecret{}, e
	}
	v := syncSecret{Secrets: r.Secrets}
	s.mu.Lock()
	p, exists := s.configSync.meta.Profiles["s3"]
	s.mu.Unlock()
	// Empty secret inputs mean keep a saved secret only for the same target.
	// Never reuse a credential on a newly typed endpoint or bucket.
	if exists && p.Config == c && (v.Secrets.AccessKeyID == "" || v.Secrets.SecretAccessKey == "" || v.Secrets.Passphrase == "") {
		old, err := s.loadSyncSecret(p.SecretRef)
		if err != nil {
			return syncProfile{}, syncSecret{}, err
		}
		if v.Secrets.AccessKeyID == "" {
			v.Secrets.AccessKeyID = old.Secrets.AccessKeyID
		}
		if v.Secrets.SecretAccessKey == "" {
			v.Secrets.SecretAccessKey = old.Secrets.SecretAccessKey
		}
		if v.Secrets.SessionToken == "" {
			v.Secrets.SessionToken = old.Secrets.SessionToken
		}
		if v.Secrets.Passphrase == "" {
			v.Secrets.Passphrase = old.Secrets.Passphrase
		}
		if v.Secrets.Passphrase == old.Secrets.Passphrase {
			v.Baseline = old.Baseline
			v.PendingDesktop, v.PendingBrowser = old.PendingDesktop, old.PendingBrowser
			v.AssociationsInitialized = old.AssociationsInitialized
			v.PendingKeys = append([]string(nil), old.PendingKeys...)
		}
	}
	if e = cloudsync.ValidateSecrets(v.Secrets); e != nil {
		return syncProfile{}, syncSecret{}, e
	}
	return syncProfile{Config: c, SecretRef: newSyncRef()}, v, nil
}
func (s *Service) testSyncConnection(ctx context.Context, r *SyncConnectionRequest) error {
	p, v, e := s.manualConnection(r)
	if e != nil {
		return e
	}
	return s.configSync.engine.Test(ctx, p.Config, v.Secrets)
}

func (s *Service) saveSyncConnection(ctx context.Context, r *SyncConnectionRequest) error {
	if r == nil {
		return &cloudsync.Error{Code: "invalid_config"}
	}
	if s.configSync.store == nil {
		return secretstore.ErrUnavailable
	}
	if r.Provider == "s3" {
		if err := s.retrySyncSecretAccess(); err != nil {
			return err
		}
	}
	var p syncProfile
	var v syncSecret
	var e error
	provider := r.Provider
	if provider == "r2" {
		p, v, e = s.prepareR2Connection(ctx, r)
	} else if provider == "s3" {
		p, v, e = s.manualConnection(r)
	} else {
		return &cloudsync.Error{Code: "invalid_config"}
	}
	if e != nil {
		return e
	}
	if e = s.reuseSyncBaseline(p, &v); e != nil {
		return e
	}
	if e = s.testPreparedSyncConnection(ctx, provider, p, v); e != nil {
		return e
	}
	// Record local secret ownership before writing it. A crash or a failed
	// activation can then retry cleanup without losing the reference.
	s.mu.Lock()
	if s.closed || ctx.Err() != nil {
		s.mu.Unlock()
		return context.Canceled
	}
	if s.configSync.meta.Candidate == nil || s.configSync.meta.Candidate.SecretRef != p.SecretRef {
		staged := p
		staged.RemoteClean = true
		next := s.configSync.meta
		next.PendingCleanup = append(append([]syncProfile(nil), next.PendingCleanup...), staged)
		e = s.saveSyncMetadata(next)
		if e == nil {
			s.configSync.meta = next
		}
	}
	s.mu.Unlock()
	if e != nil {
		return e
	}
	if e = s.storeSyncSecret(p.SecretRef, v); e != nil {
		return e
	}
	s.mu.Lock()
	if s.closed || ctx.Err() != nil {
		s.mu.Unlock()
		if provider == "s3" {
			_ = s.configSync.store.Delete(p.SecretRef)
		}
		return context.Canceled
	}
	local := s.localSyncValues(v.Baseline)
	started := s.localSyncValues(nil)
	options := s.configSyncOptions()
	associationsValid := validAssociationStore(s.prefs.BrowserAssociations)
	s.mu.Unlock()
	if options.Browser && !associationsValid {
		return &cloudsync.Error{Code: "invalid_document"}
	}
	// Validate and synchronize the candidate before replacing a working profile.
	// A wrong phrase or rejected write must leave its saved credentials intact.
	result, e := s.configSync.engine.SyncValidatedWithPolicy(ctx, p.Config, v.Secrets, local, v.Baseline, validateSyncedValues, configSyncPolicy(v, options))
	if e != nil {
		return e
	}
	v = advanceConfigSyncBaseline(v, result.Values, options)
	if e = s.storeSyncSecret(p.SecretRef, v); e != nil {
		return e
	}
	s.mu.Lock()
	if s.closed || ctx.Err() != nil {
		s.mu.Unlock()
		return context.Canceled
	}
	previousSettings, previousBrowser, previousAssociations := s.prefs.Settings, s.prefs.BrowserPreferences, s.prefs.BrowserAssociations
	if e = s.applySyncValues(result.Values, started); e != nil {
		s.mu.Unlock()
		return e
	}
	next := s.configSync.meta
	cleanup := make([]syncProfile, 0, len(next.PendingCleanup))
	for _, item := range next.PendingCleanup {
		if item.SecretRef != p.SecretRef {
			cleanup = append(cleanup, item)
		}
	}
	next.PendingCleanup = cleanup
	profiles := make(map[string]syncProfile, len(next.Profiles)+1)
	for k, v := range next.Profiles {
		profiles[k] = v
	}
	next.Profiles = profiles
	old, hadOld := next.Profiles[provider]
	if provider == "r2" && next.Candidate == nil {
		old.RemoteClean = true
	}
	if hadOld && old.SecretRef != p.SecretRef {
		next.PendingCleanup = append(append([]syncProfile(nil), next.PendingCleanup...), old)
	}
	next.Profiles[provider] = p
	next.Active = provider
	next.Enabled = true
	next.LastSyncedAt = time.Now().UTC().Format(time.RFC3339)
	if provider == "r2" {
		next.Candidate = nil
	}
	e = s.saveSyncMetadata(next)
	if e == nil {
		s.configSync.meta = next
		s.configSync.status.Ready = true
		s.configSync.status.OAuthStage = ""
		s.configSync.status.Accounts = nil
		s.configSync.status.Buckets = nil
		current, _ := json.Marshal(s.localSyncValues(result.Values))
		merged, _ := json.Marshal(result.Values)
		s.configSync.dirty = string(current) != string(merged)
	} else {
		// Activation and preference writes share this lock. Restore the state
		// captured after the network request, including any intervening edits.
		s.prefs.Settings, s.prefs.BrowserPreferences = previousSettings, previousBrowser
		s.prefs.BrowserAssociations = previousAssociations
		s.configSync.browser = previousBrowser
		if rollback := s.savePreferences(); rollback != nil {
			s.configSync.invalid = true
		}
	}
	s.mu.Unlock()
	if e != nil {
		if provider == "s3" {
			_ = s.configSync.store.Delete(p.SecretRef)
		}
		return e
	}
	if provider == "r2" {
		e = s.cleanupRetiredSyncProfiles(ctx)
	} else {
		e = s.cleanupLocalSyncProfiles(ctx)
	}
	if e != nil {
		// The new profile and preferences are already committed. Keep cleanup
		// ownership and report a warning instead of calling this a failed save.
		s.mu.Lock()
		s.configSync.warningCode = "oauth_cleanup_pending"
		s.mu.Unlock()
	}
	return nil
}

func sameSyncSpace(a, b cloudsync.Config) bool {
	return a.Endpoint == b.Endpoint && a.Bucket == b.Bucket && a.Prefix == b.Prefix
}

func syncSpaceID(c cloudsync.Config) string {
	digest := sha256.Sum256([]byte(c.Endpoint + "\n" + c.Bucket + "\n" + c.Prefix))
	return hex.EncodeToString(digest[:])
}

func (s *Service) reuseSyncBaseline(p syncProfile, v *syncSecret) error {
	s.mu.Lock()
	profiles := make([]syncProfile, 0, len(s.configSync.meta.Profiles))
	active := s.configSync.meta.Active
	if old, ok := s.configSync.meta.Profiles[active]; ok {
		profiles = append(profiles, old)
	}
	for provider, old := range s.configSync.meta.Profiles {
		if provider != active {
			profiles = append(profiles, old)
		}
	}
	s.mu.Unlock()
	for _, old := range profiles {
		if sameSyncSpace(old.Config, p.Config) {
			previous, e := s.loadSyncSecret(old.SecretRef)
			if e != nil {
				return e
			}
			if previous.Secrets.Passphrase == v.Secrets.Passphrase {
				v.Baseline = copySyncValues(previous.Baseline)
				v.PendingDesktop, v.PendingBrowser = previous.PendingDesktop, previous.PendingBrowser
				v.AssociationsInitialized = previous.AssociationsInitialized
				v.PendingKeys = append([]string(nil), previous.PendingKeys...)
				if previous.Baseline == nil {
					v.Baseline = nil
				}
				return nil
			}
		}
	}
	return nil
}

func copySyncValues(in cloudsync.Values) cloudsync.Values {
	out := cloudsync.Values{}
	for k, v := range in {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out
}
func jsonValue(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func (s *Service) localSyncValues(base cloudsync.Values) cloudsync.Values {
	v := copySyncValues(base)
	options := s.configSyncOptions()
	local := cloudsync.Values{
		"desktop.theme": jsonValue(s.prefs.Settings.Theme), "desktop.themeStyle": jsonValue(s.prefs.Settings.ThemeStyle),
		"desktop.fontScale": jsonValue(s.prefs.Settings.FontScale), "desktop.language": jsonValue(s.prefs.Settings.Language),
	}
	if string(base["desktop.themeStyle"]) == `"omarchy"` && NormalizeThemeStyle(ThemeStyleOmarchy) != ThemeStyleOmarchy && s.prefs.Settings.ThemeStyle == NormalizeThemeStyle(ThemeStyleOmarchy) {
		local["desktop.themeStyle"] = jsonValue(ThemeStyleOmarchy)
	}
	if p := s.configSync.browser; p != nil {
		if p.Theme != nil {
			local["browser.theme"] = jsonValue(*p.Theme)
		}
		if p.ThemeStyle != nil {
			local["browser.themeStyle"] = jsonValue(*p.ThemeStyle)
		}
		if p.FontScale != nil {
			local["browser.fontScale"] = jsonValue(*p.FontScale)
		}
		if p.Language != nil {
			local["browser.language"] = jsonValue(*p.Language)
		}
	}
	if options.Browser && validAssociationStore(s.prefs.BrowserAssociations) {
		for key, value := range associationSyncValues(s.prefs.BrowserAssociations) {
			local[key] = value
		}
	}
	for key, value := range local {
		if configSyncKeyEnabled(options, key) {
			v[key] = value
		}
	}
	return v
}

func (s *Service) syncConfiguration(ctx context.Context, resolution string) error {
	s.mu.Lock()
	meta := s.configSync.meta
	p, ok := meta.Profiles[meta.Active]
	s.mu.Unlock()
	if !ok || !meta.Enabled {
		return &cloudsync.Error{Code: "not_ready"}
	}
	if meta.Active == "r2" {
		if e := s.cleanupRetiredSyncProfiles(ctx); e != nil {
			return e
		}
	} else {
		if e := s.cleanupLocalSyncProfiles(ctx); e != nil {
			return e
		}
	}
	v, e := s.loadSyncSecret(p.SecretRef)
	if e != nil {
		return e
	}
	previousSecret, e := json.Marshal(persistedSyncSecret(v))
	if e != nil {
		return e
	}
	defer clear(previousSecret)
	if meta.Active == "r2" {
		if e = s.renewR2Credentials(ctx, &p, &v); e != nil {
			return e
		}
	}
	s.mu.Lock()
	local := s.localSyncValues(v.Baseline)
	started := s.localSyncValues(nil)
	options := s.configSyncOptions()
	associationsValid := validAssociationStore(s.prefs.BrowserAssociations)
	s.mu.Unlock()
	if options.Browser && !associationsValid {
		return &cloudsync.Error{Code: "invalid_document"}
	}
	baseline := v.Baseline
	if resolution == "local" || resolution == "remote" {
		remote, e := s.configSync.engine.Read(ctx, p.Config, v.Secrets)
		if e != nil {
			return e
		}
		if e = validateSyncedValues(remote.Values); e != nil {
			return e
		}
		baseline = copySyncValues(v.Baseline)
		s.mu.Lock()
		keys := append([]string(nil), s.configSync.status.ConflictKeys...)
		s.mu.Unlock()
		if len(keys) == 0 {
			return &cloudsync.Error{Code: "not_ready"}
		}
		for _, key := range keys {
			if value, ok := remote.Values[key]; ok {
				baseline[key] = value
				if resolution == "remote" {
					local[key] = value
				}
			} else {
				delete(baseline, key)
				if resolution == "remote" {
					delete(local, key)
				}
			}
		}
	}
	result, e := s.configSync.engine.SyncValidatedWithPolicy(ctx, p.Config, v.Secrets, local, baseline, validateSyncedValues, configSyncPolicy(v, options))
	if e != nil {
		return e
	}
	s.mu.Lock()
	if s.closed || ctx.Err() != nil {
		s.mu.Unlock()
		return context.Canceled
	}
	e = s.applySyncValues(result.Values, started)
	s.mu.Unlock()
	if e != nil {
		return e
	}
	v = advanceConfigSyncBaseline(v, result.Values, options)
	nextSecret, e := json.Marshal(persistedSyncSecret(v))
	if e != nil {
		return e
	}
	defer clear(nextSecret)
	// An unchanged sync must not rewrite the Keychain entry and invalidate
	// its successful read session. Failed prior writes still need retrying.
	_, pending := s.configSync.pendingSecrets[p.SecretRef]
	if pending || !bytes.Equal(previousSecret, nextSecret) {
		if e = s.storeSyncSecret(p.SecretRef, v); e != nil {
			return e
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.configSync.meta
	next.LastSyncedAt = time.Now().UTC().Format(time.RFC3339)
	if e = s.saveSyncMetadata(next); e != nil {
		return e
	}
	s.configSync.meta = next
	s.configSync.status.Ready = true
	current, _ := json.Marshal(s.localSyncValues(result.Values))
	merged, _ := json.Marshal(result.Values)
	s.configSync.dirty = string(current) != string(merged)
	return nil
}

func (s *Service) applySyncValues(values, started cloudsync.Values) error {
	if err := validateSyncedValues(values); err != nil {
		return err
	}
	settings := s.prefs.Settings
	options := s.configSyncOptions()
	decode := func(k string, dst any) error {
		if b, ok := values[k]; ok {
			return json.Unmarshal(b, dst)
		}
		return nil
	}
	// Preserve edits made while the network request was in flight.
	current := s.localSyncValues(nil)
	for _, item := range []struct {
		k   string
		dst any
	}{{"desktop.theme", &settings.Theme}, {"desktop.themeStyle", &settings.ThemeStyle}, {"desktop.fontScale", &settings.FontScale}, {"desktop.language", &settings.Language}} {
		if configSyncKeyEnabled(options, item.k) && string(current[item.k]) == string(started[item.k]) {
			if decode(item.k, item.dst) != nil {
				return &cloudsync.Error{Code: "invalid_document"}
			}
		}
	}
	settings.ThemeStyle = NormalizeThemeStyle(settings.ThemeStyle)
	if validateSettings(settings) != nil {
		return &cloudsync.Error{Code: "invalid_document"}
	}
	var browser *BrowserSyncPreferences
	if len(selectedBrowserSyncFields(options)) != 0 {
		candidate := cloneBrowserSync(s.configSync.browser)
		if candidate == nil {
			candidate = &BrowserSyncPreferences{}
		}
		applied := false
		for _, item := range []struct {
			k   string
			dst any
		}{{"browser.theme", &candidate.Theme}, {"browser.themeStyle", &candidate.ThemeStyle}, {"browser.fontScale", &candidate.FontScale}, {"browser.language", &candidate.Language}} {
			b, present := values[item.k]
			if !present || !configSyncKeyEnabled(options, item.k) {
				continue
			}
			if old, ok := current[item.k]; ok && string(old) != string(started[item.k]) {
				b = old
			}
			if len(b) == 0 || json.Unmarshal(b, item.dst) != nil {
				return &cloudsync.Error{Code: "invalid_document"}
			}
			applied = true
		}
		if applied {
			browser = candidate
			if !validBrowserPreferences(browser, false) {
				return &cloudsync.Error{Code: "invalid_document"}
			}
		}
	}
	previous := s.prefs.Settings
	previousBrowser := s.prefs.BrowserPreferences
	previousAssociations := s.prefs.BrowserAssociations
	associations, err := s.mergedAssociationSyncValues(values, current, started)
	if err != nil {
		return err
	}
	s.prefs.Settings = settings
	s.prefs.BrowserAssociations = associations
	if browser != nil {
		s.prefs.BrowserPreferences = browser
	}
	if e := s.savePreferences(); e != nil {
		s.prefs.Settings = previous
		s.prefs.BrowserPreferences = previousBrowser
		s.prefs.BrowserAssociations = previousAssociations
		return e
	}
	if browser != nil {
		s.configSync.browser = browser
	}
	return nil
}

func validBrowserPreferences(p *BrowserSyncPreferences, complete bool) bool {
	if p == nil {
		return false
	}
	if complete && (p.Theme == nil || p.ThemeStyle == nil || p.Language == nil || p.FontScale == nil) {
		return false
	}
	if p.Theme != nil && *p.Theme != "system" && *p.Theme != "light" && *p.Theme != "dark" {
		return false
	}
	if p.ThemeStyle != nil && *p.ThemeStyle != "monochrome" && *p.ThemeStyle != "onevra" {
		return false
	}
	if p.Language != nil && *p.Language != "system" && *p.Language != "en" && *p.Language != "zh-CN" {
		return false
	}
	if p.FontScale != nil && *p.FontScale != 80 && *p.FontScale != 90 && *p.FontScale != 100 && *p.FontScale != 110 && *p.FontScale != 120 {
		return false
	}
	return true
}
func (s *Service) mergeBrowserPreferences(p *BrowserSyncPreferences) error {
	if !validBrowserPreferences(p, false) {
		return &cloudsync.Error{Code: "invalid_config"}
	}
	next := BrowserSyncPreferences{}
	if s.configSync.browser != nil {
		next = *s.configSync.browser
	}
	if p.Theme != nil {
		next.Theme = p.Theme
	}
	if p.ThemeStyle != nil {
		next.ThemeStyle = p.ThemeStyle
	}
	if p.Language != nil {
		next.Language = p.Language
	}
	if p.FontScale != nil {
		next.FontScale = p.FontScale
	}
	if string(jsonValue(s.configSync.browser)) == string(jsonValue(&next)) {
		return nil
	}
	old := s.prefs.BrowserPreferences
	s.prefs.BrowserPreferences = &next
	if e := s.savePreferences(); e != nil {
		s.prefs.BrowserPreferences = old
		return e
	}
	s.configSync.browser = cloneBrowserSync(&next)
	s.prefs.BrowserPreferences = s.configSync.browser
	s.markConfigSyncDirty()
	return nil
}

func cloneBrowserSync(p *BrowserSyncPreferences) *BrowserSyncPreferences {
	if p == nil {
		return nil
	}
	out := *p
	if p.Theme != nil {
		v := *p.Theme
		out.Theme = &v
	}
	if p.ThemeStyle != nil {
		v := *p.ThemeStyle
		out.ThemeStyle = &v
	}
	if p.Language != nil {
		v := *p.Language
		out.Language = &v
	}
	if p.FontScale != nil {
		v := *p.FontScale
		out.FontScale = &v
	}
	return &out
}

func validateSyncedValues(values cloudsync.Values) error {
	if err := validateAssociationSyncValues(values); err != nil {
		return err
	}
	stringsAllowed := map[string][]string{
		"desktop.theme": {"system", "light", "dark"}, "desktop.themeStyle": {"monochrome", "onevra", "omarchy"}, "desktop.language": {"system", "en", "zh-CN"},
		"browser.theme": {"system", "light", "dark"}, "browser.themeStyle": {"monochrome", "onevra"}, "browser.language": {"system", "en", "zh-CN"},
	}
	for key, allowed := range stringsAllowed {
		if value, ok := values[key]; ok {
			var text string
			if string(value) == "null" || json.Unmarshal(value, &text) != nil {
				return &cloudsync.Error{Code: "invalid_document"}
			}
			valid := false
			for _, a := range allowed {
				if text == a {
					valid = true
				}
			}
			if !valid {
				return &cloudsync.Error{Code: "invalid_document"}
			}
		}
	}
	for _, key := range []string{"desktop.fontScale", "browser.fontScale"} {
		if value, ok := values[key]; ok {
			var scale int
			if json.Unmarshal(value, &scale) != nil || scale != 80 && scale != 90 && scale != 100 && scale != 110 && scale != 120 {
				return &cloudsync.Error{Code: "invalid_document"}
			}
		}
	}
	return nil
}
