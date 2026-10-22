package desktop

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestCreateKeyAndVaultValidatesNewPassphraseAndClearsSecrets(t *testing.T) {
	a := newApplication(nil)
	a.state.Settings.Language = "zh-CN"
	a.setup = true
	a.setupDirectory = filepath.Join(t.TempDir(), "new-vault")
	a.setupName, a.setupEmail = "My vault", "new@example.invalid"
	a.passphrase, a.confirmPassphrase = "new-private-passphrase", "different-passphrase"
	// An earlier import selection must not be used when creating a new key.
	a.setupKeySource = 1
	a.setupPrivateKey, a.setupPublicKey = "/old-private.asc", "/old-public.asc"
	callbacks := make(chan func(), 1)
	requests := make(chan app.Request, 2)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(req app.Request) app.Response {
		requests <- req
		return app.Response{Status: "ok", Data: app.Data{Configured: true, Settings: app.DefaultSettings()}}
	})
	view := ui.NewTester(a.setupView, 740, 1000)
	if !view.HasText(a.t("首次使用？创建密码库时会在本机生成受口令保护的 OpenPGP（GPG）私钥，无需安装 GnuPG 或准备已有密钥。")) || view.HasText(a.t("密钥来源")) {
		t.Fatal("new users were not shown the local key creation flow")
	}
	if err := view.Click(a.t("创建密钥与密码库")); err != nil {
		t.Fatal(err)
	}
	if a.errorText != a.t("口令至少需要 12 个字符，且两次输入必须一致。") || len(requests) != 0 {
		t.Fatal("mismatched new passphrase was submitted")
	}
	a.confirmPassphrase = a.passphrase
	a.state.Settings.Language = "en"
	view.Frame()
	if !view.HasText("Set private key passphrase") || a.passphrase != "new-private-passphrase" || a.confirmPassphrase != a.passphrase {
		t.Fatal("language switch lost the new passphrase or kept the old labels")
	}
	if err := view.Click("Create key and vault"); err != nil {
		t.Fatal(err)
	}
	a.submitSetup()
	awaitMain(t, callbacks)
	req := <-requests
	if req.Action != "create" || req.Mode != "" || req.KeyFile != "" || req.PublicKeyFile != "" || req.Directory != a.setupDirectory || req.Name != "My vault" || req.Email != "new@example.invalid" || req.Passphrase != "new-private-passphrase" {
		t.Fatal("creating a new key reused import inputs or lost the new identity")
	}
	if len(requests) != 0 || a.setup || a.passphrase != "" || a.confirmPassphrase != "" || a.setupPrivateKey != "" || a.setupPublicKey != "" {
		t.Fatal("successful creation submitted twice or retained onboarding secrets")
	}
	if a.notice != a.t("密码库与私钥已创建。请在设置的“密码库”中导出恢复私钥并验证备份。") {
		t.Fatal("new vault did not explain how to back up the generated key")
	}
}

func TestCreatedVaultBackupGuidanceSurvivesDeviceUnlockNotice(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		for _, deviceNotice := range []string{"已启用系统解锁", "系统解锁未启用，可继续使用私钥口令"} {
			t.Run(language+"/"+deviceNotice, func(t *testing.T) {
				a := newApplication(nil)
				a.state.Settings.Language = language
				a.setup = true
				a.setupDirectory = filepath.Join(t.TempDir(), "new-vault")
				a.passphrase, a.confirmPassphrase = "new-private-passphrase", "new-private-passphrase"
				callbacks := make(chan func(), 1)
				a.runOnMain = func(callback func()) { callbacks <- callback }
				settings := app.DefaultSettings()
				settings.Language = language
				a.service = backendFunc(func(req app.Request) app.Response {
					return app.Response{Status: "ok", Data: app.Data{Configured: true, Settings: settings, DeviceUnlockNotice: deviceNotice}}
				})
				a.submitSetup()
				awaitMain(t, callbacks)
				expected := a.t("密码库与私钥已创建。%s。请在设置的“密码库”中导出恢复私钥并验证备份。", a.t(deviceNotice))
				if a.notice != expected {
					t.Fatalf("device unlock result replaced the new key's backup guidance: %q", a.notice)
				}
				view := ui.NewTester(testView(a), 860, 620)
				view.SetPreferences(ui.Preferences{ReduceMotion: true})
				assertTypographyBounds(t, view, 860, 620, expected)
			})
		}
	}
}

func TestImportKeySourceSwitchAndLanguagePreserveInputs(t *testing.T) {
	a := newApplication(nil)
	a.state.Settings.Language = "zh-CN"
	a.setupMode = 1
	a.setupDirectory = "/Users/example/.password-store"
	a.setupPrivateKey, a.setupPublicKey = "/Users/example/private.asc", "/Users/example/public.pgp"
	a.passphrase = "fixture-private-passphrase"
	view := ui.NewTester(a.setupView, 740, 900)
	if view.HasText("OpenPGP 私钥文件") {
		t.Fatal("automatic import unexpectedly asks for a key file")
	}
	if err := view.Click("密钥文件"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"OpenPGP 私钥文件", "OpenPGP 公钥文件（可选）", "选择私钥文件", "选择公钥文件"} {
		if _, ok := view.Find(label); !ok {
			t.Fatalf("manual key import is missing %q", label)
		}
	}
	if err := view.Click("系统 GPG"); err != nil {
		t.Fatal(err)
	}
	if view.HasText("OpenPGP 私钥文件") {
		t.Fatal("manual-only fields remained in automatic mode")
	}
	if err := view.Click("密钥文件"); err != nil {
		t.Fatal(err)
	}
	a.state.Settings.Language = "en"
	view.Frame()
	for _, label := range []string{"Key source", "System GPG", "Key files", "OpenPGP private key file", "OpenPGP public key file (optional)", "Choose private key file", "Choose public key file"} {
		if _, ok := view.Find(label); !ok {
			t.Errorf("English import control %q is missing", label)
		}
	}
	if a.setupKeySource != 1 || a.setupDirectory != "/Users/example/.password-store" || a.setupPrivateKey != "/Users/example/private.asc" || a.setupPublicKey != "/Users/example/public.pgp" || a.passphrase != "fixture-private-passphrase" {
		t.Fatal("switching key source or language changed import inputs")
	}
}

