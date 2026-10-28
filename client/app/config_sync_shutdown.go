package app

import (
	"context"
	"time"
)

// Closing retains the active saved connection. Only unsaved authorizations and
// recorded cleanup work are discarded, with an independent bounded context.
func (s *Service) closeConfigSync() {
	defer s.clearSyncSecretSession()
	s.mu.Lock()
	skip := s.preview || s.configSync.invalid
	s.mu.Unlock()
	if skip {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for !s.configSyncMu.TryLock() {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
	defer s.configSyncMu.Unlock()
	defer clear(s.configSync.pendingSecrets)
	_ = s.discardSyncCandidate(ctx)
}
