package nativehost

import (
	"sync"

	"github.com/ebitengine/purego"
)

var browserDiscovery struct {
	once                  sync.Once
	available             bool
	stringCreate          func(uintptr, string, uint32) uintptr
	release               func(uintptr)
	equal                 func(uintptr, uintptr) bool
	applicationURLs       func(uintptr, *uintptr) uintptr
	arrayCount            func(uintptr) int
	arrayValue            func(uintptr, int) uintptr
	bundleCreate          func(uintptr, uintptr) uintptr
	bundleIdentifier      func(uintptr) uintptr
	bundleExecutableURL   func(uintptr) uintptr
	urlCreateWithPath     func(uintptr, uintptr, int, bool) uintptr
	urlFileRepresentation func(uintptr, bool, *byte, int) bool
}

func loadBrowserDiscovery() {
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return
	}
	ls, err := purego.Dlopen("/System/Library/Frameworks/CoreServices.framework/CoreServices", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return
	}
	purego.RegisterLibFunc(&browserDiscovery.stringCreate, cf, "CFStringCreateWithCString")
	purego.RegisterLibFunc(&browserDiscovery.release, cf, "CFRelease")
	purego.RegisterLibFunc(&browserDiscovery.equal, cf, "CFEqual")
	purego.RegisterLibFunc(&browserDiscovery.arrayCount, cf, "CFArrayGetCount")
	purego.RegisterLibFunc(&browserDiscovery.arrayValue, cf, "CFArrayGetValueAtIndex")
	purego.RegisterLibFunc(&browserDiscovery.bundleCreate, cf, "CFBundleCreate")
	purego.RegisterLibFunc(&browserDiscovery.bundleIdentifier, cf, "CFBundleGetIdentifier")
	purego.RegisterLibFunc(&browserDiscovery.bundleExecutableURL, cf, "CFBundleCopyExecutableURL")
	purego.RegisterLibFunc(&browserDiscovery.urlCreateWithPath, cf, "CFURLCreateWithFileSystemPath")
	purego.RegisterLibFunc(&browserDiscovery.urlFileRepresentation, cf, "CFURLGetFileSystemRepresentation")
	purego.RegisterLibFunc(&browserDiscovery.applicationURLs, ls, "LSCopyApplicationURLsForBundleIdentifier")
	browserDiscovery.available = true
}

func browserBundleID(id string) string {
	return map[string]string{
		"chrome":         "com.google.Chrome",
		"chrome-beta":    "com.google.Chrome.beta",
		"chrome-testing": "com.google.chrome.for.testing",
		"chromium":       "org.chromium.Chromium",
		"edge":           "com.microsoft.edgemac",
		"brave":          "com.brave.Browser",
		"firefox":        "org.mozilla.firefox",
	}[id]
}

// Launch Services finds applications wherever they were installed. Like the
// previous client's NSWorkspace lookup, we then verify the bundle identity and
// executable; a stale Launch Services URL does not mean a browser is installed.
func browserInstalled(target registrationTarget) bool {
	browserDiscovery.once.Do(loadBrowserDiscovery)
	bundleID := browserBundleID(target.id)
	if !browserDiscovery.available || bundleID == "" {
		return false
	}
	identifier := browserDiscovery.stringCreate(0, bundleID, 0x08000100)
	if identifier == 0 {
		return false
	}
	defer browserDiscovery.release(identifier)
	urls := browserDiscovery.applicationURLs(identifier, nil)
	if urls == 0 {
		return false
	}
	defer browserDiscovery.release(urls)
	for i, count := 0, browserDiscovery.arrayCount(urls); i < count; i++ {
		if applicationBundleInstalled(browserDiscovery.arrayValue(urls, i), identifier) {
			return true
		}
	}
	return false
}

func applicationBundleInstalled(url, expectedID uintptr) bool {
	bundle := browserDiscovery.bundleCreate(0, url)
	if bundle == 0 {
		return false
	}
	defer browserDiscovery.release(bundle)
	identifier := browserDiscovery.bundleIdentifier(bundle)
	if identifier == 0 || !browserDiscovery.equal(identifier, expectedID) {
		return false
	}
	executable := browserDiscovery.bundleExecutableURL(bundle)
	if executable == 0 {
		return false
	}
	defer browserDiscovery.release(executable)
	var path [4096]byte
	if !browserDiscovery.urlFileRepresentation(executable, true, &path[0], len(path)) {
		return false
	}
	for i, value := range path {
		if value == 0 {
			return HelperAvailable(string(path[:i]))
		}
	}
	return false
}
