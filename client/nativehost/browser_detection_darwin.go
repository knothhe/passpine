package nativehost

import (
	"errors"
	"path/filepath"
	"strings"
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
	bundleValue           func(uintptr, uintptr) uintptr
	stringGetCString      func(uintptr, *byte, int, uint32) bool
	getTypeID             func(uintptr) uintptr
	stringTypeID          func() uintptr
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
	purego.RegisterLibFunc(&browserDiscovery.bundleValue, cf, "CFBundleGetValueForInfoDictionaryKey")
	purego.RegisterLibFunc(&browserDiscovery.stringGetCString, cf, "CFStringGetCString")
	purego.RegisterLibFunc(&browserDiscovery.getTypeID, cf, "CFGetTypeID")
	purego.RegisterLibFunc(&browserDiscovery.stringTypeID, cf, "CFStringGetTypeID")
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
		"ego-lite":       "com.citrolabs.ego.lite",
		"firefox":        "org.mozilla.firefox",
	}[id]
}

// Launch Services finds applications wherever they were installed. Like the
// previous client's NSWorkspace lookup, we then verify the bundle identity and
// executable; a stale Launch Services URL does not mean a browser is installed.
func browserInstalled(target registrationTarget) bool {
	if target.configuration != nil {
		return configuredBrowserInstalled(*target.configuration)
	}
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

func browserApplicationMetadata(path string) (browserMetadata, error) {
	var metadata browserMetadata
	browserDiscovery.once.Do(loadBrowserDiscovery)
	if !browserDiscovery.available || !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".app") {
		return metadata, errors.New("invalid application bundle")
	}
	value := browserDiscovery.stringCreate(0, path, 0x08000100)
	if value == 0 {
		return metadata, errors.New("invalid bundle path")
	}
	defer browserDiscovery.release(value)
	url := browserDiscovery.urlCreateWithPath(0, value, 0, true)
	if url == 0 {
		return metadata, errors.New("invalid bundle URL")
	}
	defer browserDiscovery.release(url)
	bundle := browserDiscovery.bundleCreate(0, url)
	if bundle == 0 {
		return metadata, errors.New("invalid application bundle")
	}
	defer browserDiscovery.release(bundle)
	identifier := browserDiscovery.bundleIdentifier(bundle)
	if identifier == 0 || !applicationBundleInstalled(url, identifier) {
		return metadata, errors.New("missing application executable")
	}
	read := func(key string) string {
		keyRef := browserDiscovery.stringCreate(0, key, 0x08000100)
		if keyRef == 0 {
			return ""
		}
		defer browserDiscovery.release(keyRef)
		value := browserDiscovery.bundleValue(bundle, keyRef)
		if value == 0 || browserDiscovery.getTypeID(value) != browserDiscovery.stringTypeID() {
			return ""
		}
		var buffer [4096]byte
		if !browserDiscovery.stringGetCString(value, &buffer[0], len(buffer), 0x08000100) {
			return ""
		}
		for i, b := range buffer {
			if b == 0 {
				return string(buffer[:i])
			}
		}
		return ""
	}
	metadata = browserMetadata{name: read("CFBundleDisplayName"), bundleID: read("CFBundleIdentifier"), productDirectory: read("CrProductDirName")}
	if metadata.name == "" {
		metadata.name = read("CFBundleName")
	}
	return metadata, nil
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
