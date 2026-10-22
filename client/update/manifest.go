package update

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxManifestSize = 4 << 20
const maxPackageSize int64 = 1 << 30
const maxFileSize int64 = 512 << 20
const maxExpandedSize int64 = 2 << 30

func validURL(raw string, allowHTTP bool) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Host != "" && u.User == nil && u.Fragment == "" && (u.Scheme == "https" || allowHTTP && u.Scheme == "http")
}
func validHash(h string) bool {
	b, e := hex.DecodeString(h)
	return e == nil && len(b) == 32 && strings.ToLower(h) == h
}
func validDownload(d Download, allowHTTP bool) bool {
	return d.Size > 0 && d.Size <= maxPackageSize && validHash(d.SHA256) && validURL(d.URL, allowHTTP)
}
func safePath(name string) bool {
	return name != "" && name == path.Clean(name) && name != "." && !strings.HasPrefix(name, "/") && !strings.Contains(name, "\\") && !strings.Contains(name, ":") && name != ".." && !strings.HasPrefix(name, "../") && !strings.ContainsAny(name, "\x00\r\n")
}
func validInstallPath(name, platform string) bool {
	if !safePath(name) {
		return false
	}
	switch platform {
	case "darwin":
		return strings.HasPrefix(name, "Contents/")
	case "windows":
		return name == "passpine.exe" || name == "passpine-host.exe"
	case "linux":
		return name == "passpine" || name == "passpine-host"
	}
	return false
}
func (m *Manager) parseFeed(data []byte) (*Manifest, *Asset, error) {
	var envelope Envelope
	if len(data) > maxManifestSize || json.Unmarshal(data, &envelope) != nil {
		return nil, nil, errors.New(ErrorManifest)
	}
	payload, e := base64.StdEncoding.DecodeString(envelope.Payload)
	if e != nil {
		return nil, nil, errors.New(ErrorSignature)
	}
	signature, e := base64.StdEncoding.DecodeString(envelope.Signature)
	if e != nil || !ed25519.Verify(m.key, payload, signature) {
		return nil, nil, errors.New(ErrorSignature)
	}
	var manifest Manifest
	if json.Unmarshal(payload, &manifest) != nil || manifest.Schema != 1 || !validVersion(manifest.Version) {
		return nil, nil, errors.New(ErrorManifest)
	}
	selected := &manifest
	if manifest.InstallerUpdate != nil {
		selected = manifest.InstallerUpdate
		if selected.Schema != 2 || !validVersion(selected.Version) || selected.InstallerUpdate != nil {
			return nil, nil, errors.New(ErrorManifest)
		}
	}
	// Select the installer protocol before comparing versions: the outer
	// compatibility version remains fixed at v0.1.5. Historical feeds need
	// not contain an asset for the current platform.
	if compareVersions(selected.Version, m.cfg.CurrentVersion) <= 0 {
		return selected, nil, nil
	}
	var asset *Asset
	for i := range selected.Assets {
		a := &selected.Assets[i]
		if a.OS == m.cfg.OS && a.Arch == m.cfg.Arch {
			if asset != nil {
				return nil, nil, errors.New(ErrorManifest)
			}
			asset = a
		}
	}
	if asset == nil || !validDownload(asset.Full, m.cfg.AllowHTTP) || len(asset.Files) == 0 || len(asset.Files) > 8192 {
		return nil, nil, errors.New(ErrorManifest)
	}
	if selected.Schema == 1 && asset.Format != "" || selected.Schema == 2 && !validInstallerFormat(asset.OS, asset.Format) {
		return nil, nil, errors.New(ErrorManifest)
	}
	var total int64
	seen := map[string]bool{}
	for _, f := range asset.Files {
		folded := strings.ToLower(f.Path)
		if !validInstallPath(f.Path, m.cfg.OS) || seen[folded] || !validHash(f.SHA256) || f.Size < 0 || f.Size > maxFileSize || f.Mode != 0o755 && f.Mode != 0o644 {
			return nil, nil, errors.New(ErrorManifest)
		}
		seen[folded] = true
		total += f.Size
	}
	if total > maxExpandedSize {
		return nil, nil, errors.New(ErrorManifest)
	}
	required := []string{"passpine", "passpine-host"}
	if m.cfg.OS == "darwin" {
		required = []string{"contents/macos/passpine", "contents/macos/passpine-host"}
	}
	if m.cfg.OS == "windows" {
		required = []string{"passpine.exe", "passpine-host.exe"}
	}
	for _, name := range required {
		if !seen[name] {
			return nil, nil, errors.New(ErrorManifest)
		}
	}
	return selected, asset, nil
}

