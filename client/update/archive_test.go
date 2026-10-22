package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type installerEntry struct {
	name     string
	body     []byte
	mode     os.FileMode
	typeflag byte
}

func installerZIP(t *testing.T, entries []installerEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		mode := entry.mode
		if mode == 0 {
			mode = 0o755
		}
		header.SetMode(mode)
		out, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = out.Write(entry.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func installerTAR(t *testing.T, entries []installerEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gz)
	for _, entry := range entries {
		kind := entry.typeflag
		if kind == 0 {
			kind = tar.TypeReg
		}
		header := &tar.Header{Name: entry.name, Mode: 0o755, Typeflag: kind, Size: int64(len(entry.body))}
		if kind == tar.TypeSymlink || kind == tar.TypeLink {
			header.Linkname = "/not-an-installed-file"
			header.Size = 0
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := writer.Write(entry.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func macInstallerEntries() []installerEntry {
	return []installerEntry{
		{name: "Passpine.app/", mode: os.ModeDir | 0o755},
		{name: "Passpine.app/Contents/", mode: os.ModeDir | 0o755},
		{name: "Passpine.app/Contents/MacOS/", mode: os.ModeDir | 0o755},
		{name: "Passpine.app/Contents/MacOS/passpine", body: []byte("new main")},
		{name: "Passpine.app/Contents/MacOS/passpine-host", body: []byte("unchanged host")},
		{name: "Passpine.app/Contents/Info.plist", body: []byte("signed bundle info"), mode: 0o644},
		{name: "Passpine.app/Contents/_CodeSignature/CodeResources", body: []byte("all signed bundle resources"), mode: 0o644},
	}
}

func linuxInstallerEntries() []installerEntry {
	return []installerEntry{
		{name: "passpine", body: []byte("new main")},
		{name: "passpine-host", body: []byte("unchanged host")},
		{name: "install.sh", body: []byte("#!/bin/sh\nexit 87\n")},
		{name: "version.txt", body: []byte("1.1.0\n")},
		{name: "app-icon-monochrome.png", body: []byte("installer icon")},
	}
}

func installerFixture(t *testing.T, platform string) (*fixture, *Manager) {
	t.Helper()
	f := newFixture(t)
	files := []File{fileSpec("passpine", []byte("new main")), fileSpec("passpine-host", []byte("unchanged host"))}
	format := "tar-gzip"
	executable := filepath.Join(f.root, "passpine")
	if platform == "darwin" {
		f.root = filepath.Join(filepath.Dir(f.root), "Passpine.app")
		if err := os.MkdirAll(filepath.Join(f.root, "Contents/MacOS"), 0o755); err != nil {
			t.Fatal(err)
		}
		executable = filepath.Join(f.root, "Contents/MacOS/passpine")
		writeTestFile(t, executable, []byte("old main"))
		files = nil
		for _, entry := range macInstallerEntries() {
			if entry.mode&os.ModeDir == 0 {
				file := fileSpec(strings.TrimPrefix(entry.name, "Passpine.app/"), entry.body)
				if entry.mode != 0 {
					file.Mode = uint32(entry.mode)
				}
				files = append(files, file)
			}
		}
		format, f.full = "app-zip", installerZIP(t, macInstallerEntries())
	} else {
		f.full = installerTAR(t, linuxInstallerEntries())
	}
	f.manifest.Version = "0.1.5" // A migration bridge is intentionally older than this client.
	f.manifest.InstallerUpdate = &Manifest{Schema: 2, Version: "1.1.0", Assets: []Asset{{OS: platform, Arch: "amd64", Format: format, Full: Download{URL: f.server.URL + "/full", SHA256: checksum(f.full), Size: int64(len(f.full))}, Files: files}}}
	f.sign()
	m, err := New(Config{CurrentVersion: "1.0.0", FeedURL: f.server.URL + "/feed", PublicKey: base64.StdEncoding.EncodeToString(f.public), DataDir: f.data, InstallRoot: f.root, Executable: executable, HTTPClient: f.server.Client(), OS: platform, Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	return f, m
}

func signInstallerPackage(f *fixture) {
	asset := &f.manifest.InstallerUpdate.Assets[0]
	asset.Full.Size = int64(len(f.full))
	asset.Full.SHA256 = checksum(f.full)
	f.sign()
}

func TestInstallerFeedSelectsNestedReleaseAndPreservesInstalledTree(t *testing.T) {
	for _, platform := range []string{"darwin", "linux"} {
		t.Run(platform, func(t *testing.T) {
			f, m := installerFixture(t, platform)
			if err := m.Check(context.Background()); err != nil {
				t.Fatal(err)
			}
			if s := m.Snapshot(); s.State != StateReady || s.Version != "1.1.0" {
				t.Fatalf("nested release was mistaken for legacy latest: %+v", s)
			}
			if m.manifest.Schema != 2 || filepath.Base(m.readyArchive) != packageName(m.asset) {
				t.Fatal("wrong protocol or package cache selected")
			}
			if platform == "linux" {
				for _, name := range []string{"install.sh", "version.txt", "app-icon-monochrome.png"} {
					if _, err := os.Lstat(filepath.Join(m.readyDir, name)); !os.IsNotExist(err) {
						t.Fatalf("installer metadata was staged: %s", name)
					}
				}
			}
			assertFile(t, m.cfg.Executable, "old main")
			if err := m.verifyPending(); err != nil {
				t.Fatal(err)
			}
			restored, err := New(m.cfg)
			if err != nil || restored.Snapshot().State != StateReady || restored.Snapshot().Version != "1.1.0" {
				t.Fatalf("installer pending state not restored: %v, %+v", err, restored)
			}
			if f.count("/full") != 1 {
				t.Fatal("restore downloaded an installer")
			}
		})
	}
}

func TestInstallerNestedVersionControlsLatestState(t *testing.T) {
	for _, version := range []string{"1.0.0", "0.9.0"} {
		t.Run(version, func(t *testing.T) {
			f, m := installerFixture(t, "linux")
			f.manifest.InstallerUpdate.Version = version
			f.sign()
			if err := m.Check(context.Background()); err != nil {
				t.Fatal(err)
			}
			if s := m.Snapshot(); s.State != StateLatest || s.Version != map[bool]string{true: "1.0.0", false: ""}[version == "1.0.0"] {
				t.Fatalf("incorrect nested latest state: %+v", s)
			}
			if f.count("/full") != 0 {
				t.Fatal("downloaded non-newer installer")
			}
		})
	}
}

func TestInvalidInstallerProtocolNeverFallsBackToLegacy(t *testing.T) {
	for _, scenario := range []string{"schema", "version", "missing-format", "wrong-format", "wrong-platform", "duplicate-platform", "nested-again", "direct-schema2", "signature"} {
		t.Run(scenario, func(t *testing.T) {
			f, m := installerFixture(t, "linux")
			nested := f.manifest.InstallerUpdate
			// If a malformed nested update accidentally falls back, this valid outer
			// version would trigger the legacy download path.
			f.manifest.Version = "1.2.0"
			switch scenario {
			case "schema":
				nested.Schema = 1
			case "version":
				nested.Version = "invalid"
			case "missing-format":
				nested.Assets[0].Format = ""
			case "wrong-format":
				nested.Assets[0].Format = "app-zip"
			case "wrong-platform":
				nested.Assets[0].OS = "darwin"
			case "duplicate-platform":
				nested.Assets = append(nested.Assets, nested.Assets[0])
			case "nested-again":
				nested.InstallerUpdate = &Manifest{Schema: 2, Version: "1.2.0"}
			case "direct-schema2":
				f.manifest = *nested
			}
			f.sign()
			if scenario == "signature" {
				f.body[len(f.body)-3] ^= 1
			}
			if err := m.Check(context.Background()); err == nil {
				t.Fatal("accepted malformed installer protocol")
			}
			if f.count("/full") != 0 {
				t.Fatal("downloaded after malformed protocol")
			}
		})
	}
}

func TestMacInstallerZIPRejectsUnsafeOrIncompleteMembers(t *testing.T) {
	for _, scenario := range []string{"traversal", "symlink", "duplicate", "case-duplicate", "outside-bundle", "appledouble", "wrong-bundle", "extra-file", "extra-directory", "missing-signature-resource", "size", "directory-data"} {
		t.Run(scenario, func(t *testing.T) {
			f, m := installerFixture(t, "darwin")
			entries := macInstallerEntries()
			switch scenario {
			case "traversal":
				entries = append(entries, installerEntry{name: "Passpine.app/../vault", body: []byte("bad")})
			case "symlink":
				entries[3].mode = os.ModeSymlink | 0o755
			case "duplicate":
				entries = append(entries, entries[3])
			case "case-duplicate":
				entries = append(entries, installerEntry{name: strings.ToUpper(entries[3].name), body: entries[3].body})
			case "outside-bundle":
				entries = append(entries, installerEntry{name: "notes.txt", body: []byte("extra")})
			case "appledouble":
				entries = append(entries, installerEntry{name: "__MACOSX/._Passpine.app", body: []byte("extra")})
			case "wrong-bundle":
				entries[3].name = "Other.app/Contents/MacOS/passpine"
			case "extra-file":
				entries = append(entries, installerEntry{name: "Passpine.app/Contents/extra", body: []byte("extra")})
			case "extra-directory":
				entries = append(entries, installerEntry{name: "Passpine.app/Contents/extra/", mode: os.ModeDir | 0o755})
			case "missing-signature-resource":
				entries = entries[:len(entries)-1]
			case "size":
				entries[3].body = []byte("wrong-sized binary")
			case "directory-data":
				// ZIP directory bodies are omitted by archive/zip itself. A file
				// impersonating the application directory must still be rejected.
				entries[0] = installerEntry{name: "Passpine.app", body: []byte("not a directory")}
			}
			f.full = installerZIP(t, entries)
			signInstallerPackage(f)
			if err := m.Check(context.Background()); err == nil || m.Snapshot().ErrorCode != ErrorVerification {
				t.Fatalf("accepted unsafe ZIP or wrong state: %v, %+v", err, m.Snapshot())
			}
			assertFile(t, m.cfg.Executable, "old main")
		})
	}
}

func TestLinuxInstallerTarRejectsUnsafeOrIncompleteMembers(t *testing.T) {
	for _, scenario := range []string{"traversal", "symlink", "hardlink", "duplicate", "case-duplicate", "extra-file", "directory", "missing-file", "size", "oversized-metadata", "gzip-checksum", "truncated", "appended-stream", "appended-data"} {
		t.Run(scenario, func(t *testing.T) {
			f, m := installerFixture(t, "linux")
			entries := linuxInstallerEntries()
			switch scenario {
			case "traversal":
				entries[2].name = "../install.sh"
			case "symlink":
				entries[0].typeflag = tar.TypeSymlink
			case "hardlink":
				entries[0].typeflag = tar.TypeLink
			case "duplicate":
				entries = append(entries, entries[0])
			case "case-duplicate":
				entries = append(entries, installerEntry{name: "PASSPINE", body: entries[0].body})
			case "extra-file":
				entries = append(entries, installerEntry{name: "settings.json", body: []byte("private data")})
			case "directory":
				entries = append(entries, installerEntry{name: "directory/", typeflag: tar.TypeDir})
			case "missing-file":
				entries = entries[1:]
			case "size":
				entries[0].body = []byte("wrong-sized binary")
			case "oversized-metadata":
				entries[2].body = bytes.Repeat([]byte{'x'}, (1<<20)+1)
			}
			f.full = installerTAR(t, entries)
			switch scenario {
			case "gzip-checksum":
				f.full[len(f.full)-8] ^= 1
			case "truncated":
				f.full = f.full[:len(f.full)-4]
			case "appended-stream":
				f.full = append(f.full, installerTAR(t, linuxInstallerEntries())...)
			case "appended-data":
				f.full = append(f.full, []byte("unexpected")...)
			}
			signInstallerPackage(f)
			if err := m.Check(context.Background()); err == nil || m.Snapshot().ErrorCode != ErrorVerification {
				t.Fatalf("accepted unsafe TAR or wrong state: %v, %+v", err, m.Snapshot())
			}
			assertFile(t, m.cfg.Executable, "old main")
		})
	}
}

func TestInstallerPendingTamperingFailsBeforeHelperStarts(t *testing.T) {
	for _, platform := range []string{"darwin", "linux"} {
		for _, scenario := range []string{"archive", "tree", "extra-file"} {
			t.Run(platform+"/"+scenario, func(t *testing.T) {
				_, m := installerFixture(t, platform)
				if err := m.Check(context.Background()); err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "archive":
					writeTestFile(t, m.readyArchive, []byte("tampered installer"))
				case "tree":
					writeTestFile(t, filepath.Join(m.readyDir, filepath.FromSlash(mainExecutable(platform))), []byte("tampered binary"))
				case "extra-file":
					writeTestFile(t, filepath.Join(m.readyDir, "untrusted"), []byte("extra"))
				}
				if err := m.Install(context.Background()); err == nil || m.Snapshot().ErrorCode != ErrorVerification {
					t.Fatalf("accepted tampered installer: %v %+v", err, m.Snapshot())
				}
				restored, err := New(m.cfg)
				if err != nil || restored.Snapshot().State == StateReady {
					t.Fatalf("restored tampered installer: %v", err)
				}
				matches, err := filepath.Glob(filepath.Join(m.cacheDir(), "helper-*"))
				if err != nil || len(matches) != 0 {
					t.Fatal("tampered installer launched helper")
				}
				assertFile(t, m.cfg.Executable, "old main")
			})
		}
	}
}

func TestSignedInstallerCacheCannotUseLegacyArchiveName(t *testing.T) {
	_, m := installerFixture(t, "linux")
	if err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(m.readyArchive, filepath.Join(m.cacheDir(), "package.zip")); err != nil {
		t.Fatal(err)
	}
	restored, err := New(m.cfg)
	if err != nil || restored.Snapshot().State == StateReady {
		t.Fatalf("restored installer from unrelated legacy cache: %v", err)
	}
}

func TestInstallerFormatCacheNames(t *testing.T) {
	for format, want := range map[string]string{"": "package.zip", "app-zip": "package.zip", "tar-gzip": "package.tar.gz", "nsis": "package.exe"} {
		if got := packageName(&Asset{Format: format}); got != want {
			t.Fatalf("%s: %s != %s", format, got, want)
		}
	}
}

// Exercise the system tar writer used by the ordinary Linux packaging script,
// including its platform-specific padding and optional PAX metadata.
func TestLinuxInstallerProducedBySystemTar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux packaging uses Unix tar")
	}
	tarCommand, err := exec.LookPath("tar")
	if err != nil {
		t.Skip("system tar unavailable")
	}
	f, m := installerFixture(t, "linux")
	input := t.TempDir()
	arguments := []string{"-czf", filepath.Join(t.TempDir(), "ordinary.tar.gz"), "-C", input}
	for _, entry := range linuxInstallerEntries() {
		writeTestFile(t, filepath.Join(input, entry.name), entry.body)
		arguments = append(arguments, entry.name)
	}
	command := exec.Command(tarCommand, arguments...)
	command.Env = append(os.Environ(), "COPYFILE_DISABLE=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("system tar: %v, %s", err, output)
	}
	f.full, err = os.ReadFile(arguments[1])
	if err != nil {
		t.Fatal(err)
	}
	signInstallerPackage(f)
	if err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot().State != StateReady {
		t.Fatalf("ordinary Linux package was not staged: %+v", m.Snapshot())
	}
}
