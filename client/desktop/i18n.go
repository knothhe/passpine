package desktop

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/knothhe/passpine/client/i18n"
	sidebar "github.com/knothhe/passpine/client/internal/clientshell"
)

func (a *application) uiLocale() string {
	preference := a.state.Settings.Language
	if a.settingsLoaded || a.settings.Language != "" {
		preference = a.settings.Language
	}
	if preference != "" && preference != i18n.System {
		return i18n.Resolve(preference, "")
	}
	return i18n.Resolve(preference, i18n.SystemLocale())
}

func (a *application) t(source string, args ...any) string {
	return i18n.Text(a.uiLocale(), source, args...)
}

func (a *application) countLabel(n int) string { return a.t("%d 个条目", n) }

// Rebuild native menus after changing the language. Repainted views resolve
// their own labels from the current setting, without restarting the app.
func (a *application) applyLanguage() {
	if mygo.App.IsReady() {
		mygo.App.SetLocale(a.uiLocale())
	}
	if a.shell != nil {
		a.shell.SetLabels(sidebar.Labels{LeftToggle: a.t("切换侧栏"), LeftResize: a.t("调整侧栏宽度"), LeftOverlayClose: a.t("关闭左栏"), Minimize: a.t("最小化"), Maximize: a.t("最大化"), Restore: a.t("还原窗口"), Close: a.t("关闭窗口")})
	}
	if window, ok := a.quick.window.(*mygo.Window); ok {
		window.SetTitle(a.quickSearchTitle())
	}
	if a.window != nil && !a.closing {
		a.installMenu()
	}
	if a.tray != nil {
		a.installTrayMenu()
	}
}

var activityLabels = map[string]map[string]string{
	"action": {
		"lock": "锁定密码库", "unlock": "解锁密码库", "create": "创建密码库", "open": "打开密码库", "switch": "切换密码库",
		"vault-rename": "重命名密码库", "generate": "生成密码", "settings": "更新设置", "browser-install": "注册浏览器连接",
		"create-folder":        "新建文件夹",
		"cleanup-folders":      "清理空文件夹",
		"undo-cleanup-folders": "撤销空文件夹清理",
		"fetch":                "读取条目", "copy": "复制字段", "copy-generated": "复制生成的密码", "save": "保存条目", "delete": "移到回收站", "move": "移动条目", "trash": "移到回收站", "draft": "保存草稿",
		"recovery": "查看恢复副本", "restore": "恢复条目", "purge": "删除恢复副本", "export": "导出恢复私钥", "verify-backup": "验证恢复备份",
		"git-status": "检查 Git 状态", "git-init": "初始化 Git", "git-remote": "设置 Git 远端", "git-sync": "同步密码库", "git-commit": "提交加密条目", "clone": "下载密码库",
		"config-sync-save": "保存同步连接", "config-sync-test": "测试同步连接", "config-sync-now": "同步配置", "config-sync-resolve": "解决配置同步冲突", "config-sync-options": "保存同步选项",
		"config-sync-pause": "暂停同步", "config-sync-resume": "恢复同步", "config-sync-oauth-start": "授权 Cloudflare",
		"config-sync-oauth-buckets": "获取存储桶", "config-sync-oauth-accounts": "刷新账号", "config-sync-oauth-reload": "重新读取配置",
		"config-sync-cancel": "取消配置同步操作", "config-sync-disconnect": "断开同步连接",
	},
	"source":  {"desktop": "客户端", "browser": "浏览器", "automatic": "自动同步"},
	"outcome": {"ok": "成功", "error": "失败"},
}

func (a *application) activityLabel(kind, value string) string {
	if key, ok := activityLabels[kind][value]; ok {
		return a.t(key)
	}
	return value
}

func (a *application) localizedEditMenu() *mygo.MenuItem {
	items := []*mygo.MenuItem{
		{Role: mygo.RoleUndo, Label: a.t("撤销")}, {Role: mygo.RoleRedo, Label: a.t("重做")}, mygo.Separator(),
		{Role: mygo.RoleCut, Label: a.t("剪切")}, {Role: mygo.RoleCopy, Label: a.t("复制")}, {Role: mygo.RolePaste, Label: a.t("粘贴")},
	}
	if runtime.GOOS == "darwin" {
		items = append(items, &mygo.MenuItem{Role: mygo.RolePasteAndMatchStyle, Label: a.t("粘贴并匹配样式")})
	}
	items = append(items, &mygo.MenuItem{Role: mygo.RoleDelete, Label: a.t("删除")}, &mygo.MenuItem{Role: mygo.RoleSelectAll, Label: a.t("全选")})
	if runtime.GOOS == "darwin" {
		items = append(items, mygo.Separator(), &mygo.MenuItem{Label: a.t("语音"), Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleStartSpeaking, Label: a.t("开始朗读")}, {Role: mygo.RoleStopSpeaking, Label: a.t("停止朗读")},
		}})
	}
	return &mygo.MenuItem{Role: mygo.RoleEditMenu, Label: a.t("编辑"), Submenu: items}
}

func (a *application) localizedWindowMenu() *mygo.MenuItem {
	items := []*mygo.MenuItem{
		{Role: mygo.RoleClose, Label: a.t("关闭窗口"), Accelerator: "CmdOrCtrl+W"},
		mygo.Separator(),
		{Role: mygo.RoleMinimize, Label: a.t("最小化")},
	}
	if runtime.GOOS == "darwin" {
		items = append(items, &mygo.MenuItem{Role: mygo.RoleZoom, Label: a.t("缩放窗口")}, mygo.Separator(), &mygo.MenuItem{Role: mygo.RoleFront, Label: a.t("前置全部窗口")})
	}
	return &mygo.MenuItem{Role: mygo.RoleWindowMenu, Label: a.t("窗口"), Submenu: items}
}
