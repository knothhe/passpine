package app

import (
	"context"
	"errors"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/knothhe/passpine/client/cloudsync"
	"github.com/knothhe/passpine/client/i18n"
	"github.com/knothhe/passpine/client/r2config"
	"github.com/knothhe/passpine/client/secretstore"
	"github.com/knothhe/r2connect"
	"github.com/knothhe/r2connect/loopback"
)

// The SDK Manager is retained for the process lifetime so failed refresh writes
// keep their pending rotation inside the SDK.
func (s *Service) syncSecretAccessAllowed() bool {
	if st, ok := s.configSync.store.(interface{ Denied() bool }); ok {
		return !st.Denied()
	}
	return true
}

func (s *Service) syncSecretSessionError() error {
	if s.configSync.store == nil {
		return secretstore.ErrUnavailable
	}
	if !s.syncSecretAccessAllowed() {
		return secretstore.ErrDenied
	}
	if st, ok := s.configSync.store.(interface{ CheckSession() error }); ok {
		return st.CheckSession()
	}
	return nil
}

func (s *Service) clearSyncSecretSession() {
	if st, ok := s.configSync.store.(interface{ ClearSession() }); ok {
		st.ClearSession()
	}
}

func (s *Service) retrySyncSecretAccess() error {
	if s.configSync.store == nil {
		return secretstore.ErrUnavailable
	}
	// A failed baseline write may be the only record of a completed upload.
	// Keep it across explicit access retries unless persisted storage changed.
	if st, ok := s.configSync.store.(interface{ RetrySession() (bool, error) }); ok {
		changed, err := st.RetrySession()
		if err != nil {
			return err
		}
		if changed {
			clear(s.configSync.pendingSecrets)
		}
		return nil
	}
	if st, ok := s.configSync.store.(interface{ CheckSession() error }); ok {
		err := st.CheckSession()
		if errors.Is(err, secretstore.ErrSessionChanged) {
			clear(s.configSync.pendingSecrets)
		} else if err != nil {
			return err
		}
	}
	s.configSync.store.RetryAccess()
	return nil
}

func (s *Service) newSyncManager(clientID, statePath string) (*r2connect.Manager, *r2connect.Client, error) {
	s.configSync.sdkState = newSyncSessionState(statePath, s.clearSyncSecretSession)
	if s.configSync.managerFactory != nil {
		return s.configSync.managerFactory(clientID, s.configSync.sdkState)
	}
	if s.configSync.store == nil {
		return nil, nil, secretstore.ErrUnavailable
	}
	client := r2connect.NewClient(clientID)
	manager := r2connect.NewManager(client, r2connect.ManagerOptions{
		Secrets: s.configSync.store, State: s.configSync.sdkState, TokenNamePrefix: "Passpine-",
		OnWarning: func(r2connect.Warning) {
			s.mu.Lock()
			s.configSync.warningCode = "oauth_cleanup_pending"
			s.mu.Unlock()
		},
	})
	return manager, client, manager.LoadError()
}

func (s *Service) ensureSyncManager() error {
	if s.configSync.manager == nil {
		manager, client, err := s.newSyncManager(r2config.DefaultConfig().ClientID, filepath.Join(s.directory, "r2connect-state.json"))
		s.configSync.manager, s.configSync.client = manager, client
		if err != nil {
			return err
		}
	}
	if err := s.configSync.manager.LoadError(); err != nil {
		return err
	}
	if s.configSync.sdkState != nil {
		return s.configSync.sdkState.Check()
	}
	return nil
}

func (s *Service) initSyncSDK() error {
	err := s.ensureSyncManager()
	s.updateSyncSDKStatus()
	return err
}

