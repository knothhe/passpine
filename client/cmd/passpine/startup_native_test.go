//go:build windows || linux

package main

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

// The native event loop must run on the process main thread, so the opt-in
// smoke test starts a child whose TestMain calls the production entry point.
func TestMain(m *testing.M) {
	if directory := os.Getenv("PASSPINE_NATIVE_SMOKE_CHILD"); directory != "" {
		os.Args = []string{"passpine", "--preview", "--data-dir", directory}
		var once sync.Once
		mygo.App.OnWindowCreated(func(w *mygo.Window) {
			w.OnShow(func() {
				once.Do(func() {
					go func() {
						time.Sleep(time.Second)
						data, err := w.CapturePage()
						if err != nil {
							fmt.Fprintln(os.Stderr, err)
							os.Exit(1)
						}
						img, err := png.Decode(bytes.NewReader(data))
						if err != nil || img.Bounds().Dx() < 600 || img.Bounds().Dy() < 400 || !w.IsVisible() {
							fmt.Fprintln(os.Stderr, "native window did not render a visible client frame", err)
							os.Exit(1)
						}
						if err := os.WriteFile(filepath.Join(directory, "startup.png"), data, 0600); err != nil {
							fmt.Fprintln(os.Stderr, err)
							os.Exit(1)
						}
						fmt.Println("native client startup verified")
						mygo.RunOnMain(mygo.App.Quit)
					}()
				})
			})
		})
		main()
		return
	}
	os.Exit(m.Run())
}

func TestNativePreviewStartup(t *testing.T) {
	if os.Getenv("PASSPINE_NATIVE_SMOKE") != "1" {
		t.Skip("set PASSPINE_NATIVE_SMOKE=1 on an interactive desktop")
	}
	// A short, separate runtime directory also isolates MyGo's single-instance
	// socket from any real Passpine process already running on the desktop.
	directory, err := os.MkdirTemp("", "pp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable)
	cmd.Env = append(os.Environ(), "PASSPINE_NATIVE_SMOKE_CHILD="+directory, "XDG_RUNTIME_DIR="+directory)
	// Preserve the compositor connection while isolating the instance socket.
	if display := os.Getenv("WAYLAND_DISPLAY"); display != "" && !filepath.IsAbs(display) {
		cmd.Env = append(cmd.Env, "WAYLAND_DISPLAY="+filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), display))
	}
	output, err := cmd.CombinedOutput()
	if err != nil || !bytes.Contains(output, []byte("native client startup verified")) {
		t.Fatalf("native startup failed: %v\n%s", err, output)
	}
	if destination := os.Getenv("PASSPINE_NATIVE_SMOKE_SCREENSHOT"); destination != "" {
		data, err := os.ReadFile(filepath.Join(directory, "startup.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
