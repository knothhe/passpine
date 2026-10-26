package desktop

import (
	"errors"

	"github.com/knothhe/passpine/client/buildinfo"
)

// Called only by the explicitly requested Dev uninstaller, from the installed
// executable. This lets the platform remove the exact app's login registration.
func PrepareDevelopmentUninstall() error {
	if buildinfo.IsRelease() {
		return errors.New("cannot prepare a release installation for Dev removal")
	}
	return prepareDevelopmentUninstall()
}
