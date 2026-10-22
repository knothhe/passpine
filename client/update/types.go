// Package update downloads signed releases without interrupting the running
// application. Installation is a separate, explicit operation.
package update

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type State string

const (
	StateIdle        State = "idle"
	StateChecking    State = "checking"
	StateLatest      State = "latest"
	StateAvailable   State = "available"
	StateDownloading State = "downloading"
	StateVerifying   State = "verifying"
	StateReady       State = "ready"
	StateInstalling  State = "installing"
	StateFailed      State = "failed"
	StateDisabled    State = "disabled"
)
const (
	ErrorConfiguration = "configuration"
	ErrorNetwork       = "network"
	ErrorSignature     = "signature"
	ErrorManifest      = "manifest"
	ErrorDownload      = "download"
	ErrorVerification  = "verification"
	ErrorStorage       = "storage"
	ErrorInstall       = "install"
)

var ErrBusy = errors.New("update operation already in progress")
var ErrUnavailable = errors.New("update is not available")

type Snapshot struct {
	// Revision increases for every notification so an asynchronous UI can ignore stale callbacks.
	Revision                 uint64
	State                    State
	CurrentVersion           string
	Version                  string
	ErrorCode                string
	Progress                 float64
	Downloaded, Total        int64
	AutoDownload, Configured bool
}

type Config struct {
	CurrentVersion, FeedURL, PublicKey, DataDir string
	// AutoDownload defaults to true. A pointer distinguishes the default from false.
	AutoDownload *bool
	OnChange     func(Snapshot)
	// CheckInterval overrides the daily local 04:00 schedule when positive.
	// It is intended for isolated package tests; zero uses the daily schedule.
	CheckInterval time.Duration
	// InstallRoot and Executable normally derive from the running executable.
	InstallRoot, Executable string
	ProtectedPaths          []string
	HTTPClient              *http.Client
	// Platform overrides and HTTP are intended for isolated package tests only.
	OS, Arch  string
	AllowHTTP bool
}

type Envelope struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}
type Manifest struct {
	Schema  int     `json:"schema"`
	Version string  `json:"version"`
	Assets  []Asset `json:"assets"`
}
type Download struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
}
type Asset struct {
	OS    string   `json:"os"`
	Arch  string   `json:"arch"`
	Full  Download `json:"full"`
	Files []File   `json:"files"`
}

type Manager struct {
	lifecycle              context.Context
	mu                     sync.Mutex
	operation              chan struct{}
	cfg                    Config
	key                    ed25519.PublicKey
	snapshot               Snapshot
	manifest               *Manifest
	asset                  *Asset
	envelope               []byte
	readyDir, readyArchive string
}

func New(cfg Config) (*Manager, error) {
	if cfg.OS == "" {
		cfg.OS = runtime.GOOS
	}
	if cfg.Arch == "" {
		cfg.Arch = runtime.GOARCH
	}
	if cfg.Executable == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		cfg.Executable = exe
	}
	var err error
	cfg.Executable, err = filepath.Abs(cfg.Executable)
	if err != nil {
		return nil, err
	}
	if resolved, e := filepath.EvalSymlinks(cfg.Executable); e == nil {
		cfg.Executable = resolved
	}
	if cfg.InstallRoot == "" {
		cfg.InstallRoot = InferInstallRoot(cfg.Executable, cfg.OS)
	}
	cfg.InstallRoot, err = filepath.Abs(cfg.InstallRoot)
	if err != nil {
		return nil, err
	}
	cfg.DataDir, err = filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	cfg.DataDir = canonicalPath(cfg.DataDir)
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Minute}
	}
	m := &Manager{cfg: cfg, operation: make(chan struct{}, 1)}
	m.snapshot = Snapshot{State: StateIdle, CurrentVersion: cfg.CurrentVersion, AutoDownload: cfg.AutoDownload == nil || *cfg.AutoDownload}
	decoded, e := base64.StdEncoding.DecodeString(cfg.PublicKey)
	m.snapshot.Configured = e == nil && len(decoded) == ed25519.PublicKeySize && validVersion(cfg.CurrentVersion) && validURL(cfg.FeedURL, cfg.AllowHTTP)
	if !m.snapshot.Configured {
		m.snapshot.State = StateDisabled
		m.snapshot.ErrorCode = ErrorConfiguration
		return m, nil
	}
	if protects(cfg.InstallRoot, cfg.DataDir) || cfg.InstallRoot == string(filepath.Separator) {
		return nil, errors.New("application data cannot be inside the installation root")
	}
	for _, protected := range cfg.ProtectedPaths {
		absolute, e := filepath.Abs(protected)
		if e != nil {
			return nil, e
		}
		if protects(cfg.InstallRoot, absolute) {
			return nil, errors.New("protected data is inside the installation root")
		}
	}
	m.key = append(ed25519.PublicKey(nil), decoded...)
	if err := secureDirectory(m.cacheDir()); err != nil {
		return nil, err
	}
	m.restore()
	return m, nil
}

// InferInstallRoot identifies .app bundles; other platforms update only the two
// named application binaries, preserving neighboring files and uninstallers.
func InferInstallRoot(executable, platform string) string {
	if platform == "darwin" {
		for p := filepath.Dir(executable); p != filepath.Dir(p); p = filepath.Dir(p) {
			if filepath.Ext(p) == ".app" {
				return p
			}
		}
	}
	return filepath.Dir(executable)
}
func (m *Manager) cacheDir() string { return filepath.Join(m.cfg.DataDir, "updates") }
func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshot
}
func (m *Manager) publish(change func(*Snapshot)) {
	m.mu.Lock()
	change(&m.snapshot)
	m.snapshot.Revision++
	s := m.snapshot
	callback := m.cfg.OnChange
	m.mu.Unlock()
	if callback != nil {
		callback(s)
	}
}

// SetProtectedPaths refreshes vault locations before an explicit installation.
func (m *Manager) SetProtectedPaths(paths []string) {
	m.mu.Lock()
	m.cfg.ProtectedPaths = append([]string(nil), paths...)
	m.mu.Unlock()
}
func (m *Manager) protectedPaths() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.cfg.ProtectedPaths...)
}
func (m *Manager) SetAutoDownload(enabled bool) {
	m.publish(func(s *Snapshot) { s.AutoDownload = enabled })
	if enabled && m.Snapshot().State == StateAvailable {
		m.mu.Lock()
		ctx := m.lifecycle
		m.mu.Unlock()
		if ctx == nil {
			ctx = context.Background()
		}
		go m.Download(ctx)
	}
}

// Start returns immediately. Callers own cancellation for application shutdown.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	m.lifecycle = ctx
	m.mu.Unlock()
	if !m.Snapshot().Configured {
		return
	}
	go m.runChecks(ctx, time.Now, waitForCheck)
}
func (m *Manager) begin() bool {
	select {
	case m.operation <- struct{}{}:
		return true
	default:
		return false
	}
}
func (m *Manager) end() { <-m.operation }
func (m *Manager) fail(code string, err error) error {
	m.publish(func(s *Snapshot) { s.State = StateFailed; s.ErrorCode = code })
	return err
}
