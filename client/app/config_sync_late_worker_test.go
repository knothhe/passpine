package app

import (
	"sync"
	"testing"
	"time"
)

// Model a native credential prompt which cannot be interrupted by shutdown.
// ClearSession remains available while Load waits for the simulated response.
type lateWorkerSecrets struct {
	*syncMemorySecrets
	entered chan struct{}
	release chan struct{}
	once    sync.Once

	cacheMu         sync.Mutex
	cached          bool
	readCompleted   bool
	clears          int
	clearedLateRead bool
}

func (m *lateWorkerSecrets) Load(key string) ([]byte, error) {
	m.once.Do(func() { close(m.entered) })
	<-m.release
	value, err := m.syncMemorySecrets.Load(key)
	if err == nil {
		m.cacheMu.Lock()
		m.cached, m.readCompleted = true, true
		m.cacheMu.Unlock()
	}
	return value, err
}

func (m *lateWorkerSecrets) ClearSession() {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	m.clears++
	if m.readCompleted {
		m.clearedLateRead = true
	}
	m.cached = false
}

func TestConfigSyncLateSecretReadIsClearedAfterShutdownDrain(t *testing.T) {
	s, original, _, config, secrets, _ := newSyncAppSpace(t)
	saveSyncFixture(t, s, config, secrets)
	original.mu.Lock()
	values := make(map[string][]byte, len(original.values))
	for key, value := range original.values {
		values[key] = append([]byte(nil), value...)
	}
	original.mu.Unlock()
	store := &lateWorkerSecrets{
		syncMemorySecrets: &syncMemorySecrets{values: values},
		entered:           make(chan struct{}),
		release:           make(chan struct{}),
	}
	s.configSync.store = store
	s.configSync.pendingSecrets = map[string]syncSecret{"unused-session": {Secrets: secrets}}
	var unblock sync.Once
	t.Cleanup(func() { unblock.Do(func() { close(store.release) }) })

	done := make(chan Response, 1)
	go func() { done <- s.Execute(Request{Action: "config-sync-now"}) }()
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("synchronization did not reach the isolated credential prompt")
	}

	// Simulate Close's state after its bounded worker drain has elapsed. This
	// exercises the late completion without waiting for the real drain timeout.
	s.cancelConfigSync()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.clearSyncSecretSession()
	store.cacheMu.Lock()
	beforeClears, beforeRead := store.clears, store.readCompleted
	store.cacheMu.Unlock()
	if beforeClears != 1 || beforeRead {
		t.Fatal("shutdown clear waited for or prematurely completed the blocked read")
	}

	unblock.Do(func() { close(store.release) })
	select {
	case result := <-done:
		if result.Status != "error" || result.Code != "cancelled" {
			t.Fatalf("late worker completed after shutdown: %s %s", result.Status, result.Code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late worker did not return after the credential prompt completed")
	}
	store.cacheMu.Lock()
	defer store.cacheMu.Unlock()
	if !store.readCompleted || store.cached || !store.clearedLateRead || store.clears != 2 {
		t.Fatal("worker retained a successful credential read after shutdown")
	}
	if len(s.configSync.pendingSecrets) != 0 {
		t.Fatal("closed worker retained pending session secrets")
	}
}
