package app

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knothhe/r2connect"
)

func sessionStateFixture(t *testing.T) (*syncSessionState, string, *atomic.Int32) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "r2connect-state.json")
	cleared := &atomic.Int32{}
	state := newSyncSessionState(path, func() { cleared.Add(1) })
	return state, path, cleared
}

func validSessionState() r2connect.State {
	return r2connect.State{
		Version: 1, ClientID: "isolated-client", Authorized: true, Renewable: true,
		OAuthCredentialKey: "oauth-isolated-client",
	}
}

func TestSyncSessionStateOwnWritesAndEquivalentMetadataKeepSession(t *testing.T) {
	state, path, cleared := sessionStateFixture(t)
	if _, exists, err := state.Load(); err != nil || exists {
		t.Fatal("new session did not start without persisted metadata", exists, err)
	}
	next := validSessionState()
	// nil Connections and timestamps with monotonic clock data must not
	// differ from the SDK's normalized, JSON-decoded state after an own save.
	next.Connections = []r2connect.Connection{{
		ID: "isolated-connection", AccountID: oauthTestAccount, Bucket: "isolated-bucket", Jurisdiction: "default",
		TokenID: "0123456789abcdef0123456789abcdef", ExpiresAt: time.Now().Add(time.Hour), CredentialKey: "connection-isolated",
	}}
	if err := state.Save(next); err != nil {
		t.Fatal(err)
	}
	if err := state.Check(); err != nil {
		t.Fatal("own metadata write invalidated the session", err)
	}
	next.Connections = nil
	if err := state.Save(next); err != nil {
		t.Fatal(err)
	}
	if err := state.Check(); err != nil {
		t.Fatal("nil connections differed from the SDK normalized snapshot", err)
	}
	// A harmless JSON reformat does not create a different connection session.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err = state.Check(); err != nil || cleared.Load() != 0 {
		t.Fatal("equivalent metadata cleared the session", err, cleared.Load())
	}
}

func TestSyncSessionStateExternalChangesStopReadsAndWrites(t *testing.T) {
	for _, change := range []string{"grant", "removal", "corrupt", "version", "directory"} {
		t.Run(change, func(t *testing.T) {
			state, path, cleared := sessionStateFixture(t)
			original := validSessionState()
			if err := state.Save(original); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "grant":
				changed := original
				changed.OAuthCredentialKey = "oauth-external-grant"
				if err := (r2connect.FileStateStore{Path: path}).Save(changed); err != nil {
					t.Fatal(err)
				}
			case "removal":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "corrupt", "version":
				data := []byte(`{"version":`)
				if change == "version" {
					data = []byte(`{"version":99,"connections":[]}`)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(path)
			if err := state.Check(); !errors.Is(err, errSyncSessionChanged) || !errors.Is(err, r2connect.ErrInvalidState) {
				t.Fatal("external change did not stop stale session", err)
			}
			if _, _, err := state.Load(); !errors.Is(err, errSyncSessionChanged) {
				t.Fatal("Load adopted external metadata", err)
			}
			if err := state.Save(original); !errors.Is(err, errSyncSessionChanged) {
				t.Fatal("Save overwrote external metadata", err)
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) || cleared.Load() != 1 {
				t.Fatal("external state was changed or session cleared repeatedly", cleared.Load())
			}
			if change == "removal" {
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("stale save recreated removed metadata", err)
				}
			}
		})
	}
}

func TestSyncSessionStateChecksImmediatelyBeforeSave(t *testing.T) {
	state, path, cleared := sessionStateFixture(t)
	if err := state.Save(validSessionState()); err != nil {
		t.Fatal(err)
	}
	external := validSessionState()
	external.Renewable = false
	file := r2connect.FileStateStore{Path: path}
	if err := file.Save(external); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(validSessionState()); !errors.Is(err, errSyncSessionChanged) {
		t.Fatal("Save ignored a change since the prior operation", err)
	}
	actual, err := r2connect.LoadState(file)
	if err != nil || actual.Renewable || cleared.Load() != 1 {
		t.Fatal("stale save replaced the other process's state", err)
	}
}

func TestSyncSessionStateDistinguishesMissingFromEmptyState(t *testing.T) {
	state, path, cleared := sessionStateFixture(t)
	if _, _, err := state.Load(); err != nil {
		t.Fatal(err)
	}
	if err := (r2connect.FileStateStore{Path: path}).Save(r2connect.State{Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := state.Check(); !errors.Is(err, errSyncSessionChanged) || cleared.Load() != 1 {
		t.Fatal("new external empty state matched a missing file", err)
	}
}

func TestSyncSessionStateInitialLoadErrorKeepsReloadPath(t *testing.T) {
	state, path, cleared := sessionStateFixture(t)
	invalid := []byte(`{"version":99}`)
	if err := os.WriteFile(path, invalid, 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err := state.Load()
	if !errors.Is(err, r2connect.ErrInvalidState) || errors.Is(err, errSyncSessionChanged) || cleared.Load() != 0 {
		t.Fatal("initial validation failure was misreported as a changed session", err)
	}
	if err = state.Save(validSessionState()); !errors.Is(err, r2connect.ErrInvalidState) {
		t.Fatal("initial invalid state was overwritten", err)
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != string(invalid) {
		t.Fatal("invalid state was not preserved for recovery")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, _, err = newSyncSessionState(path, nil).Load(); err != nil {
		t.Fatal("explicitly reconstructed session could not recover", err)
	}
}

func TestSyncSessionStateConcurrentChecksClearOnceAndRemainStopped(t *testing.T) {
	state, path, cleared := sessionStateFixture(t)
	if err := state.Save(validSessionState()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 12 {
		workers.Go(func() {
			if err := state.Check(); !errors.Is(err, errSyncSessionChanged) {
				t.Error("concurrent check resumed stale session", err)
			}
		})
	}
	workers.Wait()
	if cleared.Load() != 1 {
		t.Fatal("concurrent invalidation cleared more than once", cleared.Load())
	}
	if err := (r2connect.FileStateStore{Path: path}).Save(validSessionState()); err != nil {
		t.Fatal(err)
	}
	if err := state.Check(); !errors.Is(err, errSyncSessionChanged) {
		t.Fatal("restoring a file silently resumed a stopped session", err)
	}
}