func validInstallerFormat(platform, format string) bool {
	switch platform {
	case "darwin":
		return format == "app-zip"
	case "linux":
		return format == "tar-gzip"
	case "windows":
		return format == "nsis"
	}
	return false
}

// Pending installations must still be newer than the running application.
func (m *Manager) parseEnvelope(data []byte) (*Manifest, *Asset, error) {
	manifest, asset, err := m.parseFeed(data)
	if err == nil && compareVersions(manifest.Version, m.cfg.CurrentVersion) <= 0 {
		err = errors.New(ErrorManifest)
	}
	return manifest, asset, err
}
func (m *Manager) request(ctx context.Context, raw string) (*http.Response, error) {
	if !validURL(raw, m.cfg.AllowHTTP) {
		return nil, errors.New("invalid update URL")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Accept", "application/json, application/zip, application/gzip, application/octet-stream")
	client := *m.cfg.HTTPClient
	previous := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !validURL(req.URL.String(), m.cfg.AllowHTTP) || len(via) > 8 {
			return errors.New("unsafe update redirect")
		}
		if previous != nil {
			return previous(req, via)
		}
		return nil
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("update HTTP status %d", resp.StatusCode)
	}
	return resp, nil
}

// Check never discards an already verified, pending installation.
func (m *Manager) Check(ctx context.Context) error {
	return m.check(ctx, time.Now)
}

func (m *Manager) check(ctx context.Context, now func() time.Time) error {
	return m.checkMode(ctx, now, false)
}

