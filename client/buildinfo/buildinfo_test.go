package buildinfo

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testChannel(t *testing.T, channel string) {
	t.Helper()
	prior := Channel
	Channel = channel
	t.Cleanup(func() { Channel = prior })
}

func configFixture(t *testing.T) string {
	t.Helper()
	base := filepath.Join(t.TempDir(), "config")
	prior := userConfigDir
	userConfigDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { userConfigDir = prior })
	t.Setenv("PASSPINE_HOME", "")
	t.Setenv("PASSPINE_DEV_HOME", "")
	return base
}

func TestChannelsUseDifferentSystemAndBrowserIdentities(t *testing.T) {
	for _, channel := range []string{"dev", "release", "invalid"} {
		t.Run(channel, func(t *testing.T) {
			testChannel(t, channel)
			release := channel == "release"
			if IsRelease() != release {
				t.Fatal("unknown channels must stay isolated as development builds")
			}
			if release {
				if Name() != "Passpine" || BundleID() != "com.knothhe.passpine" || HostName() != BundleID() || FirefoxID() != "passpine@knothhe.com" || ChromiumID() != ReleaseChromiumID || HomeEnvironment() != "PASSPINE_HOME" || DefaultGlobalShortcut() != "Ctrl+Alt+Space" || Version("1.2.3") != "1.2.3" || ChannelName() != "release" {
					t.Fatal("release identity changed")
				}
			} else if Name() != "Passpine Dev" || BundleID() != "com.knothhe.passpine.dev" || HostName() != BundleID() || FirefoxID() != "passpine-dev@knothhe.com" || ChromiumID() != DevChromiumID || ChromiumID() == ReleaseChromiumID || HomeEnvironment() != "PASSPINE_DEV_HOME" || DefaultGlobalShortcut() != "Ctrl+Alt+Shift+Space" || Version("1.2.3") != "1.2.3-dev" || ChannelName() != "dev" {
				t.Fatal("development identity overlaps the release")
			}
		})
	}
}

func TestDevelopmentIgnoresReleaseHomeAndUsesOwnPreferencesRoot(t *testing.T) {
	testChannel(t, "dev")
	base := configFixture(t)
	release := filepath.Join(t.TempDir(), "release-data")
	t.Setenv("PASSPINE_HOME", release)
	got, err := DataDir()
	if err != nil || got != filepath.Join(base, "Passpine Dev") {
		t.Fatalf("development loaded release home: %q, %v", got, err)
	}
	dev := filepath.Join(t.TempDir(), "dev-data")
	t.Setenv("PASSPINE_DEV_HOME", dev)
	got, err = DataDir()
	if err != nil || got != dev {
		t.Fatalf("development override ignored: %q, %v", got, err)
	}
	if _, err := os.Stat(release); !os.IsNotExist(err) {
		t.Fatal("resolving dev home created the release directory")
	}
}

func TestDevelopmentRejectsReleaseDataAndSymlinkAliasesBeforeWriting(t *testing.T) {
	testChannel(t, "dev")
	base := configFixture(t)
	stable := filepath.Join(base, "Passpine")
	if err := os.MkdirAll(stable, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(stable, "sentinel")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{stable, filepath.Join(stable, "child"), base} {
		if err := ValidateDataDir(directory); err == nil {
			t.Fatalf("accepted release data overlap: %q", directory)
		}
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(stable, alias); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
	} else if err := ValidateDataDir(filepath.Join(alias, "new-directory")); err == nil {
		t.Fatal("accepted a symlink alias of release data")
	}
	custom := filepath.Join(t.TempDir(), "custom-release")
	t.Setenv("PASSPINE_HOME", custom)
	if err := ValidateDataDir(custom); err == nil {
		t.Fatal("accepted the explicitly configured release data root")
	}
	if err := ValidateDataDir(filepath.Join(base, "Passpine Dev")); err != nil {
		t.Fatalf("rejected independent development data: %v", err)
	}
	bytes, err := os.ReadFile(sentinel)
	if err != nil || string(bytes) != "unchanged" {
		t.Fatal("validation modified release data")
	}
}

func TestReleaseKeepsOriginalHomeOverride(t *testing.T) {
	testChannel(t, "release")
	base := configFixture(t)
	got, err := DataDir()
	if err != nil || got != filepath.Join(base, "Passpine") {
		t.Fatalf("release default changed: %q, %v", got, err)
	}
	custom := filepath.Join(t.TempDir(), "release")
	t.Setenv("PASSPINE_HOME", custom)
	t.Setenv("PASSPINE_DEV_HOME", filepath.Join(t.TempDir(), "dev"))
	got, err = DataDir()
	if err != nil || got != custom {
		t.Fatalf("release override changed: %q, %v", got, err)
	}
}
