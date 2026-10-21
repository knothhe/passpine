//go:build !windows

package i18n

import (
	"context"
	"os/exec"
	"time"
)

func preferredSystemLanguages() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleLanguages").Output()
	return string(output), err
}
