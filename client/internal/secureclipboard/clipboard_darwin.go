package secureclipboard

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

var appKit = sync.OnceValue(func() error {
	_, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	return err
})

func writeSensitiveText(value string) error {
	err := errors.New("secureclipboard: application event loop is stopped")
	mygo.RunOnMain(func() {
		err = writeMacPasteboard(value, "")
	})
	return err
}

// An empty name selects the general pasteboard. Tests use a separate named
// pasteboard so verification never replaces the user's clipboard contents.
func writeMacPasteboard(value, name string) error {
	if err := appKit(); err != nil {
		return err
	}
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))

	item := objc.ID(objc.GetClass("NSPasteboardItem")).Send(objc.RegisterName("new"))
	if item == 0 {
		return errors.New("secureclipboard: cannot create pasteboard item")
	}
	defer item.Send(objc.RegisterName("release"))

	// Use raw NSPasteboard type names. MyGo treats custom formats as MIME
	// types and would otherwise encode these names into unrelated UTIs.
	for _, representation := range []struct {
		typeName string
		data     []byte
	}{
		{macTextType, []byte(value)},
		{macConcealedType, nil},
		{macTransientType, nil},
	} {
		if !objc.Send[bool](item, objc.RegisterName("setData:forType:"), macData(representation.data), macString(representation.typeName)) {
			return errors.New("secureclipboard: cannot prepare pasteboard representation")
		}
	}
	board := macPasteboard(name)
	if board == 0 {
		return errors.New("secureclipboard: cannot access pasteboard")
	}
	items := objc.ID(objc.GetClass("NSArray")).Send(objc.RegisterName("arrayWithObject:"), item)
	if items == 0 {
		return errors.New("secureclipboard: cannot prepare pasteboard contents")
	}
	// All formats are ready before the clipboard is touched. Publish the item
	// once, retaining the system's existing Universal Clipboard behavior.
	board.Send(objc.RegisterName("clearContents"))
	if !objc.Send[bool](board, objc.RegisterName("writeObjects:"), items) {
		return errors.New("secureclipboard: cannot write pasteboard contents")
	}
	return nil
}

func macPasteboard(name string) objc.ID {
	class := objc.ID(objc.GetClass("NSPasteboard"))
	if name == "" {
		return class.Send(objc.RegisterName("generalPasteboard"))
	}
	return class.Send(objc.RegisterName("pasteboardWithName:"), macString(name))
}

func macData(value []byte) objc.ID {
	var data unsafe.Pointer
	if len(value) > 0 {
		data = unsafe.Pointer(&value[0])
	}
	result := objc.ID(objc.GetClass("NSData")).Send(objc.RegisterName("dataWithBytes:length:"), data, uintptr(len(value)))
	runtime.KeepAlive(value)
	return result
}

func macString(value string) objc.ID {
	// Type identifiers and pasteboard names contain no NUL bytes. Clipboard
	// text itself uses NSData so Unicode and embedded NULs remain intact.
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), value)
}
