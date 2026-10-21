package desktop

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

// Exercise real encrypted storage through the same controls and asynchronous
// callbacks used by the native window, with exclusively generated preview data.
func TestPreviewUIWithRealVault(t *testing.T) {
	directory := t.TempDir()
	service, err := app.New(app.Options{DataDir: directory, Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	settings := service.Execute(app.Request{Action: "configure"}).Data.Settings
	settings.Language = "zh-CN"
	if response := service.Execute(app.Request{Action: "settings", Settings: &settings}); response.Status != "ok" {
		t.Fatal(response.Message)
	}
	a := newApplication(service)
	a.state = service.Execute(app.Request{Action: "configure"}).Data
	callbacks := make(chan func(), 16)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	view := ui.NewTester(testView(a), 1120, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	settle := func() {
		t.Helper()
		for a.busy {
			awaitMain(t, callbacks)
		}
		view.Frame()
		if a.errorText != "" {
			t.Fatal(a.errorText)
		}
	}
	click := func(label string) {
		t.Helper()
		if err := view.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	fill := func(label, value string) {
		t.Helper()
		click(label)
		if !view.Focused(label) {
			// field() emits a visible label before the identically labeled
			// input. Find returns that first label; its input follows a 6 DIP gap.
			bounds, ok := view.Find(label)
			if !ok {
				t.Fatalf("missing field %s", label)
			}
			view.ClickAt(bounds.X+16, bounds.Y+bounds.H+6+12)
		}
		if !view.Focused(label) {
			t.Fatalf("field did not receive focus: %s", label)
		}
		view.Key(ui.Cmd, ui.KeyA)
		view.Type(value)
	}

	// Select, edit and persist through the actual editor.
	a.selectEntry("工作/GitHub")
	settle()
	click("编辑条目")
	fill("用户名", "windows-smoke@example.test")
	click("保存")
	settle()
	if a.editor.open || a.detail.Username != "windows-smoke@example.test" {
		t.Fatalf("editor did not save to the real vault: open=%v editorUsername=%q detailUsername=%q busy=%v", a.editor.open, a.editor.username, a.detail.Username, a.busy)
	}
	managerResizeScreenshot(t, view, "preview-saved")

	// Creating a new encrypted entry also verifies text input and file creation.
	a.newEntry()
	settle()
	if !a.editor.open {
		t.Fatalf("new entry did not open: busy=%v locked=%v pending=%+v", a.busy, a.state.Locked, a.pendingNewEntry)
	}
	fill("账号名称", "Windows smoke")
	fill("密码", "fictional-windows-secret-2026")
	click("保存")
	settle()
	if a.detail.Entry != "Windows smoke" {
		t.Fatalf("entry was not created: %q", a.detail.Entry)
	}
	ciphertext, err := os.ReadFile(filepath.Join(directory, "demo-vault", "Windows smoke.gpg"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("fictional-windows-secret-2026")) {
		t.Fatal("saved plaintext instead of ciphertext")
	}

	click("锁定密码库")
	deadline := time.Now().Add(3 * time.Second)
	for !service.IsLocked() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !service.IsLocked() || a.detail.Password != "" {
		t.Fatal("lock retained unlocked state or detail secrets")
	}
	view.Frame()
	managerResizeScreenshot(t, view, "preview-locked")
	fill("私钥口令", app.DemoPassphrase)
	view.Key(0, ui.KeyEnter)
	settle()
	if a.state.Locked || a.detail.Password != "fictional-windows-secret-2026" || a.passphrase != "" {
		t.Fatal("unlock did not restore the saved entry and clear the passphrase input")
	}
	if view.HasText(a.detail.Password) {
		t.Fatal("unlocked entry revealed its password without an explicit action")
	}
	click("显示或隐藏密码")
	if !view.HasText("fictional-windows-secret-2026") {
		t.Fatal("explicit reveal failed")
	}
	click("显示或隐藏密码")
	managerResizeScreenshot(t, view, "preview-unlocked")

	// A fresh service must load the edited ciphertext, not an in-memory copy.
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := app.New(app.Options{DataDir: directory, Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if response := reopened.Execute(app.Request{Action: "unlock", Passphrase: app.DemoPassphrase}); response.Status != "ok" {
		t.Fatal(response.Message)
	}
	response := reopened.Execute(app.Request{Action: "fetch", Entry: "工作/GitHub"})
	if response.Status != "ok" || response.Data.Username != "windows-smoke@example.test" {
		t.Fatal("edited account did not survive restart")
	}
}
