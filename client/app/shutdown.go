package app

import (
	"context"
	"time"
)

// Shutdown stops admission before draining desktop, browser and Git requests.
// A timeout leaves the service available for a later retry.
func (s *Service) Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	previous := s.updatePaused.Swap(true)
	s.cancelConfigSync()
	s.cancelDeviceAuthentication()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for !s.updateRequests.TryLock() {
		select {
		case <-ctx.Done():
			s.updatePaused.Store(previous)
			return ctx.Err()
		case <-ticker.C:
		}
	}
	defer s.updateRequests.Unlock()
	if err := ctx.Err(); err != nil {
		s.updatePaused.Store(previous)
		return err
	}
	return s.Close()
}
