package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func values(pairs ...string) Values {
	out := Values{}
	for i := 0; i < len(pairs); i += 2 {
		out[pairs[i]] = json.RawMessage(pairs[i+1])
	}
	return out
}

func expectCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil || ErrorCode(err) != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}

func TestConfigValidation(t *testing.T) {
	base := Config{Endpoint: "https://s3.example.test", Bucket: "my-bucket", PathStyle: true}
	got, err := NormalizeConfig(base)
	if err != nil || got.Region != "auto" || got.Prefix != "passpine" {
		t.Fatalf("defaults: %+v %v", got, err)
	}
	for _, endpoint := range []string{"http://example.test", "https://user:secret@example.test", "https://example.test/?secret=value", "https://example.test/#fragment", "https://example.test/path", "file:///tmp", "https:///missing", "https://example.test/%2F"} {
		c := base
		c.Endpoint = endpoint
		_, err := NormalizeConfig(c)
		expectCode(t, err, "invalid_config")
	}
	for _, endpoint := range []string{"http://127.0.0.1:1234", "http://[::1]:1234", "http://localhost:1234"} {
		c := base
		c.Endpoint = endpoint
		if _, err := NormalizeConfig(c); err != nil {
			t.Fatalf("loopback %s: %v", endpoint, err)
		}
	}
	for _, prefix := range []string{"../other", "valid/../other", "valid//other", "valid\\other", "bad\x00"} {
		c := base
		c.Prefix = prefix
		_, err := NormalizeConfig(c)
		expectCode(t, err, "invalid_config")
	}
	for _, bucket := range []string{"../private", "a/b", "a@b", "a:b", "a?b", "UPPER", ""} {
		c := base
		c.Bucket = bucket
		_, err := NormalizeConfig(c)
		expectCode(t, err, "invalid_config")
	}
}

func TestMergeIndependentEditsUnknownKeysAndConflicts(t *testing.T) {
	base := values("desktop.theme", `"dark"`, "desktop.language", `"en"`)
	local := values("desktop.theme", `"light"`, "desktop.language", `"en"`)
	remote := values("desktop.theme", `"dark"`, "desktop.language", `"zh-CN"`, "future.feature", `{"enabled":true}`)
	merged, err := Merge(local, base, remote)
	if err != nil {
		t.Fatal(err)
	}
	want := values("desktop.theme", `"light"`, "desktop.language", `"zh-CN"`, "future.feature", `{"enabled":true}`)
	if !equalValues(merged, want) {
		t.Fatalf("merged %s", merged)
	}
	merged["desktop.theme"][1] = 'x'
	if string(local["desktop.theme"]) != `"light"` {
		t.Fatal("merge aliases local values")
	}
	remote["desktop.theme"] = json.RawMessage(`"system"`)
	_, err = Merge(local, base, remote)
	expectCode(t, err, "conflict")
	var conflict *Error
	if !errors.As(err, &conflict) || !reflect.DeepEqual(conflict.Keys, []string{"desktop.theme"}) {
		t.Fatalf("conflict: %#v", err)
	}
	if strings.Contains(err.Error(), "light") || strings.Contains(err.Error(), "system") {
		t.Fatal("error leaked preference values")
	}
	if string(local["desktop.theme"]) != `"light"` {
		t.Fatal("conflict mutated input")
	}
}

func TestMergeBootstrapJSONAndDeletion(t *testing.T) {
	merged, err := Merge(values("a", `1`, "b", `true`), nil, values("a", `2`))
	if err != nil || !equalValues(merged, values("a", `2`, "b", `true`)) {
		t.Fatalf("bootstrap %v %v", merged, err)
	}
	merged, err = Merge(values("a", `{"x":1,"y":2}`), values("a", `{"y":2,"x":1}`), values("a", ` { "y": 2, "x": 1 } `))
	if err != nil || len(merged) != 1 {
		t.Fatalf("semantic JSON equality: %v %v", merged, err)
	}
	merged, err = Merge(Values{}, values("a", `true`), values("a", `true`, "unknown", `4`))
	if err != nil || !equalValues(merged, values("unknown", `4`)) {
		t.Fatalf("delete: %v %v", merged, err)
	}
	_, err = Merge(Values{}, values("a", `true`), values("a", `false`))
	expectCode(t, err, "conflict")
	_, err = Merge(values("bad", `not-json`), nil, Values{})
	expectCode(t, err, "invalid_document")
}

