package app

import (
	"errors"
	"testing"
	"time"

	"github.com/knothhe/passpine/client/secretstore"
)

type checkingSyncSecrets struct {
	*syncMemorySecrets
	checkErr error
	checks   int
}

func (s *checkingSyncSecrets) CheckSession() error {
	s.checks++
	return s.checkErr
}

func TestConfigSyncChangedStoreStopsPendingSecretsAndAutomaticRetries(t *testing.T) {
	s, memory, _, config, secrets, bucket := newSyncAppSpace(t)
	saveSyncFixture(t, s, config, secrets)
	profile := s.configSync.meta.Profiles["s3"]
	s.configSync.pendingSecrets = map[string]syncSecret{profile.SecretRef: {Secrets: secrets}}
	store := &checkingSyncSecrets{syncMemorySecrets: memory, checkErr: secretstore.ErrSessionChanged}
	s.configSync.store = store
	loads := memory.loads
	_, _, requests := bucket.snapshot()
	response := s.Execute(Request{Action: "config-sync-now"})
	if response.Status != "error" || response.Code != "credentials_state_changed" || store.checks != 1 || memory.loads != loads {
		t.Fatal("pending credentials concealed a changed encrypted store")
	}
	if _, _, after := bucket.snapshot(); after != requests {
		t.Fatal("changed encrypted store reached object storage")
	}
	s.configSync.next = time.Time{}
	s.scheduleConfigSync()
	if s.configSync.queued {
		t.Fatal("changed encrypted store queued another automatic attempt")
	}
	s.executeConfigSync(Request{Action: "config-sync-now"}, "automatic")
	if store.checks != 1 {
		t.Fatal("already queued worker retried a stopped credential session")
	}
	if len(s.configSync.pendingSecrets) != 1 {
		t.Fatal("metadata check discarded failed writes before a new session was requested")
	}
	if err := s.retrySyncSecretAccess(); err != nil {
		t.Fatal(err)
	}
	if len(s.configSync.pendingSecrets) != 0 || memory.retries != 2 {
		// The initial manual Save and the explicit new session each retry once.
		t.Fatal("new connection session retained stale pending credentials")
	}
}

func TestConfigSyncPendingSecretsCheckPropagatesTemporaryFailure(t *testing.T) {
	want := errors.New("isolated metadata read failure")
	store := &checkingSyncSecrets{syncMemorySecrets: &syncMemorySecrets{}, checkErr: want}
	s := &Service{configSync: configSyncState{store: store, pendingSecrets: map[string]syncSecret{"pending": {}}}}
	if _, err := s.loadSyncSecret("pending"); !errors.Is(err, want) || store.checks != 1 || len(s.configSync.pendingSecrets) != 1 {
		t.Fatal("temporary metadata failure bypassed the guard or lost the pending retry", err)
	}
}

func TestConfigSyncAccessRetryKeepsPendingAfterMetadataReadFailure(t *testing.T) {
	want := errors.New("isolated metadata read failure")
	store := &checkingSyncSecrets{syncMemorySecrets: &syncMemorySecrets{}, checkErr: want}
	s := &Service{configSync: configSyncState{store: store, pendingSecrets: map[string]syncSecret{"pending": {}}}}
	if err := s.retrySyncSecretAccess(); !errors.Is(err, want) || store.checks != 1 || store.retries != 0 || len(s.configSync.pendingSecrets) != 1 {
		t.Fatal("failed metadata check reopened access or lost a pending baseline", err)
	}
}
