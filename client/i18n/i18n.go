// Package i18n contains the client's supported locales and UI message catalog.
// Only explicit UI message keys are translated; vault contents are never input.
package i18n

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	English           = "en"
	SimplifiedChinese = "zh-CN"
	System            = "system"
)

// Resolve selects one of the two supported locales. Unsupported languages and
// Traditional Chinese use English rather than an unrelated Chinese variant.
func Resolve(preference, system string) string {
	if preference == "" || preference == System {
		preference = system
	}
	locale := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(preference), "_", "-"))
	locale = strings.Split(strings.Split(locale, ".")[0], "@")[0]
	parts := strings.Split(locale, "-")
	if parts[0] == "zh" {
		// An explicit script is more specific than the region.
		for _, part := range parts[1:] {
			if part == "hans" {
				return SimplifiedChinese
			}
			if len(part) == 4 {
				return English
			}
		}
		for _, part := range parts[1:] {
			if part == "tw" || part == "hk" || part == "mo" {
				return English
			}
		}
		return SimplifiedChinese
	}
	return English
}

var systemLocale = sync.OnceValue(func() string {
	return detectSystemLocale(runtime.GOOS, os.Getenv, func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleLanguages").Output()
		return string(output), err
	})
})

// SystemLocale reads macOS's preferred application language even for GUI
// launches that do not inherit LANG. Other platforms use locale environment
// variables; missing or unsupported locales resolve to English.
func SystemLocale() string { return systemLocale() }

func detectSystemLocale(platform string, getenv func(string) string, appleLanguages func() (string, error)) string {
	if platform == "darwin" {
		if value, err := appleLanguages(); err == nil {
			if first := firstAppleLanguage(value); first != "" {
				return Resolve(first, "")
			}
		}
	}
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE", "LANG"} {
		if value := getenv(key); value != "" {
			return Resolve(strings.Split(value, ":")[0], "")
		}
	}
	return English
}

func firstAppleLanguage(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSpace(strings.TrimPrefix(value, "("))
	value = strings.TrimSpace(strings.TrimSuffix(value, ")"))
	value = strings.Split(value, ",")[0]
	return strings.Trim(strings.TrimSpace(value), "\"")
}

// Text looks up a complete message key before formatting its arguments. It
// never performs substring replacement, so paths, names, and secrets in format
// arguments remain byte-for-byte intact. Unknown keys remain readable.
func Text(locale, source string, args ...any) string {
	message := source
	if Resolve(locale, "") == English {
		if translated, ok := english[source]; ok {
			message = translated
		}
	}
	if len(args) != 0 {
		return fmt.Sprintf(message, args...)
	}
	return message
}

func HasEnglish(source string) bool {
	_, ok := english[source]
	return ok
}