func TestEncryptionAuthenticatesContentAndSpace(t *testing.T) {
	passphrase := "correct horse battery staple"
	want := values("desktop.language", `"zh-CN"`, "future.secret", `"private-marker-439219"`)
	encoded, err := seal(want, passphrase, "space-one")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("private-marker")) || bytes.Contains(encoded, []byte("desktop.language")) {
		t.Fatal("plaintext exposed")
	}
	got, err := openEnvelope(encoded, passphrase, "space-one")
	if err != nil || !equalValues(got, want) {
		t.Fatalf("decrypt: %v %v", got, err)
	}
	_, err = openEnvelope(encoded, "wrong passphrase long enough", "space-one")
	expectCode(t, err, "decrypt_failed")
	_, err = openEnvelope(encoded, passphrase, "space-two")
	expectCode(t, err, "decrypt_failed")
	var env envelope
	if err := json.Unmarshal(encoded, &env); err != nil {
		t.Fatal(err)
	}
	env.Ciphertext[0] ^= 1
	tampered, _ := json.Marshal(env)
	_, err = openEnvelope(tampered, passphrase, "space-one")
	expectCode(t, err, "decrypt_failed")
	_, err = openEnvelope([]byte(`{"format":"passpine-sync-v1","n":2147483647}`), passphrase, "space-one")
	expectCode(t, err, "invalid_document")
	_, err = openEnvelope(append(encoded, []byte(` {}`)...), passphrase, "space-one")
	expectCode(t, err, "invalid_document")
}

type fakeBucket struct {
	mu                   sync.Mutex
	body                 []byte
	version              int
	puts                 int
	lists                int
	requests             int
	preconditionFailures int
	beforePut            func()
	paths                []string
}

func (b *fakeBucket) serve(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=test-access/") || r.Header.Get("X-Amz-Date") == "" || r.Header.Get("X-Amz-Content-Sha256") == "" || r.Header.Get("X-Amz-Security-Token") != "temporary-session-token" {
		http.Error(w, "auth failure", http.StatusForbidden)
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.requests++
	b.paths = append(b.paths, r.URL.EscapedPath())
	if r.URL.Query().Get("list-type") == "2" {
		b.lists++
		if r.Method != http.MethodGet || r.URL.Query().Get("max-keys") != "1" {
			http.Error(w, "invalid list", 400)
			return
		}
		_, _ = io.WriteString(w, "<ListBucketResult/>")
		return
	}
	etag := fmt.Sprintf(`"revision-%d"`, b.version)
	switch r.Method {
	case http.MethodGet:
		if b.body == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("ETag", etag)
		_, _ = w.Write(b.body)
	case http.MethodPut:
		b.puts++
		if b.beforePut != nil {
			f := b.beforePut
			b.beforePut = nil
			f()
		}
		if b.preconditionFailures > 0 {
			b.preconditionFailures--
			w.WriteHeader(412)
			return
		}
		etag = fmt.Sprintf(`"revision-%d"`, b.version)
		if b.body == nil && r.Header.Get("If-None-Match") != "*" || b.body != nil && r.Header.Get("If-Match") != etag {
			w.WriteHeader(412)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, MaxDocumentBytes*2+1))
		if err != nil {
			http.Error(w, "read", 400)
			return
		}
		b.body = data
		b.version++
		w.Header().Set("ETag", fmt.Sprintf(`"revision-%d"`, b.version))
	default:
		http.Error(w, "method", 405)
	}
}

func newTestSpace(t *testing.T) (*Engine, Config, Secrets, *fakeBucket) {
	t.Helper()
	bucket := &fakeBucket{}
	server := httptest.NewServer(http.HandlerFunc(bucket.serve))
	t.Cleanup(server.Close)
	config := Config{Endpoint: server.URL, Region: "test-region", Bucket: "test-bucket", Prefix: "private settings/配置", PathStyle: true}
	secrets := Secrets{AccessKeyID: "test-access", SecretAccessKey: "test-secret", SessionToken: "temporary-session-token", Passphrase: "test encryption passphrase"}
	return New(nil), config, secrets, bucket
}

func TestReadDoesNotCreateMissingSpace(t *testing.T) {
	engine, config, secrets, bucket := newTestSpace(t)
	_, err := engine.Read(context.Background(), config, secrets)
	expectCode(t, err, "remote_missing")
	if bucket.puts != 0 || bucket.body != nil {
		t.Fatal("read created a missing remote space")
	}
	initial := values("desktop.theme", `"dark"`)
	if _, err = engine.Sync(context.Background(), config, secrets, initial, nil); err != nil {
		t.Fatal(err)
	}
	puts := bucket.puts
	result, err := engine.Read(context.Background(), config, secrets)
	if err != nil || !equalValues(result.Values, initial) || bucket.puts != puts {
		t.Fatalf("read changed existing remote: %+v %v", result, err)
	}
}

