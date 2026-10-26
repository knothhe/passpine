package r2config

import (
	"testing"
)

func TestPublisherIdentityAndEnvironmentOverrides(t *testing.T) {
	t.Setenv("PASSPINE_CF_OAUTH_CLIENT_ID", "publisher-test")
	t.Setenv("PASSPINE_CF_OAUTH_REDIRECT_URL", "http://127.0.0.1:47839/oauth/callback")
	if c := DefaultConfig(); c.ClientID != "publisher-test" || !c.Available() {
		t.Fatalf("public overrides not loaded: %+v", c)
	}
	t.Setenv("PASSPINE_CF_OAUTH_CLIENT_ID", "")
	if DefaultConfig().Available() {
		t.Fatal("explicit empty client ID enabled OAuth")
	}
}

func TestInvalidPublisherCallbackDoesNotEnableOAuth(t *testing.T) {
	for _, callback := range []string{"http://example.com/callback", "https://127.0.0.1:47839/oauth/callback", "http://127.0.0.1:0/oauth/callback", "http://127.0.0.1:47839/oauth/callback?extra=value"} {
		cfg := Config{ClientID: "publisher-test", RedirectURL: callback}
		if cfg.Available() {
			t.Fatalf("invalid callback accepted %q", callback)
		}
	}
}
