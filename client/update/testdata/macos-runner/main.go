//go:build darwin

// This executable belongs to the isolated macOS updater verification fixture.
// It is outside production packages and never selects a default user directory.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/update"
)

type fixture struct {
	root, dataDir string
	manager       *update.Manager
}

func main() {
	f, err := validateFixture()
	if err != nil {
		fmt.Fprintln(os.Stderr, "verification runner: invalid isolated fixture:", err)
		os.Exit(1)
	}
	// Preview services inspect only their fictional vault's Git metadata. Do
	// not let Git consult personal configuration or modify optional lock files.
	for key, value := range map[string]string{
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_OPTIONAL_LOCKS": "0",
	} {
		if err = os.Setenv(key, value); err != nil {
			f.fail("environment", err)
		}
	}
	switch os.Getenv("PASSPINE_VERIFY_MODE") {
	case "seed":
		err = f.seed()
	case "", "install":
		err = f.install()
	default:
		err = errors.New("unsupported verification mode")
	}
	if err != nil {
		f.fail("verification", err)
	}
}

func validateFixture() (*fixture, error) {
	root := os.Getenv("PASSPINE_VERIFY_ROOT")
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || filepath.Dir(root) != "/private/tmp" || !strings.HasPrefix(filepath.Base(root), "passpine-update-verification-") || filepath.Base(root) == "passpine-update-verification-" {
		return nil, errors.New("PASSPINE_VERIFY_ROOT must name a dedicated /private/tmp/passpine-update-verification-* directory")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("verification root must be an existing real directory")
	}
	if len(os.Args) != 4 || os.Args[1] != "--preview" || os.Args[2] != "--data-dir" {
		return nil, errors.New("expected only --preview --data-dir PATH")
	}
	f := &fixture{root: root, dataDir: os.Args[3]}
	if err = f.path(f.dataDir, false); err != nil {
		return nil, fmt.Errorf("data directory: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, errors.New("cannot locate verification runner")
	}
	if err = f.path(executable, true); err != nil {
		return nil, fmt.Errorf("verification runner executable: %w", err)
	}
	if err = os.MkdirAll(f.dataDir, 0o700); err != nil {
		return nil, err
	}
	return f, nil
}

// path rejects aliases and symlinks, including existing ancestors of paths that
// the fixture is about to create. Every application and data path is beneath
// the explicitly supplied temporary root.
func (f *fixture) path(name string, mustExist bool) error {
	if !filepath.IsAbs(name) || filepath.Clean(name) != name {
		return errors.New("path must be absolute and clean")
	}
	relative, err := filepath.Rel(f.root, name)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("path must be strictly inside the verification root")
	}
	current := f.root
	parts := strings.Split(relative, string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) && !mustExist {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || index < len(parts)-1 && !info.IsDir() {
			return errors.New("fixture path must not traverse symlinks or non-directories")
		}
	}
	return nil
}

func (f *fixture) seed() error {
	items, err := os.ReadDir(f.dataDir)
	if err != nil {
		return err
	}
	if len(items) != 0 {
		return errors.New("seed data directory must be empty")
	}
	service, err := app.New(app.Options{DataDir: f.dataDir, Preview: true})
	if err != nil {
		return err
	}
	defer service.Close()
	response := service.Execute(app.Request{Action: "configure"})
	if response.Status != "ok" {
		return errors.New("cannot inspect fictional preview fixture")
	}
	settings := response.Data.Settings
	settings.AutoDownloadUpdates = false
	settings.DeviceUnlock = false
	settings.RecordActivity = false
	settings.LaunchAtLogin = false
	settings.AutoSync = false
	if result := service.Execute(app.Request{Action: "settings", Settings: &settings}); result.Status != "ok" {
		return errors.New("cannot save isolated fixture settings")
	}
	var vaults []map[string]string
	for _, vault := range response.Data.Vaults {
		if err = f.path(vault.Directory, true); err != nil {
			return fmt.Errorf("fictional vault directory: %w", err)
		}
		vaults = append(vaults, map[string]string{"directory": vault.Directory})
	}
	var entries []map[string]string
	for _, entry := range response.Data.Entries {
		entries = append(entries, map[string]string{"path": entry.Path})
	}
	if err = service.Close(); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"dataDir": f.dataDir, "vaults": vaults, "entries": entries,
		"configured": response.Data.Configured, "preview": response.Data.Preview,
	})
}

