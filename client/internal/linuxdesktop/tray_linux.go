package linuxdesktop

import (
	"bytes"
	"context"
	"image/png"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const itemInterface = "org.kde.StatusNotifierItem"
const menuInterface = "com.canonical.dbusmenu"
const itemPath dbus.ObjectPath = "/StatusNotifierItem"
const menuPath dbus.ObjectPath = "/MenuBar"

// Tray exports StatusNotifierItem directly: AppIndicator forces ItemIsMenu,
// which makes panel hosts replace left-click activation with a menu.
type Tray struct {
	conn     *dbus.Conn
	props    *prop.Properties
	mu       sync.Mutex
	click    func()
	items    []*mygo.MenuItem
	revision uint32
	menu     *mygo.Menu
	stop     chan struct{}
	once     sync.Once
}

type pixmap struct {
	Width, Height int32
	Data          []byte
}
type layout struct {
	ID         int32
	Properties map[string]dbus.Variant
	Children   []dbus.Variant
}
type menuProperty struct {
	ID         int32
	Properties map[string]dbus.Variant
}
type tooltip struct {
	IconName           string
	IconPixmap         []pixmap
	Title, Description string
}

func NewTray(options mygo.TrayOptions) (*Tray, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	t := &Tray{conn: conn, stop: make(chan struct{})}
	fail := func(err error) (*Tray, error) { conn.Close(); return nil, err }
	if err := conn.Export(&itemMethods{t}, itemPath, itemInterface); err != nil {
		return fail(err)
	}
	if err := conn.Export(&menuMethods{t}, menuPath, menuInterface); err != nil {
		return fail(err)
	}
	props := map[string]*prop.Prop{}
	for key, value := range map[string]any{
		"Category": "ApplicationStatus", "Id": "com.knothhe.passpine", "Title": options.ToolTip,
		"Status": "Active", "WindowId": uint32(0), "ItemIsMenu": false, "Menu": menuPath,
		"IconName": "", "IconPixmap": []pixmap{}, "AttentionIconName": "", "AttentionIconPixmap": []pixmap{},
		"OverlayIconName": "", "OverlayIconPixmap": []pixmap{},
		"ToolTip": tooltip{"", []pixmap{}, options.ToolTip, ""},
	} {
		props[key] = &prop.Prop{Value: value, Emit: prop.EmitTrue}
	}
	t.props, err = prop.Export(conn, itemPath, map[string]map[string]*prop.Prop{itemInterface: props})
	if err != nil {
		return fail(err)
	}
	menuProps, err := prop.Export(conn, menuPath, map[string]map[string]*prop.Prop{menuInterface: {
		"Version": {Value: uint32(3)}, "TextDirection": {Value: "ltr"}, "Status": {Value: "normal"}, "IconThemePath": {Value: []string{}},
	}})
	if err != nil {
		return fail(err)
	}
	for _, item := range []struct {
		path    dbus.ObjectPath
		iface   string
		methods any
		props   *prop.Properties
	}{
		{itemPath, itemInterface, &itemMethods{t}, t.props}, {menuPath, menuInterface, &menuMethods{t}, menuProps},
	} {
		node := &introspect.Node{Name: string(item.path), Interfaces: []introspect.Interface{
			{Name: item.iface, Methods: introspect.Methods(item.methods), Properties: item.props.Introspection(item.iface)}, prop.IntrospectData,
		}}
		if err := conn.Export(introspect.NewIntrospectable(node), item.path, "org.freedesktop.DBus.Introspectable"); err != nil {
			return fail(err)
		}
	}
	if err := t.SetIcon(options.Icon, false); err != nil {
		return fail(err)
	}
	if err := t.register(); err != nil {
		return fail(err)
	}
	// The panel can restart independently of this process.
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	if err := conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, "org.kde.StatusNotifierWatcher")); err != nil {
		return fail(err)
	}
	go func() {
		for {
			select {
			case signal := <-signals:
				if signal != nil && len(signal.Body) == 3 && signal.Body[2] != "" {
					_ = t.register()
				}
			case <-t.stop:
				return
			}
		}
	}()
	return t, nil
}

func (t *Tray) register() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return t.conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher").CallWithContext(ctx, "org.kde.StatusNotifierWatcher.RegisterStatusNotifierItem", 0, string(itemPath)).Err
}

