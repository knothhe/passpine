//go:build !windows

package app

import "os"

func restrictPrivateFile(file *os.File) error { return file.Chmod(0600) }
