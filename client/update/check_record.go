package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"time"
)

const checkRecordName = "check-state.json"

var retryDelays = [...]time.Duration{5 * time.Minute, 15 * time.Minute, time.Hour, 3 * time.Hour, 6 * time.Hour}

// The envelope remains the authority for cached candidates. The timestamps only
// suppress redundant requests; changing the updater configuration invalidates it.
type checkRecord struct {
	Schema    int             `json:"schema"`
	Scope     string          `json:"scope"`
	CheckedAt time.Time       `json:"checkedAt,omitempty"`
	Envelope  json.RawMessage `json:"envelope,omitempty"`
	Retry     string          `json:"retry,omitempty"`
	Failures  int             `json:"failures,omitempty"`
	FailedAt  time.Time       `json:"failedAt,omitempty"`
	RetryAt   time.Time       `json:"retryAt,omitempty"`
}

func (m *Manager) checkScope() string {
	data, _ := json.Marshal([]string{m.cfg.CurrentVersion, m.cfg.FeedURL, m.cfg.PublicKey, m.cfg.OS, m.cfg.Arch})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (m *Manager) validCheckRecord(r checkRecord, now time.Time) bool {
	if r.Schema != 1 || r.Scope != m.checkScope() || r.CheckedAt.After(now) || r.FailedAt.After(now) {
		return false
	}
	if r.CheckedAt.IsZero() != (len(r.Envelope) == 0) {
		return false
	}
	if len(r.Envelope) != 0 {
		manifest, _, err := m.parseFeed(r.Envelope)
		if err != nil || r.Retry == "download" && compareVersions(manifest.Version, m.cfg.CurrentVersion) <= 0 {
			return false
		}
	}
	if r.Retry == "" {
		return r.Failures == 0 && r.FailedAt.IsZero() && r.RetryAt.IsZero() && !r.CheckedAt.IsZero()
	}
	return (r.Retry == "feed" || r.Retry == "download" && len(r.Envelope) != 0) &&
		r.Failures >= 1 && r.Failures <= len(retryDelays) && !r.FailedAt.IsZero() &&
		!r.FailedAt.Before(r.CheckedAt) && r.RetryAt.Equal(r.FailedAt.Add(retryDelays[r.Failures-1]))
}

func (m *Manager) restoreCheckRecord(now time.Time) {
	name := pathJoin(m.cacheDir(), checkRecordName)
	st, err := regularFile(name)
	if err != nil || st.Size() > maxManifestSize+4096 {
		return
	}
	data, err := os.ReadFile(name)
	var r checkRecord
	if err != nil || json.Unmarshal(data, &r) != nil || !m.validCheckRecord(r, now) {
		return
	}
	m.checkRecord = r
	if len(r.Envelope) != 0 {
		manifest, asset, _ := m.parseFeed(r.Envelope)
		m.applyFeed(manifest, asset, r.Envelope)
	}
}

// Callers hold scheduleMu. A storage failure leaves a dirty in-memory result;
// the scheduler retries the write without re-fetching or discarding verified UI.
func (m *Manager) storeCheckRecord() {
	m.recordDirty = true
	if err := secureDirectory(m.cacheDir()); err != nil {
		return
	}
	name := pathJoin(m.cacheDir(), checkRecordName)
	if _, err := regularFile(name); err != nil && !os.IsNotExist(err) {
		return
	}
	data, err := json.Marshal(m.checkRecord)
	if err == nil && writeAtomic(name, data, 0o600) == nil {
		m.recordDirty = false
	}
}

func (m *Manager) recordSuccess(envelope []byte, now time.Time) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	m.checkRecord = checkRecord{Schema: 1, Scope: m.checkScope(), CheckedAt: now, Envelope: append(json.RawMessage(nil), envelope...)}
	m.storeCheckRecord()
}

func (m *Manager) recordFailure(ctx context.Context, kind string, now time.Time) {
	if ctx.Err() != nil {
		return
	}
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	r := m.checkRecord
	if !m.validCheckRecord(r, now) {
		r = checkRecord{Schema: 1, Scope: m.checkScope()}
	}
	if r.Retry != kind {
		r.Failures = 0
	}
	r.Retry = kind
	r.Failures = min(r.Failures+1, len(retryDelays))
	r.FailedAt = now
	r.RetryAt = now.Add(retryDelays[r.Failures-1])
	m.checkRecord = r
	m.storeCheckRecord()
}

func (m *Manager) clearRetry() {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	if m.checkRecord.Retry == "" {
		return
	}
	m.checkRecord.Retry, m.checkRecord.Failures = "", 0
	m.checkRecord.FailedAt, m.checkRecord.RetryAt = time.Time{}, time.Time{}
	m.storeCheckRecord()
}

func (m *Manager) scheduledRecord(now time.Time) checkRecord {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	if !m.validCheckRecord(m.checkRecord, now) {
		m.checkRecord = checkRecord{}
		m.recordDirty = false
	}
	if m.recordDirty {
		m.storeCheckRecord()
	}
	return m.checkRecord
}
