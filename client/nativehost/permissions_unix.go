//go:build !windows

package nativehost

import (
	"os"
	"syscall"
)

func ownedPrivate(info os.FileInfo) bool {
	s, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(s.Uid) == os.Geteuid() && info.Mode().Perm()&0077 == 0
}
