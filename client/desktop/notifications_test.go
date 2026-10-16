package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestNotificationsNeverMoveContentAndReplaceEarlierOutcome(t *testing.T) {
	a := fixture()
	v := ui.NewTester(testView(a), 1120, 760)
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
