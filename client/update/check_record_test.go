package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type scheduleTransport func(*http.Request) (*http.Response, error)

func (f scheduleTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Keep short retry tests away from 04:00 while remaining in the past for New's
// real-clock restoration validation. Each test advances this private clock.
func scheduleTestTime() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day()-1, 12, 0, 0, 0, time.UTC)
}

func schedulerOnce(m *Manager, now time.Time) {
	m.runChecks(context.Background(), func() time.Time { return now }, func(context.Context, time.Duration) bool { return false })
}

func TestSuccessfulCheckPersistsAndRestoresCandidateWithoutNetwork(t *testing.T) {
	for _, latest := range []bool{false, true} {
		t.Run(map[bool]string{false: "candidate", true: "latest"}[latest], func(t *testing.T) {
			f := newFixture(t)
			if latest {
				// Preserve compatibility with signed latest feeds that have no
				// installable asset for the current platform.
				f.manifest.Version, f.manifest.Assets = "1.0.0", nil
				f.sign()
			}
			auto := false
			m := f.manager(&auto)
			now := scheduleTestTime()
			if err := m.check(context.Background(), func() time.Time { return now }); err != nil {
				t.Fatal(err)
			}
			restored := f.manager(&auto)
			want := StateAvailable
			if latest {
				want = StateLatest
			}
			if s := restored.Snapshot(); s.State != want || s.Version != f.manifest.Version {
				t.Fatalf("restored = %+v", s)
			}
			schedulerOnce(restored, now.Add(time.Second))
			if f.count("/feed") != 1 || f.count("/full") != 0 {
				t.Fatal("restart repeated the successful period")
			}
			if !latest {
				if err := restored.Download(context.Background()); err != nil || restored.Snapshot().State != StateReady {
					t.Fatalf("restored candidate cannot download: %v, %+v", err, restored.Snapshot())
				}
			}
			st, err := os.Stat(filepath.Join(m.cacheDir(), checkRecordName))
			if err != nil || st.Mode().Perm() != 0o600 {
				t.Fatalf("record permissions: %v, %v", st, err)
			}
		})
	}
}

func TestManualCheckSatisfiesPeriodAndNextPeriodCatchesUp(t *testing.T) {
	f := newFixture(t)
	auto := false
	m := f.manager(&auto)
	if err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	schedulerOnce(m, now)
	if f.count("/feed") != 1 {
		t.Fatal("manual success was ignored")
	}
	now = nextCheckTime(now, 0)
	schedulerOnce(m, now)
	if f.count("/feed") != 2 {
		t.Fatal("new local period did not check")
	}
	// Returning after days of not running checks only the current period.
	schedulerOnce(m, now.AddDate(0, 0, 4).Add(time.Hour))
	if f.count("/feed") != 3 {
		t.Fatal("missed periods replayed")
	}
}

func TestFailedCheckPersistsBackoffAndRecovers(t *testing.T) {
	f := newFixture(t)
	auto := false
	m := f.manager(&auto)
	valid := f.body
	f.body = []byte("invalid feed")
	now := scheduleTestTime()
	schedulerOnce(m, now)
	if r := m.checkRecord; r.Retry != "feed" || r.Failures != 1 || !r.RetryAt.Equal(now.Add(5*time.Minute)) || !r.CheckedAt.IsZero() {
		t.Fatalf("first failure: %+v", r)
	}
	restarted := f.manager(&auto)
	schedulerOnce(restarted, now.Add(time.Minute))
	if f.count("/feed") != 1 {
		t.Fatal("restart bypassed persisted backoff")
	}
	now = restarted.checkRecord.RetryAt
	schedulerOnce(restarted, now)
	if r := restarted.checkRecord; r.Failures != 2 || !r.RetryAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("second failure: %+v", r)
	}
	f.body = valid
	now = restarted.checkRecord.RetryAt
	schedulerOnce(restarted, now)
	if r := restarted.checkRecord; r.Retry != "" || r.Failures != 0 || !r.CheckedAt.Equal(now) || restarted.Snapshot().State != StateAvailable {
		t.Fatalf("retry did not recover: %+v, %+v", r, restarted.Snapshot())
	}
	schedulerOnce(restarted, now.Add(time.Second))
	if f.count("/feed") != 3 {
		t.Fatal("success did not stop retrying")
	}
}

