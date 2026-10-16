package vault

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionActivityRenewsIdleWithoutExtendingMaximum(t *testing.T) {
	for _, test := range []struct {
		name          string
		idle, maximum time.Duration
		activityAt    time.Duration
	}{
		{"one minute idle", time.Minute, 30 * time.Minute, 55 * time.Second},
		{"maximum session", time.Hour, 30 * time.Minute, 29*time.Minute + 55*time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := newFixture(t)
			start := time.Now()
			now := start
			var clock atomic.Int64
			clock.Store(now.UnixNano())
			v.session.Lock()
			v.clock = func() time.Time { return time.Unix(0, clock.Load()) }
			v.opened, v.touched = start, start
			v.session.Unlock()
			v.SetTimeouts(test.idle, test.maximum)
			now = start.Add(test.activityAt)
			clock.Store(now.UnixNano())
			v.Activity()
			expiry := min(test.activityAt+test.idle, test.maximum)
			now = start.Add(expiry - time.Second)
			clock.Store(now.UnixNano())
			if !v.Status().Unlocked {
				t.Fatal("session expired before the configured deadline")
			}
			now = start.Add(expiry)
			clock.Store(now.UnixNano())
			if v.Status().Unlocked {
				t.Fatal("session survived its configured deadline")
			}
			if err := v.Unlock(testPassphrase); err != nil {
				t.Fatal(err)
			}
			now = now.Add(5 * time.Second)
			clock.Store(now.UnixNano())
			if !v.Status().Unlocked {
				t.Fatal("newly authenticated session inherited the old deadline")
			}
		})
	}
}
