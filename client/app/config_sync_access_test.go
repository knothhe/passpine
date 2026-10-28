package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/knothhe/passpine/client/secretstore"
	"github.com/knothhe/r2connect"
)

type syncAccessGate struct {
	*oauthMemorySecrets
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *syncAccessGate) Save(key string, value []byte) error {
	g.once.Do(func() {
		close(g.started)
		<-g.release
	})
	return g.oauthMemorySecrets.Save(key, value)
}

func TestConfigSyncRepeatedConnectDoesNotQueueAnotherAccessPrompt(t *testing.T) {
	s, memory, _, _ := oauthFixture(t)
	gate := &syncAccessGate{oauthMemorySecrets: memory, started: make(chan struct{}), release: make(chan struct{})}
	s.configSync.store = gate
	memory.failNextSave = secretstore.ErrDenied
	var release sync.Once
	unblock := func() { release.Do(func() { close(gate.release) }) }
	defer unblock()
	request := Request{Action: "config-sync-oauth-start", SyncConnection: &SyncConnectionRequest{OpenURL: func(string) error {
		return errors.New("unexpected browser launch after denied secret access")
	}}}
	first := make(chan Response, 1)
	go func() { first <- s.Execute(request) }()
	select {
	case <-gate.started:
	case <-time.After(3 * time.Second):
		t.Fatal("connection did not reach the isolated credential store")
	}
	duplicate := make(chan Response, 1)
	go func() { duplicate <- s.Execute(request) }()
	select {
	case response := <-duplicate:
		if response.Code != "busy" || response.Status != "error" || response.Data.ConfigSync == nil || !response.Data.ConfigSync.Busy {
			t.Fatal("duplicate did not return the active connection status")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("duplicate connection queued another access attempt")
	}
	unblock()
	response := <-first
	if response.Code != "credentials_denied" || !memory.Denied() {
		t.Fatalf("first refusal was not retained: %s", response.Code)
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if memory.retries != 1 {
		t.Fatal("duplicate connection reset the access refusal")
	}
}

func TestConfigSyncExternalSDKChangeClearsReadSessionAndPreservesManager(t *testing.T) {
	s, _, _, _ := oauthFixture(t)
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
	authorizeOAuthCandidate(t, s)
	p, v := prepareOAuthCandidate(t, s, "chosen-bucket")
	if err := s.renewR2Credentials(t.Context(), &p, &v); err != nil {
		t.Fatal(err)
	}
	manager := s.configSync.manager
	state := manager.State()
	state.Connections[0].CredentialKey += "-external"
	path := filepath.Join(s.directory, "r2connect-state.json")
	if err := (r2connect.FileStateStore{Path: path}).Save(state); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loads, saves, deletes := backend.counts()
	if err := s.renewR2Credentials(t.Context(), &p, &v); !errors.Is(err, errSyncSessionChanged) {
		t.Fatal("external SDK state change did not stop cached credentials", err)
	}
	response := s.Execute(Request{Action: "config-sync-oauth-accounts"})
	if response.Code != "oauth_state_changed" || response.Data.ConfigSync.OAuthLoadErrorCode != "oauth_state_changed" {
		t.Fatal("external state change did not expose restart guidance")
	}
	if gotLoads, gotSaves, gotDeletes := backend.counts(); gotLoads != loads || gotSaves != saves || gotDeletes != deletes {
		t.Fatal("stopped SDK session accessed the native store")
	}
	if err := s.reloadSyncSDK(); !errors.Is(err, errSyncSessionChanged) || manager != s.configSync.manager {
		t.Fatal("reload discarded a healthy manager's pending rotations", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("external state was overwritten", err)
	}
	if _, err := store.Load(p.Connection.SecretKey()); err != nil {
		t.Fatal(err)
	}
	if gotLoads, _, _ := backend.counts(); gotLoads != loads+1 {
		t.Fatal("external SDK change retained a successful read copy")
	}
}
