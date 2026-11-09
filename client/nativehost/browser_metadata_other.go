//go:build !darwin

package nativehost

import "errors"

func browserApplicationMetadata(string) (browserMetadata, error) {
	return browserMetadata{}, errors.New("application bundles require macOS")
}
