package vault

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
)

func randomID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}

func privateDirectory(directory string) (string, error) {
	abs, e := filepath.Abs(directory)
	if e != nil {
		return "", e
	}
	if s, e := os.Lstat(abs); e == nil && s.Mode()&os.ModeSymlink != 0 {
		return "", ErrPath
	}
	if e = os.MkdirAll(abs, 0700); e != nil {
		return "", e
	}
	abs, e = filepath.EvalSymlinks(abs)
	if e != nil {
		return "", e
	}
	return abs, nil
}
