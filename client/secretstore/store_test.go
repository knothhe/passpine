package secretstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/knothhe/r2connect/keyring"
)

type memoryBackend struct {
	items   map[string][]byte
	calls   int
	failOp  string
	failErr error
}

func (b *memoryBackend) operation(op string) error {
	b.calls++
	if b.failOp == op {
		b.failOp = ""
		return b.failErr
	}
	return nil
}
func (b *memoryBackend) Load(service, key string) ([]byte, error) {
	if e := b.operation("load"); e != nil {
		return nil, e
	}
	value, ok := b.items[service+"/"+key]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), value...), nil
}
func (b *memoryBackend) Save(service, key string, value []byte) error {
	if e := b.operation("save"); e != nil {
		return e
	}
	b.items[service+"/"+key] = append([]byte(nil), value...)
	return nil
}
func (b *memoryBackend) Delete(service, key string) error {
	if e := b.operation("delete"); e != nil {
		return e
	}
	if _, ok := b.items[service+"/"+key]; !ok {
		return ErrNotFound
	}
	delete(b.items, service+"/"+key)
	return nil
}
func newMemoryStore(t *testing.T) (*Store, *memoryBackend) {
	t.Helper()
	b := &memoryBackend{items: map[string][]byte{}}
	s, e := NewWithBackend("dev", t.TempDir(), b)
	if e != nil {
		t.Fatal(e)
	}
	return s, b
}

func TestSDKDelegationRoundTripAndLimits(t *testing.T) {
	s, b := newMemoryStore(t)
	value := bytes.Repeat([]byte("token\x00"), 1000)
	if e := s.Save("profile", value); e != nil {
		t.Fatal(e)
	}
	if len(b.items) != 1 || !bytes.Equal(b.items[s.service+"/profile"], value) {
		t.Fatal("SDK logical key not used")
	}
	loaded, e := s.Load("profile")
	if e != nil || !bytes.Equal(value, loaded) {
		t.Fatal(e)
	}
	loaded[0] ^= 1
	if bytes.Equal(loaded, b.items[s.service+"/profile"]) {
		t.Fatal("loaded value aliases backend")
	}
	for _, bad := range [][]byte{nil, make([]byte, keyring.MaxValueBytes+1)} {
		if e := s.Save("bad", bad); !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
	}
	if e := s.Delete("profile"); e != nil {
		t.Fatal(e)
	}
	if e := s.Delete("profile"); e != nil {
		t.Fatal("missing SDK deletion not idempotent", e)
	}
	if len(b.items) != 0 {
		t.Fatal("SDK value retained after delete")
	}
}

