package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type fixture struct {
	t           *testing.T
	root, data  string
	public      ed25519.PublicKey
	private     ed25519.PrivateKey
	server      *httptest.Server
	manifest    Manifest
	full, delta []byte
	body        []byte
	mu          sync.Mutex
	hits        map[string]int
}

func checksum(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }
func fileSpec(name string, data []byte) File {
	return File{Path: name, SHA256: checksum(data), Size: int64(len(data)), Mode: 0o755}
}
func archiveBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, data := range files {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o755)
		entry, e := w.CreateHeader(header)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = entry.Write(data); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func newFixture(t *testing.T) *fixture {
	t.Helper()
	base := t.TempDir()
	f := &fixture{t: t, root: filepath.Join(base, "install"), data: filepath.Join(base, "data"), hits: map[string]int{}}
	if e := os.MkdirAll(f.root, 0o755); e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, filepath.Join(f.root, "passpine"), []byte("old main"))
	writeTestFile(t, filepath.Join(f.root, "passpine-host"), []byte("unchanged host"))
	var e error
	f.public, f.private, e = ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	f.full = archiveBytes(t, map[string][]byte{"passpine": []byte("new main"), "passpine-host": []byte("unchanged host")})
	f.delta = archiveBytes(t, map[string][]byte{"passpine": []byte("new main")})
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits[r.URL.Path]++
		f.mu.Unlock()
		switch r.URL.Path {
		case "/feed":
			w.Header().Set("Content-Type", "application/json")
			w.Write(f.body)
		case "/full":
			w.Write(f.full)
		case "/delta":
			w.Write(f.delta)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	f.manifest = Manifest{Schema: 1, Version: "1.1.0", Notes: map[string]string{"en": "Release notes", "zh-CN": "更新说明"}, Assets: []Asset{{OS: "linux", Arch: "amd64", Full: Download{URL: f.server.URL + "/full", SHA256: checksum(f.full), Size: int64(len(f.full))}, Files: []File{fileSpec("passpine", []byte("new main")), fileSpec("passpine-host", []byte("unchanged host"))}}}}
	f.sign()
	return f
}
func (f *fixture) sign() {
	payload, e := json.Marshal(f.manifest)
	if e != nil {
		f.t.Fatal(e)
	}
	f.body, e = json.Marshal(Envelope{Payload: base64.StdEncoding.EncodeToString(payload), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(f.private, payload))})
	if e != nil {
		f.t.Fatal(e)
	}
}
func (f *fixture) addDelta() {
	f.manifest.Assets[0].Deltas = []Delta{{FromVersion: "1.0.0", URL: f.server.URL + "/delta", SHA256: checksum(f.delta), Size: int64(len(f.delta))}}
	f.sign()
}
func (f *fixture) manager(auto *bool) *Manager {
	f.t.Helper()
	m, e := New(Config{CurrentVersion: "1.0.0", FeedURL: f.server.URL + "/feed", PublicKey: base64.StdEncoding.EncodeToString(f.public), DataDir: f.data, InstallRoot: f.root, Executable: filepath.Join(f.root, "passpine"), AutoDownload: auto, HTTPClient: f.server.Client(), OS: "linux", Arch: "amd64"})
	if e != nil {
		f.t.Fatal(e)
	}
	return m
}
func (f *fixture) count(name string) int { f.mu.Lock(); defer f.mu.Unlock(); return f.hits[name] }
func writeTestFile(t *testing.T, name string, data []byte) {
	t.Helper()
	if e := os.WriteFile(name, data, 0o755); e != nil {
		t.Fatal(e)
	}
}
func assertFile(t *testing.T, name, want string) {
	t.Helper()
	data, e := os.ReadFile(name)
	if e != nil || string(data) != want {
		t.Fatalf("%s = %q, %v; want %q", name, data, e, want)
	}
}

