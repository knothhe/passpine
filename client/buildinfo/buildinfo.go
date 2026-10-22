// Package buildinfo defines the identity shared by the desktop and native host.
// Source builds are development builds unless release CI explicitly sets Channel.
package buildinfo

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var Channel = "dev"
var userConfigDir = os.UserConfigDir

const ReleaseChromiumID = "obnlebeedcbbchadakacilopfdaifgjh"
const DevChromiumID = "dcfgpoaeamdmkgoepgkdllccakegldfc"

func IsRelease() bool { return Channel == "release" }
func ChannelName() string {
	if IsRelease() {
		return "release"
	}
	return "dev"
}
func Name() string {
	if IsRelease() {
		return "Passpine"
	}
	return "Passpine Dev"
}
func BundleID() string {
	if IsRelease() {
		return "com.knothhe.passpine"
	}
	return "com.knothhe.passpine.dev"
}
func HostName() string { return BundleID() }
func FirefoxID() string {
	if IsRelease() {
		return "passpine@knothhe.com"
	}
	return "passpine-dev@knothhe.com"
}
func ChromiumID() string {
	if IsRelease() {
		return ReleaseChromiumID
	}
	return DevChromiumID
}
func HomeEnvironment() string {
	if IsRelease() {
		return "PASSPINE_HOME"
	}
	return "PASSPINE_DEV_HOME"
}
func DefaultGlobalShortcut() string {
	if IsRelease() {
		return "Ctrl+Alt+Space"
	}
	return "Ctrl+Alt+Shift+Space"
}
func Version(version string) string {
	if IsRelease() || version == "development" || strings.Contains(version, "-dev") {
		return version
	}
	return version + "-dev"
}

func DataDir() (string, error) {
	var directory string
	if value := os.Getenv(HomeEnvironment()); value != "" {
		directory = value
	} else {
		base, err := userConfigDir()
		if err != nil {
			return "", err
		}
		directory = filepath.Join(base, Name())
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	if err := ValidateDataDir(directory); err != nil {
		return "", err
	}
	return directory, nil
}

// ValidateDataDir prevents a development process from loading release settings,
// keys, endpoints or update state through either a direct path or a symlink.
// It checks paths without opening the release directory or its contents.
func ValidateDataDir(directory string) error {
	if IsRelease() {
		return nil
	}
	base, err := userConfigDir()
	if err != nil {
		return err
	}
	protected := []string{filepath.Join(base, "Passpine")}
	if override := os.Getenv("PASSPINE_HOME"); override != "" {
		protected = append(protected, override)
	}
	candidate, err := canonical(directory)
	if err != nil {
		return err
	}
	for _, root := range protected {
		root, err = canonical(root)
		if err != nil {
			return err
		}
		if contains(root, candidate) || contains(candidate, root) {
			return errors.New("开发版本不能使用正式版本的数据目录。")
		}
	}
	return nil
}

func canonical(filename string) (string, error) {
	filename, err := filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filename)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(filename)
	if parent == filename {
		return "", err
	}
	parent, err = canonical(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(filename)), nil
}
func contains(parent, child string) bool {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		parent, child = strings.ToLower(parent), strings.ToLower(child)
	}
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
