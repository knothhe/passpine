package i18n

const (
	BrowserPathInvalid               = "浏览器程序路径无效，请选择存在且可执行的程序；macOS 请选择 .app。"
	BrowserDestinationInvalid        = "浏览器连接位置无效，请核对兼容类型、Native Messaging 目录或 HKCU 注册表前缀。"
	BrowserConfigurationUnreadable   = "无法读取本机浏览器配置，请检查 browsers.json 的格式和权限。"
	BrowserConfigurationSaveFailed   = "无法保存浏览器配置，请检查权限及配置数量（最多 32 个）。"
	BrowserConfigurationRemoveFailed = "无法移除浏览器连接，配置已保留，请检查权限后重试。"
	BrowserConfigurationMissing      = "浏览器配置不存在，请重新检测。"
)

func init() {
	for source, translated := range map[string]string{
		"移除":                             "Remove",
		BrowserPathInvalid:               "Invalid browser path. Select an existing executable, or an .app on macOS.",
		BrowserDestinationInvalid:        "Invalid browser connection location. Check the compatibility type, Native Messaging directory or HKCU registry prefix.",
		BrowserConfigurationUnreadable:   "Could not read local browser configurations. Check the format and permissions of browsers.json.",
		BrowserConfigurationSaveFailed:   "Could not save browser configurations. Check permissions and the configuration limit (32).",
		BrowserConfigurationRemoveFailed: "Could not remove the browser connection. The configuration was kept. Check permissions and retry.",
		BrowserConfigurationMissing:      "Browser configuration not found. Detect browsers again.",
		"手动配置浏览器":                        "Configure browsers manually",
		"指定本机浏览器程序及其连接注册位置，支持 Chromium 和 Firefox 衍生浏览器。配置仅保存在本机。": "Specify a local browser program and its connection registration location. Chromium and Firefox derivatives are supported. Configurations stay on this device.",
		"添加浏览器…":                    "Add browser…",
		"浏览器名称":                     "Browser name",
		"浏览器名称（可选）":                 "Browser name (optional)",
		"浏览器程序路径":                   "Browser program path",
		"浏览器程序的绝对路径":                "Absolute path to the browser program",
		"选择程序…":                     "Choose program…",
		"Chromium 兼容":               "Chromium compatible",
		"Firefox 兼容":                "Firefox compatible",
		"浏览器兼容类型":                   "Browser compatibility type",
		"浏览器注册表位置":                  "Browser registry location",
		"HKCU 下的 Software\\厂商\\浏览器": "Software\\Vendor\\Browser under HKCU",
		"填写浏览器文档指定的 HKCU 注册表前缀，程序会追加 NativeMessagingHosts 和当前 Host 名称。": "Enter the HKCU registry prefix specified by the browser documentation. NativeMessagingHosts and the current host name will be appended.",
		"Native Messaging 目录": "Native Messaging directory",
		"NativeMessagingHosts 或 native-messaging-hosts 的绝对路径":              "Absolute path to NativeMessagingHosts or native-messaging-hosts",
		"macOS 可留空以从 .app 推导默认目录；使用自定义用户数据目录时请填写实际连接目录。":                   "On macOS, leave blank to infer the default directory from the .app. For a custom user data directory, enter the actual connection directory.",
		"填写浏览器文档指定的 Native Messaging 目录；Flatpak、Snap 等沙箱安装还需要浏览器允许访问本机助手。": "Enter the Native Messaging directory specified by the browser documentation. Sandboxed installations such as Flatpak or Snap also need access to the native helper.",
		"保存后点击注册以连接扩展。修改注册位置或兼容类型前，请先移除旧配置。":                               "After saving, click Register to connect the extension. Remove the old configuration before changing its registration location or compatibility type.",
		"保存浏览器配置":      "Save browser configuration",
		"移除浏览器配置":      "Remove browser configuration",
		"浏览器配置已保存。":    "Browser configuration saved.",
		"浏览器配置已移除。":    "Browser configuration removed.",
		"选择浏览器程序":      "Choose browser program",
		"macOS 应用程序":   "macOS applications",
		"Windows 应用程序": "Windows applications",
		"浏览器程序不存在":     "Browser program missing",
		"编辑 %s 的浏览器配置": "Edit browser configuration for %s",
		"移除 %s 的浏览器配置": "Remove browser configuration for %s",
	} {
		english[source] = translated
	}
}
