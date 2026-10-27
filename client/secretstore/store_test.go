package secretstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

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
