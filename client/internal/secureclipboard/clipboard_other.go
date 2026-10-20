//go:build !darwin && !windows && !linux

package secureclipboard

import "errors"

func writeSensitiveText(string) error {
	return errors.New("secureclipboard: sensitive clipboard writes are unsupported on this platform")
}