func TestManualKeyImportValidatesAndRetainsInputsUntilSuccess(t *testing.T) {
	a := newApplication(nil)
	a.state.Settings.Language = "zh-CN"
	a.setup, a.setupMode, a.setupKeySource = true, 1, 1
	a.setupDirectory = filepath.Join(t.TempDir(), ".password-store")
	a.passphrase = "fixture-private-passphrase"
	callbacks := make(chan func(), 1)
	requests := make(chan app.Request, 2)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	response := app.Response{Status: "error", Message: "私钥口令错误或私钥不可用", Data: a.state}
	a.service = backendFunc(func(req app.Request) app.Response {
		requests <- req
		return response
	})
	a.submitSetup()
	if a.errorText != "请先选择私钥文件。" || len(requests) != 0 || a.busy {
		t.Fatal("missing private key was submitted instead of validated locally")
	}
	a.setupPrivateKey = "/Users/example/private.asc"
	a.setupPublicKey = "/Users/example/public.pgp"
	view := ui.NewTester(a.setupView, 740, 900)
	if err := view.Click("打开并解锁"); err != nil {
		t.Fatal(err)
	}
	// Source changes and repeated submission must not mutate the pending import.
	_ = view.Click("系统 GPG")
	_ = view.Click("创建密码库")
	a.submitSetup()
	if a.setupKeySource != 1 || a.setupMode != 1 {
		t.Fatal("busy import form allowed changing mode")
	}
	awaitMain(t, callbacks)
	req := <-requests
	if req.Action != "open" || req.Mode != "key-file" || req.KeyFile != a.setupPrivateKey || req.PublicKeyFile != a.setupPublicKey || req.Passphrase != a.passphrase {
		t.Fatal("manual import did not send both selected key files")
	}
	if a.busy || !a.setup || a.passphrase == "" || a.setupPrivateKey == "" || a.setupPublicKey == "" || a.errorText != "私钥口令错误或私钥不可用" || len(requests) != 0 {
		t.Fatal("failed import lost inputs, stayed busy, or submitted twice")
	}
	response = app.Response{Status: "ok", Data: app.Data{Configured: true, Settings: app.DefaultSettings()}}
	a.submitSetup()
	awaitMain(t, callbacks)
	<-requests
	if a.setup || a.passphrase != "" || a.setupPrivateKey != "" || a.setupPublicKey != "" {
		t.Fatal("successful import retained onboarding secrets or file selections")
	}
}

func TestAutomaticImportIgnoresSelectedManualKeyFiles(t *testing.T) {
	a := newApplication(nil)
	a.state.Settings.Language = "zh-CN"
	a.setupMode = 1
	a.setupDirectory = t.TempDir()
	a.setupPrivateKey, a.setupPublicKey = "/private.asc", "/public.asc"
	a.passphrase = "fixture-private-passphrase"
	callbacks := make(chan func(), 1)
	requests := make(chan app.Request, 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(req app.Request) app.Response {
		requests <- req
		return app.Response{Status: "error", Data: a.state}
	})
	a.submitSetup()
	awaitMain(t, callbacks)
	req := <-requests
	if req.Mode != "system-gpg" || req.KeyFile != "" || req.PublicKeyFile != "" {
		t.Fatal("automatic import reused hidden manual key file inputs")
	}
}

func TestManualImportCanScrollToSubmitInNarrowWindow(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		for _, scale := range []int{100, 120} {
			a := fixture()
			a.state.Configured = false
			a.state.Settings.Language, a.state.Settings.FontScale = language, scale
			a.setupMode, a.setupKeySource = 1, 1
			a.setupPrivateKey = "/Users/example/Keys/a-long-private-key-filename.asc"
			view := ui.NewTester(testView(a), 860, 620)
			view.Scroll(700, 500, 0, 1200)
			assertTypographyBounds(t, view, 860, 620, a.t("私钥口令"), a.t("打开并解锁"))
			if err := view.Click(a.t("打开并解锁")); err != nil {
				t.Fatal(err)
			}
			if !view.HasText(a.t("请先选择密码库目录。")) {
				t.Fatal("scrolled import submit did not run form validation")
			}
		}
	}
}

func TestCreateKeyCanScrollToConfirmationAndSubmitInNarrowWindow(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		for _, scale := range []int{100, 120} {
			a := fixture()
			a.state.Configured = false
			a.state.Settings.Language, a.state.Settings.FontScale = language, scale
			a.setupMode = 0
			a.setupDirectory = t.TempDir()
			view := ui.NewTester(testView(a), 860, 620)
			view.Scroll(700, 500, 0, 1200)
			assertTypographyBounds(t, view, 860, 620, a.t("确认口令"), a.t("创建密钥与密码库"))
			if err := view.Click(a.t("创建密钥与密码库")); err != nil {
				t.Fatal(err)
			}
			if !view.HasText(a.t("口令至少需要 12 个字符，且两次输入必须一致。")) {
				t.Fatal("scrolled create submit did not validate the new key passphrase")
			}
		}
	}
}
