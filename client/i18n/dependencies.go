package i18n

// DependencyDescriptionKey returns a UI message key, never vault data.
func DependencyDescriptionKey(id string) string {
	return map[string]string{
		"crypto":  "OpenPGP 密钥管理与密码库加解密。",
		"purego":  "调用原生系统接口，检测已安装的浏览器。",
		"dbus":    "Linux 托盘与全局快捷键的桌面通信。",
		"mygo":    "原生窗口、界面、托盘、菜单与全局快捷键。",
		"onevra":  "桌面界面组件、主题与图标。",
		"sys":     "底层系统调用与安全文件访问。",
		"xcrypto": "Windows 系统解锁的密码密钥派生。",
	}[id]
}
