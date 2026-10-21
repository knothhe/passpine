//go:build darwin

// Package traybutton keeps a macOS status button selected while its panel is
// visible. Controllers and their callbacks must be used on the UI main thread.
package traybutton

import (
	"errors"

	"github.com/ebitengine/purego/objc"
)

type point struct{ X, Y float64 }
type size struct{ Width, Height float64 }
type rect struct {
	Origin point
	Size   size
}

// Controller owns a local event monitor for one status button.
type Controller struct {
	button, window, monitor objc.ID
	click                   func()
}

// Install finds the status button with this exact tooltip. It takes over its
// left mouse-down before AppKit's tracking can clear the panel's highlight on
// mouse-up. Right clicks and Command-dragging retain their normal behavior.
// The status item must already exist. Call Install on the UI main thread.
func Install(toolTip string, click func()) (*Controller, error) {
	if toolTip == "" || click == nil {
		return nil, errors.New("tray button: missing tooltip or click callback")
	}
	app := send(objc.ID(objc.GetClass("NSApplication")), "sharedApplication")
	windows := send(app, "windows")
	for index, count := uintptr(0), uintptr(send(windows, "count")); index < count; index++ {
		window := send(windows, "objectAtIndex:", index)
		if button := findButton(send(window, "contentView"), toolTip); button != 0 {
			return installButton(button, click)
		}
	}
	return nil, errors.New("tray button: status button unavailable")
}

func installButton(button objc.ID, click func()) (*Controller, error) {
	window := send(button, "window")
	if window == 0 {
		return nil, errors.New("tray button: status window unavailable")
	}
	c := &Controller{button: send(button, "retain"), window: send(window, "retain"), click: click}
	block := objc.NewBlock(func(_ objc.Block, event objc.ID) objc.ID {
		return c.handleEvent(event)
	})
	// NSEvent copies the block for the lifetime of its monitor.
	defer block.Release()
	c.monitor = send(send(objc.ID(objc.GetClass("NSEvent")), "addLocalMonitorForEventsMatchingMask:handler:",
		uintptr(1<<1), block), "retain") // NSEventMaskLeftMouseDown
	if c.monitor == 0 {
		c.Close()
		return nil, errors.New("tray button: cannot install click monitor")
	}
	return c, nil
}

func findButton(view objc.ID, toolTip string) objc.ID {
	if view == 0 {
		return 0
	}
	if send(view, "isKindOfClass:", objc.GetClass("NSStatusBarButton")) != 0 {
		tip := send(view, "toolTip")
		if tip != 0 && objc.Send[string](tip, objc.RegisterName("UTF8String")) == toolTip {
			return view
		}
	}
	children := send(view, "subviews")
	for index, count := uintptr(0), uintptr(send(children, "count")); index < count; index++ {
		if button := findButton(send(children, "objectAtIndex:", index), toolTip); button != 0 {
			return button
		}
	}
	return 0
}

func (c *Controller) handleEvent(event objc.ID) objc.ID {
	if c == nil || c.button == 0 || c.click == nil || event == 0 ||
		send(event, "type") != 1 || // NSEventTypeLeftMouseDown
		send(event, "window") != c.window || send(event, "modifierFlags")&(1<<20) != 0 {
		return event
	}
	location := objc.Send[point](event, objc.RegisterName("locationInWindow"))
	location = objc.Send[point](c.button, objc.RegisterName("convertPoint:fromView:"), location, objc.ID(0))
	bounds := objc.Send[rect](c.button, objc.RegisterName("bounds"))
	if location.X < bounds.Origin.X || location.Y < bounds.Origin.Y ||
		location.X >= bounds.Origin.X+bounds.Size.Width || location.Y >= bounds.Origin.Y+bounds.Size.Height {
		return event
	}
	c.click()
	return 0
}

// SetHighlighted applies the panel's visibility even if AppKit just ended menu
// tracking. Call it on the UI main thread.
func (c *Controller) SetHighlighted(highlighted bool) {
	if c != nil && c.button != 0 {
		send(c.button, "highlight:", highlighted)
	}
}

// Close removes the monitor and releases its status button. It is safe to call
// more than once, on the UI main thread, before destroying the status item.
func (c *Controller) Close() {
	if c == nil || c.button == 0 {
		return
	}
	if c.monitor != 0 {
		send(objc.ID(objc.GetClass("NSEvent")), "removeMonitor:", c.monitor)
		send(c.monitor, "release")
		c.monitor = 0
	}
	c.SetHighlighted(false)
	send(c.button, "release")
	send(c.window, "release")
	c.button, c.window, c.click = 0, 0, nil
}

func send(object objc.ID, selector string, args ...any) objc.ID {
	return object.Send(objc.RegisterName(selector), args...)
}
