package desktop

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestDetailReadReplacesTheReadOnlyPresentationOnce(t *testing.T) {
	for _, width := range []int{860, 1120} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := fixture()
			a.clipboardWriter = func(string, bool) error {
				t.Fatal("a pending presentation copied the previous entry's path")
				return nil
			}
			a.detail.Username = strings.Repeat("long-account-", 8) + "@example.test"
			a.detail.Notes = strings.Repeat("previous note ", 20)
			a.detail.OTP = "123456"
			a.detail.Contents += "recovery: previous-custom-secret\n"
			a.reveal = true
			original, state := a.detail, a.state
			callbacks := make(chan func(), 1)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.service = backendFunc(func(request app.Request) app.Response {
				data := state
				data.Entry, data.Password, data.Notes = request.Entry, "replacement-password", "replacement note"
				return app.Response{Status: "ok", Data: data}
			})
			view := ui.NewTester(testView(a), width, 760)
			view.Advance(time.Second)
			positions := make(map[string]ui.Rect)
			for _, label := range []string{"账号字段", "编辑条目", "复制密码", "复制用户名", "复制网址"} {
				positions[label], _ = view.Find(a.t(label))
			}
			a.selectEntry("工作/Figma")
			view.Frame()
			assertWaiting := func() {
				t.Helper()
				for _, value := range []string{original.Username, original.URL, original.Notes} {
					if !view.HasText(value) {
						t.Fatalf("loading replaced the existing presentation with a blank or placeholder: %q", value)
					}
				}
				for _, secret := range []string{original.Password, original.OTP, "previous-custom-secret"} {
					if view.HasText(secret) || strings.Contains(fmt.Sprintf("%+v", a.detailTransition), secret) {
						t.Fatal("loading retained a revealed credential in the presentation")
					}
				}
				if a.detail.Entry != "" || a.detail.Contents != "" || a.detail.Password != "" {
					t.Fatal("the pending presentation remained usable as an active credential")
				}
				for label, before := range positions {
					if after, found := view.Find(a.t(label)); !found || after != before {
						t.Fatalf("loading moved %q: before=%v after=%v", label, before, after)
					}
				}
			}
			assertWaiting()
			a.detailLoadingStarted = view.Now().Add(-detailLoadingDelay - time.Millisecond)
			view.Frame()
			assertWaiting()
			if !view.HasText(a.t("正在读取账号…")) {
				t.Fatal("a slow read did not show its local feedback")
			}
			for _, label := range []string{"显示全部字段", "编辑条目", "移到回收站", "复制密码", "复制用户名", "打开网站", "复制条目完整路径"} {
				if err := view.Click(a.t(label)); err != nil {
					t.Fatal(err)
				}
			}
			if a.editor.open || a.deleteOpen || a.reveal || len(view.OpenedURLs()) != 0 {
				t.Fatal("a pending presentation allowed an action on the previous entry")
			}
			awaitMain(t, callbacks)
			view.Frame()
			if !view.HasText("replacement note") || view.HasText(original.Username) || view.HasText(original.Notes) || view.HasText(a.t("正在读取账号…")) {
				t.Fatal("the response did not replace the previous presentation in one frame")
			}
		})
	}
}

func TestUnlockWaitsForCompleteDetailWithoutAnIntermediateSkeleton(t *testing.T) {
	for _, browse := range []bool{false, true} {
		for _, copyEntry := range []bool{false, true} {
			t.Run(fmt.Sprintf("browse=%t/copy=%t", browse, copyEntry), func(t *testing.T) {
				a := fixture()
				state, original := a.state, a.detail
				state.Settings.BrowseWhileLocked = browse
				a.state.Settings.BrowseWhileLocked = browse
				a.clearSecrets()
				a.state.Locked = true
				if copyEntry {
					a.pendingCopyEntry, a.pendingCopyVaultID = a.selected, a.state.VaultID
				}
				callbacks := make(chan func(), 3)
				a.runOnMain = func(callback func()) { callbacks <- callback }
				a.service = backendFunc(func(request app.Request) app.Response {
					data := state
					if request.Action == "fetch" {
						data.Entry, data.Password, data.Username, data.URL, data.Notes = request.Entry, original.Password, original.Username, original.URL, original.Notes
					}
					return app.Response{Status: "ok", Data: data}
				})
				view := ui.NewTester(testView(a), 1120, 760)
				view.Advance(time.Second)
				unlockBounds, _ := view.Find(a.t("解锁密码库"))
				a.unlock("")
				awaitMain(t, callbacks)
				view.Frame()
				assertWaiting := func() {
					t.Helper()
					if !view.HasText(a.t("解锁密码库")) || view.HasText(a.t("选择一个条目")) {
						t.Fatal("unlock replaced its surface before the selected detail was ready")
					}
					if _, found := view.Find(a.t("账号字段")); found {
						t.Fatal("unlock showed a partial password-only credential card")
					}
					if bounds, found := view.Find(a.t("解锁密码库")); !found || bounds != unlockBounds {
						t.Fatalf("unlock moved its waiting surface: before=%v pending=%v", unlockBounds, bounds)
					}
				}
				assertWaiting()
				if copyEntry {
					awaitMain(t, callbacks)
					view.Frame()
					assertWaiting()
				}
				a.detailLoadingStarted = view.Now().Add(-detailLoadingDelay - time.Millisecond)
				view.Frame()
				assertWaiting()
				if !view.HasText(a.t("正在读取账号…")) {
					t.Fatal("unlock did not show feedback for a slow detail read")
				}
				awaitMain(t, callbacks)
				view.Frame()
				if !view.HasText(original.Username) || view.HasText(a.t("解锁密码库")) || view.HasText(a.t("正在读取账号…")) {
					t.Fatal("unlock did not replace its waiting surface with the complete detail")
				}
			})
		}
	}
}

