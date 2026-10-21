package nativehost

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func platformRegistrationTargets(home string) []registrationTarget {
	targets := registrationTargets(home, "windows")
	if local, err := os.UserCacheDir(); err == nil {
		for i := range targets {
			targets[i].directory = filepath.Join(local, "Passpine", "NativeMessagingHosts", targets[i].id)
		}
	}
	return targets
}

func hostRegistryPath(id string) string {
	prefixes := map[string]string{"chrome": `Software\Google\Chrome`, "edge": `Software\Microsoft\Edge`, "brave": `Software\BraveSoftware\Brave-Browser`, "firefox": `Software\Mozilla`}
	return prefixes[id] + `\NativeMessagingHosts\` + Name
}

func registerPlatformHost(target registrationTarget) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, hostRegistryPath(target.id), registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue("", filepath.Join(target.directory, Name+".json"))
}

func platformRegistrationMatches(item BrowserRegistration) bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, hostRegistryPath(item.ID), registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetStringValue("")
	return err == nil && strings.EqualFold(filepath.Clean(value), filepath.Clean(item.ManifestPath))
}

func browserInstalled(target registrationTarget) bool {
	names := map[string]string{"chrome": "chrome.exe", "edge": "msedge.exe", "brave": "brave.exe", "firefox": "firefox.exe"}
	name, ok := names[target.id]
	if !ok {
		return false
	}
	for _, hive := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			key, err := registry.OpenKey(hive, `Software\Microsoft\Windows\CurrentVersion\App Paths\`+name, registry.QUERY_VALUE|view)
			if err != nil {
				continue
			}
			value, _, err := key.GetStringValue("")
			key.Close()
			if err == nil {
				if info, err := os.Stat(strings.Trim(value, `"`)); err == nil && info.Mode().IsRegular() {
					return true
				}
			}
		}
	}
	paths := map[string]string{"chrome": `Google\Chrome\Application\chrome.exe`, "edge": `Microsoft\Edge\Application\msedge.exe`, "brave": `BraveSoftware\Brave-Browser\Application\brave.exe`, "firefox": `Mozilla Firefox\firefox.exe`}
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
		if base != "" {
			if info, err := os.Stat(filepath.Join(base, paths[target.id])); err == nil && info.Mode().IsRegular() {
				return true
			}
		}
	}
	return false
}