// Called outside s.mu: Manager callbacks take s.mu while the SDK is locked.
func (s *Service) updateSyncSDKStatus() {
	p := s.oauthProfile()
	manager, _, err := s.managerForSyncProfile(p)
	status := r2connect.Status{}
	if err == nil {
		err = manager.LoadError()
		if err == nil {
			status = manager.Status()
		}
	}
	s.mu.Lock()
	s.configSync.status.OAuthAuthorized = status.Authorized
	s.configSync.status.OAuthRenewable = status.Renewable
	s.configSync.status.OAuthLoadErrorCode = syncErrorCode(err)
	s.mu.Unlock()
}

// Only failed SDK managers may be reconstructed. Healthy instances retain
// pending refresh rotations and cleanup state across transient failures.
func (s *Service) reloadSyncSDK() error {
	if s.configSync.manager != nil && s.configSync.manager.LoadError() != nil {
		s.clearSyncSecretSession()
		s.configSync.manager, s.configSync.client = nil, nil
	}
	err := s.ensureSyncManager()
	s.updateSyncSDKStatus()
	return err
}

func (s *Service) managerForSyncProfile(p syncProfile) (*r2connect.Manager, *r2connect.Client, error) {
	if err := s.ensureSyncManager(); err != nil {
		return nil, nil, err
	}
	if p.ClientID != "" && p.ClientID != s.configSync.client.ClientID {
		return nil, nil, r2connect.ErrClientChanged
	}
	if err := s.configSync.manager.CheckClient(); err != nil {
		return nil, nil, err
	}
	return s.configSync.manager, s.configSync.client, nil
}

func (s *Service) oauthProfile() syncProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.configSync.meta.Candidate != nil {
		return *s.configSync.meta.Candidate
	}
	return s.configSync.meta.Profiles["r2"]
}

