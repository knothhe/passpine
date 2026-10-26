package desktop

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestFeedbackExpiresWhileWindowIsUnfocusedAndReplacementRestartsDeadline(t *testing.T) {
	a := fixture()
	v := ui.NewTester(testView(a), 1120, 760)
	v.SetPreferences(ui.Preferences{ReduceMotion: true})
	a.notice = "已复制密码"
	v.Frame()
	first := a.notifications.expiresAt
	if first.IsZero() || !v.HasText("已复制密码") {
		t.Fatal("copy feedback did not start its deadline")
	}
	v.SetFocused(false)
	// Simulate a frame after the deadline without sleeping or relying on the
	// underlying toolkit's focus-paused toast timer.
	a.notifications.expiresAt = time.Now().Add(-time.Second)
	v.Frame()
	if v.HasText("已复制密码") || !a.notifications.expiresAt.IsZero() {
		t.Fatal("background focus paused operation feedback expiry")
	}
	a.warn("请先保存或关闭当前编辑的条目。")
	v.Frame()
	if !a.notifications.expiresAt.After(first) {
		t.Fatal("new feedback inherited an earlier deadline")
	}
	a.errorText = "保存失败，请重试。"
	v.Frame()
	if !a.notifications.expiresAt.IsZero() || !v.HasText(a.t("保存失败，请重试。")) {
		t.Fatal("persistent failure inherited the previous success deadline")
	}
}

func TestNotificationsNeverMoveContentAndReplaceEarlierOutcome(t *testing.T) {
	a := fixture()
	v := ui.NewTester(testView(a), 1120, 760)
	v.SetLocale(a.uiLocale())
	v.SetPreferences(ui.Preferences{ReduceMotion: true})
	before, _ := v.Find("显示或隐藏密码")
	a.notice = "已复制密码"
	v.Frame()
	if !v.HasText("已复制密码") {
		t.Fatal("operation feedback is missing")
	}
	a.errorText = "无法保存条目，请重试。"
	v.Frame()
	if !v.HasText("操作未完成") || !v.HasText("无法保存条目，请重试。") || v.HasText("已复制密码") {
		t.Fatal("failure did not replace the earlier outcome")
	}
	if after, _ := v.Find("显示或隐藏密码"); after != before {
		t.Fatal("notification shifted the detail controls")
	}
	if err := v.Click("关闭提示"); err != nil {
		t.Fatal(err)
	}
	v.Frame()
	if v.HasText("无法保存条目，请重试。") {
		t.Fatal("dismissed failure remained visible")
	}
}

func TestUnlockFeedbackSurvivesFollowupFetch(t *testing.T) {
	a := fixture()
	a.state.Locked = true
	a.detail = app.Data{}
	state := a.state
	state.Locked = false
	state.DeviceUnlockNotice = "系统解锁未启用，请在设置中重试。"
	callbacks := make(chan func(), 2)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		data := state
		if request.Action == "fetch" {
			data.Entry = request.Entry
		}
		return app.Response{Status: "ok", Data: data}
	})
	a.unlock("")
	awaitMain(t, callbacks)
	awaitMain(t, callbacks)
	v := ui.NewTester(testView(a), 1120, 760)
	if !v.HasText(state.DeviceUnlockNotice) || a.detail.Entry != a.selected {
		t.Fatal("follow-up entry fetch swallowed the unlock feedback")
	}
}

func TestDialogFailureUsesSingleGlobalFeedbackAndLockClearsIt(t *testing.T) {
	a := fixture()
	a.editEntry()
	a.errorText = "保存失败，请重试。"
	v := ui.NewTester(testView(a), 1120, 760)
	v.SetPreferences(ui.Preferences{ReduceMotion: true})
	count := 0
	for _, text := range v.Texts() {
		if text == "保存失败，请重试。" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("dialog error was shown %d times", count)
	}
	a.clearSecrets()
	v.Frame()
	v.Frame()
	if v.HasText("保存失败，请重试。") {
		t.Fatal("feedback survived clearing secrets")
	}
}

