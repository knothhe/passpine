package nativehost

import "os"

func ownedPrivate(info os.FileInfo) bool { return info.Mode().IsRegular() }
