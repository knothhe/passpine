package desktop

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/nativehost"
	"github.com/oneo-me/onevra/source/components"
)

type browserConfigurationDraft struct {
	open, picking bool
	value         nativehost.BrowserConfiguration
}

func (a *application) manualBrowserView(c *ui.Context) {
	draft := &a.preferences.browser
	disabled := a.busy || a.state.Preview || draft.picking
	ui.Column(c).FillWidth().MinWidth(0).Gap(10).Children(func() {
		components.Text(c, a.t("手动配置浏览器")).Bold()
		muted(c, a.t("指定本机浏览器程序及其连接注册位置，支持 Chromium 和 Firefox 衍生浏览器。配置仅保存在本机。"))
		if !draft.open {
			if components.Button(c, a.t("添加浏览器…"), components.ButtonOutline).Disabled(disabled).Clicked() {
				draft.open = true
				draft.value = nativehost.BrowserConfiguration{Kind: "chromium"}
			}
			return
		}
		textBox(c, &draft.value.Name, components.TextBoxOptions{}).FillWidth().Label(a.t("浏览器名称")).Placeholder(a.t("浏览器名称（可选）")).Disabled(disabled)
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			textBox(c, &draft.value.Path, components.TextBoxOptions{}).Grow(1).MinWidth(0).Label(a.t("浏览器程序路径")).Placeholder(a.t("浏览器程序的绝对路径")).Disabled(disabled)
			if components.Button(c, a.t("选择程序…"), components.ButtonOutline).Disabled(disabled).Clicked() {
				a.chooseBrowserProgram()
			}
		})
		kind := 0
		if draft.value.Kind == "firefox" {
			kind = 1
		}
		if ui.Segmented(c, &kind, a.t("Chromium 兼容"), a.t("Firefox 兼容")).Label(a.t("浏览器兼容类型")).Disabled(disabled || draft.value.ID != "").Changed() {
			if kind == 0 {
				draft.value.Kind = "chromium"
			} else {
				draft.value.Kind = "firefox"
			}
		}
		if runtime.GOOS == "windows" {
			textBox(c, &draft.value.RegistryPrefix, components.TextBoxOptions{}).FillWidth().Label(a.t("浏览器注册表位置")).Placeholder(a.t("HKCU 下的 Software\\厂商\\浏览器")).Disabled(disabled || draft.value.ID != "")
			muted(c, a.t("填写浏览器文档指定的 HKCU 注册表前缀，程序会追加 NativeMessagingHosts 和当前 Host 名称。"))
		} else {
			textBox(c, &draft.value.Directory, components.TextBoxOptions{}).FillWidth().Label(a.t("Native Messaging 目录")).Placeholder(a.t("NativeMessagingHosts 或 native-messaging-hosts 的绝对路径")).Disabled(disabled || draft.value.ID != "")
			if runtime.GOOS == "darwin" {
				muted(c, a.t("macOS 可留空以从 .app 推导默认目录；使用自定义用户数据目录时请填写实际连接目录。"))
			} else {
				muted(c, a.t("填写浏览器文档指定的 Native Messaging 目录；Flatpak、Snap 等沙箱安装还需要浏览器允许访问本机助手。"))
			}
		}
		muted(c, a.t("保存后点击注册以连接扩展。修改注册位置或兼容类型前，请先移除旧配置。"))
		ui.Row(c).Gap(8).Children(func() {
			if components.Button(c, a.t("保存浏览器配置"), components.ButtonPrimary).Disabled(disabled || draft.value.Path == "").Clicked() {
				value := draft.value
				a.call(app.Request{Action: "browser-save", BrowserConfiguration: &value}, func(app.Data) {
					draft.open = false
					draft.value = nativehost.BrowserConfiguration{}
					a.notice = a.t("浏览器配置已保存。")
				})
			}
			if components.Button(c, a.t("取消"), components.ButtonGhost).Disabled(disabled).Clicked() {
				draft.open = false
				draft.value = nativehost.BrowserConfiguration{}
			}
		})
	})
}

func (a *application) chooseBrowserProgram() {
	draft := &a.preferences.browser
	if a.busy || a.state.Preview || draft.picking {
		return
	}
	draft.picking = true
	current := a.preferencesLease()
	options := mygo.OpenDialogOptions{Parent: a.dialogParent(), Title: a.t("选择浏览器程序"), DefaultPath: draft.value.Path}
	if runtime.GOOS == "darwin" {
		options.Filters = []mygo.FileFilter{{Name: a.t("macOS 应用程序"), Extensions: []string{"app"}}}
	}
	if runtime.GOOS == "windows" {
		options.Filters = []mygo.FileFilter{{Name: a.t("Windows 应用程序"), Extensions: []string{"exe"}}}
	}
	open := a.preferences.openDialog
	if open == nil {
		open = mygo.Dialog.Open
	}
	go func() {
		paths, err := open(options)
		a.runOnMain(func() {
			draft.picking = false
			if !current() || !draft.open {
				return
			}
			if err != nil {
				a.errorText = a.t("无法打开文件选择器，请重试。")
			} else if len(paths) != 0 {
				draft.value.Path = paths[0]
			}
			a.invalidate()
		})
	}()
}
