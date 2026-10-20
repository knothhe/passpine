package secureclipboard

import (
	"fmt"
	"os"
	"runtime"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

func TestSensitiveTextOnNamedMacPasteboard(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := appKit(); err != nil {
		t.Fatal(err)
	}
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	name := fmt.Sprintf("com.knothhe.passpine.secureclipboard-test.%d.%d", os.Getpid(), time.Now().UnixNano())
	board := macPasteboard(name)
	if board == 0 {
		t.Fatal("cannot create isolated test pasteboard")
	}
	defer board.Send(objc.RegisterName("releaseGlobally"))
	for _, text := range []string{"clipboard-test", "密码 🔑\x00\nsecond line", ""} {
		if err := writeMacPasteboard(text, name); err != nil {
			t.Fatal(err)
		}
		items := board.Send(objc.RegisterName("pasteboardItems"))
		if count := objc.Send[uintptr](items, objc.RegisterName("count")); count != 1 {
			t.Fatalf("expected one marked text item, got %d", count)
		}
		item := items.Send(objc.RegisterName("objectAtIndex:"), uintptr(0))
		types := item.Send(objc.RegisterName("types"))
		var gotTypes []string
		for index, count := uintptr(0), objc.Send[uintptr](types, objc.RegisterName("count")); index < count; index++ {
			typeName := types.Send(objc.RegisterName("objectAtIndex:"), index)
			gotTypes = append(gotTypes, objc.Send[string](typeName, objc.RegisterName("UTF8String")))
		}
		wantTypes := []string{macTextType, macConcealedType, macTransientType}
		if len(gotTypes) != len(wantTypes) {
			t.Fatalf("unexpected raw types %q", gotTypes)
		}
		for _, typeName := range wantTypes {
			if !slices.Contains(gotTypes, typeName) {
				t.Fatalf("missing raw type %q in %q", typeName, gotTypes)
			}
			data := item.Send(objc.RegisterName("dataForType:"), macString(typeName))
			if data == 0 {
				t.Fatalf("cannot read raw type %q", typeName)
			}
			length := objc.Send[uintptr](data, objc.RegisterName("length"))
			pointer := objc.Send[unsafe.Pointer](data, objc.RegisterName("bytes"))
			got := string(unsafe.Slice((*byte)(pointer), length))
			want := ""
			if typeName == macTextType {
				want = text
			}
			if got != want {
				t.Errorf("incorrect data for %q: got %q, want %q", typeName, got, want)
			}
		}
	}
}
