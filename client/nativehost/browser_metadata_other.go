//go:build !darwin

package nativehost

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/knothhe/passpine/client/i18n"
)

func configuredBrowserInstalled(c BrowserConfiguration) bool {
	return HelperAvailable(c.Path) && (runtime.GOOS != "windows" || strings.EqualFold(filepath.Ext(c.Path), ".exe"))
}

func normalizeBrowserProgram(c *BrowserConfiguration) error {
	if !configuredBrowserInstalled(*c) {
		return errors.New(i18n.BrowserPathInvalid)
	}
	return nil
}