func TestManualCheckBypassesBackoffAndClearsFailures(t *testing.T) {
	f := newFixture(t)
	auto := false
	m := f.manager(&auto)
	valid := f.body
	f.body = []byte("bad")
	if m.Check(context.Background()) == nil {
		t.Fatal("invalid feed succeeded")
	}
	f.body = valid
	if err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	schedulerOnce(m, time.Now())
	if f.count("/feed") != 2 || m.checkRecord.Retry != "" {
		t.Fatal("manual success did not clear retry")
	}
}

func TestAutomaticDownloadRetriesWithoutRefetchingFeed(t *testing.T) {
	f := newFixture(t)
	m := f.manager(nil)
	baseTransport := m.cfg.HTTPClient.Transport
	attempts := 0
	transport := scheduleTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/full" {
			attempts++
			if attempts <= 2 {
				return nil, errors.New("offline")
			}
		}
		return baseTransport.RoundTrip(r)
	})
	client := *m.cfg.HTTPClient
	client.Transport = transport
	m.cfg.HTTPClient = &client
	now := scheduleTestTime()
	schedulerOnce(m, now)
	if r := m.checkRecord; r.Retry != "download" || r.CheckedAt.IsZero() || r.Failures != 1 {
		t.Fatalf("download retry = %+v", r)
	}
	restored := f.manager(nil)
	restored.cfg.HTTPClient = &client
	schedulerOnce(restored, now.Add(time.Minute))
	if attempts != 1 {
		t.Fatal("restart bypassed download backoff")
	}
	now = restored.checkRecord.RetryAt
	schedulerOnce(restored, now)
	if r := restored.checkRecord; r.Failures != 2 || !r.RetryAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("second download retry: %+v", r)
	}
	now = restored.checkRecord.RetryAt
	schedulerOnce(restored, now)
	if restored.Snapshot().State != StateReady || restored.checkRecord.Retry != "" || attempts != 3 || f.count("/feed") != 1 {
		t.Fatalf("download recovery: %+v; attempts=%d feeds=%d", restored.Snapshot(), attempts, f.count("/feed"))
	}
}

func TestBackoffCapsAndCanceledChecksDoNotScheduleRetry(t *testing.T) {
	f := newFixture(t)
	auto := false
	m := f.manager(&auto)
	f.body = []byte("bad")
	now := scheduleTestTime()
	for i := 0; i < 8; i++ {
		schedulerOnce(m, now)
		want := retryDelays[min(i, len(retryDelays)-1)]
		if got := m.checkRecord.RetryAt.Sub(now); got != want {
			t.Fatalf("failure %d delay = %s, want %s", i+1, got, want)
		}
		now = m.checkRecord.RetryAt
	}
	fresh := newFixture(t).manager(&auto)
	ctx, cancel := context.WithCancel(context.Background())
	base := fresh.cfg.HTTPClient.Transport
	client := *fresh.cfg.HTTPClient
	client.Transport = scheduleTransport(func(r *http.Request) (*http.Response, error) {
		cancel()
		return base.RoundTrip(r)
	})
	fresh.cfg.HTTPClient = &client
	if fresh.Check(ctx) == nil || fresh.checkRecord.Retry != "" || !fresh.checkRecord.CheckedAt.IsZero() {
		t.Fatalf("cancellation created a record: %+v", fresh.checkRecord)
	}
	if _, err := os.Stat(filepath.Join(fresh.cacheDir(), checkRecordName)); !os.IsNotExist(err) {
		t.Fatalf("cancellation persisted record: %v", err)
	}
}

