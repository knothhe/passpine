//go:build darwin

package traybutton

import (
	"runtime"
	"sync"
	"testing"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var loadAppKit = sync.OnceValue(func() error {
	_, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	return err
})

// These views are detached from any window. Tests never create an application
// or a real status item, and cannot change the user's menu bar.
func nativeViews(t *testing.T) {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	if err := loadAppKit(); err != nil {
		t.Fatal(err)
	}
	pool := send(objc.ID(objc.GetClass("NSAutoreleasePool")), "new")
	t.Cleanup(func() { send(pool, "drain") })
}

func newView(class string, frame rect) objc.ID {
	return send(send(objc.ID(objc.GetClass(class)), "alloc"), "initWithFrame:", frame)
}

func setToolTip(view objc.ID, value string) {
	send(view, "setToolTip:", send(objc.ID(objc.GetClass("NSString")), "stringWithUTF8String:", value))
}

func TestFindStatusButtonRequiresExactTooltipAndStatusButtonClass(t *testing.T) {
	nativeViews(t)
	root := newView("NSView", rect{Size: size{Width: 100, Height: 100}})
	defer send(root, "release")
	group := newView("NSView", rect{Size: size{Width: 100, Height: 100}})
	defer send(group, "release")
	send(root, "addSubview:", group)
	decoy := newView("NSButton", rect{Size: size{Width: 22, Height: 22}})
	defer send(decoy, "release")
	setToolTip(decoy, "Passpine")
	send(root, "addSubview:", decoy)
	button := newView("NSStatusBarButton", rect{Size: size{Width: 22, Height: 22}})
	defer send(button, "release")
	setToolTip(button, "Passpine")
	send(group, "addSubview:", button)
	if got := findButton(root, "Passpine"); got != button {
		t.Fatalf("found button %x, want %x", got, button)
	}
	if got := findButton(root, "Passpine "); got != 0 {
		t.Fatalf("matched a different tooltip: %x", got)
	}
	if _, err := installButton(button, func() {}); err == nil {
		t.Fatal("installed a monitor on a detached button")
	}
}

func mouseEvent(kind, flags uintptr, location point) objc.ID {
	return send(objc.ID(objc.GetClass("NSEvent")),
		"mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:",
		kind, location, flags, float64(0), int(0), objc.ID(0), int(0), int(1), float32(0))
}

func TestStatusClickTakesOverOnlyLeftMouseDownInsideItsBounds(t *testing.T) {
	nativeViews(t)
	button := newView("NSStatusBarButton", rect{Size: size{Width: 22, Height: 22}})
	defer send(button, "release")
	clicks := 0
	c := &Controller{button: button, click: func() { clicks++ }}
	bounds := objc.Send[rect](button, objc.RegisterName("bounds"))
	center := point{bounds.Size.Width / 2, bounds.Size.Height / 2}
	for _, test := range []struct {
		name        string
		kind, flags uintptr
		location    point
		consume     bool
	}{
		{"left down", 1, 0, center, true},
		{"left up", 2, 0, center, false},
		{"right down", 3, 0, center, false},
		{"right up", 4, 0, center, false},
		{"command drag", 1, 1 << 20, center, false},
		{"left edge", 1, 0, point{-1, center.Y}, false},
		{"right edge", 1, 0, point{bounds.Size.Width, center.Y}, false},
		{"top edge", 1, 0, point{center.X, bounds.Size.Height}, false},
	} {
		location := objc.Send[point](button, objc.RegisterName("convertPoint:toView:"), test.location, objc.ID(0))
		event := mouseEvent(test.kind, test.flags, location)
		before := clicks
		got := c.handleEvent(event)
		if test.consume {
			if got != 0 || clicks != before+1 {
				t.Fatalf("%s: click was not consumed exactly once: event=%x, clicks=%d", test.name, got, clicks-before)
			}
		} else if got != event || clicks != before {
			t.Fatalf("%s: unrelated event was consumed: event=%x, clicks=%d", test.name, got, clicks-before)
		}
	}
	otherWindow := send(objc.ID(objc.GetClass("NSObject")), "new")
	defer send(otherWindow, "release")
	c.window = otherWindow
	event := mouseEvent(1, 0, point{11, 11})
	before := clicks
	if got := c.handleEvent(event); got != event || clicks != before {
		t.Fatal("consumed a click from another window")
	}
}

func TestStatusHighlightPersistsUntilClearedAndCloseIsIdempotent(t *testing.T) {
	nativeViews(t)
	button := newView("NSStatusBarButton", rect{Size: size{Width: 22, Height: 22}})
	defer send(button, "release")
	c := &Controller{button: send(button, "retain"), click: func() {}}
	c.SetHighlighted(true)
	if send(button, "isHighlighted") == 0 {
		t.Fatal("status button is not highlighted while panel is open")
	}
	c.SetHighlighted(true)
	if send(button, "isHighlighted") == 0 {
		t.Fatal("reapplying highlight cleared the status button")
	}
	c.SetHighlighted(false)
	if send(button, "isHighlighted") != 0 {
		t.Fatal("status button stayed highlighted after dismissal")
	}
	c.SetHighlighted(true)
	c.Close()
	if send(button, "isHighlighted") != 0 {
		t.Fatal("closing controller left a highlighted status button")
	}
	c.Close()
	c.SetHighlighted(true)
	if send(button, "isHighlighted") != 0 {
		t.Fatal("closed controller still changes the status button")
	}
}