func (f *fixture) install() error {
	installRoot := os.Getenv("PASSPINE_VERIFY_INSTALL_ROOT")
	if err := f.path(installRoot, true); err != nil {
		return fmt.Errorf("installation root: %w", err)
	}
	if filepath.Ext(installRoot) != ".app" {
		return errors.New("installation root must be a fixture .app bundle")
	}
	executable := filepath.Join(installRoot, "Contents", "MacOS", "passpine")
	if err := f.path(executable, true); err != nil {
		return fmt.Errorf("application executable: %w", err)
	}
	info, err := os.Lstat(executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return errors.New("fixture application executable is not executable")
	}
	client, err := f.httpClient()
	if err != nil {
		return err
	}
	protected, err := f.vaultDirectories()
	if err != nil {
		return err
	}
	automatic := false
	f.manager, err = update.New(update.Config{
		CurrentVersion: os.Getenv("PASSPINE_VERIFY_CURRENT_VERSION"),
		PublicKey:      os.Getenv("PASSPINE_VERIFY_PUBLIC_KEY"), FeedURL: os.Getenv("PASSPINE_VERIFY_FEED_URL"),
		DataDir: f.dataDir, InstallRoot: installRoot, Executable: executable,
		ProtectedPaths: protected, AutoDownload: &automatic, HTTPClient: client,
	})
	if err != nil {
		return err
	}
	if !f.manager.Snapshot().Configured {
		return errors.New("fixture update manager is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err = f.manager.Check(ctx); err != nil {
		return fmt.Errorf("check: %w", err)
	}
	if err = f.manager.Download(ctx); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if err = f.writeJSON("probe-ready.json", f.manager.Snapshot()); err != nil {
		return err
	}
	if err = f.waitForInstallSignal(ctx); err != nil {
		return err
	}
	if err = f.manager.Install(ctx); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	if err = f.writeJSON("probe-helper-started.json", map[string]any{
		"pid": os.Getpid(), "executable": executable, "snapshot": f.manager.Snapshot(),
	}); err != nil {
		return err
	}
	// The genuine application replaces this runner in-place, keeping the PID
	// recorded by Manager.Install. The real helper must wait until the script
	// explicitly ends that application's lifetime before replacing its bundle.
	return syscall.Exec(executable, append([]string{executable}, os.Args[1:]...), os.Environ())
}

func (f *fixture) vaultDirectories() ([]string, error) {
	name := filepath.Join(f.dataDir, "preferences.json")
	if err := f.path(name, true); err != nil {
		return nil, err
	}
	info, err := os.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("invalid isolated preferences file")
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var preferences struct {
		Vaults []struct {
			Directory string `json:"directory"`
		} `json:"vaults"`
	}
	if err = json.Unmarshal(data, &preferences); err != nil {
		return nil, errors.New("invalid isolated preferences JSON")
	}
	if len(preferences.Vaults) == 0 {
		return nil, errors.New("fixture must contain a fictional vault")
	}
	var paths []string
	for _, vault := range preferences.Vaults {
		if err = f.path(vault.Directory, true); err != nil {
			return nil, fmt.Errorf("protected fictional vault: %w", err)
		}
		paths = append(paths, vault.Directory)
	}
	return paths, nil
}

func loopbackURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" || parsed.Port() == "" {
		return nil, errors.New("fixture URL must use HTTPS on an explicit loopback port")
	}
	host := strings.ToLower(parsed.Hostname())
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("fixture URL must target loopback")
	}
	return parsed, nil
}

func (f *fixture) httpClient() (*http.Client, error) {
	feed, err := loopbackURL(os.Getenv("PASSPINE_VERIFY_FEED_URL"))
	if err != nil {
		return nil, err
	}
	caFile := os.Getenv("PASSPINE_VERIFY_CA_FILE")
	if err = f.path(caFile, true); err != nil {
		return nil, fmt.Errorf("fixture CA file: %w", err)
	}
	info, err := os.Lstat(caFile)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("fixture CA must be a small regular file")
	}
	certificate, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certificate) {
		return nil, errors.New("fixture CA certificate is invalid")
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil || port != feed.Port() {
				return nil, errors.New("connection outside fixture loopback service")
			}
			if strings.EqualFold(host, "localhost") {
				host = "127.0.0.1"
			}
			if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
				return nil, errors.New("connection outside loopback")
			}
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(host, port))
		},
	}
	return &http.Client{
		Timeout:   2 * time.Minute,
		Transport: fixtureTransport{transport: transport, origin: feed.Scheme + "://" + feed.Host},
	}, nil
}

type fixtureTransport struct {
	transport *http.Transport
	origin    string
}

func (t fixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	parsed, err := loopbackURL(request.URL.String())
	if err != nil || parsed.Scheme+"://"+parsed.Host != t.origin {
		return nil, errors.New("request outside fixture HTTPS origin")
	}
	return t.transport.RoundTrip(request)
}

func (f *fixture) waitForInstallSignal(ctx context.Context) error {
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	name := filepath.Join(f.dataDir, "install.signal")
	for {
		if info, err := os.Lstat(name); err == nil {
			if !info.Mode().IsRegular() {
				return errors.New("install signal must be a regular file")
			}
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("timed out waiting for explicit fixture install signal")
		case <-ticker.C:
		}
	}
}

func (f *fixture) writeJSON(base string, value any) error {
	name := filepath.Join(f.dataDir, base)
	if err := f.path(name, false); err != nil {
		return err
	}
	file, err := os.CreateTemp(f.dataDir, ".probe-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = json.NewEncoder(file).Encode(value); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), name)
}

func (f *fixture) fail(phase string, err error) {
	report := map[string]any{"phase": phase, "error": err.Error()}
	if f.manager != nil {
		report["snapshot"] = f.manager.Snapshot()
	}
	_ = f.writeJSON("probe-error.json", report)
	fmt.Fprintln(os.Stderr, "verification runner:", phase, err)
	os.Exit(1)
}
