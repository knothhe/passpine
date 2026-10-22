//go:build darwin

package update

import "golang.org/x/sys/unix"

// Both bundles stay reachable throughout the same-volume exchange. The prior
// bundle remains at the staging path until startup health is acknowledged.
func swapDirectories(from, to string) error {
	return unix.RenamexNp(from, to, unix.RENAME_SWAP)
}
