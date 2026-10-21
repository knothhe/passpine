package linuxdesktop

import "unsafe"

// GdkEventKey has the same state/keyval offsets on Linux amd64 and arm64.
func recordEvent(event uintptr) (key, state uint32) {
	pointer := *(*unsafe.Pointer)(unsafe.Pointer(&event))
	return *(*uint32)(unsafe.Add(pointer, 28)), *(*uint32)(unsafe.Add(pointer, 24))
}

func recordPhysicalLetter(event uintptr) string {
	pointer := *(*unsafe.Pointer)(unsafe.Pointer(&event))
	code := *(*uint16)(unsafe.Add(pointer, 48))
	if code < 8 {
		return ""
	}
	code -= 8
	for _, row := range []struct {
		start   uint16
		letters string
	}{{16, "QWERTYUIOP"}, {30, "ASDFGHJKL"}, {44, "ZXCVBNM"}} {
		if code >= row.start && int(code-row.start) < len(row.letters) {
			return string(row.letters[code-row.start])
		}
	}
	return ""
}