func TestDefaultAutomaticDownloadAndRestoredPending(t *testing.T) {
	f := newFixture(t)
	m := f.manager(nil)
	var states []State
	m.cfg.OnChange = func(s Snapshot) { states = append(states, s.State) }
	writeTestFile(t, filepath.Join(f.root, "unrelated-vault.gpg"), []byte("secret"))
	if e := m.Check(context.Background()); e != nil {
		t.Fatal(e)
	}
	if s := m.Snapshot(); s.State != StateReady || !s.AutoDownload || s.Version != "1.1.0" {
		t.Fatalf("snapshot: %+v", s)
	}
	if f.count("/full") != 1 {
		t.Fatal("default did not download")
	}
	assertFile(t, filepath.Join(f.root, "passpine"), "old main")
	assertFile(t, filepath.Join(f.root, "unrelated-vault.gpg"), "secret")
	found := map[State]bool{}
	for _, s := range states {
		found[s] = true
	}
	for _, s := range []State{StateChecking, StateDownloading, StateVerifying, StateReady} {
		if !found[s] {
			t.Fatalf("missing %s", s)
		}
	}
	restored := f.manager(nil)
	if restored.Snapshot().State != StateReady {
		t.Fatalf("pending not restored: %+v", restored.Snapshot())
	}
	f.body = []byte("network no longer returns valid feed")
	if e := restored.Check(context.Background()); e != nil {
		t.Fatal(e)
	}
	if f.count("/feed") != 1 {
		t.Fatal("check discarded ready release")
	}
}
func TestDisabledAutomaticDownloadStillChecks(t *testing.T) {
	f := newFixture(t)
	disabled := false
	m := f.manager(&disabled)
	if e := m.Check(context.Background()); e != nil {
		t.Fatal(e)
	}
	if m.Snapshot().State != StateAvailable || f.count("/full") != 0 {
		t.Fatal("downloaded while disabled")
	}
	if e := m.Check(context.Background()); e != nil {
		t.Fatal(e)
	}
	if f.count("/feed") != 2 {
		t.Fatal("automatic checks disabled")
	}
	if e := m.Download(context.Background()); e != nil {
		t.Fatal(e)
	}
	if m.Snapshot().State != StateReady {
		t.Fatal("manual download failed")
	}
}
func TestDeltaPreferredAndFullFallback(t *testing.T) {
	for _, scenario := range []string{"success", "corrupt-delta", "changed-base", "symlink-base"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			f.addDelta()
			switch scenario {
			case "corrupt-delta":
				f.delta = []byte(strings.Repeat("x", len(f.delta)))
			case "changed-base":
				writeTestFile(t, filepath.Join(f.root, "passpine-host"), []byte("unexpected host"))
			case "symlink-base":
				outside := filepath.Join(t.TempDir(), "user-data")
				writeTestFile(t, outside, []byte("unchanged host"))
				if e := os.Remove(filepath.Join(f.root, "passpine-host")); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(outside, filepath.Join(f.root, "passpine-host")); e != nil {
					t.Skip(e)
				}
			}
			m := f.manager(nil)
			if e := m.Check(context.Background()); e != nil {
				t.Fatal(e)
			}
			s := m.Snapshot()
			if s.State != StateReady {
				t.Fatalf("not ready: %+v", s)
			}
			if f.count("/delta") != 1 {
				t.Fatal("delta was not preferred")
			}
			if scenario == "success" {
				if !s.UsedDelta || s.Fallback || f.count("/full") != 0 {
					t.Fatalf("unexpected fallback: %+v", s)
				}
			} else if s.UsedDelta || !s.Fallback || f.count("/full") != 1 {
				t.Fatalf("missing full fallback: %+v", s)
			}
		})
	}
}
func TestUntrustedManifestDoesNotDownload(t *testing.T) {
	for _, scenario := range []string{"signature", "traversal", "unmanaged-file", "missing-host", "duplicate", "invalid-size"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			switch scenario {
			case "signature":
				f.private = make(ed25519.PrivateKey, ed25519.PrivateKeySize)
			case "traversal":
				f.manifest.Assets[0].Files[0].Path = "../vault"
			case "unmanaged-file":
				f.manifest.Assets[0].Files[0].Path = "settings.json"
			case "missing-host":
				f.manifest.Assets[0].Files = f.manifest.Assets[0].Files[:1]
			case "duplicate":
				f.manifest.Assets[0].Files = append(f.manifest.Assets[0].Files, f.manifest.Assets[0].Files[0])
			case "invalid-size":
				f.manifest.Assets[0].Full.Size = maxPackageSize + 1
			}
			f.sign()
			m := f.manager(nil)
			if e := m.Check(context.Background()); e == nil {
				t.Fatal("accepted untrusted manifest")
			}
			if f.count("/full") != 0 {
				t.Fatal("downloaded untrusted manifest")
			}
			assertFile(t, filepath.Join(f.root, "passpine"), "old main")
		})
	}
}
func TestSignedPackageCannotEscapeOrLie(t *testing.T) {
	for _, scenario := range []string{"hash", "archive-traversal", "missing-file", "extra-file", "member-size"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			switch scenario {
			case "hash":
				f.full[0] ^= 1
			case "archive-traversal":
				f.full = archiveBytes(t, map[string][]byte{"../vault": []byte("bad"), "passpine": []byte("new main"), "passpine-host": []byte("unchanged host")})
			case "missing-file":
				f.full = archiveBytes(t, map[string][]byte{"passpine": []byte("new main")})
			case "extra-file":
				f.full = archiveBytes(t, map[string][]byte{"settings.json": []byte("bad"), "passpine": []byte("new main"), "passpine-host": []byte("unchanged host")})
			case "member-size":
				f.full = archiveBytes(t, map[string][]byte{"passpine": []byte("different size"), "passpine-host": []byte("unchanged host")})
			}
			if scenario != "hash" {
				f.manifest.Assets[0].Full.SHA256 = checksum(f.full)
				f.manifest.Assets[0].Full.Size = int64(len(f.full))
				f.sign()
			}
			m := f.manager(nil)
			if e := m.Check(context.Background()); e == nil {
				t.Fatal("accepted invalid package")
			}
			if m.Snapshot().ErrorCode != ErrorVerification {
				t.Fatalf("wrong code: %+v", m.Snapshot())
			}
			assertFile(t, filepath.Join(f.root, "passpine"), "old main")
		})
	}
}
func TestInstallRevalidatesPackageAndStaging(t *testing.T) {
	for _, scenario := range []string{"archive", "tree", "extra-file"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			m := f.manager(nil)
			if e := m.Check(context.Background()); e != nil {
				t.Fatal(e)
			}
			switch scenario {
			case "archive":
				writeTestFile(t, m.readyArchive, []byte("corrupt"))
			case "tree":
				writeTestFile(t, filepath.Join(m.readyDir, "passpine"), []byte("tampered"))
			case "extra-file":
				writeTestFile(t, filepath.Join(m.readyDir, "unexpected"), []byte("bad"))
			}
			if e := m.Install(context.Background()); e == nil {
				t.Fatal("installed tampered package")
			}
			if m.Snapshot().ErrorCode != ErrorVerification {
				t.Fatalf("wrong error: %+v", m.Snapshot())
			}
			assertFile(t, filepath.Join(f.root, "passpine"), "old main")
			if e := m.Download(context.Background()); e != nil {
				t.Fatal(e)
			}
			if m.Snapshot().State != StateReady {
				t.Fatal("could not redownload after install validation failure")
			}
		})
	}
}
func TestInstallProtectsVaultAndNeighboringFiles(t *testing.T) {
	f := newFixture(t)
	m := f.manager(nil)
	if e := m.Check(context.Background()); e != nil {
		t.Fatal(e)
	}
	m.SetProtectedPaths([]string{filepath.Join(f.root, "my-vault")})
	if e := m.Install(context.Background()); e == nil {
		t.Fatal("did not protect vault within install directory")
	}
	assertFile(t, filepath.Join(f.root, "passpine"), "old main")
}
func TestReplacementRollbackAndNeighborPreservation(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "manual-rollback", true: "partial-failure"}[fail], func(t *testing.T) {
			f := newFixture(t)
			m := f.manager(nil)
			if e := m.Check(context.Background()); e != nil {
				t.Fatal(e)
			}
			writeTestFile(t, filepath.Join(f.root, "Uninstall.exe"), []byte("preserve"))
			if fail {
				if e := os.Remove(filepath.Join(f.root, "passpine-host")); e != nil {
					t.Fatal(e)
				}
				if e := os.Mkdir(filepath.Join(f.root, "passpine-host"), 0o755); e != nil {
					t.Fatal(e)
				}
			}
			transaction, e := m.replaceInstalled()
			if fail {
				if e == nil {
					t.Fatal("replacement unexpectedly succeeded")
				}
			} else {
				if e != nil {
					t.Fatal(e)
				}
				assertFile(t, filepath.Join(f.root, "passpine"), "new main")
				if e = transaction.rollback(); e != nil {
					t.Fatal(e)
				}
				assertFile(t, filepath.Join(f.root, "passpine-host"), "unchanged host")
			}
			assertFile(t, filepath.Join(f.root, "passpine"), "old main")
			assertFile(t, filepath.Join(f.root, "Uninstall.exe"), "preserve")
		})
	}
}
func TestInvalidPendingIsNotRestored(t *testing.T) {
	f := newFixture(t)
	m := f.manager(nil)
	if e := m.Check(context.Background()); e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, filepath.Join(m.readyDir, "passpine"), []byte("tampered"))
	restored := f.manager(nil)
	if restored.Snapshot().State == StateReady {
		t.Fatal("restored tampered tree")
	}
}
func TestDevelopmentAndMissingKeyRemainOffline(t *testing.T) {
	f := newFixture(t)
	m, e := New(Config{CurrentVersion: "development", FeedURL: f.server.URL + "/feed", DataDir: f.data, Executable: filepath.Join(f.root, "passpine")})
	if e != nil {
		t.Fatal(e)
	}
	m.Start(context.Background())
	if m.Snapshot().Configured || m.Snapshot().State != StateDisabled {
		t.Fatal("development updater enabled")
	}
	if e = m.Check(context.Background()); e != ErrUnavailable {
		t.Fatalf("wrong result %v", e)
	}
	if f.count("/feed") != 0 {
		t.Fatal("unconfigured updater contacted network")
	}
}
func TestVersions(t *testing.T) {
	ordered := []string{"0.9.9", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.10", "1.0.0-beta", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.1.0", "2.0.0"}
	for i, v := range ordered {
		if !validVersion(v) {
			t.Fatal(v)
		}
		for j, w := range ordered {
			got := compareVersions(v, w)
			if i < j && got >= 0 || i == j && got != 0 || i > j && got <= 0 {
				t.Fatalf("compare %s %s = %d", v, w, got)
			}
		}
	}
	for _, v := range []string{"development", "v1.0.0", "01.0.0", "1.0", "1.0.0-alpha.01", "1.0.0-", "1.0.0.0"} {
		if validVersion(v) {
			t.Fatalf("accepted %s", v)
		}
	}
}
func TestSnapshotCannotMutateReleaseNotes(t *testing.T) {
	f := newFixture(t)
	m := f.manager(nil)
	if e := m.Check(context.Background()); e != nil {
		t.Fatal(e)
	}
	s := m.Snapshot()
	s.Notes["en"] = "mutated"
	if m.Snapshot().Notes["en"] != "Release notes" {
		t.Fatal("snapshot aliases mutable notes")
	}
}
func TestHelperUsesCompiledTrustAnchors(t *testing.T) {
	f := newFixture(t)
	plan := installPlan{CurrentVersion: "1.0.0", PublicKey: base64.StdEncoding.EncodeToString(f.public)}
	data, e := json.Marshal(plan)
	if e != nil {
		t.Fatal(e)
	}
	name := filepath.Join(t.TempDir(), "plan.json")
	writeTestFile(t, name, data)
	for _, trust := range [][2]string{{"1.0.0", ""}, {"0.9.0", plan.PublicKey}, {"1.0.0", base64.StdEncoding.EncodeToString(make([]byte, 32))}} {
		if e = runHelper(name, trust[0], trust[1]); e == nil || !strings.Contains(e.Error(), "trusted running release") {
			t.Fatalf("accepted modified trust %v: %v", trust, e)
		}
	}
}
func TestStartupAcknowledgementCannotOverwriteUserFiles(t *testing.T) {
	base := t.TempDir()
	cache := filepath.Join(base, "updates")
	if e := os.Mkdir(cache, 0o700); e != nil {
		t.Fatal(e)
	}
	name := "startup-install-0123456789abcdef01234567.json.ok"
	outside := filepath.Join(base, "important-user-data")
	writeTestFile(t, outside, []byte("preserve"))
	startupAcknowledgement = outside
	ConfirmStartup(base)
	assertFile(t, outside, "preserve")
	ack := filepath.Join(cache, name)
	if e := os.Symlink(outside, ack); e != nil {
		t.Skip(e)
	}
	startupAcknowledgement = ack
	ConfirmStartup(base)
	assertFile(t, outside, "preserve")
	if e := os.Remove(ack); e != nil {
		t.Fatal(e)
	}
	startupAcknowledgement = ack
	ConfirmStartup(base)
	assertFile(t, ack, "ready")
	other := t.TempDir()
	if e := os.Remove(ack); e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(cache); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(other, cache); e != nil {
		t.Fatal(e)
	}
	startupAcknowledgement = ack
	ConfirmStartup(base)
	if _, e := os.Stat(filepath.Join(other, name)); !os.IsNotExist(e) {
		t.Fatal("wrote through symlinked cache")
	}
}
func TestCurrentVersionFeedKeepsLocalizedReleaseNotes(t *testing.T) {
	f := newFixture(t)
	f.manifest.Version = "1.0.0"
	f.sign()
	m := f.manager(nil)
	if e := m.Check(context.Background()); e != nil {
		t.Fatal(e)
	}
	s := m.Snapshot()
	if s.State != StateLatest || s.Version != "1.0.0" || s.Notes["en"] != "Release notes" || s.Notes["zh-CN"] != "更新说明" {
		t.Fatalf("missing current release notes %+v", s)
	}
	if f.count("/full") != 0 {
		t.Fatal("downloaded current version")
	}
}
func TestFailedInstallationIsVisibleAfterRollbackAndRetryable(t *testing.T) {
	f := newFixture(t)
	m := f.manager(nil)
	if e := m.Check(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := writeAtomic(filepath.Join(m.cacheDir(), "install-result.json"), []byte(`{"errorCode":"install"}`), 0o600); e != nil {
		t.Fatal(e)
	}
	restored := f.manager(nil)
	if s := restored.Snapshot(); s.State != StateFailed || s.ErrorCode != ErrorInstall {
		t.Fatalf("rollback failure disappeared: %+v", s)
	}
	if e := restored.Check(context.Background()); e != nil {
		t.Fatal(e)
	}
	if restored.Snapshot().State != StateFailed {
		t.Fatal("background check hid install failure")
	}
	if e := restored.Download(context.Background()); e != nil {
		t.Fatal(e)
	}
	if restored.Snapshot().State != StateReady {
		t.Fatal("install failure not retryable")
	}
}
func TestProtectedSymlinkAliasesCannotBypassInstallationProtection(t *testing.T) {
	f := newFixture(t)
	inside := filepath.Join(f.root, "vault")
	if e := os.Mkdir(inside, 0o700); e != nil {
		t.Fatal(e)
	}
	outside := filepath.Join(t.TempDir(), "external-link")
	if e := os.Symlink(inside, outside); e != nil {
		t.Skip(e)
	}
	m := f.manager(nil)
	m.SetProtectedPaths([]string{outside})
	if e := m.validateInstallRoot(); e == nil {
		t.Fatal("vault symlink into application passed protection")
	}
	m.SetProtectedPaths([]string{filepath.Join(outside, "not-created-yet")})
	if e := m.validateInstallRoot(); e == nil {
		t.Fatal("nonexistent vault child of symlink into application passed protection")
	}
	_, e := New(Config{CurrentVersion: "1.0.0", FeedURL: f.server.URL + "/feed", PublicKey: base64.StdEncoding.EncodeToString(f.public), DataDir: outside, InstallRoot: f.root, Executable: filepath.Join(f.root, "passpine"), OS: "linux", Arch: "amd64"})
	if e == nil {
		t.Fatal("application data symlink into application passed protection")
	}
	assertFile(t, filepath.Join(f.root, "passpine"), "old main")
}

func TestHelperModeNeverDeletesTheInstalledApplication(t *testing.T) {
	f := newFixture(t)
	m := f.manager(nil)
	key := base64.StdEncoding.EncodeToString(f.public)
	plan := installPlan{CurrentVersion: "1.0.0", PublicKey: key, FeedURL: "https://example.test/feed", DataDir: f.data, InstallRoot: f.root, Executable: filepath.Join(f.root, "passpine"), Envelope: f.body, ParentPID: os.Getpid()}
	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(m.cacheDir(), "install-0123456789abcdef01234567.json")
	writeTestFile(t, filename, payload)
	originalArgs := os.Args
	os.Args = []string{plan.Executable, helperFlag, filename}
	defer func() { os.Args = originalArgs }()
	if err = runHelper(filename, plan.CurrentVersion, key); err == nil {
		t.Fatal("main executable accepted helper mode")
	}
	assertFile(t, plan.Executable, "old main")
}

func TestAtomicBundleExchangeAndRollback(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS atomic directory exchange")
	}
	base := t.TempDir()
	original := filepath.Join(base, "Passpine.app")
	temporary := filepath.Join(base, ".passpine-update-test")
	staged := filepath.Join(temporary, "new.app")
	for _, dir := range []string{filepath.Join(original, "Contents"), filepath.Join(staged, "Contents")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(original, "Contents", "version"), []byte("old"))
	writeTestFile(t, filepath.Join(staged, "Contents", "version"), []byte("new"))
	if err := swapDirectories(staged, original); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(original, "Contents", "version"), "new")
	assertFile(t, filepath.Join(staged, "Contents", "version"), "old")
	transaction := &installTransaction{temporary: temporary, items: []replacement{{original: original, backup: staged, installed: true, swapped: true}}}
	if err := transaction.rollback(); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(original, "Contents", "version"), "old")
	if _, err := os.Stat(temporary); !os.IsNotExist(err) {
		t.Fatal("rollback left staging tree")
	}
}
