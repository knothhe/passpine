package linuxdesktop

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/godbus/dbus/v5"
)

func TestTrayMenuProtocol(t *testing.T) {
	tray := &Tray{revision: 3, items: []*mygo.MenuItem{
		{Label: "fixture", Disabled: true, Hidden: true}, mygo.Separator(), {Label: "other"},
	}}
	menu := &menuMethods{tray}
	revision, root, err := menu.GetLayout(0, -1, nil)
	if err != nil || revision != 3 || len(root.Children) != 3 {
		t.Fatalf("layout: %d %+v %v", revision, root, err)
	}
	if signature := dbus.SignatureOf(root).String(); signature != "(ia{sv}av)" {
		t.Fatal(signature)
	}
	props, err := menu.GetGroupProperties([]int32{1, 2}, nil)
	if err != nil || props[0].Properties["enabled"].Value() != false || props[0].Properties["visible"].Value() != false || props[1].Properties["type"].Value() != "separator" {
		t.Fatalf("menu properties: %+v %v", props, err)
	}
	if _, _, err := menu.GetLayout(999, -1, nil); err == nil {
		t.Fatal("accepted nonexistent parent")
	}
}