func (s *Service) startSyncOAuth(ctx context.Context, request *SyncConnectionRequest) error {
	cfg := r2config.DefaultConfig()
	if request == nil || request.OpenURL == nil || cfg.ClientID == "" {
		return &cloudsync.Error{Code: "oauth_unavailable"}
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if s.configSync.store == nil {
		return secretstore.ErrUnavailable
	}
	// Only a new explicit connection or disconnect may clear a refusal.
	if err := s.retrySyncSecretAccess(); err != nil {
		return err
	}
	if err := s.discardSyncCandidate(ctx); err != nil {
		return err
	}
	p := s.oauthProfile()
	if p.ClientID != "" && p.ClientID != cfg.ClientID {
		return r2connect.ErrClientChanged
	}
	manager, client, err := s.managerForSyncProfile(p)
	if err != nil {
		return err
	}
	if err = manager.CheckClient(); err != nil {
		return err
	}
	// Probe without loading an old account's credentials before authorization.
	probe := newSyncRef()
	if err = s.configSync.store.Save(probe, []byte("{}")); err != nil {
		return err
	}
	if err = s.configSync.store.Delete(probe); err != nil {
		return err
	}
	candidate := syncProfile{SecretRef: newSyncRef(), ClientID: client.ClientID}
	s.mu.Lock()
	if s.closed || ctx.Err() != nil {
		s.mu.Unlock()
		return context.Canceled
	}
	next := s.configSync.meta
	next.Candidate = &candidate
	if err = s.saveSyncMetadata(next); err != nil {
		s.mu.Unlock()
		return err
	}
	s.configSync.meta = next
	locale := i18n.Resolve(s.prefs.Settings.Language, i18n.SystemLocale())
	s.configSync.status.OAuthStage = "waiting"
	s.mu.Unlock()
	fixedMessage := i18n.Text(locale, "授权已处理，请返回 Passpine 查看结果。")
	flow, err := loopback.Start(client, loopback.Options{
		RedirectURI: cfg.RedirectURL,
		Accept:      manager.Authorize,
		Done: func(result error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if result == nil {
				s.configSync.status.OAuthStage = "bucket"
			} else {
				s.configSync.status.OAuthStage = ""
			}
			s.configSync.status.ErrorCode = syncErrorCode(result)
		},
		Messages: loopback.Messages{Connected: fixedMessage, Denied: fixedMessage, InvalidScope: fixedMessage, NotAuthorized: fixedMessage, Canceled: fixedMessage, Failed: fixedMessage},
	})
	if err != nil {
		s.mu.Lock()
		s.configSync.status.OAuthStage = ""
		s.mu.Unlock()
		return err
	}
	defer flow.Close()
	waited := make(chan error, 1)
	go func() { _, err := flow.Wait(ctx); waited <- err }()
	if ctx.Err() != nil {
		_ = flow.Close()
		<-waited
		return ctx.Err()
	}
	if err = request.OpenURL(flow.URL()); err != nil {
		_ = flow.Close()
		<-waited
		return &cloudsync.Error{Code: "oauth_network_failed"}
	}
	if err = <-waited; err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return s.resumeSyncOAuth(ctx)
}

func (s *Service) resumeSyncOAuth(ctx context.Context) error {
	p := s.oauthProfile()
	manager, _, err := s.managerForSyncProfile(p)
	if err != nil {
		return err
	}
	if !s.syncSecretAccessAllowed() {
		return secretstore.ErrDenied
	}
	accounts, err := manager.Accounts(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.configSync.status.Accounts = accounts
	s.configSync.status.OAuthStage = "bucket"
	s.mu.Unlock()
	return nil
}

func (s *Service) listSyncBuckets(ctx context.Context, request *SyncConnectionRequest) error {
	if request == nil {
		return r2connect.ErrBucketUnavailable
	}
	p := s.oauthProfile()
	manager, _, err := s.managerForSyncProfile(p)
	if err != nil {
		return err
	}
	if !s.syncSecretAccessAllowed() {
		return secretstore.ErrDenied
	}
	jurisdiction := normalizeSyncJurisdiction(request.Jurisdiction)
	buckets, err := manager.Buckets(ctx, request.AccountID, jurisdiction)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.configSync.status.Buckets = buckets
	s.configSync.status.AccountID = request.AccountID
	s.configSync.status.Jurisdiction = jurisdiction
	s.configSync.status.OAuthStage = "bucket"
	s.mu.Unlock()
	return nil
}

func normalizeSyncJurisdiction(value string) string {
	if value == "" {
		return "default"
	}
	return value
}

func (s *Service) prepareR2Connection(ctx context.Context, request *SyncConnectionRequest) (syncProfile, syncSecret, error) {
	if request == nil {
		return syncProfile{}, syncSecret{}, r2connect.ErrBucketUnavailable
	}
	s.mu.Lock()
	candidate := s.configSync.meta.Candidate
	s.mu.Unlock()
	if candidate == nil {
		return s.prepareSavedR2Connection(ctx, request)
	}
	p := *candidate
	if utf8.RuneCountInString(request.Secrets.Passphrase) < 12 || len(request.Secrets.Passphrase) > 4096 {
		return p, syncSecret{}, &cloudsync.Error{Code: "invalid_passphrase"}
	}
	manager, _, err := s.managerForSyncProfile(p)
	if err != nil {
		return p, syncSecret{}, err
	}
	if p.Connection != nil {
		present := false
		for _, saved := range manager.State().Connections {
			if saved.ID == p.Connection.ID {
				if saved.AccountID != p.Connection.AccountID || saved.Bucket != p.Connection.Bucket || saved.Jurisdiction != p.Connection.Jurisdiction {
					return p, syncSecret{}, r2connect.ErrCredentialsInvalid
				}
				present = true
			}
		}
		if !present {
			// A prior discard can succeed before app metadata fails. This is
			// still a draft, so retry creates a replacement SDK candidate.
			p.Connection = nil
		}
	}
	jurisdiction := normalizeSyncJurisdiction(request.Jurisdiction)
	if p.Connection != nil && (p.Connection.AccountID != request.AccountID || p.Connection.Bucket != request.Bucket || p.Connection.Jurisdiction != jurisdiction) {
		if err = manager.Discard(ctx, p.Connection.ID); err != nil {
			return p, syncSecret{}, err
		}
		p.Connection = nil
	}
	if p.Connection == nil {
		if err = s.discardUnreferencedSyncConnections(ctx, manager); err != nil {
			return p, syncSecret{}, err
		}
		connection, err := manager.Connect(ctx, request.AccountID, request.Bucket, jurisdiction)
		if err != nil {
			return p, syncSecret{}, err
		}
		p.Connection = &connection
	}
	endpoint, err := r2connect.Endpoint(p.Connection.AccountID, p.Connection.Jurisdiction)
	if err != nil {
		return p, syncSecret{}, err
	}
	p.Config, err = cloudsync.NormalizeConfig(cloudsync.Config{Endpoint: endpoint, Region: "auto", Bucket: p.Connection.Bucket, Prefix: request.Config.Prefix, PathStyle: true})
	if err != nil {
		return p, syncSecret{}, err
	}
	p.AccountID, p.Jurisdiction = p.Connection.AccountID, p.Connection.Jurisdiction
	s.mu.Lock()
	if s.closed || ctx.Err() != nil {
		err = context.Canceled
	} else {
		next := s.configSync.meta
		next.Candidate = &p
		err = s.saveSyncMetadata(next)
		if err == nil {
			s.configSync.meta = next
		}
	}
	s.mu.Unlock()
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if s.syncSecretAccessAllowed() {
			_ = manager.Discard(cleanup, p.Connection.ID)
		}
		cancel()
		return p, syncSecret{}, err
	}
	v := syncSecret{Secrets: cloudsync.Secrets{Passphrase: request.Secrets.Passphrase}, ConnectionID: p.Connection.ID}
	if err = s.renewR2Credentials(ctx, &p, &v); err != nil {
		return p, v, err
	}
	return p, v, nil
}

func (s *Service) prepareSavedR2Connection(ctx context.Context, request *SyncConnectionRequest) (syncProfile, syncSecret, error) {
	s.mu.Lock()
	p, exists := s.configSync.meta.Profiles["r2"]
	s.mu.Unlock()
	if !exists {
		return p, syncSecret{}, r2connect.ErrNotAuthorized
	}
	v, err := s.loadSyncSecret(p.SecretRef)
	if err != nil {
		return p, v, err
	}
	if p.Connection == nil {
		return p, v, r2connect.ErrUnknownConnection
	}
	connection := *p.Connection
	if request.AccountID != "" && request.AccountID != connection.AccountID || request.Bucket != "" && request.Bucket != connection.Bucket || request.Jurisdiction != "" && request.Jurisdiction != connection.Jurisdiction {
		return p, v, r2connect.ErrBucketUnavailable
	}
	config, err := cloudsync.NormalizeConfig(cloudsync.Config{Endpoint: p.Config.Endpoint, Region: "auto", Bucket: connection.Bucket, Prefix: request.Config.Prefix, PathStyle: true})
	if err != nil {
		return p, v, err
	}
	phrase := request.Secrets.Passphrase
	if phrase == "" {
		if !sameSyncSpace(config, p.Config) {
			return p, v, &cloudsync.Error{Code: "invalid_passphrase"}
		}
		phrase = v.Secrets.Passphrase
	}
	if !sameSyncSpace(config, p.Config) || phrase != v.Secrets.Passphrase {
		v.Baseline = nil
	}
	p.Config = config
	v.Secrets.Passphrase = phrase
	if err = s.renewR2Credentials(ctx, &p, &v); err != nil {
		return p, v, err
	}
	p.SecretRef = newSyncRef()
	return p, v, nil
}

// This name is retained at the service boundary; all credential validity,
// refresh, token creation, renewal and compensation belong to SDK Credentials.
func (s *Service) renewR2Credentials(ctx context.Context, p *syncProfile, v *syncSecret) error {
	if p.Connection == nil {
		return r2connect.ErrUnknownConnection
	}
	endpoint, err := r2connect.Endpoint(p.Connection.AccountID, p.Connection.Jurisdiction)
	if err != nil || endpoint != p.Config.Endpoint || p.Config.Bucket != p.Connection.Bucket || p.AccountID != "" && p.AccountID != p.Connection.AccountID || p.Jurisdiction != "" && p.Jurisdiction != p.Connection.Jurisdiction {
		return r2connect.ErrCredentialsInvalid
	}
	if !s.syncSecretAccessAllowed() {
		return secretstore.ErrDenied
	}
	manager, _, err := s.managerForSyncProfile(*p)
	if err != nil {
		return err
	}
	credentials, err := manager.Credentials(ctx, *p.Connection)
	if err != nil {
		return err
	}
	// Early renewal can return still-valid credentials after a failed state
	// write. An external state change must stop that fallback as well.
	if err := s.ensureSyncManager(); err != nil {
		return err
	}
	// SDK early-renew fallback must not resume app background work after a
	// system store prompt was refused during that renewal.
	if err := s.syncSecretSessionError(); err != nil {
		return err
	}
	v.Secrets.AccessKeyID, v.Secrets.SecretAccessKey = credentials.AccessKeyID, credentials.SecretAccessKey
	v.Secrets.SessionToken = ""
	v.ConnectionID = p.Connection.ID
	return cloudsync.ValidateSecrets(v.Secrets)
}

func (s *Service) otherSyncProfileUsesSession(p syncProfile) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, saved := range s.configSync.meta.Profiles {
		if saved.SecretRef != p.SecretRef && saved.ClientID != "" && !saved.RemoteClean {
			return true
		}
	}
	candidate := s.configSync.meta.Candidate
	return candidate != nil && candidate.SecretRef != p.SecretRef && candidate.ClientID != "" && !candidate.RemoteClean
}

