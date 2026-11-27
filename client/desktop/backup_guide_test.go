package desktop

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestRecoveryGuideSkipsAndResumesForCurrentVault(t *testing.T) {
	for _, lang := range []string{"en", "zh-CN"} {
		t.Run(lang, func(t *testing.T) {
			a := fixture()
			a.state.Settings.Language = lang
			a.page, a.state.VaultID = "backup-guide", "pending"
			a.state.Vaults = []app.VaultInfo{
				{ID: "verified", RecoveryKeyStage: app.RecoveryKeyVerified},
				{ID: "pending", RecoveryKeyStage: app.RecoveryKeyExported},
			}
			view := ui.NewTester(testView(a), 1000, 950)
			if !view.HasText(a.t("恢复私钥已导出，等待验证。")) {
				t.Fatal("guide did not show this vault's pending step")
			}
			if err := view.Click(a.t("稍后再做")); err != nil || a.page != "entries" {
				t.Fatal("could not skip the guide", err)
			}
			a.navigateSettingsCategory("vault")
			view.Frame()
			if err := view.Click(a.t("查看备份步骤")); err != nil || a.page != "backup-guide" {
				t.Fatal("could not resume from settings", err)
			}
			a.state.VaultID = "verified"
			view.Frame()
			if !view.HasText(a.t("本机已验证此密码库的恢复私钥。")) || view.HasText(a.t("稍后再做")) {
				t.Fatal("guide showed the other vault's status")
			}
			if err := view.Click(a.t("完成")); err != nil || a.page != "entries" {
				t.Fatal("could not finish the guide", err)
			}
		})
	}
}

func TestGuidedExportOpensVerificationForTheSavedFile(t *testing.T) {
	a, callbacks, _ := keyExportFixture()
	a.page = "backup-guide"
	path := filepath.Join(t.TempDir(), "backup key.asc")
	a.preferences.saveDialog = func(mygo.SaveDialogOptions) (string, error) { return path, nil }
	a.exportKey()
	awaitMain(t, callbacks)
	awaitMain(t, callbacks)
	if !a.backupOpen || a.backupPath != path || a.backupPassphrase != "" {
		t.Fatal("successful export did not offer verification of the chosen file")
	}
	a.backupPassphrase = "sample input"
	a.navigate("entries", "")
	if a.backupOpen || a.backupPassphrase != "" || a.backupPath != "" {
		t.Fatal("leaving the guide retained verification inputs")
	}
}
