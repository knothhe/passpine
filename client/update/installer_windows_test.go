//go:build windows

package update

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNSISStageRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "stage-提取 full")
	if err := os.Mkdir(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateNSISStage(empty); err != nil {
		t.Fatalf("valid isolated staging directory: %v", err)
	}
	for _, path := range []string{"", ".", "stage-relative", `C:stage-relative`, `\\server\share\stage`, `\\?\C:\stage`, root + `\..\stage`, empty + `\`, empty + ".", empty + " ", empty + ":stream", filepath.Join(root, "missing")} {
		if err := validateNSISStage(path); err == nil {
			t.Errorf("accepted unsafe stage %q", path)
		}
	}
	if err := os.WriteFile(filepath.Join(empty, "existing-data"), []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateNSISStage(empty); err == nil {
		t.Fatal("accepted a non-empty staging directory")
	}
}

func TestNSISStageRejectsCurrentInstallation(t *testing.T) {
	current, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNSISStage(filepath.Dir(current)); err == nil {
		t.Fatal("accepted the current installation directory")
	}
}

func TestNSISStageRejectsReparseAncestor(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	stage := filepath.Join(target, "stage")
	if err := os.MkdirAll(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink privilege unavailable: %v", err)
	}
	if err := validateNSISStage(filepath.Join(link, "stage")); err == nil {
		t.Fatal("accepted a reparse point ancestor")
	}
}

func TestNSISExtractionCancellationPrecedesProcessLaunch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := extractNSIS(ctx, `C:\does-not-exist.exe`, `C:\does-not-exist`); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancellation", err)
	}
}

func TestNSISExtractionRequiresExecutableAndEmptyStage(t *testing.T) {
	stage := t.TempDir()
	if err := extractNSIS(context.Background(), filepath.Join(t.TempDir(), "package.zip"), stage); err == nil {
		t.Fatal("accepted an archive with the wrong executable extension")
	}
	if err := extractNSIS(context.Background(), filepath.Join(t.TempDir(), "package.exe"), stage); err == nil {
		t.Fatal("accepted a missing installer")
	}
}

func TestNSISUnmarkedExecutableIsNotRun(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "unexpected-process-start")
	source := filepath.Join(root, "legacy.go")
	if err := os.WriteFile(source, []byte(`package main
import "os"
func main() { if err := os.WriteFile(os.Getenv("PASSPINE_NSIS_LEGACY_MARKER"), []byte("started"), 0600); err != nil { panic(err) } }
`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(root, "legacy-setup.exe")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go.exe"), "build", "-o", legacy, source)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build unmarked fixture: %v\n%s", err, output)
	}
	t.Setenv("PASSPINE_NSIS_LEGACY_MARKER", marker)
	stage := filepath.Join(root, "stage")
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := extractNSIS(context.Background(), legacy, stage); err == nil || !strings.Contains(err.Error(), "supported extraction protocol") {
		t.Fatalf("legacy executable: %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unmarked executable was started: %v", err)
	}
}

// Opt-in only: the Windows packaging job passes its freshly built installer and
// payload directory. This executes extraction into t.TempDir, never installation.
func TestNSISBuiltInstallerExtraction(t *testing.T) {
	installer := os.Getenv("PASSPINE_TEST_NSIS_INSTALLER")
	payload := os.Getenv("PASSPINE_TEST_NSIS_PAYLOAD")
	if installer == "" || payload == "" {
		t.Skip("freshly built NSIS fixture not configured")
	}
	stage := filepath.Join(t.TempDir(), "stage-提取 full")
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := extractNSIS(context.Background(), installer, stage); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(stage)
	if err != nil || len(entries) != 2 {
		t.Fatalf("unexpected extraction tree: %v, %v", entries, err)
	}
	for _, name := range []string{"passpine.exe", "passpine-host.exe"} {
		got, err := os.ReadFile(filepath.Join(stage, name))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(payload, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("extracted %s differs from the original build", name)
		}
	}
}