func TestS3RoundTripBootstrapAndIndependentEdits(t *testing.T) {
	engine, config, secrets, bucket := newTestSpace(t)
	first := values("desktop.theme", `"dark"`, "desktop.language", `"en"`)
	a, err := engine.Sync(context.Background(), config, secrets, first, nil)
	if err != nil || !a.Uploaded || a.Revision == "" {
		t.Fatalf("initial upload %+v %v", a, err)
	}
	b, err := engine.Sync(context.Background(), config, secrets, values("desktop.theme", `"system"`), nil)
	if err != nil || b.Uploaded || !equalValues(a.Values, b.Values) {
		t.Fatalf("device import %+v %v", b, err)
	}
	localA := cloneValues(a.Values)
	localA["desktop.theme"] = json.RawMessage(`"light"`)
	a, err = engine.Sync(context.Background(), config, secrets, localA, a.Values)
	if err != nil {
		t.Fatal(err)
	}
	localB := cloneValues(b.Values)
	localB["desktop.language"] = json.RawMessage(`"zh-CN"`)
	b, err = engine.Sync(context.Background(), config, secrets, localB, b.Values)
	if err != nil || !equalValues(b.Values, values("desktop.theme", `"light"`, "desktop.language", `"zh-CN"`)) {
		t.Fatalf("concurrent edit merge %v %v", b, err)
	}
	if err := engine.Test(context.Background(), config, secrets); err != nil {
		t.Fatal(err)
	}
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	if bucket.puts != 3 || bucket.lists != 1 {
		t.Fatalf("puts %d lists %d", bucket.puts, bucket.lists)
	}
	for _, p := range bucket.paths {
		if strings.Contains(p, "settings.v1.enc") && p != "/test-bucket/private%20settings/%E9%85%8D%E7%BD%AE/settings.v1.enc" {
			t.Fatalf("bad escaped path: %s", p)
		}
	}
}

func TestSyncFailuresNeverOverwriteRemoteOrInputs(t *testing.T) {
	engine, config, secrets, bucket := newTestSpace(t)
	base := values("theme", `"dark"`)
	_, err := engine.Sync(context.Background(), config, secrets, base, nil)
	if err != nil {
		t.Fatal(err)
	}
	local := values("theme", `"light"`)
	_, err = engine.Sync(context.Background(), config, secrets, values("theme", `"system"`), base)
	if err != nil {
		t.Fatal(err)
	}
	bucket.mu.Lock()
	before := append([]byte(nil), bucket.body...)
	puts := bucket.puts
	bucket.mu.Unlock()
	_, err = engine.Sync(context.Background(), config, secrets, local, base)
	expectCode(t, err, "conflict")
	wrong := secrets
	wrong.Passphrase = "a wrong encryption passphrase"
	_, err = engine.Sync(context.Background(), config, wrong, local, base)
	expectCode(t, err, "decrypt_failed")
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	if bucket.puts != puts || !bytes.Equal(before, bucket.body) || string(local["theme"]) != `"light"` {
		t.Fatal("failed sync changed state")
	}
}

func TestConditionalRetryMergesRacingWriter(t *testing.T) {
	engine, config, secrets, bucket := newTestSpace(t)
	base := values("theme", `"dark"`, "language", `"en"`)
	_, err := engine.Sync(context.Background(), config, secrets, base, nil)
	if err != nil {
		t.Fatal(err)
	}
	identity := config.Endpoint + "\n" + config.Bucket + "\n" + config.Prefix + "/" + objectName
	racing, err := seal(values("theme", `"dark"`, "language", `"zh-CN"`), secrets.Passphrase, identity)
	if err != nil {
		t.Fatal(err)
	}
	bucket.mu.Lock()
	bucket.beforePut = func() { bucket.body = racing; bucket.version++ }
	bucket.mu.Unlock()
	result, err := engine.Sync(context.Background(), config, secrets, values("theme", `"light"`, "language", `"en"`), base)
	if err != nil || !equalValues(result.Values, values("theme", `"light"`, "language", `"zh-CN"`)) {
		t.Fatalf("racing writer lost: %+v %v", result, err)
	}
	bucket.mu.Lock()
	bucket.preconditionFailures = 20
	bucket.mu.Unlock()
	_, err = engine.Sync(context.Background(), config, secrets, values("theme", `"system"`, "language", `"zh-CN"`), result.Values)
	expectCode(t, err, "concurrent_update")
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	if bucket.preconditionFailures != 17 {
		t.Fatal("unbounded conditional retries")
	}
}

