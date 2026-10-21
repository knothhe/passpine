package i18n

import "golang.org/x/sys/windows"

func preferredSystemLanguages() (string, error) {
	languages, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(languages) == 0 {
		return "", err
	}
	return languages[0], nil
}
