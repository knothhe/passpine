package i18n

import (
	"errors"
	"testing"
)

func TestLocaleSelection(t *testing.T) {
	for _, test := range []struct{ setting, system, want string }{
		{System, "zh-Hans-CN", SimplifiedChinese},
		{System, "zh_CN.UTF-8", SimplifiedChinese},
		{System, "zh-SG", SimplifiedChinese},
		{System, "zh-Hans-TW", SimplifiedChinese},
		{System, "zh-Latn", English},
		{System, "en-GB", English},
		{System, "fr-FR", English},
		{System, "zh-Hant-TW", English},
		{System, "zh-TW", English},
		{"", "", English},
		{English, "zh-CN", English},
		{SimplifiedChinese, "en-US", SimplifiedChinese},
	} {
		if got := Resolve(test.setting, test.system); got != test.want {
			t.Errorf("Resolve(%q, %q) = %q, want %q", test.setting, test.system, got, test.want)
		}
	}
}

func TestMacOSPreferredLanguageWinsOverShellLocale(t *testing.T) {
	env := func(string) string { return "en_US.UTF-8" }
	got := detectSystemLocale("darwin", env, func() (string, error) { return "(\n    \"zh-Hans-CN\",\n    \"en-US\"\n)", nil })
	if got != SimplifiedChinese {
		t.Fatalf("GUI preference lost to shell locale: %q", got)
	}
	got = detectSystemLocale("darwin", env, func() (string, error) { return "(\n    \"fr-FR\",\n    \"zh-Hans-CN\"\n)", nil })
	if got != English {
		t.Fatalf("unsupported system language did not fall back to English: %q", got)
	}
}

func TestLocaleEnvironmentFallback(t *testing.T) {
	env := map[string]string{"LANG": "en_US.UTF-8", "LC_MESSAGES": "zh_CN.UTF-8"}
	failedAppleLanguages := func() (string, error) { return "", errors.New("unavailable") }
	if got := detectSystemLocale("darwin", func(k string) string { return env[k] }, failedAppleLanguages); got != SimplifiedChinese {
		t.Fatalf("locale fallback = %q", got)
	}
	env["LC_ALL"] = "C"
	if got := detectSystemLocale("linux", func(k string) string { return env[k] }, failedAppleLanguages); got != English {
		t.Fatalf("LC_ALL did not take priority: %q", got)
	}
}

func TestMessagesTranslateBeforeFormattingAndPreserveData(t *testing.T) {
	if got := Text(English, "%d 个条目", 17); got != "17 entries" {
		t.Fatalf("English count = %q", got)
	}
	if got := Text(SimplifiedChinese, "%d 个条目", 17); got != "17 个条目" {
		t.Fatalf("Chinese count = %q", got)
	}
	value := "密码/%s/设置"
	if got := Text(English, "账号 %s", value); got != "Account "+value {
		t.Fatalf("entry data was transformed: %q", got)
	}
	if got := Text(English, "custom-value-密码"); got != "custom-value-密码" {
		t.Fatalf("unknown message was transformed: %q", got)
	}
	if got := Text(English, "100% safe"); got != "100% safe" {
		t.Fatalf("unformatted percent was changed: %q", got)
	}
}
