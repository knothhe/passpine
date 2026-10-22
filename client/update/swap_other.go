//go:build !darwin

package update

import "errors"

func swapDirectories(string, string) error {
	return errors.New("atomic bundle exchange requires macOS")
}
