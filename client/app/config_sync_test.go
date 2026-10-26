package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/knothhe/passpine/client/cloudsync"
	"github.com/knothhe/passpine/client/secretstore"
	"github.com/knothhe/r2connect"
)

type syncMemorySecrets struct {
	mu        sync.Mutex
	values    map[string][]byte
	loadErr   error
	saveErr   error
	deleteErr error
	saveHook  func()
	loads     int
	retries   int
}

func (m *syncMemorySecrets) Load(key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loads++
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	b, ok := m.values[key]
	if !ok {
		return nil, secretstore.ErrNotFound
	}
	return append([]byte(nil), b...), nil
}
func (m *syncMemorySecrets) Save(key string, b []byte) error {
	m.mu.Lock()
	hook := m.saveHook
	m.saveHook = nil
	m.mu.Unlock()
	if hook != nil {
		hook()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	if m.values == nil {
		m.values = map[string][]byte{}
	}
	m.values[key] = append([]byte(nil), b...)
	return nil
}
func (m *syncMemorySecrets) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.values, key)
	return nil
}
func (m *syncMemorySecrets) RetryAccess() { m.mu.Lock(); defer m.mu.Unlock(); m.retries++ }

type syncTestBucket struct {
	mu       sync.Mutex
	body     []byte
	version  int
	puts     int
	requests int
	getHook  func()
}