func TestActionNotificationReplacingExpiredToastKeepsOnlyNewAction(t *testing.T) {
	a := fixture()
	v := ui.NewTester(testView(a), 1120, 760)
	v.SetPreferences(ui.Preferences{ReduceMotion: true})
	oldCalls, newCalls := 0, 0
	a.notifyWithAction(a.t("已将 %d 项移动到“%s”", 1, "归档"), "", a.t("撤销"), 8*time.Second, func() { oldCalls++ })
	oldAction := a.notifications.onAction
	v.Frame()
	if a.notifications.expiresAt.IsZero() {
		t.Fatal("the old toast did not start its deadline")
	}
	// A new completion can arrive after the old toast's deadline but before
	// the next frame has expired it. Do not let that expiry consume the new
	// action's serial while the replacement is being painted.
	a.notifications.expiresAt = time.Now().Add(-time.Second)
	a.notifyWithAction(a.t("已修改 %d 项的文件夹前缀", 1), "", a.t("撤销"), 8*time.Second, func() { newCalls++ })
	newAction := a.notifications.onAction
	v.Frame()
	oldAction()
	if err := v.Click(a.t("撤销")); err != nil {
		t.Fatal(err)
	}
	if oldCalls != 0 || newCalls != 1 {
		t.Fatalf("replacement toast executed the wrong action: old=%d new=%d", oldCalls, newCalls)
	}
	oldAction()
	newAction()
	if oldCalls != 0 || newCalls != 1 {
		t.Fatal("an obsolete or already consumed toast action ran again")
	}
}

func TestPersistentErrorActionSurvivesTimeAndRunsOnce(t *testing.T) {
	a := fixture()
	v := ui.NewTester(testView(a), 1120, 760)
	v.SetPreferences(ui.Preferences{ReduceMotion: true})
	calls := 0
	message := a.t("已撤销 %d 项，%d 项未完成", 1, 1)
	a.notifyErrorWithAction(message, "", a.t("重试撤销"), func() { calls++ })
	action := a.notifications.onAction
	v.Frame()
	v.Advance(10 * time.Second)
	if !v.HasText(message) || !a.notifications.expiresAt.IsZero() {
		t.Fatal("a persistent undo failure inherited a toast expiry")
	}
	if err := v.Click(a.t("重试撤销")); err != nil {
		t.Fatal(err)
	}
	action()
	if calls != 1 {
		t.Fatalf("persistent action executed %d times, want once", calls)
	}
}

func TestSuccessActionReplacingPersistentErrorInvalidatesOldAction(t *testing.T) {
	a := fixture()
	v := ui.NewTester(testView(a), 1120, 760)
	v.SetPreferences(ui.Preferences{ReduceMotion: true})
	oldCalls, newCalls := 0, 0
	oldMessage := a.t("已撤销 %d 项，%d 项未完成", 1, 1)
	a.notifyErrorWithAction(oldMessage, "", a.t("重试撤销"), func() { oldCalls++ })
	oldAction := a.notifications.onAction
	v.Frame()
	newMessage := a.t("已将 %d 项移到回收站", 2)
	a.notifyWithAction(newMessage, "", a.t("撤销"), 8*time.Second, func() { newCalls++ })
	newAction := a.notifications.onAction
	v.Frame()
	if v.HasText(oldMessage) || !v.HasText(newMessage) {
		t.Fatal("new success did not replace the persistent failure")
	}
	oldAction()
	if err := v.Click(a.t("撤销")); err != nil {
		t.Fatal(err)
	}
	newAction()
	if oldCalls != 0 || newCalls != 1 {
		t.Fatalf("replacing a persistent toast corrupted action ownership: old=%d new=%d", oldCalls, newCalls)
	}
}

func TestDisplayedActionIsInvalidatedByNewOperation(t *testing.T) {
	a := fixture()
	v := ui.NewTester(testView(a), 1120, 760)
	v.SetPreferences(ui.Preferences{ReduceMotion: true})
	calls := 0
	a.notifyWithAction(a.t("已修改 %d 项的文件夹前缀", 1), "", a.t("撤销"), 8*time.Second, func() { calls++ })
	oldAction := a.notifications.onAction
	v.Frame()
	if a.notice != "" || !v.HasText(a.t("撤销")) {
		t.Fatal("the earlier actionable outcome was not painted")
	}
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	state := a.state
	a.service = backendFunc(func(app.Request) app.Response {
		return app.Response{Status: "ok", Data: state}
	})
	a.call(app.Request{Action: "list"}, nil)
	awaitMain(t, callbacks)
	oldAction()
	if calls != 0 {
		t.Fatal("a previously displayed action survived a later explicit operation")
	}
}
