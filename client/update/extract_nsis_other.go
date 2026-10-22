//go:build !windows

package update

import (
	"context"
	"errors"
)

func extractNSIS(_ context.Context, _, _ string) error {
	return errors.New("Windows installers can only be extracted on Windows")
}