func (m *Manager) checkMode(ctx context.Context, now func() time.Time, scheduled bool) error {
	if !m.Snapshot().Configured {
		return ErrUnavailable
	}
	if !m.begin() {
		return ErrBusy
	}
	defer m.end()
	if m.pendingInstallation() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if scheduled && m.cfg.CheckInterval <= 0 {
		// A manual operation may have completed after the scheduler decided
		// to check. Re-read the record only after acquiring the operation.
		current := now()
		r := m.scheduledRecord(current)
		satisfied := !r.CheckedAt.IsZero() && !r.CheckedAt.Before(currentCheckPeriod(current))
		if r.Retry == "feed" && current.Before(r.RetryAt) || satisfied && r.Retry != "feed" {
			return nil
		}
	}
	m.publish(func(s *Snapshot) { s.State = StateChecking; s.ErrorCode = "" })
	resp, err := m.request(ctx, m.cfg.FeedURL)
	if err != nil {
		m.recordFailure(ctx, "feed", now())
		return m.fail(ErrorNetwork, err)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestSize+1))
	resp.Body.Close()
	if err != nil {
		m.recordFailure(ctx, "feed", now())
		return m.fail(ErrorNetwork, err)
	}
	manifest, asset, err := m.parseFeed(body)
	if err != nil {
		m.recordFailure(ctx, "feed", now())
		code := ErrorManifest
		if err.Error() == ErrorSignature {
			code = ErrorSignature
		}
		return m.fail(code, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.applyFeed(manifest, asset, body)
	// Feed success is independent of the subsequent package download. Record
	// completion time so a slow request crossing 04:00 satisfies the new period.
	m.recordSuccess(body, now())
	if m.Snapshot().State == StateAvailable && m.Snapshot().AutoDownload {
		return m.downloadWithRetry(ctx, now)
	}
	return nil
}

func (m *Manager) applyFeed(manifest *Manifest, asset *Asset, body []byte) {
	m.manifest, m.asset, m.envelope = manifest, asset, bytes.Clone(body)
	m.publish(func(s *Snapshot) {
		s.ErrorCode, s.Progress, s.Downloaded, s.Total = "", 0, 0, 0
		if compareVersions(manifest.Version, m.cfg.CurrentVersion) <= 0 {
			s.State, s.Version = StateLatest, ""
			if manifest.Version == m.cfg.CurrentVersion {
				s.Version = manifest.Version
			}
		} else {
			s.State, s.Version = StateAvailable, manifest.Version
		}
	})
}

// pendingInstallation is called only while holding the operation token.
func (m *Manager) pendingInstallation() bool {
	s := m.Snapshot()
	return s.State == StateReady || s.State == StateInstalling || m.readyDir != "" && s.State == StateFailed && s.ErrorCode == ErrorInstall
}

func (m *Manager) Download(ctx context.Context) error {
	if !m.Snapshot().Configured {
		return ErrUnavailable
	}
	if !m.begin() {
		return ErrBusy
	}
	defer m.end()
	if m.Snapshot().State == StateReady {
		return nil
	}
	return m.downloadWithRetry(ctx, time.Now)
}
func (m *Manager) downloadWithRetry(ctx context.Context, now func() time.Time) error {
	err := m.download(ctx)
	if err == nil {
		m.clearRetry()
	} else if err != ErrUnavailable {
		m.recordFailure(ctx, "download", now())
	}
	return err
}
func (m *Manager) download(ctx context.Context) error {
	if m.manifest == nil || m.asset == nil || compareVersions(m.manifest.Version, m.cfg.CurrentVersion) <= 0 {
		return ErrUnavailable
	}
	m.publish(func(s *Snapshot) { s.ErrorCode = "" })
	e := m.downloadAndStage(ctx)
	if e != nil {
		if ce, ok := e.(*codedError); ok {
			return m.fail(ce.code, e)
		}
		return m.fail(ErrorDownload, e)
	}
	m.readyArchive = pathJoin(m.cacheDir(), packageName(m.asset))
	pending := pendingState{Envelope: m.envelope}
	data, _ := json.Marshal(pending)
	if e = writeAtomic(pathJoin(m.cacheDir(), "pending.json"), data, 0o600); e != nil {
		return m.fail(ErrorStorage, e)
	}
	_ = os.Remove(pathJoin(m.cacheDir(), "install-result.json"))
	m.publish(func(s *Snapshot) { s.State = StateReady; s.Progress = 1; s.ErrorCode = "" })
	return nil
}

type pendingState struct {
	Envelope json.RawMessage `json:"envelope"`
}

func (m *Manager) restore() {
	data, e := os.ReadFile(pathJoin(m.cacheDir(), "pending.json"))
	if e != nil || len(data) > maxManifestSize+1024 {
		return
	}
	var pending pendingState
	if json.Unmarshal(data, &pending) != nil {
		return
	}
	manifest, asset, e := m.parseEnvelope(pending.Envelope)
	if e != nil {
		return
	}
	archive := pathJoin(m.cacheDir(), packageName(asset))
	stage := pathJoin(m.cacheDir(), "ready")
	if verifyFile(archive, asset.Full.SHA256, asset.Full.Size) != nil || verifyTree(stage, asset.Files) != nil {
		return
	}
	m.manifest, m.asset, m.envelope = manifest, asset, bytes.Clone(pending.Envelope)
	m.readyArchive, m.readyDir = archive, stage
	m.snapshot.State = StateReady
	m.snapshot.Version = manifest.Version
	m.snapshot.Progress = 1
	if result, e := os.ReadFile(pathJoin(m.cacheDir(), "install-result.json")); e == nil && len(result) < 1024 {
		var failure struct {
			ErrorCode string `json:"errorCode"`
		}
		if json.Unmarshal(result, &failure) == nil && (failure.ErrorCode == ErrorInstall || failure.ErrorCode == ErrorVerification) {
			m.snapshot.State = StateFailed
			m.snapshot.ErrorCode = failure.ErrorCode
		}
	}
}

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func validVersion(v string) bool {
	matches := versionPattern.FindStringSubmatch(v)
	if matches == nil {
		return false
	}
	for _, n := range matches[1:4] {
		if _, e := strconv.ParseUint(n, 10, 64); e != nil {
			return false
		}
	}
	for _, id := range strings.Split(matches[4], ".") {
		if numeric(id) && len(id) > 1 && id[0] == '0' {
			return false
		}
	}
	return true
}
func numeric(v string) bool {
	if v == "" {
		return false
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func compareVersions(a, b string) int {
	aa := versionPattern.FindStringSubmatch(a)
	bb := versionPattern.FindStringSubmatch(b)
	if aa == nil || bb == nil {
		return 0
	}
	for i := 1; i <= 3; i++ {
		an, _ := strconv.ParseUint(aa[i], 10, 64)
		bn, _ := strconv.ParseUint(bb[i], 10, 64)
		if an < bn {
			return -1
		}
		if an > bn {
			return 1
		}
	}
	if aa[4] == bb[4] {
		return 0
	}
	if aa[4] == "" {
		return 1
	}
	if bb[4] == "" {
		return -1
	}
	ap, bp := strings.Split(aa[4], "."), strings.Split(bb[4], ".")
	for i := 0; i < len(ap) && i < len(bp); i++ {
		if ap[i] == bp[i] {
			continue
		}
		an, bn := numeric(ap[i]), numeric(bp[i])
		if an && bn {
			if len(ap[i]) < len(bp[i]) {
				return -1
			}
			if len(ap[i]) > len(bp[i]) {
				return 1
			}
		} else if an != bn {
			if an {
				return -1
			}
			return 1
		}
		if ap[i] < bp[i] {
			return -1
		}
		return 1
	}
	if len(ap) < len(bp) {
		return -1
	}
	return 1
}