func (b *syncTestBucket) serve(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	b.requests++
	hook := b.getHook
	if r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "" {
		b.getHook = nil
	} else {
		hook = nil
	}
	b.mu.Unlock()
	if hook != nil {
		hook()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.URL.Query().Get("list-type") == "2" {
		_, _ = io.WriteString(w, "<ListBucketResult/>")
		return
	}
	if r.Header.Get("Authorization") == "" {
		w.WriteHeader(403)
		return
	}
	etag := fmt.Sprintf(`"%d"`, b.version)
	switch r.Method {
	case http.MethodGet:
		if b.body == nil {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("ETag", etag)
		_, _ = w.Write(b.body)
	case http.MethodPut:
		if b.body == nil && r.Header.Get("If-None-Match") != "*" || b.body != nil && r.Header.Get("If-Match") != etag {
			w.WriteHeader(412)
			return
		}
		data, e := io.ReadAll(io.LimitReader(r.Body, 2*cloudsync.MaxDocumentBytes+1))
		if e != nil {
			w.WriteHeader(400)
			return
		}
		b.body = data
		b.version++
		b.puts++
		w.Header().Set("ETag", fmt.Sprintf(`"%d"`, b.version))
	default:
		w.WriteHeader(405)
	}
}
func (b *syncTestBucket) snapshot() ([]byte, int, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.body...), b.puts, b.requests
}

func syncTestValues(pairs ...string) cloudsync.Values {
	out := cloudsync.Values{}
	for i := 0; i < len(pairs); i += 2 {
		out[pairs[i]] = json.RawMessage(pairs[i+1])
	}
	return out
}

func newSyncServiceFixture(t *testing.T, dir string, memory *syncMemorySecrets, engine *cloudsync.Engine) *Service {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	s := &Service{directory: dir, prefs: preferences{Vaults: []VaultInfo{}, Settings: DefaultSettings()}, done: make(chan struct{})}
	if b, e := os.ReadFile(filepath.Join(dir, "preferences.json")); e == nil {
		if e = json.Unmarshal(b, &s.prefs); e != nil {
			t.Fatal(e)
		}
	}
	s.initConfigSync()
	s.configSync.store = memory
	// SDK initialization reads public state only. Recreate managers after
	// injecting the fake so a later test action can never access native secrets.
	s.configSync.manager, s.configSync.client = nil, nil
	s.configSync.engine = engine
	// This service fixture does not run the background watcher or register a
	// Native Host. Its only external surface is the isolated httptest bucket.
	s.browser.checkedAt = time.Now().Add(24 * time.Hour)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newSyncAppSpace(t *testing.T) (*Service, *syncMemorySecrets, *cloudsync.Engine, cloudsync.Config, cloudsync.Secrets, *syncTestBucket) {
	t.Helper()
	bucket := &syncTestBucket{}
	server := httptest.NewServer(http.HandlerFunc(bucket.serve))
	t.Cleanup(server.Close)
	engine := cloudsync.New(server.Client())
	config := cloudsync.Config{Endpoint: server.URL, Region: "test-region", Bucket: "app-settings", Prefix: "isolated-passpine", PathStyle: true}
	secrets := cloudsync.Secrets{AccessKeyID: "integration-access-marker", SecretAccessKey: "integration-secret-marker", SessionToken: "integration-session-marker", Passphrase: "integration encryption phrase marker"}
	memory := &syncMemorySecrets{values: map[string][]byte{}}
	return newSyncServiceFixture(t, "", memory, engine), memory, engine, config, secrets, bucket
}
func saveSyncFixture(t *testing.T, s *Service, c cloudsync.Config, v cloudsync.Secrets) Data {
	t.Helper()
	return must(t, s, Request{Action: "config-sync-save", SyncConnection: &SyncConnectionRequest{Provider: "s3", Config: c, Secrets: v}})
}

func TestConfigSyncCredentialsPrivateAndDeviceSettingsLocal(t *testing.T) {
	s, _, engine, c, secrets, _ := newSyncAppSpace(t)
	_, e := engine.Sync(context.Background(), c, secrets, syncTestValues("desktop.theme", `"dark"`, "desktop.language", `"zh-CN"`, "desktop.idleMinutes", `9999`, "desktop.deviceUnlock", `false`, "future.feature", `{"version":4}`), nil)
	if e != nil {
		t.Fatal(e)
	}
	s.prefs.Settings.IdleMinutes = 5
	s.prefs.Settings.ClipboardSeconds = 15
	s.prefs.Settings.DeviceUnlock = true
	s.prefs.Settings.LaunchAtLogin = false
	s.prefs.Current = "local-vault-id"
	s.prefs.Vaults = []VaultInfo{{ID: "local-vault-id", Name: "Local", Directory: filepath.Join(s.directory, "private-vault")}}
	d := saveSyncFixture(t, s, c, secrets)
	if d.Settings.Theme != "dark" || d.Settings.Language != "zh-CN" || d.Settings.IdleMinutes != 5 || d.Settings.ClipboardSeconds != 15 || !d.Settings.DeviceUnlock || d.Settings.LaunchAtLogin {
		t.Fatalf("sync changed local safety settings: %+v", d.Settings)
	}
	if s.prefs.Current != "local-vault-id" || len(s.prefs.Vaults) != 1 || s.prefs.Vaults[0].Directory != filepath.Join(s.directory, "private-vault") {
		t.Fatal("sync changed local vault paths")
	}
	if d.ConfigSync == nil || !d.ConfigSync.Enabled || !d.ConfigSync.Ready || d.ConfigSync.LastSyncedAt == "" {
		t.Fatalf("missing successful state: %+v", d.ConfigSync)
	}
	snapshot, _ := json.Marshal(d)
	if e = filepath.WalkDir(s.directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		for _, secret := range []string{secrets.AccessKeyID, secrets.SecretAccessKey, secrets.SessionToken, secrets.Passphrase} {
			if bytes.Contains(b, []byte(secret)) {
				t.Errorf("secret in local file %s", filepath.Base(path))
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{secrets.AccessKeyID, secrets.SecretAccessKey, secrets.SessionToken, secrets.Passphrase} {
		if bytes.Contains(snapshot, []byte(secret)) {
			t.Fatal("secret in service snapshot")
		}
	}
}

func TestConfigSyncRestartMergeAndConflictResolution(t *testing.T) {
	s, memory, engine, c, secrets, _ := newSyncAppSpace(t)
	d := saveSyncFixture(t, s, c, secrets)
	remoteBase, e := engine.Sync(context.Background(), c, secrets, cloudsync.Values{}, nil)
	if e != nil {
		t.Fatal(e)
	}
	settings := d.Settings
	settings.Theme = "dark"
	must(t, s, Request{Action: "settings", Settings: &settings})
	must(t, s, Request{Action: "config-sync-now"})
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	reopened := newSyncServiceFixture(t, s.directory, memory, engine)
	remote := copySyncValues(remoteBase.Values)
	remote["desktop.language"] = json.RawMessage(`"zh-CN"`)
	// Another device retained the initial baseline while this device changed
	// theme. The transport merges those independent changes first.
	_, e = engine.Sync(context.Background(), c, secrets, remote, remoteBase.Values)
	if e != nil {
		t.Fatal(e)
	}
	d = must(t, reopened, Request{Action: "config-sync-now"})
	if d.Settings.Theme != "dark" || d.Settings.Language != "zh-CN" {
		t.Fatalf("restart lost merge baseline: %+v", d.Settings)
	}
	current, e := engine.Sync(context.Background(), c, secrets, cloudsync.Values{}, nil)
	if e != nil {
		t.Fatal(e)
	}
	settings = d.Settings
	settings.Theme = "light"
	must(t, reopened, Request{Action: "settings", Settings: &settings})
	remote = copySyncValues(current.Values)
	remote["desktop.theme"] = json.RawMessage(`"system"`)
	remote["desktop.language"] = json.RawMessage(`"en"`)
	_, e = engine.Sync(context.Background(), c, secrets, remote, current.Values)
	if e != nil {
		t.Fatal(e)
	}
	conflict := reopened.Execute(Request{Action: "config-sync-now"})
	if conflict.Status != "error" || conflict.Code != "conflict" || conflict.Data.Settings.Theme != "light" {
		t.Fatalf("missing preserved conflict: %+v", conflict)
	}
	d = must(t, reopened, Request{Action: "config-sync-resolve", SyncConnection: &SyncConnectionRequest{Resolution: "local"}})
	if d.Settings.Theme != "light" || d.Settings.Language != "en" {
		t.Fatalf("local conflict resolution overwrote unrelated remote edit: %+v", d.Settings)
	}
}

func TestConfigSyncInvalidRemotePreservesSettingsAndObject(t *testing.T) {
	for _, invalid := range []string{`"invalid-theme"`, `null`, `123`} {
		t.Run(invalid, func(t *testing.T) {
			s, _, engine, c, secrets, bucket := newSyncAppSpace(t)
			_, e := engine.Sync(context.Background(), c, secrets, syncTestValues("desktop.theme", invalid), nil)
			if e != nil {
				t.Fatal(e)
			}
			before, puts, _ := bucket.snapshot()
			settings := s.prefs.Settings
			out := s.Execute(Request{Action: "config-sync-save", SyncConnection: &SyncConnectionRequest{Provider: "s3", Config: c, Secrets: secrets}})
			if out.Status != "error" || out.Code != "invalid_document" || s.prefs.Settings != settings {
				t.Fatalf("invalid remote applied: %s %+v", invalid, out)
			}
			after, newPuts, _ := bucket.snapshot()
			if puts != newPuts || !bytes.Equal(before, after) {
				t.Fatal("invalid remote was uploaded before owner validation")
			}
		})
	}
}

func TestConfigSyncKeepsLocalEditMadeDuringNetwork(t *testing.T) {
	s, _, engine, c, secrets, bucket := newSyncAppSpace(t)
	d := saveSyncFixture(t, s, c, secrets)
	remote, e := engine.Sync(context.Background(), c, secrets, cloudsync.Values{}, nil)
	if e != nil {
		t.Fatal(e)
	}
	remoteEdit := copySyncValues(remote.Values)
	remoteEdit["desktop.theme"] = json.RawMessage(`"dark"`)
	_, e = engine.Sync(context.Background(), c, secrets, remoteEdit, remote.Values)
	if e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	bucket.mu.Lock()
	bucket.getHook = func() { close(entered); <-release }
	bucket.mu.Unlock()
	done := make(chan Response, 1)
	go func() { done <- s.Execute(Request{Action: "config-sync-now"}) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("sync did not reach isolated server")
	}
	settings := d.Settings
	settings.Theme = "light"
	must(t, s, Request{Action: "settings", Settings: &settings})
	releaseOnce.Do(func() { close(release) })
	select {
	case out := <-done:
		if out.Status != "ok" || out.Data.Settings.Theme != "light" {
			t.Fatalf("in-flight sync lost newer local edit: %+v", out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sync did not complete")
	}
	d = must(t, s, Request{Action: "config-sync-now"})
	if d.Settings.Theme != "light" {
		t.Fatal("next sync lost preserved local edit")
	}
}

func TestConfigSyncBrowserReadinessValidationAndPause(t *testing.T) {
	s, _, _, c, secrets, bucket := newSyncAppSpace(t)
	theme := "dark"
	out := s.Execute(Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{Theme: &theme}})
	if out.Status != "error" || out.Code != "not_ready" {
		t.Fatalf("accepted browser patch before readiness: %+v", out)
	}
	saveSyncFixture(t, s, c, secrets)
	d := must(t, s, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{Theme: &theme}})
	if d.SyncedBrowserPreferences == nil || *d.SyncedBrowserPreferences.Theme != "dark" || d.SyncedBrowserPreferences.Language == nil {
		t.Fatal("partial browser patch did not preserve complete namespace")
	}
	invalid := "invalid-theme"
	out = s.Execute(Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{Theme: &invalid}})
	if out.Status != "error" || out.Code != "invalid_config" || *out.Data.SyncedBrowserPreferences.Theme != "dark" {
		t.Fatal("invalid browser patch changed saved preferences")
	}
	before, _, requests := bucket.snapshot()
	d = must(t, s, Request{Action: "config-sync-pause"})
	if d.ConfigSync.Enabled {
		t.Fatal("pause remained enabled")
	}
	out = s.Execute(Request{Action: "config-sync-now"})
	if out.Status != "error" || out.Code != "not_ready" {
		t.Fatal("paused sync ran")
	}
	after, _, newRequests := bucket.snapshot()
	if !bytes.Equal(before, after) || requests != newRequests {
		t.Fatal("paused sync contacted bucket")
	}
	must(t, s, Request{Action: "config-sync-resume"})
	must(t, s, Request{Action: "config-sync-now"})
	d = must(t, s, Request{Action: "config-sync-disconnect"})
	if d.ConfigSync.Enabled || d.ConfigSync.Provider != "" {
		t.Fatal("disconnect left connection active")
	}
	after, _, _ = bucket.snapshot()
	if len(after) == 0 {
		t.Fatal("disconnect removed remote objects")
	}
}

func TestConfigSyncBrowserCannotChangeConnectionOrSafetySettings(t *testing.T) {
	s, _, _, _, _, _ := newSyncAppSpace(t)
	for _, payload := range []string{
		`{"protocol":1,"action":"config-sync-save","syncConnection":{"provider":"s3"}}`,
		`{"protocol":1,"action":"config-sync-now"}`,
		`{"protocol":1,"action":"config-sync-browser","syncConnection":{"provider":"s3"},"syncPreferences":{"theme":"dark"}}`,
		`{"protocol":1,"action":"config-sync-browser","settings":{"idleMinutes":0}}`,
	} {
		var out Response
		if e := json.Unmarshal(s.handleBrowser([]byte(payload)), &out); e != nil {
			t.Fatal(e)
		}
		if out.Status != "error" || out.Code != "unsupported_action" && out.Code != "invalid_request" {
			t.Fatalf("browser connection escape accepted %s: %+v", payload, out)
		}
	}
}

func TestConfigSyncSecretFailureRetainsPreviousConnection(t *testing.T) {
	s, memory, _, c, secrets, bucket := newSyncAppSpace(t)
	saveSyncFixture(t, s, c, secrets)
	s.mu.Lock()
	old := s.configSync.meta.Profiles["s3"]
	s.mu.Unlock()
	memory.mu.Lock()
	memory.saveErr = secretstore.ErrDenied
	memory.mu.Unlock()
	changed := secrets
	changed.SecretAccessKey = "replacement-secret-marker"
	out := s.Execute(Request{Action: "config-sync-save", SyncConnection: &SyncConnectionRequest{Provider: "s3", Config: c, Secrets: changed}})
	if out.Status != "error" || out.Code != "credentials_denied" {
		t.Fatalf("missing secret store denial: %+v", out)
	}
	s.mu.Lock()
	active := s.configSync.meta.Profiles["s3"]
	s.mu.Unlock()
	if active.SecretRef != old.SecretRef {
		t.Fatal("failed secret save replaced active connection")
	}
	memory.mu.Lock()
	memory.saveErr = nil
	memory.loadErr = secretstore.ErrDenied
	loads := memory.loads
	retries := memory.retries
	memory.mu.Unlock()
	out = s.Execute(Request{Action: "config-sync-now"})
	if out.Code != "credentials_denied" {
		t.Fatal("denied sync not reported")
	}
	_, _, requests := bucket.snapshot()
	for i := 0; i < 3; i++ {
		must(t, s, Request{Action: "config-sync-status"})
	}
	memory.mu.Lock()
	if memory.loads != loads+1 || memory.retries != retries {
		t.Fatal("status retried denied credential access")
	}
	memory.mu.Unlock()
	_, _, newRequests := bucket.snapshot()
	if requests != newRequests {
		t.Fatal("status caused network sync")
	}
}

func TestConfigSyncBrokenMetadataPreserved(t *testing.T) {
	dir := t.TempDir()
	original := []byte(`{"version":999,"profiles":{},"pendingCleanup":[{"secretRef":"future-owned-ref"}],"futureMetadata":{"preserve":true}}`)
	if e := os.WriteFile(filepath.Join(dir, "config-sync.json"), original, 0600); e != nil {
		t.Fatal(e)
	}
	memory := &syncMemorySecrets{values: map[string][]byte{"future-owned-ref": []byte(`{"secrets":{}}`)}}
	s := newSyncServiceFixture(t, dir, memory, cloudsync.New(nil))
	out := s.Execute(Request{Action: "config-sync-save", SyncConnection: &SyncConnectionRequest{Provider: "s3"}})
	if out.Status != "error" || !s.configSync.invalid {
		t.Fatal("future metadata was treated as first setup")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	b, e := os.ReadFile(filepath.Join(dir, "config-sync.json"))
	if e != nil || !bytes.Equal(b, original) {
		t.Fatal("future metadata overwritten")
	}
	memory.mu.Lock()
	loads, remaining := memory.loads, len(memory.values)
	memory.mu.Unlock()
	if loads != 0 || remaining != 1 {
		t.Fatal("future metadata triggered credential cleanup during close")
	}
}

func TestConfigSyncBrowserPatchStopsAtShutdownAndUpdate(t *testing.T) {
	for _, phase := range []string{"update", "closed"} {
		t.Run(phase, func(t *testing.T) {
			s, _, _, c, secrets, _ := newSyncAppSpace(t)
			saveSyncFixture(t, s, c, secrets)
			if phase == "update" {
				s.updatePaused.Store(true)
			} else if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			theme := "dark"
			out := s.Execute(Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{Theme: &theme}})
			if out.Status != "error" {
				t.Fatalf("browser patch accepted after %s", phase)
			}
			if s.configSync.browser != nil {
				t.Fatalf("browser settings changed after %s", phase)
			}
			if _, err := os.Stat(filepath.Join(s.directory, "browser-preferences.json")); !os.IsNotExist(err) {
				t.Fatalf("browser settings persisted after %s: %v", phase, err)
			}
		})
	}
}

func TestConfigSyncDisconnectRetainsFailedSecretCleanupAcrossRestart(t *testing.T) {
	s, memory, engine, c, secrets, bucket := newSyncAppSpace(t)
	saveSyncFixture(t, s, c, secrets)
	memory.mu.Lock()
	memory.deleteErr = errors.New("isolated cleanup failure")
	memory.mu.Unlock()
	failed := s.Execute(Request{Action: "config-sync-disconnect"})
	if failed.Status != "error" {
		t.Fatal("failed credential cleanup reported success")
	}
	memory.mu.Lock()
	retained := len(memory.values)
	memory.deleteErr = nil
	memory.mu.Unlock()
	if retained == 0 {
		t.Fatal("failed credential cleanup unexpectedly removed ownership")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := newSyncServiceFixture(t, s.directory, memory, engine)
	must(t, reopened, Request{Action: "config-sync-disconnect"})
	memory.mu.Lock()
	remaining := len(memory.values)
	memory.mu.Unlock()
	if remaining != 0 {
		t.Fatal("restart lost failed local cleanup reference")
	}
	before, _, _ := bucket.snapshot()
	if len(before) == 0 {
		t.Fatal("disconnect removed encrypted bucket data")
	}
	must(t, reopened, Request{Action: "config-sync-disconnect"})
}

func TestConfigSyncBrowserPatchesAndSnapshotsDoNotAliasState(t *testing.T) {
	s, _, _, c, secrets, _ := newSyncAppSpace(t)
	saveSyncFixture(t, s, c, secrets)
	theme := "dark"
	data := must(t, s, Request{Action: "config-sync-browser", SyncPreferences: &BrowserSyncPreferences{Theme: &theme}})
	theme = "light"
	*data.SyncedBrowserPreferences.Language = "en"
	next := must(t, s, Request{Action: "config-sync-status"})
	if *next.SyncedBrowserPreferences.Theme != "dark" || *next.SyncedBrowserPreferences.Language != "system" {
		t.Fatal("caller-owned pointer mutated saved browser preferences")
	}
}

func TestConfigSyncSaveCannotActivateAfterCancelOrClose(t *testing.T) {
	for _, phase := range []string{"cancel", "close"} {
		t.Run(phase, func(t *testing.T) {
			s, memory, _, c, secrets, bucket := newSyncAppSpace(t)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			t.Cleanup(func() { once.Do(func() { close(release) }) })
			memory.mu.Lock()
			memory.saveHook = func() { close(entered); <-release }
			memory.mu.Unlock()
			done := make(chan Response, 1)
			go func() {
				done <- s.Execute(Request{Action: "config-sync-save", SyncConnection: &SyncConnectionRequest{Provider: "s3", Config: c, Secrets: secrets}})
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("save did not reach isolated store")
			}
			var closing chan error
			if phase == "cancel" {
				s.cancelConfigSync()
			} else {
				closing = make(chan error, 1)
				go func() { closing <- s.Close() }()
				deadline := time.Now().Add(5 * time.Second)
				for {
					s.mu.Lock()
					closed := s.closed
					s.mu.Unlock()
					if closed {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("close did not stop new requests")
					}
					time.Sleep(time.Millisecond)
				}
			}
			once.Do(func() { close(release) })
			select {
			case out := <-done:
				if out.Status != "error" || out.Code != "cancelled" {
					t.Fatalf("save succeeded after %s: %+v", phase, out.Data.ConfigSync)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("save stuck after %s", phase)
			}
			s.mu.Lock()
			active, enabled := s.configSync.meta.Active, s.configSync.meta.Enabled
			s.mu.Unlock()
			if active != "" || enabled {
				t.Fatalf("save activated profile after %s", phase)
			}
			_, puts, _ := bucket.snapshot()
			if puts != 0 {
				t.Fatalf("save uploaded settings after %s", phase)
			}
			if closing != nil {
				select {
				case err := <-closing:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("close did not finish after canceled save")
				}
			}
		})
	}
}

func TestConfigSyncSameSpaceResavePreservesUnsyncedEdits(t *testing.T) {
	for _, source := range []string{"s3", "r2"} {
		t.Run(source, func(t *testing.T) {
			s, _, _, c, secrets, _ := newSyncAppSpace(t)
			data := saveSyncFixture(t, s, c, secrets)
			if source == "r2" {
				// Model the same space reached with OAuth-derived credentials. The
				// switch to explicit manual keys must retain the merge history, and
				// must not require any Cloudflare discovery API to load S3 settings.
				s.mu.Lock()
				profile := s.configSync.meta.Profiles["s3"]
				s.configSync.meta.Profiles = map[string]syncProfile{"r2": profile}
				s.configSync.meta.Active = "r2"
				s.mu.Unlock()
			}
			settings := data.Settings
			settings.Theme = "dark"
			must(t, s, Request{Action: "settings", Settings: &settings})
			data = saveSyncFixture(t, s, c, secrets)
			if data.Settings.Theme != "dark" {
				t.Fatalf("same-space %s -> manual save discarded unsynced local theme", source)
			}
		})
	}
}

func TestConfigSyncShutdownCancelsNetworkAndRejectsQueuedWork(t *testing.T) {
	s, _, _, c, secrets, bucket := newSyncAppSpace(t)
	saveSyncFixture(t, s, c, secrets)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	bucket.mu.Lock()
	bucket.getHook = func() { close(entered); <-release }
	bucket.mu.Unlock()
	active := make(chan Response, 1)
	go func() { active <- s.Execute(Request{Action: "config-sync-now"}) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("sync did not reach stalled isolated network")
	}
	queued := make(chan Response, 1)
	started := make(chan struct{})
	go func() { close(started); queued <- s.Execute(Request{Action: "config-sync-now"}) }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed to cancel/drain sync: %v", err)
	}
	once.Do(func() { close(release) })
	for name, ch := range map[string]<-chan Response{"active": active, "queued": queued} {
		select {
		case out := <-ch:
			if out.Status != "error" || out.Code != "cancelled" {
				t.Fatalf("%s request admitted across shutdown: %+v", name, out)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s request did not leave update/sync lanes", name)
		}
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if !closed {
		t.Fatal("shutdown did not close service")
	}
}

func TestConfigSyncDerivedKeysRemainOwnedBySDK(t *testing.T) {
	s, memory, _, _, _, _ := newSyncAppSpace(t)
	for _, derived := range []bool{false, true} {
		value := syncSecret{Secrets: cloudsync.Secrets{AccessKeyID: "derived-access-marker", SecretAccessKey: "derived-secret-marker", SessionToken: "session-marker", Passphrase: "sync encryption phrase"}, Baseline: syncTestValues("desktop.theme", `"dark"`)}
		if derived {
			value.ConnectionID = "sdk-connection"
		}
		if err := s.storeSyncSecret("sync-credential-check", value); err != nil {
			t.Fatal(err)
		}
		memory.mu.Lock()
		saved := append([]byte(nil), memory.values["sync-credential-check"]...)
		memory.mu.Unlock()
		var decoded syncSecret
		if err := json.Unmarshal(saved, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Secrets.Passphrase != value.Secrets.Passphrase || string(decoded.Baseline["desktop.theme"]) != `"dark"` {
			t.Fatal("app sync phrase/baseline not preserved")
		}
		if derived {
			if decoded.Secrets.AccessKeyID != "" || decoded.Secrets.SecretAccessKey != "" || decoded.Secrets.SessionToken != "" || bytes.Contains(saved, []byte("derived-secret-marker")) {
				t.Fatal("SDK keypair copied into app sync record")
			}
		} else if decoded.Secrets.AccessKeyID != value.Secrets.AccessKeyID || decoded.Secrets.SecretAccessKey != value.Secrets.SecretAccessKey {
			t.Fatal("manual S3 credentials not preserved")
		}
	}
}

func addUnrelatedCorruptSDKCleanup(t *testing.T, s *Service) []byte {
	t.Helper()
	original := []byte(`{"version":999,"clientId":"test-client","future":{"preserve":true}}`)
	if err := os.WriteFile(filepath.Join(s.directory, "r2connect-state.json"), original, 0600); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.configSync.meta.PendingCleanup = append(s.configSync.meta.PendingCleanup, syncProfile{SecretRef: "sync-0123456789abcdef0123456789abcdef", ClientID: "test-client", Connection: &r2connect.Connection{ID: "pending-sdk-connection", AccountID: "test-account", Bucket: "test-bucket", Jurisdiction: "default", CredentialKey: "connection-pending", TokenID: "test-token", ExpiresAt: time.Now().Add(time.Hour)}})
	s.configSync.manager, s.configSync.client = nil, nil
	s.mu.Unlock()
	return original
}

func TestConfigSyncPausedStateDoesNotAttemptUnrelatedSDKCleanup(t *testing.T) {
	s, memory, _, c, secrets, _ := newSyncAppSpace(t)
	saveSyncFixture(t, s, c, secrets)
	original := addUnrelatedCorruptSDKCleanup(t, s)
	must(t, s, Request{Action: "config-sync-pause"})
	memory.mu.Lock()
	loads := memory.loads
	memory.mu.Unlock()
	out := s.Execute(Request{Action: "config-sync-now"})
	if out.Status != "error" || out.Code != "not_ready" {
		t.Fatalf("paused data sync attempted unrelated SDK cleanup: status=%s code=%s", out.Status, out.Code)
	}
	memory.mu.Lock()
	newLoads := memory.loads
	memory.mu.Unlock()
	if newLoads != loads {
		t.Fatal("paused sync accessed credentials")
	}
	stored, err := os.ReadFile(filepath.Join(s.directory, "r2connect-state.json"))
	if err != nil || !bytes.Equal(stored, original) {
		t.Fatal("paused sync modified invalid SDK state")
	}
}

func TestConfigSyncManualDataContinuesWithUnrelatedSDKLoadFailure(t *testing.T) {
	s, _, _, c, secrets, bucket := newSyncAppSpace(t)
	data := saveSyncFixture(t, s, c, secrets)
	original := addUnrelatedCorruptSDKCleanup(t, s)
	settings := data.Settings
	settings.Theme = "dark"
	must(t, s, Request{Action: "settings", Settings: &settings})
	_, puts, _ := bucket.snapshot()
	for _, action := range []string{"config-sync-now", "config-sync-save"} {
		request := Request{Action: action}
		if action == "config-sync-save" {
			request.SyncConnection = &SyncConnectionRequest{Provider: "s3", Config: c, Secrets: secrets}
		}
		out := s.Execute(request)
		if out.Status != "ok" || out.Data.ConfigSync.Provider != "s3" || out.Data.Settings.Theme != "dark" {
			t.Fatalf("manual data sync blocked by unrelated SDK state: %s %s", action, out.Code)
		}
		if out.Data.ConfigSync.OAuthLoadErrorCode == "" {
			t.Fatal("SDK load error was hidden")
		}
	}
	_, newPuts, _ := bucket.snapshot()
	if newPuts <= puts {
		t.Fatal("manual edits did not reach independent S3 space")
	}
	stored, err := os.ReadFile(filepath.Join(s.directory, "r2connect-state.json"))
	if err != nil || !bytes.Equal(stored, original) {
		t.Fatal("manual sync overwrote invalid SDK state")
	}
	s.mu.Lock()
	remaining := len(s.configSync.meta.PendingCleanup)
	s.mu.Unlock()
	if remaining == 0 {
		t.Fatal("manual sync lost unrelated cleanup ownership")
	}
}