func TestRecordStorageFailureKeepsVerifiedStateAndRetriesWrite(t *testing.T) {
	f := newFixture(t)
	auto := false
	m := f.manager(&auto)
	name := filepath.Join(m.cacheDir(), checkRecordName)
	if err := os.Mkdir(name, 0o700); err != nil {
		t.Fatal(err)
	}
	now := scheduleTestTime()
	schedulerOnce(m, now)
	if !m.recordDirty || m.Snapshot().State != StateAvailable {
		t.Fatalf("storage failure discarded result: dirty=%v %+v", m.recordDirty, m.Snapshot())
	}
	schedulerOnce(m, now.Add(time.Second))
	if f.count("/feed") != 1 {
		t.Fatal("storage failure repeated the feed")
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	schedulerOnce(m, now.Add(2*time.Second))
	if m.recordDirty || f.count("/feed") != 1 {
		t.Fatal("record was not independently retried")
	}
	if restored := f.manager(&auto); restored.Snapshot().State != StateAvailable {
		t.Fatal("recovered record was not restorable")
	}
}

func TestBadOrMismatchedRecordsNeverSuppressChecks(t *testing.T) {
	for _, damage := range []string{"truncated", "schema", "scope", "signature", "future-success", "future-retry", "symlink"} {
		t.Run(damage, func(t *testing.T) {
			f := newFixture(t)
			auto := false
			m := f.manager(&auto)
			now := scheduleTestTime()
			if err := m.check(context.Background(), func() time.Time { return now }); err != nil {
				t.Fatal(err)
			}
			r := m.checkRecord
			name := filepath.Join(m.cacheDir(), checkRecordName)
			switch damage {
			case "schema":
				r.Schema++
			case "scope":
				r.Scope = "different version/feed/key/platform"
			case "signature":
				r.Envelope = json.RawMessage(`{"payload":"e30=","signature":"bad"}`)
			case "future-success":
				r.CheckedAt = time.Now().Add(24 * time.Hour)
			case "future-retry":
				r.Retry, r.Failures = "feed", 1
				r.FailedAt = time.Now().Add(24 * time.Hour)
				r.RetryAt = r.FailedAt.Add(5 * time.Minute)
			}
			data, _ := json.Marshal(r)
			if damage == "truncated" {
				data = []byte("{")
			}
			if damage == "symlink" {
				outside := filepath.Join(t.TempDir(), "outside.json")
				writeTestFile(t, outside, data)
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, name); err != nil {
					t.Skip(err)
				}
			} else if err := os.WriteFile(name, data, 0o600); err != nil {
				t.Fatal(err)
			}
			restored := f.manager(&auto)
			if restored.Snapshot().State != StateIdle {
				t.Fatal("bad record restored UI")
			}
			schedulerOnce(restored, now.Add(time.Second))
			if f.count("/feed") != 2 || restored.Snapshot().State != StateAvailable {
				t.Fatal("bad record suppressed check")
			}
		})
	}
}

func TestCheckScopeTracksVersionFeedKeyAndPlatform(t *testing.T) {
	f := newFixture(t)
	m := f.manager(nil)
	original := m.checkScope()
	for _, field := range []*string{&m.cfg.CurrentVersion, &m.cfg.FeedURL, &m.cfg.PublicKey, &m.cfg.OS, &m.cfg.Arch} {
		before := *field
		*field += "changed"
		if m.checkScope() == original {
			t.Fatal("configuration did not affect scope")
		}
		*field = before
	}
}

func TestNewPeriodRefreshesFeedBeforeRetryingBrokenOldPackage(t *testing.T) {
	f := newFixture(t)
	m := f.manager(nil)
	baseTransport := m.cfg.HTTPClient.Transport
	client := *m.cfg.HTTPClient
	client.Transport = scheduleTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/full" {
			return nil, errors.New("old package unavailable")
		}
		return baseTransport.RoundTrip(r)
	})
	m.cfg.HTTPClient = &client
	now := time.Date(2026, 10, 22, 3, 59, 0, 0, time.UTC)
	schedulerOnce(m, now)
	if m.checkRecord.Retry != "download" || !m.checkRecord.RetryAt.After(now.Add(time.Minute)) {
		t.Fatal("test did not arrange a later download retry")
	}
	f.manifest.Version = "1.2.0"
	f.sign()
	schedulerOnce(m, now.Add(time.Minute))
	if f.count("/feed") != 2 || m.Snapshot().Version != "1.2.0" {
		t.Fatalf("old download hid new period feed: %+v, feeds=%d", m.Snapshot(), f.count("/feed"))
	}
}

