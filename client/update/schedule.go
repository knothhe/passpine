package update

import (
	"context"
	"time"
)

func nextCheckTime(now time.Time, interval time.Duration) time.Time {
	if interval > 0 {
		return now.Add(interval)
	}
	next := time.Date(now.Year(), now.Month(), now.Day(), 4, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func waitForCheck(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}

func (m *Manager) runChecks(ctx context.Context, now func() time.Time, wait func(context.Context, time.Duration) bool) {
	if ctx.Err() != nil {
		return
	}
	if s := m.Snapshot(); s.State != StateReady && !(s.State == StateFailed && s.ErrorCode == ErrorInstall && s.Version != "") {
		_ = m.Check(ctx)
	}
	// Compute after startup work, so a check that spans 04:00 is not repeated.
	next := nextCheckTime(now(), m.cfg.CheckInterval)
	for ctx.Err() == nil {
		if delay := next.Sub(now()); delay > 0 {
			// Some monotonic clocks pause during sleep. Bound each wait so a
			// missed wall-clock deadline is noticed within a minute of waking.
			if !wait(ctx, min(delay, time.Minute)) {
				return
			}
			continue
		}
		// Check already serializes operations and preserves pending installs.
		_ = m.Check(ctx)
		// Skip historical deadlines after sleep or a long-running check.
		next = nextCheckTime(now(), m.cfg.CheckInterval)
	}
}
