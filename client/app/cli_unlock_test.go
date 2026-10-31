package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/knothhe/passpine/client/internal/clisession"
	"github.com/knothhe/passpine/client/nativehost"
)

func lockedCLIChallenge(t *testing.T, s *Service) clisession.Request {
	t.Helper()
	directory := must(t, s, Request{Action: "status"}).Directory
	must(t, s, Request{Action: "lock"})
	r := clisession.Request{Protocol: clisession.Protocol, PayloadVersion: clisession.PayloadVersion, Action: "status", Directory: directory}
	out := cliRequest(t, s, r)
	if out.Code != "locked" || !out.CanUnlock || len(out.Session) != 64 || out.Document != nil || len(out.Entries) != 0 {
		t.Fatal("locked desktop did not publish a bounded authentication challenge")
	}
	r.Session = out.Session
	return r
}

func relayCLIRequest(t *testing.T, client *nativehost.RelayClient, r clisession.Request) clisession.Response {
	t.Helper()
	r.Protocol = clisession.Protocol
	r.PayloadVersion = clisession.PayloadVersion
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	response := client.Send(raw)
	defer clear(response)
	var out clisession.Response
	if err := json.Unmarshal(response, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCLIBridgePassphraseUnlocksDesktopAndBrowser(t *testing.T) {
	s := testService(t)
	entry := must(t, s, Request{Action: "save", Entry: "fixture", Contents: "fixture-secret", Mode: "create"})
	r := lockedCLIChallenge(t, s)
	client := nativehost.NewRelayClientWithFrameLimit(clisession.Directory(s.directory), clisession.MaxFrame)
	defer client.Close()
	changed := make(chan struct{}, 1)
	s.SetOnChange(func() { changed <- struct{}{} })
	version := must(t, s, Request{Action: "status"}).EntriesVersion

	// A locked challenge must not read contents or write before authentication.
	r.Action, r.Path = "read", entry.Entry
	if out := relayCLIRequest(t, client, r); out.Code != "locked" || out.Document != nil {
		t.Fatal("locked challenge could read entry contents")
	}
	r.Action, r.Path, r.Mode, r.Passphrase = "unlock", "", "passphrase", []byte("incorrect-fixture-passphrase")
	if out := relayCLIRequest(t, client, r); out.Code != "invalid_passphrase" || !s.IsLocked() {
		t.Fatalf("incorrect CLI passphrase changed desktop state: %s", out.Code)
	}
	r.Passphrase = []byte("test-passphrase-123")
	out := relayCLIRequest(t, client, r)
	if out.Status != "ok" || len(out.Session) != 64 || out.Session == r.Session || s.IsLocked() || out.Document != nil {
		t.Fatalf("CLI unlock did not establish a desktop session: %s", out.Code)
	}
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("desktop was not notified about CLI authentication")
	}
	state := must(t, s, Request{Action: "status"})
	if state.Locked || state.EntriesVersion <= version {
		t.Fatal("desktop metadata did not reflect the shared unlocked session")
	}
	if stale := relayCLIRequest(t, client, r); stale.Code != "cancelled" {
		t.Fatal("successful unlock left its old locked challenge reusable")
	}
	r.Action, r.Mode, r.Passphrase, r.Path, r.Session = "read", "", nil, entry.Entry, out.Session
	if got := relayCLIRequest(t, client, r); got.Status != "ok" || got.Document == nil || string(got.Document.Raw) != entry.Contents {
		t.Fatal("CLI could not use its new desktop session")
	}
	browserRaw, _ := json.Marshal(Request{Protocol: 1, Action: "fetch", VaultID: state.VaultID, Entry: entry.Entry})
	var browser Response
	if err := json.Unmarshal(s.handleBrowser(browserRaw), &browser); err != nil || browser.Status != "ok" || browser.Data.Contents != entry.Contents {
		t.Fatal("browser did not share the CLI-authenticated desktop session")
	}
	activity, err := os.ReadFile(filepath.Join(s.directory, "activity.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"test-passphrase-123", "incorrect-fixture-passphrase", entry.Contents} {
		if strings.Contains(string(activity), secret) {
			t.Fatal("CLI authentication persisted private input in activity")
		}
	}
	if !strings.Contains(string(activity), `"source":"cli"`) && !strings.Contains(string(activity), `"source": "cli"`) {
		t.Fatal("CLI authentication did not record its source")
	}
}

func TestCLIBridgeSystemAuthenticationUnlocksDesktop(t *testing.T) {
	d := &fixtureDevice{available: true}
	s := deviceService(t, d)
	r := lockedCLIChallenge(t, s)
	status := r
	status.Session = ""
	if out := cliRequest(t, s, status); !out.DeviceUnlock || !s.IsLocked() || d.unlocks != 0 {
		t.Fatal("status did not advertise passive system authentication capability")
	}
	r.Action, r.Mode = "unlock", "device"
	out := cliRequest(t, s, r)
	if out.Status != "ok" || s.IsLocked() || d.unlocks != 1 || out.Session == r.Session {
		t.Fatalf("CLI system authentication did not unlock the desktop: %s", out.Code)
	}
}

func TestCLIBridgeSystemAuthenticationFailureKeepsPassphraseChallenge(t *testing.T) {
	d := &fixtureDevice{available: true}
	s := deviceService(t, d)
	clear(d.secret)
	d.secret = []byte("incorrect-fixture-passphrase")
	r := lockedCLIChallenge(t, s)
	r.Action, r.Mode = "unlock", "device"
	if out := cliRequest(t, s, r); out.Code != "authentication_failed" || !s.IsLocked() {
		t.Fatalf("failed device authentication changed shared state: %s", out.Code)
	}
	r.Mode, r.Passphrase = "passphrase", []byte("device-fixture-passphrase")
	if out := cliRequest(t, s, r); out.Status != "ok" || s.IsLocked() {
		t.Fatalf("passphrase could not complete CLI authentication: %s", out.Code)
	}
}

func TestCLIBridgeAuthenticationCannotSurviveLockOrSwitch(t *testing.T) {
	for _, action := range []string{"lock", "switch"} {
		t.Run(action, func(t *testing.T) {
			d := &fixtureDevice{available: true, started: make(chan struct{}), block: true}
			s := deviceService(t, d)
			var replacement string
			if action == "switch" {
				first := must(t, s, Request{Action: "status"}).VaultID
				replacement = must(t, s, Request{Action: "create", Directory: filepath.Join(t.TempDir(), "replacement"), Passphrase: "replacement-fixture-passphrase"}).VaultID
				must(t, s, Request{Action: "switch", ID: first})
			}
			r := lockedCLIChallenge(t, s)
			r.Action, r.Mode = "unlock", "device"
			result := make(chan clisession.Response, 1)
			go func() { result <- cliRequest(t, s, r) }()
			select {
			case <-d.started:
			case <-time.After(3 * time.Second):
				t.Fatal("fixture authentication did not begin")
			}
			must(t, s, Request{Action: action, ID: replacement})
			select {
			case out := <-result:
				if out.Code != "cancelled" || out.Session != "" || !s.IsLocked() {
					t.Fatalf("cancelled CLI authentication revived a session: %s", out.Code)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("lock or switch did not cancel CLI authentication")
			}
		})
	}
}

func TestCLIBridgeAuthenticationRejectsInvalidFieldsAndStaleChallenge(t *testing.T) {
	s := testService(t)
	r := lockedCLIChallenge(t, s)
	r.Action, r.Mode, r.Passphrase = "unlock", "passphrase", []byte("test-passphrase-123")
	for _, mutate := range []func(*clisession.Request){
		func(r *clisession.Request) { r.Session = "" },
		func(r *clisession.Request) { r.Mode = "unsupported" },
		func(r *clisession.Request) { r.Path = "fixture" },
		func(r *clisession.Request) { r.Mode = "device" },
		func(r *clisession.Request) { r.Action = "list" },
		func(r *clisession.Request) { r.Action = "read"; r.Path = "fixture" },
	} {
		bad := r
		mutate(&bad)
		if out := cliRequest(t, s, bad); out.Code != "invalid_request" || !s.IsLocked() {
			t.Fatalf("invalid authentication request was accepted: %s", out.Code)
		}
	}
	must(t, s, Request{Action: "lock"})
	if out := cliRequest(t, s, r); out.Code != "cancelled" || !s.IsLocked() {
		t.Fatal("old locked challenge survived an explicit lock")
	}
}
