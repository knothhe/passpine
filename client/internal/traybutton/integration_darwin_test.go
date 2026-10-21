//go:build darwin

package traybutton

import (
	"fmt"
	"os"
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

const integrationEnvironment = "PASSPINE_TRAY_BUTTON_INTEGRATION"

// The real event loop is opt-in. Ordinary package tests only use detached
// views, and never initialize NSApplication or create a menu bar item.
func TestMain(m *testing.M) {
	if os.Getenv(integrationEnvironment) != "1" {
		os.Exit(m.Run())
	}
	directory, err := os.MkdirTemp("", "passpine-tray-button-integration-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	mygo.App.SetName("Passpine Tray Button Integration")
	mygo.App.SetPath(mygo.PathAppData, directory)
	mygo.App.SetPath(mygo.PathUserData, directory)
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	result := make(chan int, 1)
	mygo.App.WhenReady(func() {
		go func() {
			result <- m.Run()
			mygo.App.Quit()
		}()
	})
	err = mygo.App.Run()
	code := 1
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else {
		select {
		case code = <-result:
		default:
			fmt.Fprintln(os.Stderr, "tray button integration: event loop stopped before tests completed")
		}
	}
	_ = os.RemoveAll(directory)
	os.Exit(code)
}

// Run with PASSPINE_TRAY_BUTTON_INTEGRATION=1 go test ./internal/traybutton
// -run TestNativeStatusButtonIntegration -count=1. A temporary status item is
// driven entirely by in-process NSEvents. The cursor and user apps are untouched.
func TestNativeStatusButtonIntegration(t *testing.T) {
	if os.Getenv(integrationEnvironment) != "1" {
		t.Skip("set " + integrationEnvironment + "=1 to run the native event-loop smoke test")
	}
	var err error
	mygo.RunOnMain(func() { err = nativeStatusButtonSmoke() })
	if err != nil {
		t.Fatal(err)
	}
}

func nativeStatusButtonSmoke() error {
	pool := send(objc.ID(objc.GetClass("NSAutoreleasePool")), "new")
	defer send(pool, "drain")
	toolTip := fmt.Sprintf("Passpine tray button integration %d", os.Getpid())
	tray, err := mygo.NewTray(mygo.TrayOptions{Title: "PP", ToolTip: toolTip})
	if err != nil {
		return err
	}
	defer tray.Destroy()
	fallbackClicks := 0
	tray.OnClick(func() { fallbackClicks++ })
	clicks, highlighted := 0, false
	var controller *Controller
	controller, err = Install(toolTip, func() {
		clicks++
		highlighted = !highlighted
		controller.SetHighlighted(highlighted)
	})
	if err != nil {
		return err
	}
	defer controller.Close()
	// Retain independently so monitor cleanup can be checked while the tray
	// itself remains present and its normal AppKit click handler is restored.
	button, window := send(controller.button, "retain"), send(controller.window, "retain")
	defer send(button, "release")
	defer send(window, "release")
	app := send(objc.ID(objc.GetClass("NSApplication")), "sharedApplication")
	click := func() {
		bounds := objc.Send[rect](button, objc.RegisterName("bounds"))
		location := point{bounds.Origin.X + bounds.Size.Width/2, bounds.Origin.Y + bounds.Size.Height/2}
		location = objc.Send[point](button, objc.RegisterName("convertPoint:toView:"), location, objc.ID(0))
		windowNumber := int(send(window, "windowNumber"))
		event := func(kind uintptr) objc.ID {
			return send(objc.ID(objc.GetClass("NSEvent")),
				"mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:",
				kind, location, uintptr(0), float64(0), windowNumber, objc.ID(0), int(0), int(1), float32(0))
		}
		// A queued mouse-up also lets AppKit finish its normal tracking if
		// the monitor failed, or after Close intentionally restores tracking.
		send(app, "postEvent:atStart:", event(2), false)
		send(app, "sendEvent:", event(1))
		until := send(objc.ID(objc.GetClass("NSDate")), "distantPast")
		mode := send(objc.ID(objc.GetClass("NSString")), "stringWithUTF8String:", "kCFRunLoopDefaultMode")
		if up := send(app, "nextEventMatchingMask:untilDate:inMode:dequeue:", uintptr(1<<2), until, mode, true); up != 0 {
			send(app, "sendEvent:", up)
		}
	}
	for wantClicks := 1; wantClicks <= 3; wantClicks++ {
		click()
		wantHighlight := wantClicks%2 == 1
		if clicks != wantClicks || fallbackClicks != 0 {
			return fmt.Errorf("tray click invoked monitor %d times and fallback %d times, want %d monitor calls", clicks, fallbackClicks, wantClicks)
		}
		if got := send(button, "isHighlighted") != 0; got != wantHighlight {
			return fmt.Errorf("tray highlight after mouse-up = %t, want %t", got, wantHighlight)
		}
	}
	send(button, "highlight:", false) // AppKit ending menu tracking.
	controller.SetHighlighted(true)
	if send(button, "isHighlighted") == 0 {
		return fmt.Errorf("cannot reassert panel highlight after AppKit clears it")
	}
	controller.Close()
	controller.Close()
	controller.SetHighlighted(true)
	if controller.monitor != 0 || send(button, "isHighlighted") != 0 {
		return fmt.Errorf("closing controller did not clear its monitor and highlight")
	}
	click()
	if clicks != 3 || fallbackClicks != 1 {
		return fmt.Errorf("closing controller did not restore AppKit clicks: monitor=%d fallback=%d", clicks, fallbackClicks)
	}
	return nil
}
