// Package cloudsync synchronizes small, encrypted, namespaced application data.
// It owns neither local preferences nor credentials; callers commit Result only
// after Sync succeeds and retain that snapshot as the next merge baseline.
package cloudsync

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"unicode/utf8"
)

const MaxDocumentBytes = 1 << 20

type Config struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	PathStyle bool   `json:"pathStyle"`
}

// Secrets must only be persisted using the application's system secret store.
type Secrets struct {
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	SessionToken    string `json:"sessionToken,omitempty"`
	Passphrase      string `json:"passphrase"`
}

type Values map[string]json.RawMessage

type Result struct {
	Values   Values
	Uploaded bool
	Revision string
}

// Error contains only controlled diagnostic data, never service responses,
// endpoint URLs, credentials, decrypted values, or underlying transport errors.
type Error struct {
	Code   string
	Status int
	Keys   []string
}

func (e *Error) Error() string { return "cloudsync: " + e.Code }

func ErrorCode(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return "network"
}

// NormalizeConfig rejects credentials embedded in URLs and non-TLS endpoints
// except literal loopback hosts, which are useful for isolated local services.
func NormalizeConfig(c Config) (Config, error) {
	c.Endpoint = strings.TrimRight(strings.TrimSpace(c.Endpoint), "/")
	c.Region = strings.TrimSpace(c.Region)
	c.Bucket = strings.TrimSpace(c.Bucket)
	c.Prefix = strings.Trim(strings.TrimSpace(c.Prefix), "/")
	if c.Region == "" {
		c.Region = "auto"
	}
	if c.Prefix == "" {
		c.Prefix = "passpine"
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.User != nil || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
		return Config{}, &Error{Code: "invalid_config"}
	}
	ip := net.ParseIP(u.Hostname())
	loopback := strings.EqualFold(u.Hostname(), "localhost") || ip != nil && ip.IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return Config{}, &Error{Code: "invalid_config"}
	}
	if c.Bucket == "" || len(c.Bucket) > 63 || strings.ContainsAny(c.Bucket, "/\\?#@:\x00\r\n\t ") || c.Bucket == "." || c.Bucket == ".." {
		return Config{}, &Error{Code: "invalid_config"}
	}
	for _, r := range c.Bucket {
		if r > 127 || !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-') {
			return Config{}, &Error{Code: "invalid_config"}
		}
	}
	if len(c.Region) > 64 || strings.ContainsAny(c.Region, "/\\\x00\r\n\t ") || len(c.Prefix) > 512 || strings.ContainsAny(c.Prefix, "\\\x00\r\n") {
		return Config{}, &Error{Code: "invalid_config"}
	}
	for _, part := range strings.Split(c.Prefix, "/") {
		if part == "" || part == "." || part == ".." {
			return Config{}, &Error{Code: "invalid_config"}
		}
	}
	if !c.PathStyle && (net.ParseIP(u.Hostname()) != nil || strings.EqualFold(u.Hostname(), "localhost")) {
		return Config{}, &Error{Code: "invalid_config"}
	}
	return c, nil
}

func validateCredentials(s Secrets) error {
	if s.AccessKeyID == "" || s.SecretAccessKey == "" || len(s.AccessKeyID) > 1024 || len(s.SecretAccessKey) > 4096 || len(s.SessionToken) > 16384 || strings.ContainsAny(s.AccessKeyID+s.SecretAccessKey+s.SessionToken, "\x00\r\n") {
		return &Error{Code: "invalid_credentials"}
	}
	return nil
}

func ValidateSecrets(s Secrets) error {
	if err := validateCredentials(s); err != nil {
		return err
	}
	if utf8.RuneCountInString(s.Passphrase) < 12 || len(s.Passphrase) > 4096 {
		return &Error{Code: "invalid_passphrase"}
	}
	return nil
}
