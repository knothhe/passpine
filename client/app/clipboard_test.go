package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
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

func TestBrowserCopyCustomField(t *testing.T) {
	s := testService(t)
	config, _ := browserClipboardRequest(t, s, Request{Action: "configure"})
	if !slices.Contains(config.Data.Capabilities, "copy-custom-field-v1") {
		t.Fatal("custom field copy was not advertised")
	}
	saved := must(t, s, Request{Action: "save", Entry: "example", Mode: "create", Contents: "secret\r\nusername: first\r\nUsername: second\r\ntoken: one\r\ntoken:  two \r\nOTP: JBSWY3DPEHPK3PXP\r\n"})
	var last ClipboardWrite
	writes := 0
	s.SetClipboard(func(write ClipboardWrite) error {
		return write.Lease(func() {
			last = write
			writes++
		})
	})
	request := Request{Action: "copy", VaultID: saved.VaultID, Entry: saved.Entry, Field: "custom", Revision: saved.Revision, FieldIndex: new(3), FieldKey: "token"}
	out, encoded := browserClipboardRequest(t, s, request)
	if out.Status != "ok" || writes != 1 || last.Text != " two " || !last.Sensitive || last.Seconds != s.prefs.Settings.ClipboardSeconds || strings.Contains(string(encoded), last.Text) {
		t.Fatal("copy lost duplicate field identity, whitespace, clipboard policy, or response privacy", out.Code)
	}
	alias := request
	alias.FieldIndex, alias.FieldKey = new(1), "Username"
	if out, _ := browserClipboardRequest(t, s, alias); out.Status != "ok" || last.Text != "second" || !last.Sensitive {
		t.Fatal("failed to copy secondary username independently")
	}
	for _, change := range []func(*Request){
		func(r *Request) { r.FieldIndex = nil },
		func(r *Request) { r.FieldIndex = new(-1) },
		func(r *Request) { r.FieldIndex = new(5) },
		func(r *Request) { r.FieldIndex = new(4); r.FieldKey = "OTP" },
		func(r *Request) { r.FieldKey = "" },
		func(r *Request) { r.FieldKey = "different" },
		func(r *Request) { r.Revision = "" },
		func(r *Request) { r.Field = "password" },
		func(r *Request) { r.Contents = "substitute" },
		func(r *Request) { r.VaultID = "another" },
	} {
		invalid := request
		change(&invalid)
		if out, _ := browserClipboardRequest(t, s, invalid); out.Status != "error" || writes != 2 {
			t.Fatal("invalid field selector wrote to the clipboard")
		}
	}
	must(t, s, Request{Action: "save", Entry: saved.Entry, Mode: "edit", Revision: saved.Revision, Contents: strings.Replace(saved.Contents, " two ", "three", 1)})
	if out, _ := browserClipboardRequest(t, s, request); out.Code != "revision_conflict" || writes != 2 {
		t.Fatal("stale field selection reached clipboard")
	}
	must(t, s, Request{Action: "lock"})
	if err := last.Lease(func() { t.Error("revoked field copy performed a native write") }); err == nil {
		t.Fatal("field clipboard lease survived lock")
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
