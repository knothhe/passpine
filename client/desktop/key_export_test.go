package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/knothhe/passpine/client/app"
)

func keyExportFixture() (*application, chan func(), chan app.Request) {
	a := fixture()
	a.page, a.state.VaultID = "settings", "export-vault"
	callbacks, requests := make(chan func(), 8), make(chan app.Request, 8)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	state := a.state
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		return app.Response{Status: "ok", Data: state}
	})
	return a, callbacks, requests
}

func takeExportCallback(t *testing.T, callbacks <-chan func()) func() {
	t.Helper()
	select {
	case callback := <-callbacks:
		return callback
	case <-time.After(3 * time.Second):
		t.Fatal("export did not return a main-thread callback")
		return nil
	}
}

// Closing a native save sheet restores focus to the manager. Hold that focus
// refresh's response until after the chosen path has reached the main thread.
func TestRecoveryKeyExportWaitsForFocusRefresh(t *testing.T) {
	service, err := app.New(app.Options{DataDir: t.TempDir(), Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	a := newApplication(service)
	a.state = service.Execute(app.Request{Action: "configure"}).Data
	a.page = "settings"
	callbacks := make(chan func(), 8)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	path := filepath.Join(t.TempDir(), "recovery key.asc")
	chosen := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-chosen:
		default:
			close(chosen)
		}
	})
	a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) {
		<-chosen
		return path, nil
	}
	a.exportKey()
	a.queueMetadataRefresh(false)
	refresh := takeExportCallback(t, callbacks)
	if !a.busy || !a.backgroundBusy || a.activeRequestAction != "configure" {
		t.Fatal("focus did not start its background refresh")
	}
	close(chosen)
	awaitMain(t, callbacks) // The save sheet returns while configure is busy.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("export ran before the current request finished", err)
	}
	refresh()
	if !a.busy || a.activeRequestAction != "export" {
		t.Fatal("the confirmed export was dropped behind the focus refresh")
	}
	awaitMain(t, callbacks)
	if a.busy || a.errorText != "" || a.notice != a.t("恢复私钥已导出。请使用验证备份确认可恢复。") {
		t.Fatalf("export did not report success: busy=%v error=%q notice=%q", a.busy, a.errorText, a.notice)
	}
	if result := service.Execute(app.Request{Action: "verify-backup", KeyFile: path, Passphrase: app.DemoPassphrase}); result.Status != "ok" {
		t.Fatalf("exported recovery key is not usable: %s", result.Message)
	}
}

func TestRecoveryKeyExportDoesNotRepeatDialogOrWrite(t *testing.T) {
	a, callbacks, requests := keyExportFixture()
	path := filepath.Join(t.TempDir(), "recovery.asc")
	dialogs := make(chan mygo.SaveDialogOptions, 8)
	a.preferences.saveDialog = func(options mygo.SaveDialogOptions) (string, error) {
		dialogs <- options
		return path, nil
	}
	a.exportKey()
	a.exportKey() // A second click while the sheet is open.
	awaitMain(t, callbacks)
	a.exportKey() // A second click while the export response is pending.
	awaitMain(t, callbacks)
	if len(dialogs) != 1 || len(requests) != 1 {
		t.Fatalf("repeated export: dialogs=%d requests=%d", len(dialogs), len(requests))
	}
	request := <-requests
	if request.Action != "export" || request.Destination != path || request.VaultID != "export-vault" {
		t.Fatalf("export did not retain the chosen path and vault: %#v", request)
	}
}

func TestRecoveryKeyExportCancelledOrFailedDialogCanRetry(t *testing.T) {
	for _, outcome := range []string{"cancel", "error"} {
		t.Run(outcome, func(t *testing.T) {
			a, callbacks, requests := keyExportFixture()
			a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) {
				if outcome == "error" {
					return "", errors.New("save dialog failed")
				}
				return "", nil
			}
			a.exportKey()
			awaitMain(t, callbacks)
			if a.busy || len(requests) != 0 || a.notice != "" || (a.errorText != "") != (outcome == "error") {
				t.Fatal("dismissed or failed save dialog submitted an export or lost its error")
			}
			path := filepath.Join(t.TempDir(), "retry.asc")
			a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) { return path, nil }
			a.exportKey()
			awaitMain(t, callbacks)
			if !a.busy || a.activeRequestAction != "export" {
				t.Fatal("dismissed save dialog prevented retry")
			}
			awaitMain(t, callbacks)
			if request := <-requests; request.Action != "export" || request.Destination != path || a.errorText != "" {
				t.Fatal("retry did not complete")
			}
		})
	}
}

func TestRecoveryKeyExportDiscardsStaleDestination(t *testing.T) {
	for _, stage := range []string{"dialog", "queued"} {
		for _, cancel := range []string{"navigate", "lock", "vault", "quit", "update"} {
			t.Run(stage+"/"+cancel, func(t *testing.T) {
				a, callbacks, requests := keyExportFixture()
				path := filepath.Join(t.TempDir(), "stale.asc")
				a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) {
					return path, nil
				}
				a.exportKey()
				result := takeExportCallback(t, callbacks)
				if stage == "queued" {
					a.busy = true
					result()
				}
				switch cancel {
				case "navigate":
					a.hidePreferences()
				case "lock":
					a.lock()
					select {
					case request := <-requests:
						if request.Action != "lock" {
							t.Fatal("unexpected request before lock")
						}
					case <-time.After(3 * time.Second):
						t.Fatal("lock did not reach the service")
					}
				case "vault":
					a.state.VaultID = "different-vault"
				case "quit":
					a.closing = true
				case "update":
					a.updates.preparing = true
				}
				if stage == "dialog" {
					result()
				}
				a.busy = false
				a.flushPendingKeyExport()
				if a.busy || len(requests) != 0 || a.notice != "" {
					t.Fatal("stale destination exported a different session or vault")
				}
			})
		}
	}
}

func TestRecoveryKeyExportPreservesExistingFileAndReportsFailure(t *testing.T) {
	service, err := app.New(app.Options{DataDir: t.TempDir(), Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	a := newApplication(service)
	a.state = service.Execute(app.Request{Action: "configure"}).Data
	callbacks := make(chan func(), 8)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	path := filepath.Join(t.TempDir(), "existing.asc")
	const original = "existing recovery backup"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) { return path, nil }
	a.exportKey()
	awaitMain(t, callbacks)
	awaitMain(t, callbacks)
	if a.busy || a.errorText == "" || a.notice != "" {
		t.Fatal("failed export did not report its error")
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != original {
		t.Fatal("export replaced an existing recovery backup", err)
	}
}
