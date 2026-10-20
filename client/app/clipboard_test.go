package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func browserClipboardRequest(t *testing.T, s *Service, r Request) (Response, []byte) {
	t.Helper()
	r.Protocol = 1
	r.RequestID = "clipboard-test"
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	encoded := s.handleBrowser(raw)
	var out Response
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatal(err)
	}
	return out, encoded
}

func unconfiguredClipboardService(t *testing.T) *Service {
	t.Helper()
	s, err := New(Options{DataDir: t.TempDir(), DeviceUnlock: &fixtureDevice{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestClipboardWritesCarrySensitivityAndSessionLease(t *testing.T) {
	s := testService(t)
	saved := must(t, s, Request{Action: "save", Entry: "clipboard", Mode: "create", Contents: "fixture-password\nusername: alex\notp: JBSWY3DPEHPK3PXP\n"})
	var last ClipboardWrite
	s.SetClipboard(func(write ClipboardWrite) error {
		last = write
		return write.Lease(func() {})
	})
	for _, field := range []string{"", "password", "username", "otp", "raw"} {
		t.Run(field, func(t *testing.T) {
			must(t, s, Request{Action: "copy", Entry: saved.Entry, Field: field})
			if last.Sensitive != (field != "username") || last.Text == "" || last.Seconds != s.prefs.Settings.ClipboardSeconds {
				t.Fatal("clipboard request omitted its contents, sensitivity, or timeout")
			}
			if field == "username" && last.Text != "alex" {
				t.Fatal("username copy changed its contents")
			}
		})
	}
	must(t, s, Request{Action: "lock"})
	written := false
	if err := last.Lease(func() { written = true }); err == nil || written {
		t.Fatal("entry clipboard lease survived the vault lock")
	}
}

func TestBrowserGeneratedClipboardWithoutUnlockedVault(t *testing.T) {
	for _, configured := range []bool{false, true} {
		name := "unconfigured"
		if configured {
			name = "locked"
		}
		t.Run(name, func(t *testing.T) {
			var s *Service
			if configured {
				s = testService(t)
				must(t, s, Request{Action: "lock"})
			} else {
				s = unconfiguredClipboardService(t)
			}
			settings := s.prefs.Settings
			settings.ClipboardSeconds = 45
			must(t, s, Request{Action: "settings", Settings: &settings})
			password := strings.Repeat("g", 128)
			writes := 0
			s.SetClipboard(func(write ClipboardWrite) error {
				if write.Text != password || !write.Sensitive || write.Seconds != 45 || write.Lease == nil {
					t.Error("generated copy omitted sensitivity, timeout, contents, or lease")
				}
				// The native clipboard callback runs on the UI thread while
				// dispatch holds the service mutex on a worker thread.
				finished := make(chan error, 1)
				go func() { finished <- write.Lease(func() { writes++ }) }()
				select {
				case err := <-finished:
					return err
				case <-time.After(time.Second):
					return errors.New("clipboard lease blocked on the service mutex")
				}
			})
			out, encoded := browserClipboardRequest(t, s, Request{Action: "copy-generated", Contents: password})
			if out.Status != "ok" || out.RequestID != "clipboard-test" || writes != 1 || !out.Data.Locked || out.Data.Configured != configured {
				t.Fatal("generated copy failed without an unlocked vault", out.Code)
			}
			if strings.Contains(string(encoded), password) || out.Data.Password != "" || out.Data.Contents != "" {
				t.Fatal("generated copy returned the password")
			}
			activity, err := os.ReadFile(filepath.Join(s.directory, "activity.json"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(activity), password) {
				t.Fatal("generated copy logged the password")
			}
		})
	}
}

func TestBrowserGeneratedClipboardRejectsInvalidArguments(t *testing.T) {
	s := unconfiguredClipboardService(t)
	writes := 0
	s.SetClipboard(func(ClipboardWrite) error { writes++; return nil })
	requests := []Request{
		{},
		{Contents: strings.Repeat("a", 129)},
		{Contents: strings.Repeat("密", 43)},
		{Contents: "fixture-password", Interactive: true},
		{Contents: "fixture-password", VaultID: "vault"},
		{Contents: "fixture-password", Entry: "entry"},
		{Contents: "fixture-password", Field: "password"},
		{Contents: "fixture-password", Mode: "device"},
		{Contents: "fixture-password", Length: 12},
		{Contents: "fixture-password", Lowercase: true},
		{Contents: "fixture-password", URL: "https://example.invalid"},
		{Contents: "fixture-password", Passphrase: "fixture-passphrase"},
	}
	for i, request := range requests {
		request.Action = "copy-generated"
		out, _ := browserClipboardRequest(t, s, request)
		if out.Status != "error" || out.Code != "invalid_request" {
			t.Fatalf("request %d bypassed generated copy validation", i)
		}
	}
	if writes != 0 {
		t.Fatal("invalid request reached the clipboard")
	}
}

func TestGeneratedClipboardLeaseExpiresOnCancellationAndClose(t *testing.T) {
	for _, closeService := range []bool{false, true} {
		t.Run(map[bool]string{false: "lock", true: "close"}[closeService], func(t *testing.T) {
			s := unconfiguredClipboardService(t)
			var lease func(func()) error
			s.SetClipboard(func(write ClipboardWrite) error {
				lease = write.Lease
				return write.Lease(func() {})
			})
			out, _ := browserClipboardRequest(t, s, Request{Action: "copy-generated", Contents: "fixture-password"})
			if out.Status != "ok" || lease == nil {
				t.Fatal("could not capture generated clipboard lease")
			}
			if closeService {
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				must(t, s, Request{Action: "lock"})
			}
			written := false
			if err := lease(func() { written = true }); err == nil || written {
				t.Fatal("cancelled generated clipboard lease still wrote its password")
			}
			if closeService {
				out, _ := browserClipboardRequest(t, s, Request{Action: "copy-generated", Contents: "fixture-password"})
				if out.Status != "error" || out.Code != "app_unavailable" {
					t.Fatal("closed service accepted a generated clipboard write")
				}
			} else {
				out, _ := browserClipboardRequest(t, s, Request{Action: "copy-generated", Contents: "fixture-password"})
				if out.Status != "ok" {
					t.Fatal("fresh generated clipboard write failed after locking")
				}
			}
		})
	}
}

func TestClipboardCallbackFailuresReachTheCaller(t *testing.T) {
	s := testService(t)
	saved := must(t, s, Request{Action: "save", Entry: "clipboard-failure", Mode: "create", Contents: "fixture-password"})
	s.SetClipboard(func(ClipboardWrite) error { return errors.New("fixture native clipboard failure") })
	for _, request := range []Request{
		{Action: "copy", VaultID: s.prefs.Current, Entry: saved.Entry, Field: "password"},
		{Action: "copy-generated", Contents: "fixture-password"},
	} {
		out, encoded := browserClipboardRequest(t, s, request)
		if out.Status != "error" || strings.Contains(string(encoded), "fixture-password") {
			t.Fatal("clipboard failure returned success or a password")
		}
	}
}

func TestGeneratedClipboardRequestCannotCrossCancellation(t *testing.T) {
	s := unconfiguredClipboardService(t)
	writes := 0
	s.SetClipboard(func(write ClipboardWrite) error {
		s.cancellation.Add(1)
		return write.Lease(func() { writes++ })
	})
	out, _ := browserClipboardRequest(t, s, Request{Action: "copy-generated", Contents: "fixture-password"})
	if out.Status != "error" || out.Code != "locked" || writes != 0 {
		t.Fatal("cancelled generated password request reached the clipboard")
	}
	// A browser request captured before cancellation must not get a new lease
	// from the epoch current at dispatch time.
	epoch := s.cancellation.Load()
	s.cancellation.Add(1)
	out = s.executeWithBrowserLease(Request{Action: "copy-generated", Contents: "fixture-password"}, "browser", &browserResponseLease{epoch: epoch})
	if out.Status != "error" || out.Code != "locked" || writes != 0 {
		t.Fatal("stale browser generated password request was revived")
	}
}
