package cloudsync

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const objectName = "settings.v1.enc"

type Engine struct {
	client *http.Client
	now    func() time.Time
}

func New(client *http.Client) *Engine {
	copyClient := http.Client{Timeout: 20 * time.Second}
	if client != nil {
		copyClient = *client
	}
	if copyClient.Timeout <= 0 || copyClient.Timeout > 20*time.Second {
		copyClient.Timeout = 20 * time.Second
	}
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Engine{client: &copyClient, now: time.Now}
}

func awsEscape(value string, preserveSlash bool) string {
	const digits = "0123456789ABCDEF"
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-_.~", rune(c)) || preserveSlash && c == '/' {
			out.WriteByte(c)
		} else {
			out.WriteByte('%')
			out.WriteByte(digits[c>>4])
			out.WriteByte(digits[c&15])
		}
	}
	return out.String()
}

func objectURL(config Config) *url.URL {
	u, _ := url.Parse(config.Endpoint)
	key := config.Prefix + "/" + objectName
	if config.PathStyle {
		u.Path = "/" + config.Bucket + "/" + key
	} else {
		u.Host = config.Bucket + "." + u.Host
		u.Path = "/" + key
	}
	u.RawPath = awsEscape(u.Path, true)
	return u
}

func hmacSum(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}

func sign(req *http.Request, body []byte, config Config, secrets Secrets, now time.Time) {
	now = now.UTC()
	timestamp, date := now.Format("20060102T150405Z"), now.Format("20060102")
	payload := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(payload[:])
	req.Header.Set("X-Amz-Date", timestamp)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if secrets.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", secrets.SessionToken)
	}
	headers := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		name := strings.ToLower(k)
		if strings.HasPrefix(name, "x-amz-") || name == "if-match" || name == "if-none-match" {
			headers[name] = strings.Join(strings.Fields(strings.Join(v, ",")), " ")
		}
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name + ":" + headers[name] + "\n")
	}
	signedHeaders := strings.Join(names, ";")
	// Queries here are generated internally in canonical order. Never use
	// path.Clean, which would change S3 object identities.
	canonical := req.Method + "\n" + req.URL.EscapedPath() + "\n" + req.URL.RawQuery + "\n" + canonicalHeaders.String() + "\n" + signedHeaders + "\n" + payloadHash
	canonicalHash := sha256.Sum256([]byte(canonical))
	scope := date + "/" + config.Region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + timestamp + "\n" + scope + "\n" + hex.EncodeToString(canonicalHash[:])
	dateKey := hmacSum([]byte("AWS4"+secrets.SecretAccessKey), date)
	regionKey := hmacSum(dateKey, config.Region)
	serviceKey := hmacSum(regionKey, "s3")
	signingKey := hmacSum(serviceKey, "aws4_request")
	signature := hmacSum(signingKey, toSign)
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+secrets.AccessKeyID+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+hex.EncodeToString(signature))
	clear(dateKey)
	clear(regionKey)
	clear(serviceKey)
	clear(signingKey)
}

func (e *Engine) request(ctx context.Context, method string, config Config, secrets Secrets, body []byte, revision string, absent bool) (*http.Response, error) {
	u := objectURL(config)
	return e.requestURL(ctx, method, u, config, secrets, body, revision, absent)
}

func (e *Engine) requestURL(ctx context.Context, method string, u *url.URL, config Config, secrets Secrets, body []byte, revision string, absent bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, &Error{Code: "invalid_config"}
	}
	req.Header.Set("Cache-Control", "no-store")
	if method == http.MethodPut {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	if revision != "" {
		req.Header.Set("If-Match", revision)
	}
	if absent {
		req.Header.Set("If-None-Match", "*")
	}
	sign(req, body, config, secrets, e.now())
	res, err := e.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &Error{Code: "network"}
	}
	return res, nil
}

func responseError(status int) error {
	code := "remote_error"
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		code = "unauthorized"
	}
	return &Error{Code: code, Status: status}
}

func closeResponse(res *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	_ = res.Body.Close()
}

func validRevision(revision string) bool {
	// If-Match requires a strong entity tag. A wildcard or a weak tag cannot
	// protect this read-modify-write cycle against lost updates.
	if len(revision) < 2 || len(revision) > 1024 || revision[0] != '"' || revision[len(revision)-1] != '"' {
		return false
	}
	for _, c := range []byte(revision[1 : len(revision)-1]) {
		if c < 0x21 || c == '"' || c == 0x7f {
			return false
		}
	}
	return true
}

func (e *Engine) read(ctx context.Context, config Config, secrets Secrets) (encoded []byte, revision string, exists bool, err error) {
	res, err := e.request(ctx, http.MethodGet, config, secrets, nil, "", false)
	if err != nil {
		return nil, "", false, err
	}
	defer closeResponse(res)
	if res.StatusCode == http.StatusNotFound {
		return nil, "", false, nil
	}
	if res.StatusCode != http.StatusOK {
		return nil, "", false, responseError(res.StatusCode)
	}
	encoded, err = io.ReadAll(io.LimitReader(res.Body, MaxDocumentBytes*2+1))
	if err != nil {
		return nil, "", false, &Error{Code: "network"}
	}
	if len(encoded) > MaxDocumentBytes*2 {
		return nil, "", false, &Error{Code: "invalid_document"}
	}
	revision = res.Header.Get("ETag")
	if !validRevision(revision) {
		return nil, "", false, &Error{Code: "invalid_document"}
	}
	return encoded, revision, true, nil
}

// Test verifies bucket access with a read-only list request. HEAD on a missing
// object cannot distinguish a missing bucket from an empty synchronization
// space, so a bounded ListObjectsV2 query is used instead; no object is changed.
func (e *Engine) Test(ctx context.Context, config Config, secrets Secrets) error {
	config, err := NormalizeConfig(config)
	if err != nil {
		return err
	}
	if err := validateCredentials(secrets); err != nil {
		return err
	}
	u := objectURL(config)
	if config.PathStyle {
		u.Path = "/" + config.Bucket
	} else {
		u.Path = "/"
	}
	u.RawPath = awsEscape(u.Path, true)
	query := url.Values{"list-type": {"2"}, "max-keys": {"1"}, "prefix": {config.Prefix + "/"}}
	u.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	res, err := e.requestURL(ctx, http.MethodGet, u, config, secrets, nil, "", false)
	if err != nil {
		return err
	}
	defer closeResponse(res)
	if res.StatusCode != http.StatusOK {
		return responseError(res.StatusCode)
	}
	return nil
}
