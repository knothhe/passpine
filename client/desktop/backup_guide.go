package desktop

import (
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
)

func (a *application) recoveryKeyStage() app.RecoveryKeyStage {
	for _, info := range a.state.Vaults {
		if info.ID == a.state.VaultID {
			return info.RecoveryKeyStage
		}
	}
	return ""
}

func (a *application) recoveryKeyStatus() string {
	switch a.recoveryKeyStage() {
	case app.RecoveryKeyExported:
		return a.t("恢复私钥已导出，等待验证。")
	case app.RecoveryKeyVerified:
		return a.t("本机已验证此密码库的恢复私钥。")
	default:
		return a.t("本机尚无恢复私钥验证记录。")
	}
}

func (a *application) backupGuideView(c *ui.Context) {
	stage := a.recoveryKeyStage()
	components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(28).Gap(18).Children(func() {
			section(c, a.t("恢复私钥备份"), a.t("换设备恢复时，需要密码库目录的副本、恢复私钥和私钥口令。请单独备份密码库目录。"), nil)
			muted(c, a.recoveryKeyStatus())
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				components.Text(c, a.t("1. 导出受口令保护的私钥")).Bold().Wrap()
				muted(c, a.t("将私钥文件保存在这台设备以外的安全位置。"))
				style := components.ButtonOutline
				if stage == "" {
					style = components.ButtonPrimary
				}
				if components.Button(c, a.t("导出恢复私钥"), style).AlignSelf(ui.Start).Disabled(a.busy || a.preferences.keyExport != nil).Clicked() {
					a.exportKey()
				}
			})
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				components.Text(c, a.t("2. 验证私钥文件")).Bold().Wrap()
				muted(c, a.t("使用备份文件和私钥口令，检查能否解密此库。"))
				style := components.ButtonOutline
				if stage == app.RecoveryKeyExported {
					style = components.ButtonPrimary
				}
				if components.Button(c, a.t("选择并验证…"), style).AlignSelf(ui.Start).Disabled(a.busy).Clicked() {
					a.verifyBackup()
				}
			})
			label := a.t("稍后再做")
			if stage == app.RecoveryKeyVerified {
				label = a.t("完成")
			} else {
				muted(c, a.t("可随时在设置的“密码库”中继续。"))
			}
			if components.Button(c, label, components.ButtonSecondary).AlignSelf(ui.Start).Clicked() {
				a.navigate("entries", "")
			}
		})
	})
}
