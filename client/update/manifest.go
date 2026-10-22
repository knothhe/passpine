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
func (m *Manager) parseEnvelope(data []byte) (*Manifest, *Asset, error) {
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
	if json.Unmarshal(payload, &manifest) != nil || manifest.Schema != 1 || !validVersion(manifest.Version) || compareVersions(manifest.Version, m.cfg.CurrentVersion) <= 0 {
		return nil, nil, errors.New(ErrorManifest)
	}
	var asset *Asset
	for i := range manifest.Assets {
		a := &manifest.Assets[i]
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
	return &manifest, asset, nil
}
func (m *Manager) request(ctx context.Context, raw string) (*http.Response, error) {
	if !validURL(raw, m.cfg.AllowHTTP) {
		return nil, errors.New("invalid update URL")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Accept", "application/json, application/zip")
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
	if !m.Snapshot().Configured {
		return ErrUnavailable
	}
	if !m.begin() {
		return ErrBusy
	}
	defer m.end()
	if s := m.Snapshot(); s.State == StateReady || s.State == StateInstalling || m.readyDir != "" && s.State == StateFailed && s.ErrorCode == ErrorInstall {
		return nil
	}
	m.publish(func(s *Snapshot) { s.State = StateChecking; s.ErrorCode = "" })
	resp, e := m.request(ctx, m.cfg.FeedURL)
	if e != nil {
		return m.fail(ErrorNetwork, e)
	}
	body, e := io.ReadAll(io.LimitReader(resp.Body, maxManifestSize+1))
	resp.Body.Close()
	if e != nil {
		return m.fail(ErrorNetwork, e)
	}
	// A signed feed for the current or an older version is a normal latest state.
	var envelope Envelope
	if len(body) <= maxManifestSize && json.Unmarshal(body, &envelope) == nil {
		payload, pe := base64.StdEncoding.DecodeString(envelope.Payload)
		sig, se := base64.StdEncoding.DecodeString(envelope.Signature)
		if pe == nil && se == nil && ed25519.Verify(m.key, payload, sig) {
			var candidate Manifest
			if json.Unmarshal(payload, &candidate) == nil && candidate.Schema == 1 && validVersion(candidate.Version) && compareVersions(candidate.Version, m.cfg.CurrentVersion) <= 0 {
				m.publish(func(s *Snapshot) {
					s.State = StateLatest
					s.ErrorCode = ""
					s.Version = ""
					if candidate.Version == m.cfg.CurrentVersion {
						s.Version = candidate.Version
					}
				})
				return nil
			}
		}
	}
	manifest, asset, e := m.parseEnvelope(body)
	if e != nil {
		code := ErrorManifest
		if e.Error() == ErrorSignature {
			code = ErrorSignature
		}
		return m.fail(code, e)
	}
	m.manifest, m.asset, m.envelope = manifest, asset, bytes.Clone(body)
	m.publish(func(s *Snapshot) {
		s.State = StateAvailable
		s.Version = manifest.Version
		s.ErrorCode = ""
		s.Progress = 0
		s.Downloaded = 0
		s.Total = 0
	})
	if m.Snapshot().AutoDownload {
		return m.download(ctx)
	}
	return nil
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
	return m.download(ctx)
}
func (m *Manager) download(ctx context.Context) error {
	if m.manifest == nil || m.asset == nil {
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
	m.readyArchive = pathJoin(m.cacheDir(), "package.zip")
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
	archive := pathJoin(m.cacheDir(), "package.zip")
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
