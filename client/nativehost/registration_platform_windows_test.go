package nativehost

import (
	"github.com/knothhe/passpine/client/buildinfo"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsRegistrationUsesRedirectedLocalAppData(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	targets := platformRegistrationTargets(t.TempDir())
	if len(targets) != 4 {
		t.Fatalf("expected four Windows browser targets, got %d", len(targets))
	}
	for _, target := range targets {
		want := filepath.Join(local, buildinfo.Name(), "NativeMessagingHosts", target.id)
		if target.directory != want {
			t.Errorf("%s manifest directory = %q, want %q", target.id, target.directory, want)
		}
	}
}

func TestWindowsManualBrowserRegistryAndBuiltInOverride(t *testing.T) {
	_, _, c := manualBrowserFixture(t)
	c.RegistryPrefix = `Software\Google\Chrome`
	c, err := NormalizeBrowserConfiguration(c)
	if err != nil {
		t.Fatal(err)
	}
	target := configurationTarget(c)
	want := filepath.Join(os.Getenv("LOCALAPPDATA"), buildinfo.Name(), "NativeMessagingHosts", "chrome")
	if target.directory != want || targetRegistryPath(target) != hostRegistryPath("chrome") {
		t.Fatalf("manual override strands built-in registration: %+v", target)
	}
	c.Kind, c.RegistryPrefix = "firefox", `Software\Mozilla`
	c, err = NormalizeBrowserConfiguration(c)
	if err != nil {
		t.Fatal(err)
	}
	if targetRegistryPath(configurationTarget(c)) != hostRegistryPath("firefox") {
		t.Fatal("Firefox's two-component registry prefix was rejected")
	}
	c.Kind = "chromium"
	c.RegistryPrefix = `Software\Test Vendor\Custom Browser`
	c, err = NormalizeBrowserConfiguration(c)
	if err != nil {
		t.Fatal(err)
	}
	if got := targetRegistryPath(configurationTarget(c)); got != c.RegistryPrefix+`\NativeMessagingHosts\`+Name {
		t.Fatalf("wrong custom registry path: %s", got)
	}
}
