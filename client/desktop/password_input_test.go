package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestUnlockPasswordVisibilityPreservesInputAcrossLanguageChange(t *testing.T) {
	a := fixture()
	a.state.Locked = true
	a.clearSecrets()
	view := ui.NewTester(testView(a), 900, 680)
	view.Type("fictional-passphrase-密码")
	secret := a.passphrase
	if secret == "" || !view.HasText("显示私钥口令") {
		t.Fatal("unlock input must accept text while masking it by default")
	}
	if err := view.Click("显示私钥口令"); err != nil {
		t.Fatal(err)
	}
	if !view.HasText("隐藏私钥口令") || a.passphrase != secret {
		t.Fatal("revealing changed or failed to show the entered passphrase")
	}
	a.settings.Language = "en"
	view.Frame()
	if !view.HasText(a.t("隐藏%s", a.t("私钥口令"))) || a.passphrase != secret {
		t.Fatal("language change cleared the input or reset visibility")
	}
	if err := view.Click(a.t("隐藏%s", a.t("私钥口令"))); err != nil {
		t.Fatal(err)
	}
	if !view.HasText(a.t("显示%s", a.t("私钥口令"))) || a.passphrase != secret {
		t.Fatal("hiding changed the passphrase or left it visible")
	}
}

func TestPasswordInputsRevealIndependentlyAndResetAfterClearing(t *testing.T) {
	a := fixture()
	first, second := "first-fictional-secret", "second-fictional-secret"
	disabled, mounted := false, true
	view := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			if mounted {
				a.passwordInput(c, &first, a.t("私钥口令"), "", disabled)
				a.passwordInput(c, &second, a.t("确认口令"), "", false)
			}
		})
	}, 460, 240)
	assertPasswordInputMode(t, view, "私钥口令", first, true)
	assertPasswordInputMode(t, view, "确认口令", second, true)
	if err := view.Click("显示私钥口令"); err != nil {
		t.Fatal(err)
	}
	assertPasswordInputMode(t, view, "私钥口令", first, false)
	assertPasswordInputMode(t, view, "确认口令", second, true)
	disabled = true
	view.Frame()
	if err := view.Click("隐藏私钥口令"); err != nil {
		t.Fatal(err)
	}
	if !view.HasText("隐藏私钥口令") {
		t.Fatal("disabled input allowed visibility to change")
	}
	disabled, first = false, ""
	view.Frame()
	first = "replacement-fictional-secret"
	view.Frame()
	assertPasswordInputMode(t, view, "私钥口令", first, true)
	if err := view.Click("显示私钥口令"); err != nil {
		t.Fatal(err)
	}
	mounted = false
	view.Frame()
	mounted = true
	view.Frame()
	assertPasswordInputMode(t, view, "私钥口令", first, true)
	assertPasswordInputMode(t, view, "确认口令", second, true)
}

func TestEmptyPasswordCanBeRevealedBeforeTypingAndStillSubmit(t *testing.T) {
	a := fixture()
	value, submitted := "", false
	view := ui.NewTester(func(c *ui.Context) {
		if a.passwordInput(c, &value, a.t("私钥口令"), "", false).Submitted() {
			submitted = true
		}
	}, 460, 100)
	if err := view.Click("显示私钥口令"); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("私钥口令"); err != nil {
		t.Fatal(err)
	}
	view.Type("fictional-secret")
	if value != "fictional-secret" {
		t.Fatal("reveal on an empty field did not apply to subsequent input")
	}
	assertPasswordInputMode(t, view, "私钥口令", value, false)
	view.Key(0, ui.KeyEnter)
	if !submitted || value != "fictional-secret" {
		t.Fatal("visibility toggle disrupted submit handling")
	}
}

// MyGo omits editable text from Tester.Texts. Its password mode also prevents
// copying, so exercise that behavior rather than treating labels as contents.
func assertPasswordInputMode(t *testing.T, view *ui.Tester, label, value string, masked bool) {
	t.Helper()
	if err := view.Click(label); err != nil {
		t.Fatal(err)
	}
	view.SetClipboard("")
	view.Command("selectAll")
	view.Command("copy")
	want := value
	if masked {
		want = ""
	}
	if view.Clipboard() != want {
		t.Fatalf("input %q has incorrect password mode (masked=%v)", label, masked)
	}
}
