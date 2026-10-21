package linuxdesktop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/godbus/dbus/v5"
)

const shortcutInterface = "com.knothhe.Passpine.Shortcuts"
const shortcutPath dbus.ObjectPath = "/Shortcuts"

// Hyprland's portal registers actions but does not bind preferred_trigger.
// Keep runtime bindings owned by this process; never edit desktop config or
// replace another application's key. The session bus connection owns callbacks.
type Shortcuts struct {
	mu       sync.Mutex
	conn     *dbus.Conn
	events   net.Conn
	bindings map[string]shortcutBinding
	closed   bool
}
type shortcutBinding struct {
	keys     string
	mask     int
	key      string
	command  string
	activate func()
	owner    string
}
type hyprBinding struct {
	Modmask     int
	Key         string
	Dispatcher  string
	Arg         string
	Description string
}

func HyprlandSession() bool {
	return os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" && os.Getenv("WAYLAND_DISPLAY") != ""
}
func hypr(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "hyprctl", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("hyprctl: %w: %s", err, output)
	}
	return output, nil
}
func NewShortcuts() (*Shortcuts, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	r := &Shortcuts{conn: conn, bindings: map[string]shortcutBinding{}}
	if err := conn.Export(&shortcutMethods{r}, shortcutPath, shortcutInterface); err != nil {
		conn.Close()
		return nil, err
	}
	socket := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "hypr", os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"), ".socket2.sock")
	r.events, err = net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		conn.Close()
		return nil, err
	}
	go r.watchReloads()
	return r, nil
}

func shortcutKeys(shortcut string) (shortcutBinding, error) {
	parts := strings.Split(shortcut, "+")
	key := parts[len(parts)-1]
	masks := map[string]int{"Ctrl": 4, "Alt": 8, "Shift": 1, "Cmd": 64}
	names := map[string]string{"Ctrl": "CTRL", "Alt": "ALT", "Shift": "SHIFT", "Cmd": "SUPER"}
	mods := []string{}
	mask := 0
	for _, part := range parts[:len(parts)-1] {
		bit, ok := masks[part]
		if !ok {
			return shortcutBinding{}, fmt.Errorf("unsupported modifier %q", part)
		}
		mask |= bit
		mods = append(mods, names[part])
	}
	aliases := map[string]string{
		"Space": "space", "Enter": "Return", "Escape": "Escape", "Backspace": "BackSpace", "PageUp": "Prior", "PageDown": "Next", "Plus": "plus",
		"CapsLock": "Caps_Lock", "NumLock": "Num_Lock", "ScrollLock": "Scroll_Lock", "PrintScreen": "Print",
		"NumDec": "KP_Decimal", "NumAdd": "KP_Add", "NumSub": "KP_Subtract", "NumMult": "KP_Multiply", "NumDiv": "KP_Divide",
		"VolumeUp": "XF86AudioRaiseVolume", "VolumeDown": "XF86AudioLowerVolume", "VolumeMute": "XF86AudioMute",
		"MediaNextTrack": "XF86AudioNext", "MediaPreviousTrack": "XF86AudioPrev", "MediaStop": "XF86AudioStop", "MediaPlayPause": "XF86AudioPlay",
		"-": "minus", "=": "equal", "[": "bracketleft", "]": "bracketright", "\\": "backslash", ";": "semicolon", "'": "apostrophe", ",": "comma", ".": "period", "/": "slash", "`": "grave",
	}
	if alias, ok := aliases[key]; ok {
		key = alias
	} else if len(key) == 4 && strings.HasPrefix(key, "Num") {
		key = "KP_" + key[3:]
	}
	keys := append(mods, key)
	return shortcutBinding{keys: strings.Join(keys, " + "), mask: mask, key: key}, nil
}
func (r *Shortcuts) install(binding shortcutBinding) error {
	data, err := hypr("-j", "binds")
	if err != nil {
		return err
	}
	var bindings []hyprBinding
	if err := json.Unmarshal(data, &bindings); err != nil {
		return err
	}
	for _, other := range bindings {
		if other.Modmask == binding.mask && strings.EqualFold(other.Key, binding.key) {
			if other.Description == binding.owner {
				return nil
			}
			// Recover our runtime binding after an unclean process exit. Never
			// remove bindings belonging to a live process or desktop config.
			if strings.HasPrefix(other.Description, "Passpine shortcut :") {
				name := strings.TrimPrefix(other.Description, "Passpine shortcut ")
				var alive bool
				if err := r.conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, name).Store(&alive); err == nil && !alive {
					stale := binding
					stale.owner = other.Description
					r.remove(stale)
					continue
				}
			}
			return fmt.Errorf("Hyprland shortcut %s is already in use", binding.keys)
		}
	}
	index := "_G[" + strconv.Quote(binding.owner) + "]"
	code := index + " = " + index + " or {}; " + index + "[" + strconv.Quote(binding.keys) + "] = hl.bind(" + strconv.Quote(binding.keys) + ", hl.dsp.exec_cmd(" + strconv.Quote(binding.command) + "), { description = " + strconv.Quote(binding.owner) + " })"
	output, err := hypr("eval", code)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(output)) != "ok" {
		return fmt.Errorf("Hyprland bind: %s", output)
	}
	return nil
}
func (r *Shortcuts) Register(shortcut string, activate func()) error {
	binding, err := shortcutKeys(shortcut)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("shortcut registry is closed")
	}
	binding.activate = activate
	binding.owner = "Passpine shortcut " + r.conn.Names()[0]
	// The unique bus name is allocated by D-Bus and is valid only while we run.
	// Accelerator names are normalized by app.NormalizeGlobalShortcut.
	binding.command = "busctl --user call " + r.conn.Names()[0] + " " + string(shortcutPath) + " " + shortcutInterface + " Activate s '" + strings.ReplaceAll(shortcut, "'", "'\\''") + "'"
	if err := r.install(binding); err != nil {
		return err
	}
	r.bindings[shortcut] = binding
	return nil
}
func (r *Shortcuts) Unregister(shortcut string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if binding, ok := r.bindings[shortcut]; ok {
		// Remove only our binding, leaving all other combinations intact.
		r.remove(binding)
		delete(r.bindings, shortcut)
	}
}
func (r *Shortcuts) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	for _, binding := range r.bindings {
		r.remove(binding)
	}
	r.bindings = map[string]shortcutBinding{}
	r.mu.Unlock()
	r.events.Close()
	r.conn.Close()
}

func (r *Shortcuts) remove(binding shortcutBinding) {
	index := "_G[" + strconv.Quote(binding.owner) + "]"
	entry := index + "[" + strconv.Quote(binding.keys) + "]"
	hypr("eval", "if "+index+" and "+entry+" then "+entry+":unbind(); "+entry+" = nil end")
}
func (r *Shortcuts) watchReloads() {
	scanner := bufio.NewScanner(r.events)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "configreloaded>>") {
			r.mu.Lock()
			if !r.closed {
				for _, binding := range r.bindings {
					if err := r.install(binding); err != nil {
						log.Printf("Passpine shortcuts: %v", err)
					}
				}
			}
			r.mu.Unlock()
		}
	}
}

type shortcutMethods struct{ r *Shortcuts }

func (m *shortcutMethods) Activate(shortcut string) *dbus.Error {
	m.r.mu.Lock()
	binding, ok := m.r.bindings[shortcut]
	closed := m.r.closed
	m.r.mu.Unlock()
	if ok && !closed {
		mygo.RunOnMain(binding.activate)
	}
	return nil
}
