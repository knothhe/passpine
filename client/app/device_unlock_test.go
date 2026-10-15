package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixtureDevice struct {
	available   bool
	secret      []byte
	enrollError error
	started     chan struct{}
	block       bool
	unlocks     int
}

func (d *fixtureDevice) Available() bool { return d.available }
func (d *fixtureDevice) Enroll(_ context.Context, _ string, secret []byte) ([]byte, error) {
	if d.enrollError != nil {
		return nil, d.enrollError
	}
	d.secret = append([]byte{}, secret...)
	return []byte(`{"version":1,"ciphertext":"fixture-device-protected-data"}`), nil
}
func (d *fixtureDevice) Unlock(ctx context.Context, _ string, _ []byte) ([]byte, error) {
	d.unlocks++
	if d.block {
		close(d.started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return append([]byte{}, d.secret...), nil
}
func deviceService(t *testing.T, d *fixtureDevice) *Service {
	t.Helper()
	s, err := New(Options{DataDir: t.TempDir(), DeviceUnlock: d})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(); clear(d.secret) })
	must(t, s, Request{Action: "create", Directory: filepath.Join(t.TempDir(), "store"), Passphrase: "device-fixture-passphrase"})
	return s
}
func TestDeviceUnlockDefaultsEnrollmentAndRevocation(t *testing.T) {
	d := &fixtureDevice{available: true}
	s := deviceService(t, d)
	state := must(t, s, Request{Action: "configure"})
	if !state.Settings.BrowseWhileLocked || !state.Settings.DeviceUnlock || !state.DeviceUnlockEnrolled {
		t.Fatal("device unlock or locked metadata is not enabled by default")
	}
	file := s.deviceFile()
	envelope, err := os.ReadFile(file)
	if err != nil || strings.Contains(string(envelope), "device-fixture-passphrase") {
		t.Fatal("invalid persisted envelope", err)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("device envelope is not private", err)
	}
	saved := must(t, s, Request{Action: "save", Entry: "visible-name", Contents: "fixture-entry-secret", Mode: "create"})
	must(t, s, Request{Action: "lock"})
	locked := must(t, s, Request{Action: "configure"})
	if !locked.Locked || len(locked.Entries) != 1 || locked.Contents != "" {
		t.Fatal("locked metadata is unavailable or exposes content")
	}
	raw, _ := json.Marshal(Request{Action: "unlock", Protocol: 1})
	var browser Response
	if err = json.Unmarshal(s.handleBrowser(raw), &browser); err != nil {
		t.Fatal(err)
	}
	if browser.Code != "locked" || d.unlocks != 0 {
		t.Fatal("browser directly performed device authentication")
	}
	must(t, s, Request{Action: "unlock", Mode: "device"})
	if got := must(t, s, Request{Action: "fetch", Entry: saved.Entry}); got.Contents != saved.Contents {
		t.Fatal("device unlock cannot access the existing vault")
	}
	settings := state.Settings
	settings.DeviceUnlock = false
	must(t, s, Request{Action: "settings", Settings: &settings})
	if _, err = os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("disabling device unlock retained its envelope")
	}
	must(t, s, Request{Action: "lock"})
	if r := s.Execute(Request{Action: "unlock", Mode: "device"}); r.Status != "error" {
		t.Fatal("disabled device unlock still works")
	}
	must(t, s, Request{Action: "unlock", Passphrase: "device-fixture-passphrase"})
}
func TestDeviceAuthenticationIsCancelledByLock(t *testing.T) {
	d := &fixtureDevice{available: true, started: make(chan struct{}), block: true}
	s := deviceService(t, d)
	must(t, s, Request{Action: "lock"})
	finished := make(chan Response, 1)
	go func() { finished <- s.Execute(Request{Action: "unlock", Mode: "device"}) }()
	select {
	case <-d.started:
	case <-time.After(3 * time.Second):
		t.Fatal("system authentication never started")
	}
	must(t, s, Request{Action: "lock"})
	select {
	case result := <-finished:
		if result.Status != "error" || !result.Data.Locked {
			t.Fatal("cancelled authentication revived the session")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("locking did not cancel system authentication")
	}
}
func TestDeviceEnrollmentFailureKeepsPassphraseUnlock(t *testing.T) {
	for _, available := range []bool{true, false} {
		d := &fixtureDevice{available: available, enrollError: errors.New("fixture cancellation")}
		s := deviceService(t, d)
		state := must(t, s, Request{Action: "configure"})
		if state.Locked || state.DeviceUnlockEnrolled {
			t.Fatal("failed or unsupported enrollment broke passphrase access")
		}
		must(t, s, Request{Action: "lock"})
		must(t, s, Request{Action: "unlock", Passphrase: "device-fixture-passphrase"})
	}
}