func TestScheduledOperationsRecheckManualSuccessAndAutoDownload(t *testing.T) {
	f := newFixture(t)
	auto := false
	m := f.manager(&auto)
	now := scheduleTestTime()
	if err := m.check(context.Background(), func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	// Model a stale scheduler decision made before a manual check completed.
	if err := m.checkMode(context.Background(), func() time.Time { return now }, true); err != nil {
		t.Fatal(err)
	}
	if f.count("/feed") != 1 {
		t.Fatal("automatic check did not re-read the completed manual result")
	}
	if err := m.downloadScheduled(context.Background(), func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	if f.count("/full") != 0 {
		t.Fatal("a stale automatic decision ignored disabled auto-download")
	}
	// Desktop applies saved settings before Start. Restoring an Available
	// candidate must not launch a detached context.Background download.
	requests := make(chan struct{}, 1)
	client := *m.cfg.HTTPClient
	client.Transport = scheduleTransport(func(*http.Request) (*http.Response, error) {
		select {
		case requests <- struct{}{}:
		default:
		}
		return nil, errors.New("unexpected detached request")
	})
	m.cfg.HTTPClient = &client
	m.SetAutoDownload(true)
	select {
	case <-requests:
		t.Fatal("pre-Start settings unexpectedly launched a download")
	case <-time.After(100 * time.Millisecond):
	}
	if m.Snapshot().State != StateAvailable {
		t.Fatal("pre-Start settings changed the candidate")
	}
}

func TestBusyAndPendingOperationsDoNotCreateCheckRecords(t *testing.T) {
	for _, state := range []State{StateDownloading, StateReady, StateInstalling} {
		t.Run(string(state), func(t *testing.T) {
			f := newFixture(t)
			m := f.manager(nil)
			m.publish(func(s *Snapshot) { s.State = state })
			if state == StateDownloading {
				if !m.begin() {
					t.Fatal("could not hold operation")
				}
				defer m.end()
			}
			schedulerOnce(m, time.Now())
			if f.count("/feed") != 0 || !m.checkRecord.CheckedAt.IsZero() || m.checkRecord.Retry != "" || m.Snapshot().State != state {
				t.Fatalf("skipped operation counted as check: %+v, %+v", m.checkRecord, m.Snapshot())
			}
		})
	}
}

func TestPeriodStartBeforeFourAcrossDST(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ now, want string }{
		{"2026-03-08 03:59:00", "2026-03-07 04:00:00"},
		{"2026-03-08 04:00:00", "2026-03-08 04:00:00"},
		{"2026-11-01 03:59:00", "2026-10-31 04:00:00"},
		{"2026-11-01 04:00:00", "2026-11-01 04:00:00"},
	} {
		now, _ := time.ParseInLocation(time.DateTime, tc.now, location)
		want, _ := time.ParseInLocation(time.DateTime, tc.want, location)
		if got := currentCheckPeriod(now); !got.Equal(want) {
			t.Fatalf("period for %s = %s, want %s", now, got, want)
		}
	}
}

func TestScheduledDownloadRechecksNewManualBackoff(t *testing.T) {
	f := newFixture(t)
	auto := false
	m := f.manager(&auto)
	now := scheduleTestTime()
	if err := m.check(context.Background(), func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	m.publish(func(s *Snapshot) { s.AutoDownload = true })
	m.recordFailure(context.Background(), "download", now)
	// The stale scheduler decision predates a manual failure recorded above.
	if err := m.downloadScheduled(context.Background(), func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	if f.count("/full") != 0 {
		t.Fatal("automatic download bypassed the new manual failure backoff")
	}
	if err := m.downloadScheduled(context.Background(), func() time.Time { return now.Add(5 * time.Minute) }); err != nil {
		t.Fatal(err)
	}
	if f.count("/full") != 1 || m.Snapshot().State != StateReady {
		t.Fatal("download did not recover at its retry deadline")
	}
}
