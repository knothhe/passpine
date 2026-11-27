package desktop

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/nativehost"
	"github.com/oneo-me/onevra/source/components"
)

type browserConfigurationDraft struct {
	open, picking, customDirectory bool
	value                          nativehost.BrowserConfiguration
}

func (a *application) manualBrowserView(c *ui.Context) {
	draft := &a.preferences.browser
	disabled := a.busy || a.state.Preview || draft.picking
	ui.Column(c).FillWidth().MinWidth(0).Gap(10).Children(func() {
		components.Text(c, a.t("手动配置浏览器")).Bold()
		if runtime.GOOS == "darwin" {
			muted(c, a.t("选择一个 Chromium 或 Firefox 系浏览器。保存时，Passpine 会自动查找它的扩展连接目录。"))
		} else {
			muted(c, a.t("指定本机浏览器程序及其连接注册位置，支持 Chromium 和 Firefox 衍生浏览器。配置仅保存在本机。"))
		}
		if !draft.open {
			if components.Button(c, a.t("添加浏览器…"), components.ButtonOutline).Disabled(disabled).Clicked() {
				*draft = browserConfigurationDraft{open: true, value: nativehost.BrowserConfiguration{Kind: "chromium"}}
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
		switch runtime.GOOS {
		case "windows":
			textBox(c, &draft.value.RegistryPrefix, components.TextBoxOptions{}).FillWidth().Label(a.t("浏览器注册表位置")).Placeholder(a.t("HKCU 下的 Software\\厂商\\浏览器")).Disabled(disabled || draft.value.ID != "")
			muted(c, a.t("填写浏览器文档指定的 HKCU 注册表前缀，程序会追加 NativeMessagingHosts 和当前 Host 名称。"))
		case "darwin":
			if draft.value.ID == "" {
				components.Checkbox(c, &draft.customDirectory, a.t("自定义连接目录")).Disabled(disabled)
			}
			if draft.customDirectory || draft.value.ID != "" {
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					textBox(c, &draft.value.Directory, components.TextBoxOptions{}).Grow(1).MinWidth(0).Label(a.t("Native Messaging 目录")).Placeholder(a.t("NativeMessagingHosts 或 native-messaging-hosts 的绝对路径")).Disabled(disabled || draft.value.ID != "")
					if draft.value.ID == "" && components.Button(c, a.t("选择文件夹…"), components.ButtonOutline).Disabled(disabled).Clicked() {
						a.chooseBrowserDirectory()
					}
				})
			}
		default:
			textBox(c, &draft.value.Directory, components.TextBoxOptions{}).FillWidth().Label(a.t("Native Messaging 目录")).Placeholder(a.t("NativeMessagingHosts 或 native-messaging-hosts 的绝对路径")).Disabled(disabled || draft.value.ID != "")
			muted(c, a.t("填写浏览器文档指定的 Native Messaging 目录；Flatpak、Snap 等沙箱安装还需要浏览器允许访问本机助手。"))
		}
		muted(c, a.t("保存后点击注册以连接扩展。修改注册位置或兼容类型前，请先移除旧配置。"))
		ui.Row(c).Gap(8).Children(func() {
			missingDirectory := draft.customDirectory && draft.value.ID == "" && draft.value.Directory == ""
			if components.Button(c, a.t("保存浏览器配置"), components.ButtonPrimary).Disabled(disabled || draft.value.Path == "" || missingDirectory).Clicked() {
				value := draft.value
				if runtime.GOOS == "darwin" && value.ID == "" && !draft.customDirectory {
					value.Directory = ""
				}
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
	options := mygo.OpenDialogOptions{Parent: a.dialogParent(), Title: a.t("选择浏览器程序"), DefaultPath: draft.value.Path}
	switch runtime.GOOS {
	case "darwin":
		// An "app" extension filter can resolve to com.apple.application-file,
		// which excludes application bundles. Saving validates the bundle itself.
		options.DefaultPath = "/Applications"
		if filepath.IsAbs(draft.value.Path) {
			options.DefaultPath = filepath.Dir(filepath.Clean(draft.value.Path))
		}
	case "windows":
		options.Filters = []mygo.FileFilter{{Name: a.t("Windows 应用程序"), Extensions: []string{"exe"}}}
	}
	a.openBrowserPathDialog(options, &draft.value.Path)
}

func (a *application) chooseBrowserDirectory() {
	directory := &a.preferences.browser.value.Directory
	start := *directory
	if start == "" {
		start, _ = os.UserConfigDir()
	}
	a.openBrowserPathDialog(mygo.OpenDialogOptions{
		Parent: a.dialogParent(), Title: a.t("选择浏览器连接目录"), DefaultPath: start, Directory: true,
	}, directory)
}

func (a *application) openBrowserPathDialog(options mygo.OpenDialogOptions, destination *string) {
	draft := &a.preferences.browser
	if a.busy || a.state.Preview || draft.picking {
		return
	}
	draft.picking = true
	current := a.preferencesLease()
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
				*destination = paths[0]
			}
			a.invalidate()
		})
	}()
}
