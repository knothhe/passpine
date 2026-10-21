package linuxdesktop

import (
	"encoding/json"
	"os"
	"testing"
)

func TestShortcutKeys(t *testing.T) {
	for _, test := range []struct {
		shortcut, keys, key string
		mask                int
	}{
		{"Ctrl+Alt+Space", "CTRL + ALT + space", "space", 12},
		{"Cmd+Shift+P", "SUPER + SHIFT + P", "P", 65},
		{"Ctrl+Plus", "CTRL + plus", "plus", 4},
		{"Alt+'", "ALT + apostrophe", "apostrophe", 8},
		{"Ctrl+Num4", "CTRL + KP_4", "KP_4", 4},
	} {
		got, err := shortcutKeys(test.shortcut)
		if err != nil || got.keys != test.keys || got.key != test.key || got.mask != test.mask {
			t.Fatalf("%s: %+v, %v", test.shortcut, got, err)
		}
	}
}

func TestHyprlandRuntimeShortcut(t *testing.T) {
	if os.Getenv("PASSPINE_NATIVE_SMOKE") != "1" || !HyprlandSession() {
		t.Skip("requires an interactive Hyprland session")
	}
	registry, err := NewShortcuts()
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	const accelerator = "Ctrl+Alt+F24"
	if err := registry.Register(accelerator, func() {}); err != nil {
		t.Fatal(err)
	}
	data, err := hypr("-j", "binds")
	if err != nil {
		t.Fatal(err)
	}
	var bindings []hyprBinding
	if err := json.Unmarshal(data, &bindings); err != nil {
		t.Fatal(err)
	}
	t.Logf("registered binding: %+v", registry.bindings[accelerator])
	found := false
	for _, binding := range bindings {
		if binding.Modmask == 12 && binding.Key == "F24" {
			t.Logf("compositor binding: %+v", binding)
			found = true
		}
	}
	if !found {
		t.Fatal("compositor did not install shortcut")
	}
	conflict, err := NewShortcuts()
	if err != nil {
		t.Fatal(err)
	}
	defer conflict.Close()
	if err := conflict.Register(accelerator, func() {}); err == nil {
		t.Fatal("conflicting registration replaced the original")
	}
	registry.Unregister(accelerator)
	if err := conflict.Register(accelerator, func() {}); err != nil {
		t.Fatalf("unregister did not release shortcut: %v", err)
	}
}