func TestSlowDetailFeedbackStaysVisibleWithoutMovingScrolledContent(t *testing.T) {
	a := fixture()
	a.detail.Notes = strings.Repeat("long visible note ", 300)
	a.busy, a.backgroundBusy = true, true
	view := ui.NewTester(testView(a), 1120, 760)
	a.detailScroll.Y = a.detailScroll.MaxY
	view.Frame()
	previousOffset := a.detailScroll.Y
	if previousOffset <= 0 {
		t.Fatal("fixture did not scroll the long detail")
	}
	a.selectEntry("工作/Figma")
	a.detailLoadingStarted = view.Now().Add(-detailLoadingDelay - time.Millisecond)
	view.Frame()
	feedback, found := view.Find(a.t("正在读取账号…"))
	if !found || feedback.W <= 0 || feedback.H <= 0 || feedback.Y < 48 || feedback.Y+feedback.H > 760 {
		t.Fatalf("slow-read feedback scrolled out of the inspector: %v", feedback)
	}
	if a.detailScroll.Y != previousOffset {
		t.Fatal("showing feedback moved the previous presentation")
	}
}

func TestFirstDetailReadKeepsTheEmptySurfaceUntilReady(t *testing.T) {
	a := fixture()
	a.dropDetail()
	state := a.state
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		data := state
		data.Entry, data.Password = request.Entry, "synthetic-password"
		return app.Response{Status: "ok", Data: data}
	})
	view := ui.NewTester(testView(a), 1120, 760)
	before, _ := view.Find(a.t("选择一个条目"))
	a.selectEntry("工作/Figma")
	view.Frame()
	if after, found := view.Find(a.t("选择一个条目")); !found || before != after {
		t.Fatal("a first read flashed a blank inspector instead of retaining its empty surface")
	}
	if err := view.Click(a.t("新建条目")); err != nil {
		t.Fatal(err)
	}
	if a.editor.open {
		t.Fatal("the pending empty surface allowed a new editor")
	}
	awaitMain(t, callbacks)
	view.Frame()
	if view.HasText(a.t("选择一个条目")) || a.detail.Entry != a.selected {
		t.Fatal("the complete detail did not replace the empty surface")
	}
}

func TestFailedUnlockContinuationClearsTheWaitingSurface(t *testing.T) {
	for _, copyEntry := range []bool{false, true} {
		t.Run(fmt.Sprint(copyEntry), func(t *testing.T) {
			a := fixture()
			state := a.state
			a.clearSecrets()
			a.state.Locked = true
			if copyEntry {
				a.pendingCopyEntry, a.pendingCopyVaultID = a.selected, a.state.VaultID
			}
			callbacks := make(chan func(), 2)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.service = backendFunc(func(request app.Request) app.Response {
				if request.Action == "unlock" {
					return app.Response{Status: "ok", Data: state}
				}
				return app.Response{Status: "error", Message: "synthetic read failure", Data: state}
			})
			a.unlock("")
			awaitMain(t, callbacks)
			if !a.detailTransition.unlocking {
				t.Fatal("fixture did not enter the unlock continuation")
			}
			awaitMain(t, callbacks)
			if a.detailTransition.entry != "" || a.detailTransition.unlocking || !a.detailLoadingStarted.IsZero() {
				t.Fatal("failed continuation retained an unlock waiting surface")
			}
			// An unrelated metadata read must not revive a failed transition.
			a.busy, a.backgroundBusy = true, true
			if a.detailTransitionPresentation(time.Now()).Pending {
				t.Fatal("a later metadata read revived a failed unlock continuation")
			}
		})
	}
}