func TestTransportRejectsRedirectsAndSanitizesErrors(t *testing.T) {
	hits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; w.WriteHeader(200) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL+"?secret=leaked", 307) }))
	defer redirect.Close()
	config := Config{Endpoint: redirect.URL, Bucket: "test", PathStyle: true}
	secret := Secrets{AccessKeyID: "test-access", SecretAccessKey: "extremely-secret-key", Passphrase: "long enough encryption phrase"}
	err := New(nil).Test(context.Background(), config, secret)
	expectCode(t, err, "remote_error")
	if hits != 0 || strings.Contains(err.Error(), target.URL) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("redirect leaked credentials: %v %d", err, hits)
	}
	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = io.WriteString(w, "extremely-secret-key /private/account")
	}))
	defer unauthorized.Close()
	config.Endpoint = unauthorized.URL
	err = New(nil).Test(context.Background(), config, secret)
	expectCode(t, err, "unauthorized")
	if strings.Contains(err.Error(), "extremely-secret-key") || strings.Contains(err.Error(), "private") {
		t.Fatal("remote error exposed body")
	}
}

func TestOwnerValidationRunsBeforeRemoteWrite(t *testing.T) {
	engine, config, secrets, bucket := newTestSpace(t)
	_, err := engine.Sync(context.Background(), config, secrets, values("desktop.theme", `"invalid-owner-value"`), nil)
	if err != nil {
		t.Fatal(err)
	}
	bucket.mu.Lock()
	before := append([]byte(nil), bucket.body...)
	puts := bucket.puts
	bucket.mu.Unlock()
	validated := false
	_, err = engine.SyncValidated(context.Background(), config, secrets, values("desktop.language", `"en"`), nil, func(merged Values) error {
		validated = true
		if string(merged["desktop.theme"]) == `"invalid-owner-value"` {
			return &Error{Code: "invalid_document"}
		}
		return nil
	})
	expectCode(t, err, "invalid_document")
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	if !validated || bucket.puts != puts || !bytes.Equal(bucket.body, before) {
		t.Fatal("invalid owner data was written before validation")
	}
}

func TestSignatureAndAddressing(t *testing.T) {
	config := Config{Endpoint: "https://s3.example.test:8443", Bucket: "my-bucket", Region: "us-east-1", Prefix: "a +/雪", PathStyle: false}
	u := objectURL(config)
	if u.Host != "my-bucket.s3.example.test:8443" || u.EscapedPath() != "/a%20%2B/%E9%9B%AA/settings.v1.enc" {
		t.Fatalf("virtual addressing %s", u)
	}
	u.RawQuery = "list-type=2&max-keys=1&prefix=a%20%2B%2F%E9%9B%AA%2F"
	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	secret := Secrets{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "secret", SessionToken: "session"}
	sign(req, nil, config, secret, time.Date(2026, 10, 26, 12, 0, 0, 0, time.FixedZone("offset", 3600)))
	auth := req.Header.Get("Authorization")
	// Oracle: aws-sdk-go-v2 v1.36.3 SignHTTP with S3 URI path escaping
	// disabled and the same request, credentials, and timestamp. This fixture
	// checks the complete signature independently of our HTTP test service.
	wantAuth := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20261026/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-content-sha256;x-amz-date;x-amz-security-token, Signature=b25a1825e189abbfb5af37b1d56f2ecf27668f3d28f5ee70625013a1707430b4"
	if auth != wantAuth {
		t.Fatalf("signature differs from official AWS signer: %s", auth)
	}
	if !strings.Contains(auth, "Credential=AKIDEXAMPLE/20261026/us-east-1/s3/aws4_request") || !strings.Contains(auth, "SignedHeaders=host;x-amz-content-sha256;x-amz-date;x-amz-security-token") || req.Header.Get("X-Amz-Date") != "20261026T110000Z" {
		t.Fatalf("bad signature headers %s", auth)
	}
	changed, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	changed.URL.RawQuery = strings.ReplaceAll(u.RawQuery, "%20", "+")
	sign(changed, nil, config, secret, time.Date(2026, 10, 26, 11, 0, 0, 0, time.UTC))
	if auth == changed.Header.Get("Authorization") {
		t.Fatal("signature did not authenticate exact query encoding")
	}
	parsed, err := url.Parse(u.String())
	if err != nil || parsed.Path != "/a +/雪/settings.v1.enc" {
		t.Fatal("escaped path lost original key")
	}
}
