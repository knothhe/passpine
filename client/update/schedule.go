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

// currentCheckPeriod uses calendar days, preserving 04:00 across DST changes.
func currentCheckPeriod(now time.Time) time.Time {
	start := time.Date(now.Year(), now.Month(), now.Day(), 4, 0, 0, 0, now.Location())
	if now.Before(start) {
		start = start.AddDate(0, 0, -1)
	}
	return start
}

func (m *Manager) runChecks(ctx context.Context, now func() time.Time, wait func(context.Context, time.Duration) bool) {
	var overrideNext time.Time
	for ctx.Err() == nil {
		current := now()
		r := m.scheduledRecord(current)
		satisfied := !r.CheckedAt.IsZero() && !r.CheckedAt.Before(currentCheckPeriod(current))
		if m.cfg.CheckInterval > 0 {
			satisfied = !overrideNext.IsZero() && current.Before(overrideNext)
		}
		delay := time.Minute
		s := m.Snapshot()
		pending := s.State == StateReady || s.State == StateInstalling || s.State == StateFailed && s.ErrorCode == ErrorInstall && s.Version != ""
		if !pending {
			switch {
			case !satisfied && r.Retry == "download":
				// A broken old package must not hide a corrected/new feed on
				// the next daily boundary, even if its download retry is later.
				_ = m.checkMode(ctx, now, true)
			case r.Retry != "" && current.Before(r.RetryAt):
				delay = min(delay, r.RetryAt.Sub(current))
			case r.Retry == "feed" || !satisfied:
				_ = m.checkMode(ctx, now, true)
				if m.cfg.CheckInterval > 0 {
					overrideNext = nextCheckTime(now(), m.cfg.CheckInterval)
				}
			case r.Retry == "download" && m.Snapshot().AutoDownload:
				_ = m.downloadScheduled(ctx, now)
			case r.Retry == "" && m.Snapshot().AutoDownload && m.Snapshot().State == StateAvailable:
				_ = m.downloadScheduled(ctx, now)
			}
		}
		if ctx.Err() != nil {
			return
		}
		current = now()
		r = m.scheduledRecord(current)
		if r.Retry != "" && r.RetryAt.After(current) {
			delay = min(delay, r.RetryAt.Sub(current))
		}
		if m.cfg.CheckInterval > 0 && overrideNext.After(current) {
			delay = min(delay, overrideNext.Sub(current))
		} else {
			delay = min(delay, nextCheckTime(current, 0).Sub(current))
		}
		// Re-read wall time at least once per minute. Waking after several
		// missed days causes one current-period catch-up, not historical checks.
		if !wait(ctx, delay) {
			return
		}
	}
}

func (m *Manager) downloadScheduled(ctx context.Context, now func() time.Time) error {
	return m.downloadAutomatic(ctx, now, true)
}

func (m *Manager) downloadAutomatic(ctx context.Context, now func() time.Time, scheduled bool) error {
	if !m.begin() {
		return ErrBusy
	}
	defer m.end()
	if m.pendingInstallation() || !m.Snapshot().AutoDownload || ctx.Err() != nil {
		return ctx.Err()
	}
	if scheduled {
		// A manual retry may have just failed while this operation waited.
		// Respect its new backoff and let the scheduler refresh an old feed.
		current := now()
		r := m.scheduledRecord(current)
		if r.CheckedAt.IsZero() || r.CheckedAt.Before(currentCheckPeriod(current)) ||
			r.Retry != "" && (r.Retry != "download" || current.Before(r.RetryAt)) {
			return nil
		}
	}
	return m.downloadWithRetry(ctx, now)
}
