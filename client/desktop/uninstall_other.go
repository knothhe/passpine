//go:build !darwin

package desktop

func prepareDevelopmentUninstall() error {
	// These installers inspect the exact registration target themselves;
	// the runtime's name-only removal could affect another copy.
	return nil
}