func (t *Tray) SetIcon(data []byte, _ bool) error {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	bounds := img.Bounds()
	pixels := make([]byte, 0, bounds.Dx()*bounds.Dy()*4)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			pixels = append(pixels, byte(a>>8), byte(r>>8), byte(g>>8), byte(b>>8))
		}
	}
	t.props.SetMust(itemInterface, "IconPixmap", []pixmap{{int32(bounds.Dx()), int32(bounds.Dy()), pixels}})
	return t.conn.Emit(itemPath, itemInterface+".NewIcon")
}
func (t *Tray) OnClick(fn func()) func() {
	t.mu.Lock()
	t.click = fn
	t.mu.Unlock()
	return func() { t.mu.Lock(); t.click = nil; t.mu.Unlock() }
}
func (t *Tray) OnRightClick(func()) func() { return func() {} }
func (t *Tray) SetMenu(menu *mygo.Menu) {
	t.mu.Lock()
	t.items = nil
	t.menu = menu
	if menu != nil {
		t.items = menu.Items()
	}
	t.revision++
	revision := t.revision
	t.mu.Unlock()
	t.conn.Emit(menuPath, menuInterface+".LayoutUpdated", revision, int32(0))
}
func (t *Tray) Bounds() mygo.Rectangle { return mygo.Rectangle{} }
func (t *Tray) PopUpMenu() {
	t.mu.Lock()
	menu := t.menu
	t.mu.Unlock()
	if menu != nil {
		menu.Popup(nil)
	}
}
func (t *Tray) Destroy() { t.once.Do(func() { close(t.stop); t.conn.Close() }) }

type itemMethods struct{ t *Tray }

func (m *itemMethods) Activate(x, y int32) *dbus.Error {
	m.t.mu.Lock()
	fn := m.t.click
	m.t.mu.Unlock()
	if fn != nil {
		mygo.RunOnMain(fn)
	}
	return nil
}
func (m *itemMethods) SecondaryActivate(x, y int32) *dbus.Error           { return m.Activate(x, y) }
func (m *itemMethods) ContextMenu(x, y int32) *dbus.Error                 { mygo.RunOnMain(m.t.PopUpMenu); return nil }
func (m *itemMethods) Scroll(delta int32, orientation string) *dbus.Error { return nil }

type menuMethods struct{ t *Tray }

func itemProperties(item *mygo.MenuItem) map[string]dbus.Variant {
	if item.Type == mygo.MenuItemSeparator {
		return map[string]dbus.Variant{"type": dbus.MakeVariant("separator")}
	}
	return map[string]dbus.Variant{"label": dbus.MakeVariant(item.Label), "enabled": dbus.MakeVariant(!item.Disabled), "visible": dbus.MakeVariant(!item.Hidden)}
}
func (m *menuMethods) GetLayout(parent, depth int32, names []string) (uint32, layout, *dbus.Error) {
	m.t.mu.Lock()
	defer m.t.mu.Unlock()
	root := layout{0, map[string]dbus.Variant{"children-display": dbus.MakeVariant("submenu")}, []dbus.Variant{}}
	if parent != 0 {
		return m.t.revision, root, dbus.NewError("com.canonical.dbusmenu.Error.InvalidMenu", nil)
	}
	if depth != 0 {
		for i, item := range m.t.items {
			root.Children = append(root.Children, dbus.MakeVariant(layout{int32(i + 1), itemProperties(item), []dbus.Variant{}}))
		}
	}
	return m.t.revision, root, nil
}
func (m *menuMethods) GetGroupProperties(ids []int32, names []string) ([]menuProperty, *dbus.Error) {
	m.t.mu.Lock()
	defer m.t.mu.Unlock()
	result := []menuProperty{}
	if len(ids) == 0 {
		for i := range m.t.items {
			ids = append(ids, int32(i+1))
		}
	}
	for _, id := range ids {
		if id > 0 && int(id) <= len(m.t.items) {
			result = append(result, menuProperty{id, itemProperties(m.t.items[id-1])})
		}
	}
	return result, nil
}
func (m *menuMethods) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	props, _ := m.GetGroupProperties([]int32{id}, nil)
	if len(props) > 0 {
		if value, ok := props[0].Properties[name]; ok {
			return value, nil
		}
	}
	return dbus.MakeVariant(""), dbus.NewError("com.canonical.dbusmenu.Error.InvalidProperty", nil)
}
func (m *menuMethods) AboutToShow(id int32) (bool, *dbus.Error) { return false, nil }
func (m *menuMethods) AboutToShowGroup(ids []int32) ([]int32, []int32, *dbus.Error) {
	return []int32{}, []int32{}, nil
}
func (m *menuMethods) Event(id int32, event string, data dbus.Variant, timestamp uint32) *dbus.Error {
	// Resolve on the UI thread so menu replacement and callbacks stay ordered.
	mygo.RunOnMain(func() {
		m.t.mu.Lock()
		var item *mygo.MenuItem
		if id > 0 && int(id) <= len(m.t.items) {
			item = m.t.items[id-1]
		}
		m.t.mu.Unlock()
		if event == "clicked" && item != nil && !item.Disabled && !item.Hidden && item.Click != nil {
			item.Click(item, nil)
		}
	})
	return nil
}

type menuEvent struct {
	ID        int32
	Event     string
	Data      dbus.Variant
	Timestamp uint32
}

func (m *menuMethods) EventGroup(events []menuEvent) ([]int32, *dbus.Error) {
	for _, event := range events {
		m.Event(event.ID, event.Event, event.Data, event.Timestamp)
	}
	return []int32{}, nil
}
