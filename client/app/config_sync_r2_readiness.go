package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/knothhe/passpine/client/cloudsync"
)

func (s *Service) testPreparedSyncConnection(ctx context.Context, provider string, p syncProfile, v syncSecret) error {
	s.mu.Lock()
	candidate := s.configSync.meta.Candidate
	waitForCredentials := provider == "r2" && candidate != nil && candidate.SecretRef == p.SecretRef
	s.mu.Unlock()

	// R2 IAM changes are eventually consistent: a newly created token can be
	// rejected by the S3 API for up to a minute. Only retry the draft's read-only
	// probe, keeping the same SDK connection and credentials throughout.
	// https://developers.cloudflare.com/r2/reference/consistency/
	delays := [...]time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second, 15 * time.Second, 15 * time.Second}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := s.configSync.engine.Test(ctx, p.Config, v.Secrets)
		if err == nil || !waitForCredentials || attempt == len(delays) {
			return err
		}
		var remote *cloudsync.Error
		if !errors.As(err, &remote) || remote.Code != "unauthorized" || remote.Status != http.StatusUnauthorized && remote.Status != http.StatusForbidden {
			return err
		}
		timer := time.NewTimer(delays[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