// A crash or application metadata failure can leave a Connect result only in
// SDK state. Discard those candidates through the SDK while retaining every
// connection currently referenced by a saved profile or the current draft.
func (s *Service) discardUnreferencedSyncConnections(ctx context.Context, manager *r2connect.Manager) error {
	keep := map[string]bool{}
	s.mu.Lock()
	for _, saved := range s.configSync.meta.Profiles {
		if saved.Connection != nil {
			keep[saved.Connection.ID] = true
		}
	}
	if draft := s.configSync.meta.Candidate; draft != nil && draft.Connection != nil {
		keep[draft.Connection.ID] = true
	}
	s.mu.Unlock()
	for _, connection := range manager.State().Connections {
		if !keep[connection.ID] {
			if err := manager.Discard(ctx, connection.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) cleanSyncProfile(ctx context.Context, p syncProfile) error {
	if p.RemoteClean || p.ClientID == "" && p.Connection == nil {
		return nil
	}
	if !s.syncSecretAccessAllowed() {
		return secretstore.ErrDenied
	}
	manager, _, err := s.managerForSyncProfile(p)
	if err != nil {
		return err
	}
	if s.otherSyncProfileUsesSession(p) {
		if p.Connection != nil {
			return manager.Discard(ctx, p.Connection.ID)
		}
		return s.discardUnreferencedSyncConnections(ctx, manager)
	}
	if err := manager.Disconnect(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	s.configSync.warningCode = ""
	s.mu.Unlock()
	return nil
}

func (s *Service) cleanupRetiredSyncProfiles(ctx context.Context) error {
	return s.cleanupSyncProfiles(ctx, false)
}

// Manual S3 remains usable even when an unrelated Cloudflare grant cannot be
// loaded or cleaned. Only records which are already locally owned are eligible.
func (s *Service) cleanupLocalSyncProfiles(ctx context.Context) error {
	return s.cleanupSyncProfiles(ctx, true)
}

func (s *Service) cleanupSyncProfiles(ctx context.Context, localOnly bool) error {
	for {
		s.mu.Lock()
		index := -1
		for i, profile := range s.configSync.meta.PendingCleanup {
			if !localOnly || profile.RemoteClean || profile.ClientID == "" && profile.Connection == nil {
				index = i
				break
			}
		}
		if index < 0 {
			if len(s.configSync.meta.PendingCleanup) > 0 {
				s.configSync.warningCode = "oauth_cleanup_pending"
			}
			s.mu.Unlock()
			return nil
		}
		p := s.configSync.meta.PendingCleanup[index]
		for _, active := range s.configSync.meta.Profiles {
			if active.SecretRef == p.SecretRef {
				s.mu.Unlock()
				return &cloudsync.Error{Code: "invalid_document"}
			}
		}
		s.mu.Unlock()
		if !p.RemoteClean {
			if err := s.cleanSyncProfile(ctx, p); err != nil {
				return err
			}
			s.mu.Lock()
			next := s.configSync.meta
			next.PendingCleanup = append([]syncProfile(nil), next.PendingCleanup...)
			next.PendingCleanup[index].RemoteClean = true
			err := s.saveSyncMetadata(next)
			if err == nil {
				s.configSync.meta = next
			}
			s.mu.Unlock()
			if err != nil {
				return err
			}
		}
		if err := s.deleteSyncProfileSecret(p); err != nil && !errors.Is(err, secretstore.ErrNotFound) {
			return err
		}
		delete(s.configSync.pendingSecrets, p.SecretRef)
		s.mu.Lock()
		next := s.configSync.meta
		next.PendingCleanup = append(append([]syncProfile(nil), next.PendingCleanup[:index]...), next.PendingCleanup[index+1:]...)
		err := s.saveSyncMetadata(next)
		if err == nil {
			s.configSync.meta = next
		}
		s.mu.Unlock()
		if err != nil {
			return err
		}
	}
}

func (s *Service) discardSyncCandidate(ctx context.Context) error {
	s.mu.Lock()
	candidate := s.configSync.meta.Candidate
	s.mu.Unlock()
	if candidate == nil {
		return s.cleanupRetiredSyncProfiles(ctx)
	}
	p := *candidate
	if err := s.cleanSyncProfile(ctx, p); err != nil {
		return err
	}
	p.RemoteClean = true
	s.mu.Lock()
	next := s.configSync.meta
	next.Candidate = nil
	next.PendingCleanup = append(append([]syncProfile(nil), next.PendingCleanup...), p)
	err := s.saveSyncMetadata(next)
	if err == nil {
		s.configSync.meta = next
		s.configSync.status.OAuthStage = ""
		s.configSync.status.Accounts = nil
		s.configSync.status.Buckets = nil
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.cleanupRetiredSyncProfiles(ctx)
}

func (s *Service) disconnectSync(ctx context.Context) error {
	s.mu.Lock()
	manualOnly := s.configSync.meta.Active == "s3"
	s.mu.Unlock()
	if s.configSync.store != nil {
		if err := s.retrySyncSecretAccess(); err != nil {
			return err
		}
	}
	if !manualOnly {
		if err := s.discardSyncCandidate(ctx); err != nil {
			return err
		}
	}
	s.mu.Lock()
	meta := s.configSync.meta
	p, exists := meta.Profiles[meta.Active]
	s.mu.Unlock()
	if !exists {
		return nil
	}
	if err := s.cleanSyncProfile(ctx, p); err != nil {
		return err
	}
	p.RemoteClean = true
	s.mu.Lock()
	next := s.configSync.meta
	profiles := map[string]syncProfile{}
	for key, profile := range next.Profiles {
		if key != meta.Active {
			profiles[key] = profile
		}
	}
	next.Profiles, next.Active, next.Enabled, next.LastSyncedAt = profiles, "", false, ""
	next.PendingCleanup = append(append([]syncProfile(nil), next.PendingCleanup...), p)
	err := s.saveSyncMetadata(next)
	if err == nil {
		s.configSync.meta = next
		s.configSync.status.Ready = false
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if manualOnly {
		return s.cleanupLocalSyncProfiles(ctx)
	}
	return s.cleanupRetiredSyncProfiles(ctx)
}

func (s *Service) deleteSyncProfileSecret(p syncProfile) error {
	return s.configSync.store.Delete(p.SecretRef)
}
