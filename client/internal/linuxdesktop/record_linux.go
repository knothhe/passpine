package linuxdesktop

import (
	"fmt"
	"github.com/ebitengine/purego"
	"strings"
	"sync"
)

var keyboard struct {
	once       sync.Once
	err        error
	connect    func(uintptr, *byte, uintptr, uintptr, uintptr, uint32) uint64
	disconnect func(uintptr, uint64)
	window     func(uintptr) uintptr
	display    func(uintptr) uintptr
	seat       func(uintptr) uintptr
	grab       func(uintptr, uintptr, uint32, bool, uintptr, uintptr, uintptr, uintptr) int32
	ungrab     func(uintptr)
	keyName    func(uint32) string
	keyUnicode func(uint32) uint32
	callback   uintptr
	next       uintptr
	handlers   map[uintptr]func(string)
}

func loadKeyboard() error {
	keyboard.once.Do(func() {
		gdk, err := purego.Dlopen("libgdk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			keyboard.err = err
			return
		}
		gtk, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			keyboard.err = err
			return
		}
		object, err := purego.Dlopen("libgobject-2.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			keyboard.err = err
			return
		}
		for _, symbol := range []struct {
			lib    uintptr
			target any
			name   string
		}{
			{object, &keyboard.connect, "g_signal_connect_data"}, {object, &keyboard.disconnect, "g_signal_handler_disconnect"},
			{gtk, &keyboard.window, "gtk_widget_get_window"}, {gdk, &keyboard.display, "gdk_window_get_display"},
			{gdk, &keyboard.seat, "gdk_display_get_default_seat"}, {gdk, &keyboard.grab, "gdk_seat_grab"},
			{gdk, &keyboard.ungrab, "gdk_seat_ungrab"}, {gdk, &keyboard.keyName, "gdk_keyval_name"},
			{gdk, &keyboard.keyUnicode, "gdk_keyval_to_unicode"},
		} {
			address, err := purego.Dlsym(symbol.lib, symbol.name)
			if err != nil {
				keyboard.err = err
				return
			}
			purego.RegisterFunc(symbol.target, address)
		}
		keyboard.handlers = map[uintptr]func(string){}
		keyboard.callback = purego.NewCallback(func(window, event, id uintptr) bool {
			handler := keyboard.handlers[id]
			if handler == nil {
				return false
			}
			key, state := recordEvent(event)
			name := keyboard.keyName(key)
			if keyboard.keyUnicode(key) > 127 && state&(1<<2|1<<3|1<<26|1<<28) != 0 {
				if physical := recordPhysicalLetter(event); physical != "" {
					name = physical
				}
			}
			if accelerator := recordAccelerator(name, state); accelerator != "" {
				handler(accelerator)
			}
			return true
		})
	})
	return keyboard.err
}

// CaptureKeyboard runs on GTK's main thread. A keyboard seat grab inhibits
// compositor shortcuts on Wayland; the window handler precedes GTK menu
// accelerators and does not depend on an immediate-mode control's focus.
func CaptureKeyboard(window uintptr, handler func(string)) (func(), error) {
	if err := loadKeyboard(); err != nil {
		return nil, err
	}
	gdkWindow := keyboard.window(window)
	if gdkWindow == 0 {
		return nil, fmt.Errorf("shortcut recorder window is not realized")
	}
	seat := keyboard.seat(keyboard.display(gdkWindow))
	if seat == 0 {
		return nil, fmt.Errorf("shortcut recorder has no keyboard seat")
	}
	const keyboardCapability = 1 << 3
	if status := keyboard.grab(seat, gdkWindow, keyboardCapability, true, 0, 0, 0, 0); status != 0 {
		return nil, fmt.Errorf("shortcut keyboard grab failed (%d)", status)
	}
	keyboard.next++
	id := keyboard.next
	keyboard.handlers[id] = handler
	signal := append([]byte("key-press-event"), 0)
	connection := keyboard.connect(window, &signal[0], keyboard.callback, id, 0, 0)
	if connection == 0 {
		delete(keyboard.handlers, id)
		keyboard.ungrab(seat)
		return nil, fmt.Errorf("shortcut key handler could not be installed")
	}
	return func() {
		if _, ok := keyboard.handlers[id]; !ok {
			return
		}
		delete(keyboard.handlers, id)
		keyboard.disconnect(window, connection)
		keyboard.ungrab(seat)
	}, nil
}

func recordAccelerator(name string, state uint32) string {
	if name == "" || strings.HasPrefix(name, "Shift_") || strings.HasPrefix(name, "Control_") || strings.HasPrefix(name, "Alt_") || strings.HasPrefix(name, "Super_") || strings.HasPrefix(name, "Meta_") || name == "ISO_Level3_Shift" {
		return ""
	}
	aliases := map[string]string{
		"exclam": "1", "at": "2", "numbersign": "3", "dollar": "4", "percent": "5", "asciicircum": "6", "ampersand": "7", "asterisk": "8", "parenleft": "9", "parenright": "0",
		"underscore": "-", "braceleft": "[", "braceright": "]", "colon": ";", "quotedbl": "'", "less": ",", "greater": ".", "question": "/", "asciitilde": "`",
		"space": "Space", "Return": "Enter", "KP_Enter": "Enter", "ISO_Left_Tab": "Tab", "BackSpace": "Backspace", "Prior": "PageUp", "Next": "PageDown",
		"minus": "-", "equal": "=", "bracketleft": "[", "bracketright": "]", "backslash": "\\", "semicolon": ";", "apostrophe": "'", "comma": ",", "period": ".", "slash": "/", "grave": "`", "plus": "Plus",
		"Caps_Lock": "CapsLock", "Num_Lock": "NumLock", "Scroll_Lock": "ScrollLock", "Print": "PrintScreen",
		"KP_Decimal": "NumDec", "KP_Add": "NumAdd", "KP_Subtract": "NumSub", "KP_Multiply": "NumMult", "KP_Divide": "NumDiv",
		"XF86AudioRaiseVolume": "VolumeUp", "XF86AudioLowerVolume": "VolumeDown", "XF86AudioMute": "VolumeMute", "XF86AudioNext": "MediaNextTrack", "XF86AudioPrev": "MediaPreviousTrack", "XF86AudioStop": "MediaStop", "XF86AudioPlay": "MediaPlayPause",
	}
	if alias, ok := aliases[name]; ok {
		name = alias
	} else if len(name) == 4 && strings.HasPrefix(name, "KP_") && name[3] >= '0' && name[3] <= '9' {
		name = "Num" + name[3:]
	} else if len(name) == 1 {
		name = strings.ToUpper(name)
	}
	parts := []string{}
	for _, modifier := range []struct {
		mask uint32
		name string
	}{{1<<26 | 1<<28, "Cmd"}, {1 << 2, "Ctrl"}, {1 << 3, "Alt"}, {1, "Shift"}} {
		if state&modifier.mask != 0 {
			parts = append(parts, modifier.name)
		}
	}
	return strings.Join(append(parts, name), "+")
}
