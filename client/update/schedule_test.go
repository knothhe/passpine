package update

import (
	"context"
	"testing"
	"time"
)

func TestNextCheckTimeUsesLocalCalendar(t *testing.T) {
	local := time.FixedZone("UTC+8", 8*60*60)
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, now, want string
		location        *time.Location
		elapsed         time.Duration
	}{
		{"before four", "2026-10-22 03:59:00", "2026-10-22 04:00:00", local, time.Minute},
		{"at four", "2026-10-22 04:00:00", "2026-10-23 04:00:00", local, 24 * time.Hour},
		{"after four", "2026-10-22 12:00:00", "2026-10-23 04:00:00", local, 16 * time.Hour},
		{"year boundary", "2026-12-31 23:00:00", "2027-01-01 04:00:00", local, 5 * time.Hour},
		{"spring DST", "2026-03-07 04:00:00", "2026-03-08 04:00:00", newYork, 23 * time.Hour},
		{"fall DST", "2026-10-31 04:00:00", "2026-11-01 04:00:00", newYork, 25 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now, err := time.ParseInLocation(time.DateTime, tc.now, tc.location)
			if err != nil {
				t.Fatal(err)
			}
			want, err := time.ParseInLocation(time.DateTime, tc.want, tc.location)
			if err != nil {
				t.Fatal(err)
			}
			got := nextCheckTime(now, 0)
			if !got.Equal(want) || got.Location() != tc.location || got.Sub(now) != tc.elapsed {
				t.Fatalf("next = %s (%s away), want %s (%s away)", got, got.Sub(now), want, tc.elapsed)
			}
		})
	}
}

func TestChecksRunOnStartupAndAtFour(t *testing.T) {
	f := newFixture(t)
	auto := false
	m := f.manager(&auto)
	if m.cfg.CheckInterval != 0 {
		t.Fatalf("default interval = %s, want daily schedule", m.cfg.CheckInterval)
	}
	now := time.Date(2026, 10, 22, 3, 58, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waits := 0
	m.runChecks(ctx, func() time.Time { return now }, func(_ context.Context, delay time.Duration) bool {
		waits++
		if delay != time.Minute {
			t.Fatalf("wait = %s, want one minute", delay)
		}
		if waits == 3 {
			if now.Hour() != 4 || now.Minute() != 0 || f.count("/feed") != 2 {
				t.Fatalf("daily check at %s: %d feed requests", now, f.count("/feed"))
			}
			cancel()
			return false
		}
		if f.count("/feed") != 1 {
			t.Fatal("startup check missing or daily check ran early")
		}
		now = now.Add(delay)
		return true
	})
	if waits != 3 || f.count("/full") != 0 {
		t.Fatalf("waits = %d, downloads = %d", waits, f.count("/full"))
	}
}

func TestStartupAtOrAcrossFourDoesNotRepeat(t *testing.T) {
	for _, across := range []bool{false, true} {
		t.Run(map[bool]string{false: "at four", true: "across four"}[across], func(t *testing.T) {
			f := newFixture(t)
			auto := false
			m := f.manager(&auto)
			now := time.Date(2026, 10, 22, 4, 0, 0, 0, time.UTC)
			if across {
				now = now.Add(-time.Second)
				m.cfg.OnChange = func(s Snapshot) {
					if s.State == StateAvailable {
						now = now.Add(2 * time.Second)
					}
				}
			}
			m.runChecks(context.Background(), func() time.Time { return now }, func(_ context.Context, delay time.Duration) bool {
				if f.count("/feed") != 1 || delay != time.Minute {
					t.Fatalf("startup repeated: %d requests, wait %s", f.count("/feed"), delay)
				}
				return false
			})
		})
	}
}

func TestMissedDailyChecksCatchUpOnce(t *testing.T) {
	f := newFixture(t)
	auto := false
	m := f.manager(&auto)
	now := time.Date(2026, 10, 22, 3, 59, 0, 0, time.UTC)
	waits := 0
	m.runChecks(context.Background(), func() time.Time { return now }, func(_ context.Context, delay time.Duration) bool {
		waits++
		if delay != time.Minute {
			t.Fatalf("wait = %s, want one minute", delay)
		}
		if waits == 1 {
			// Simulate sleep across several deadlines, or a wall-clock jump.
			now = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
			return true
		}
		if f.count("/feed") != 2 {
			t.Fatalf("missed days caused %d requests, want startup and one catch-up", f.count("/feed"))
		}
		return false
	})
	if waits != 2 {
		t.Fatalf("waits = %d, want two", waits)
	}
}

func TestScheduledChecksPreservePendingAndBusyOperations(t *testing.T) {
	for _, state := range []State{StateReady, StateInstalling, StateFailed, StateDownloading} {
		t.Run(string(state), func(t *testing.T) {
			f := newFixture(t)
			m := f.manager(nil)
			if state == StateReady || state == StateFailed {
				if err := m.Check(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			m.publish(func(s *Snapshot) {
				s.State = state
				if state == StateFailed {
					s.ErrorCode = ErrorInstall
				}
			})
			if state == StateDownloading {
				if !m.begin() {
					t.Fatal("could not reserve operation")
				}
				defer m.end()
			}
			before := m.Snapshot()
			feeds, downloads := f.count("/feed"), f.count("/full")
			now := time.Date(2026, 10, 22, 3, 59, 0, 0, time.UTC)
			waits := 0
			m.runChecks(context.Background(), func() time.Time { return now }, func(_ context.Context, delay time.Duration) bool {
				waits++
				if waits == 1 {
					now = now.Add(delay)
					return true
				}
				return false
			})
			if m.Snapshot() != before || f.count("/feed") != feeds || f.count("/full") != downloads {
				t.Fatalf("scheduled check changed pending/busy operation: before %+v, after %+v", before, m.Snapshot())
			}
		})
	}
}

func TestExplicitCheckIntervalOverride(t *testing.T) {
	f := newFixture(t)
	auto := false
	m := f.manager(&auto)
	m.cfg.CheckInterval = 10 * time.Millisecond
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	waits := 0
	m.runChecks(context.Background(), func() time.Time { return now }, func(_ context.Context, delay time.Duration) bool {
		waits++
		if delay != m.cfg.CheckInterval {
			t.Fatalf("wait = %s, want interval override %s", delay, m.cfg.CheckInterval)
		}
		if waits == 1 {
			now = now.Add(delay)
			return true
		}
		if f.count("/feed") != 2 {
			t.Fatalf("override caused %d checks, want two", f.count("/feed"))
		}
		return false
	})
}

func TestScheduledChecksStopOnCancellation(t *testing.T) {
	for _, cancelDuringWait := range []bool{false, true} {
		t.Run(map[bool]string{false: "before startup", true: "while waiting"}[cancelDuringWait], func(t *testing.T) {
			f := newFixture(t)
			auto := false
			m := f.manager(&auto)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if !cancelDuringWait {
				cancel()
			}
			m.runChecks(ctx, time.Now, func(_ context.Context, _ time.Duration) bool {
				cancel()
				// Even if a timer and cancellation become ready together, no
				// subsequent check should run after this canceled wait.
				return true
			})
			want := 0
			if cancelDuringWait {
				want = 1
			}
			if f.count("/feed") != want || waitForCheck(ctx, time.Hour) {
				t.Fatalf("canceled scheduler made %d requests, want %d", f.count("/feed"), want)
			}
		})
	}
}
