//go:build !windows

package nativehost

import "runtime"

func platformRegistrationTargets(home string) []registrationTarget {
	return registrationTargets(home, runtime.GOOS)
}

func platformRegistrationMatches(BrowserRegistration) bool    { return true }
func registerPlatformHost(registrationTarget) error           { return nil }
func unregisterPlatformHost(registrationTarget, string) error { return nil }
