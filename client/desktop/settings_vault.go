package desktop

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine-neo/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func displayVaultDirectory(directory string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if directory == home {
			return "~"
		}
		if strings.HasPrefix(directory, home+string(filepath.Separator)) {
			return "~" + strings.TrimPrefix(directory, home)
		}
	}
	return directory
}

func (a *application) vaultNamePlaceholder() string {
	if strings.TrimSpace(a.setupDirectory) != "" {
		return displayVaultDirectory(a.setupDirectory)
	}
	return a.t("默认使用密码库目录")
}

func (a *application) vaultNameControls(c *ui.Context) {
	if a.vaultNameID != a.state.VaultID {
		a.vaultNameID, a.vaultNameDraft = a.state.VaultID, a.state.VaultName
	}
	ui.Row(c).Width(300).MaxWidth(360).Gap(6).AlignItems(ui.Center).Children(func() {
		input := components.TextBox(c, &a.vaultNameDraft, components.TextBoxOptions{}).Grow(1).MinWidth(0).Label(a.t("编辑密码库名称")).Placeholder(displayVaultDirectory(a.state.Directory)).Disabled(!a.state.Configured || a.busy)
		if iconButton(c, lucide.Check(), a.t("保存密码库名称"), components.ButtonGhost).Disabled(!a.state.Configured || a.busy || a.vaultNameDraft == a.state.VaultName).Clicked() || input.Submitted() {
			a.call(app.Request{Action: "vault-rename", ID: a.state.VaultID, Name: a.vaultNameDraft}, func(d app.Data) {
				a.vaultNameDraft = d.VaultName
				a.notice = a.t("密码库名称已更新")
			})
		}
	})
}

func (a *application) browserRegistrationView(c *ui.Context) {
	ui.Column(c).FillWidth().MinWidth(0).Gap(14).Children(func() {
		if len(a.state.BrowserRegistrations) == 0 {
			if a.state.Preview {
				muted(c, a.t("预览模式不会注册个人浏览器"))
			} else {
				muted(c, a.t("未检测到支持的浏览器"))
			}
			return
		}
		for _, registration := range a.state.BrowserRegistrations {
			ui.Row(c).Key(registration.ID).FillWidth().MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
				icon, color, status := lucide.Circle(), c.Theme().TextMuted, a.t("未注册")
				if registration.Registered {
					icon, color, status = lucide.CircleCheck(), c.Theme().Success, a.t("已注册")
				} else if registration.Status == "invalid" || registration.Status == "helper_missing" {
					icon, color, status = lucide.CircleAlert(), c.Theme().Warning, a.t("需要修复")
				}
				components.Icon(c, icon).Size(16, 16).TextColor(color)
				ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
					components.Text(c, registration.Name).FontSize(13)
					components.Text(c, registration.ExtensionID).FontSize(11).TextColor(c.Theme().TextMuted).Wrap()
				})
				components.Text(c, status).FontSize(12).TextColor(color)
				if !registration.Registered {
					label := a.t("注册")
					if registration.Status == "invalid" || registration.Status == "helper_missing" {
						label = a.t("修复")
					}
					if components.Button(c, label, components.ButtonOutline).Label(a.t("注册 %s", registration.Name)).Disabled(a.busy || a.state.Preview || !a.state.BrowserHelperAvailable).Clicked() {
						a.call(app.Request{Action: "browser-install", ID: registration.ID}, nil)
					}
				}
			})
		}
		if !a.state.BrowserHelperAvailable {
			muted(c, a.t("本机助手不可用，请重新安装客户端。"))
		}
	})
}
