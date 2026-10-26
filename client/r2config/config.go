// Package r2config contains Passpine's public Cloudflare application identity.
// OAuth, the callback listener, credentials and lifecycle belong to r2connect.
package r2config

import (
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/knothhe/r2connect/loopback"
)

var ErrUnavailable = errors.New("oauth_unavailable")
var ErrConfig = errors.New("oauth_invalid_config")

//go:embed publisher.json
var publisherConfig []byte

type Config struct {
	ClientID    string `json:"clientId"`
	RedirectURL string `json:"redirectUrl"`
}

func DefaultConfig() Config {
	var cfg Config
	_ = json.Unmarshal(publisherConfig, &cfg)
	if value, ok := os.LookupEnv("PASSPINE_CF_OAUTH_CLIENT_ID"); ok {
		cfg.ClientID = value
	}
	if value, ok := os.LookupEnv("PASSPINE_CF_OAUTH_REDIRECT_URL"); ok {
		cfg.RedirectURL = value
	}
	return cfg
}

func (cfg Config) Available() bool { return cfg.Validate() == nil }
func (cfg Config) Validate() error {
	if cfg.ClientID == "" {
		return ErrUnavailable
	}
	if len(cfg.ClientID) > 256 || strings.ContainsAny(cfg.ClientID, " \t\r\n\x00") {
		return ErrConfig
	}
	if _, err := loopback.ParseRedirect(cfg.RedirectURL); err != nil {
		return ErrConfig
	}
	return nil
}