func TestRefusalStopsReadWriteDeleteUntilExplicitRetry(t *testing.T) {
	for _, op := range []string{"load", "save", "delete"} {
		t.Run(op, func(t *testing.T) {
			s, b := newMemoryStore(t)
			if e := s.Save("profile", []byte("old")); e != nil {
				t.Fatal(e)
			}
			b.failOp, b.failErr = op, ErrDenied
			var err error
			switch op {
			case "load":
				_, err = s.Load("profile")
			case "save":
				err = s.Save("profile", []byte("new"))
			case "delete":
				err = s.Delete("profile")
			}
			if !errors.Is(err, keyring.ErrAccessDenied) || !s.Denied() {
				t.Fatal("refusal not propagated/latching", err)
			}
			calls := b.calls
			_, _ = s.Load("profile")
			_ = s.Save("profile", []byte("new"))
			_ = s.Delete("profile")
			if b.calls != calls {
				t.Fatal("refusal retriggered platform access")
			}
			s.RetryAccess()
			if s.Denied() {
				t.Fatal("explicit retry did not clear refusal")
			}
			if e := s.Save("other", []byte("new connection")); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestConcurrentRequestsCannotReopenDeniedStore(t *testing.T) {
	s, b := newMemoryStore(t)
	b.failOp, b.failErr = "load", ErrDenied
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() { defer group.Done(); _, _ = s.Load("profile") }()
	}
	group.Wait()
	if b.calls != 1 || !s.Denied() {
		t.Fatal("queued access reopened refused keyring")
	}
}

func TestFailedSDKSaveKeepsPreviousValue(t *testing.T) {
	s, b := newMemoryStore(t)
	if e := s.Save("profile", []byte("old")); e != nil {
		t.Fatal(e)
	}
	b.failOp, b.failErr = "save", ErrUnavailable
	if e := s.Save("profile", []byte("replacement")); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	got, e := s.Load("profile")
	if e != nil || string(got) != "old" {
		t.Fatal("failed SDK write replaced committed value", e)
	}
}

func TestNamespaceIsolationAndCanonicalPath(t *testing.T) {
	dir := t.TempDir()
	backend := &memoryBackend{items: map[string][]byte{}}
	dev, e := NewWithBackend("dev", dir, backend)
	if e != nil {
		t.Fatal(e)
	}
	release, _ := NewWithBackend("release", dir, backend)
	other, _ := NewWithBackend("dev", t.TempDir(), backend)
	if dev.service == release.service || dev.service == other.service {
		t.Fatal("credentials shared across identity")
	}
	expected, e := Namespace("dev", dir)
	if e != nil || dev.service != expected || backend.calls != 0 {
		t.Fatal("namespace changed or opened credentials")
	}
	if e = dev.Save("profile", []byte("private")); e != nil {
		t.Fatal(e)
	}
	if _, e = release.Load("profile"); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	link := filepath.Join(t.TempDir(), "alias")
	if e = os.Symlink(dir, link); e != nil {
		t.Skip("symlinks unavailable")
	}
	alias, e := Namespace("dev", link)
	if e != nil || alias != dev.service {
		t.Fatal("canonical alias differs", e)
	}
	missing, e := Namespace("dev", filepath.Join(link, "missing"))
	if e != nil {
		t.Fatal(e)
	}
	direct, e := Namespace("dev", filepath.Join(dir, "missing"))
	if e != nil || missing != direct {
		t.Fatal("missing leaf alias differs", e)
	}
}

func TestSDKErrorAliasesAndUnavailableMapping(t *testing.T) {
	if ErrNotFound != keyring.ErrNotFound || ErrDenied != keyring.ErrAccessDenied || ErrUnavailable != keyring.ErrUnavailable {
		t.Fatal("SDK sentinels copied instead of aliased")
	}
	s, b := newMemoryStore(t)
	b.failOp, b.failErr = "load", keyring.ErrStoreMissing
	if _, e := s.Load("profile"); !errors.Is(e, ErrUnavailable) {
		t.Fatal("missing platform store not mapped", e)
	}
	b.failOp, b.failErr = "load", keyring.ErrIncomplete
	if _, e := s.Load("profile"); !errors.Is(e, keyring.ErrIncomplete) {
		t.Fatal("SDK incomplete-value classification was lost", e)
	}
}

func TestRetrySessionWithoutEncryptedMetadataResetsAccessWithoutNativeIO(t *testing.T) {
	s, backend := newMemoryStore(t)
	if err := s.Save("profile", []byte("saved")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("profile"); err != nil {
		t.Fatal(err)
	}
	calls := backend.calls
	if changed, err := s.RetrySession(); err != nil || changed || len(s.loaded) != 0 || backend.calls != calls {
		t.Fatal("plain backend retry did not clear only the read session", changed, err)
	}
	backend.failOp, backend.failErr = "load", ErrDenied
	if _, err := s.Load("profile"); !errors.Is(err, ErrDenied) || !s.Denied() {
		t.Fatal("native refusal did not close access", err)
	}
	calls = backend.calls
	if changed, err := s.RetrySession(); err != nil || changed || s.Denied() || backend.calls != calls {
		t.Fatal("plain backend retry accessed native credentials", changed, err)
	}
	backend.failOp, backend.failErr = "", nil
	if value, err := s.Load("profile"); err != nil || string(value) != "saved" {
		t.Fatal("plain backend retry did not reopen access", err)
	}
}

func TestSuccessfulReadsShareOneNativeReadAndReturnCopies(t *testing.T) {
	s, b := newMemoryStore(t)
	if err := s.Save("profile", []byte("native secret")); err != nil {
		t.Fatal(err)
	}
	calls := b.calls
	var group sync.WaitGroup
	for range 16 {
		group.Go(func() {
			value, err := s.Load("profile")
			if err != nil || string(value) != "native secret" {
				t.Errorf("load = %q, %v", value, err)
			}
			// Neither the cache nor another caller owns this mutable slice.
			if len(value) > 0 {
				value[0] = 'X'
			}
		})
	}
	group.Wait()
	if b.calls != calls+1 {
		t.Fatalf("concurrent successful loads made %d native reads", b.calls-calls)
	}
	value, err := s.Load("profile")
	if err != nil || string(value) != "native secret" {
		t.Fatalf("caller changed the read session: %q, %v", value, err)
	}
}

func TestReadFailuresAreNeverCached(t *testing.T) {
	for _, failure := range []error{ErrNotFound, ErrUnavailable, keyring.ErrIncomplete} {
		t.Run(failure.Error(), func(t *testing.T) {
			s, b := newMemoryStore(t)
			if err := s.Save("profile", []byte("native secret")); err != nil {
				t.Fatal(err)
			}
			b.failOp, b.failErr = "load", failure
			if _, err := s.Load("profile"); !errors.Is(err, failure) {
				t.Fatal(err)
			}
			calls := b.calls
			if value, err := s.Load("profile"); err != nil || string(value) != "native secret" || b.calls != calls+1 {
				t.Fatalf("failed load suppressed a native retry: %q, %v", value, err)
			}
		})
	}
}

func TestWriteAndDeleteInvalidateOnlyTheirReadValue(t *testing.T) {
	for _, op := range []string{"save", "delete"} {
		for _, failure := range []error{nil, ErrUnavailable} {
			name := op + "/success"
			if failure != nil {
				name = op + "/failure"
			}
			t.Run(name, func(t *testing.T) {
				s, b := newMemoryStore(t)
				for _, key := range []string{"profile", "oauth"} {
					if err := s.Save(key, []byte("old")); err != nil {
						t.Fatal(err)
					}
					if _, err := s.Load(key); err != nil {
						t.Fatal(err)
					}
				}
				retained := s.loaded["profile"]
				if failure != nil {
					b.failOp, b.failErr = op, failure
				}
				var err error
				if op == "save" {
					err = s.Save("profile", []byte("replacement"))
				} else {
					err = s.Delete("profile")
				}
				if !errors.Is(err, failure) {
					t.Fatalf("operation error = %v, want %v", err, failure)
				}
				if !bytes.Equal(retained, make([]byte, len(retained))) {
					t.Fatal("invalidated read bytes were retained")
				}
				calls := b.calls
				if _, err := s.Load("oauth"); err != nil || b.calls != calls {
					t.Fatal("unrelated credential read was invalidated", err)
				}
				value, err := s.Load("profile")
				if b.calls != calls+1 {
					t.Fatal("write or delete bypassed the next real read")
				}
				switch {
				case failure != nil:
					if err != nil || string(value) != "old" {
						t.Fatal("failed operation hid the committed native value", err)
					}
				case op == "save":
					if err != nil || string(value) != "replacement" {
						t.Fatal("rotation did not reread native value", err)
					}
				default:
					if !errors.Is(err, ErrNotFound) {
						t.Fatal("deleted secret was returned from cache", err)
					}
				}
			})
		}
	}
}

func TestRefusalErasesAllSuccessfulReadsAndSessionClearCannotReopenAccess(t *testing.T) {
	for _, op := range []string{"load", "save", "delete"} {
		t.Run(op, func(t *testing.T) {
			s, b := newMemoryStore(t)
			if err := s.Save("profile", []byte("private")); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Load("profile"); err != nil {
				t.Fatal(err)
			}
			retained := s.loaded["profile"]
			b.failOp, b.failErr = op, ErrDenied
			switch op {
			case "load":
				_, _ = s.Load("other")
			case "save":
				_ = s.Save("other", []byte("new"))
			case "delete":
				_ = s.Delete("other")
			}
			if !s.Denied() || len(s.loaded) != 0 || !bytes.Equal(retained, make([]byte, len(retained))) {
				t.Fatal("refusal retained successful reads")
			}
			calls := b.calls
			s.ClearSession()
			if _, err := s.Load("profile"); !errors.Is(err, ErrDenied) || b.calls != calls {
				t.Fatal("cached read bypassed denied access", err)
			}
			s.RetryAccess()
			if _, err := s.Load("profile"); err != nil || b.calls != calls+1 {
				t.Fatal("explicit retry did not start a real native read", err)
			}
		})
	}
}

func TestSessionResetAndNewStoreReadNativeValues(t *testing.T) {
	dir := t.TempDir()
	b := &memoryBackend{items: map[string][]byte{}}
	s, err := NewWithBackend("dev", dir, b)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save("profile", []byte("old")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("profile"); err != nil {
		t.Fatal(err)
	}
	retained := s.loaded["profile"]
	b.items[s.service+"/profile"] = []byte("external update")
	if value, err := s.Load("profile"); err != nil || string(value) != "old" {
		t.Fatal("session did not reuse a successful read", err)
	}
	s.ClearSession()
	if !bytes.Equal(retained, make([]byte, len(retained))) {
		t.Fatal("session clear retained secret bytes")
	}
	calls := b.calls
	if value, err := s.Load("profile"); err != nil || string(value) != "external update" || b.calls != calls+1 {
		t.Fatal("session reset did not observe updated native value", err)
	}
	s.RetryAccess()
	calls = b.calls
	if _, err := s.Load("profile"); err != nil || b.calls != calls+1 {
		t.Fatal("reconnecting reused the old read session", err)
	}
	restarted, err := NewWithBackend("dev", dir, b)
	if err != nil {
		t.Fatal(err)
	}
	calls = b.calls
	if value, err := restarted.Load("profile"); err != nil || string(value) != "external update" || b.calls != calls+1 {
		t.Fatal("new process/store inherited a read session", err)
	}
}

type blockedReadBackend struct {
	*memoryBackend
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockedReadBackend) Load(service, key string) ([]byte, error) {
	b.once.Do(func() {
		close(b.started)
		<-b.release
	})
	return b.memoryBackend.Load(service, key)
}

func TestSessionClearDoesNotWaitForNativeReadOrCacheItsLateResult(t *testing.T) {
	b := &blockedReadBackend{
		memoryBackend: &memoryBackend{items: map[string][]byte{}},
		started:       make(chan struct{}), release: make(chan struct{}),
	}
	s, err := NewWithBackend("dev", t.TempDir(), b)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Save("profile", []byte("native secret")); err != nil {
		t.Fatal(err)
	}
	var release sync.Once
	unblock := func() { release.Do(func() { close(b.release) }) }
	defer unblock()
	finished := make(chan error, 1)
	go func() {
		value, err := s.Load("profile")
		if err == nil && string(value) != "native secret" {
			err = errors.New("native read returned the wrong value")
		}
		finished <- err
	}()
	select {
	case <-b.started:
	case <-time.After(3 * time.Second):
		t.Fatal("native read did not start")
	}
	cleared := make(chan struct{})
	go func() { s.ClearSession(); close(cleared) }()
	select {
	case <-cleared:
	case <-time.After(3 * time.Second):
		t.Fatal("session clear waited for a native prompt")
	}
	unblock()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	calls := b.calls
	if _, err := s.Load("profile"); err != nil || b.calls != calls+1 {
		t.Fatal("late native read repopulated the cleared session", err)
	}
	if _, err := s.Load("profile"); err != nil || b.calls != calls+1 {
		t.Fatal("fresh read did not establish the new session", err)
	}
}
