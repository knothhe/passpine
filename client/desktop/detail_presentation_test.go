package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine-neo/client/app"
)

func TestDetailRevealAllKeepsIndividualFieldsIndependent(t *testing.T) {
	a := fixture()
	a.detail.OTP = "123456"
	a.detail.Contents += "recovery: synthetic-recovery-code\n"
	view := ui.NewTester(testView(a), 1120, 760)
	secrets := []string{a.detail.Password, a.detail.OTP, "synthetic-recovery-code"}
	assertVisible := func(want ...bool) {
		t.Helper()
		view.Frame()
		for i, value := range secrets {
			if got := view.HasText(value); got != want[i] {
				t.Fatalf("secret %d visible=%t, want %t", i, got, want[i])
			}
		}
	}
	click := func(label string) {
		t.Helper()
		if err := view.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	assertVisible(false, false, false)
	click(a.t("显示或隐藏%s", a.t("密码")))
	assertVisible(true, false, false)
	click(a.t("显示全部字段"))
	assertVisible(true, true, true)
	click(a.t("显示或隐藏%s", a.t("密码")))
	assertVisible(false, true, true)
	click(a.t("显示全部字段"))
	assertVisible(true, true, true)
	click(a.t("隐藏全部字段"))
	assertVisible(false, false, false)
}

func TestDetailRevealAllIsImmediatelyBeforeEdit(t *testing.T) {
	for _, size := range [][2]int{{860, 620}, {1120, 760}} {
		a := fixture()
		view := ui.NewTester(testView(a), size[0], size[1])
		reveal, found := view.Find(a.t("显示全部字段"))
		if !found {
			t.Fatal("show-all control is missing")
		}
		edit, found := view.Find(a.t("编辑条目"))
		if !found || reveal.X+reveal.W > edit.X || edit.X-(reveal.X+reveal.W) > 8 || reveal.Y != edit.Y {
			t.Fatalf("show-all is not immediately before Edit: reveal=%v edit=%v", reveal, edit)
		}
		if reveal.X < 310 || edit.X+edit.W > float32(size[0]) {
			t.Fatalf("detail actions overflowed the detail pane: reveal=%v edit=%v", reveal, edit)
		}
	}
}

func TestDetailRevealStateClearsAtSecretBoundaries(t *testing.T) {
	for _, clear := range []string{"detail", "session"} {
		t.Run(clear, func(t *testing.T) {
			a := fixture()
			a.reveal = true
			a.revealedFields = map[string]bool{"password": true, "custom:0": true}
			if clear == "detail" {
				a.dropDetail()
			} else {
				a.clearSecrets()
			}
			if a.reveal || len(a.revealedFields) != 0 {
				t.Fatal("field visibility survived a secret boundary")
			}
		})
	}
}

func TestDetailDeleteFollowsEditAndUsesConfirmation(t *testing.T) {
	for _, size := range [][2]int{{860, 620}, {1120, 760}} {
		a := fixture()
		view := ui.NewTester(testView(a), size[0], size[1])
		edit, found := view.Find(a.t("编辑条目"))
		if !found {
			t.Fatal("edit control missing")
		}
		remove, found := view.Find(a.t("移到回收站"))
		if !found || remove.X < edit.X+edit.W || remove.Y != edit.Y || remove.X+remove.W > float32(size[0]) {
			t.Fatalf("delete is not beside Edit: edit=%v delete=%v", edit, remove)
		}
		copyControls := 0
		for _, label := range view.Texts() {
			if label == a.t("复制密码") {
				copyControls++
			}
		}
		if copyControls != 1 {
			t.Fatalf("expected one password field copy control, got %d", copyControls)
		}
		if err := view.Click(a.t("移到回收站")); err != nil {
			t.Fatal(err)
		}
		if !a.deleteOpen {
			t.Fatal("delete skipped its confirmation")
		}
	}
}

func TestEnglishDetailCopyUsesStableBackendFields(t *testing.T) {
	for _, field := range []struct{ key, label string }{{"password", "密码"}, {"username", "用户名"}, {"otp", "一次性密码"}} {
		t.Run(field.key, func(t *testing.T) {
			a := fixture()
			a.state.Settings.Language = "en"
			a.detail.OTP = "123456"
			callbacks := make(chan func(), 1)
			requests := make(chan app.Request, 1)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.service = backendFunc(func(req app.Request) app.Response {
				requests <- req
				return app.Response{Status: "ok", Data: a.state}
			})
			view := ui.NewTester(testView(a), 1120, 760)
			if err := view.Click(a.t("复制%s", a.t(field.label))); err != nil {
				t.Fatal(err)
			}
			awaitMain(t, callbacks)
			req := <-requests
			if req.Action != "copy" || req.Field != field.key || req.Entry != a.detail.Entry {
				t.Fatalf("localized copy lost its backend field: %#v", req)
			}
		})
	}
}
