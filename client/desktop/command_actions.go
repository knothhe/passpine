package desktop

import (
	"runtime"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/i18n"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

type paletteCommand struct {
	id, label, description, group, shortcut string
	icon                                    *ui.SVG
	disabled                                string
}

// Commands contain UI metadata only. Vault names, entry contents and copied
// values are deliberately absent from the search index.
func paletteCommandSources() []paletteCommand {
	modifier := "Ctrl"
	if runtime.GOOS == "darwin" {
		modifier = "⌘"
	}
	return []paletteCommand{
		{id: "search", label: "搜索条目", description: "按名称或路径查找密码库中的条目", group: "操作", shortcut: modifier + "+F", icon: lucide.Search()},
		{id: "new", label: "新建条目", description: "添加账号；未解锁时先验证身份", group: "操作", shortcut: modifier + "+N", icon: lucide.Plus()},
		{id: "copy-password", label: "复制密码", description: "复制当前条目的密码，并按设置自动清理", group: "操作", icon: lucide.KeyRound()},
		{id: "copy-username", label: "复制用户名", description: "复制当前条目的用户名", group: "操作", icon: lucide.User()},
		{id: "edit", label: "编辑当前条目", description: "修改当前条目的账号、密码及其他字段", group: "操作", icon: lucide.FilePenLine()},
		{id: "lock", label: "锁定密码库", description: "结束当前解锁会话，隐藏密码与账号详情", group: "操作", shortcut: modifier + "+Shift+L", icon: lucide.LockKeyhole()},
		{id: "all-entries", label: "全部条目", description: "回到密码库，清除文件夹与搜索筛选", group: "前往", icon: lucide.KeyRound()},
		{id: "drafts", label: "加密草稿", description: "查看并继续编辑已保存的加密草稿", group: "前往", icon: lucide.FilePenLine()},
		{id: "trash", label: "回收站", description: "查看可以恢复的已删除条目", group: "前往", icon: lucide.Trash()},
		{id: "open-vault", label: "打开或创建密码库", description: "选择已有密码库，或配置新的密码库", group: "前往", icon: lucide.FolderPlus()},
		{id: "settings", label: "外观与通用设置", description: "调整主题、语言、字号与窗口行为", group: "前往", shortcut: modifier + "+,", icon: lucide.Settings()},
		{id: "security", label: "安全设置", description: "管理解锁方式、自动锁定与剪贴板清理", group: "前往", icon: lucide.LockKeyhole()},
		{id: "browser", label: "浏览器扩展", description: "查看浏览器连接与注册状态", group: "前往", icon: lucide.Globe()},
		{id: "vault-settings", label: "密码库设置", description: "管理密码库名称、恢复备份与 Git 同步", group: "前往", icon: lucide.FolderKey()},
		{id: "config-sync", label: "配置同步", description: "管理配置同步连接与同步选项", group: "前往", icon: lucide.Cloud()},
		{id: "cli", label: "命令行", description: "查看 CLI 安装状态与命令链接", group: "前往", icon: lucide.SquareTerminal()},
		{id: "updates", label: "软件更新", description: "查看当前版本与可用更新", group: "前往", icon: lucide.Download()},
		{id: "about", label: "关于 Passpine", description: "查看应用信息、开源组件与许可证", group: "前往", icon: lucide.Info()},
	}
}

func (a *application) paletteCommands() []paletteCommand {
	commands := paletteCommandSources()
	for index := range commands {
		command := &commands[index]
		command.disabled = a.paletteCommandDisabled(command.id)
		command.label, command.description, command.group = a.t(command.label), a.t(command.description), a.t(command.group)
	}
	return commands
}

// Keep the default order fixed as asynchronous state changes. Search results
// use a stable relevance sort, so equally relevant commands keep that order.
func (a *application) matchingPaletteCommands(query string) []paletteCommand {
	commands := a.paletteCommands()
	query = strings.ToLower(strings.Join(strings.Fields(strings.TrimPrefix(strings.TrimSpace(query), ">")), " "))
	if query == "" {
		return commands
	}
	type match struct {
		command paletteCommand
		score   int
	}
	terms := strings.Fields(query)
	sources := paletteCommandSources()
	matches := make([]match, 0, len(commands))
	for index, command := range commands {
		source := sources[index]
		label := strings.ToLower(source.label + " " + i18n.Text(i18n.English, source.label))
		search := strings.ToLower(label + " " + source.description + " " + i18n.Text(i18n.English, source.description) + " " + paletteCommandAliases[command.id])
		score, found := 0, true
		for _, term := range terms {
			if !strings.Contains(search, term) {
				found = false
				break
			}
			if strings.Contains(label, term) {
				score += 20
			}
		}
		if !found {
			continue
		}
		for _, title := range []string{source.label, i18n.Text(i18n.English, source.label)} {
			title = strings.ToLower(title)
			switch {
			case title == query:
				score += 1000
			case strings.HasPrefix(title, query):
				score += 100
			case strings.Contains(title, query):
				score += 50
			}
		}
		matches = append(matches, match{command, score})
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	result := make([]paletteCommand, len(matches))
	for index, match := range matches {
		result[index] = match.command
	}
	return result
}

// These are search aliases, never displayed or interpreted as vault data.
var paletteCommandAliases = map[string]string{
	"search": "find lookup 搜索 查找 查找密码", "new": "create add account 新增 添加 新建账号",
	"copy-password": "clipboard credential secret 拷贝 复制密码", "copy-username": "clipboard login email account 拷贝 登录名 邮箱",
	"edit": "change modify entry 编辑 修改", "lock": "secure session 锁屏 锁定",
	"all-entries": "home list all entries 主页 首页 全部", "drafts": "draft recovery 恢复 草稿",
	"trash": "recycle bin deleted recovery restore 恢复 删除 回收站", "open-vault": "create vault open folder 打开 新建密码库 创建密码库",
	"settings": "preferences general appearance theme language font dark light shortcut 外观 主题 语言 字号 深色 浅色 快捷键",
	"security": "security unlock lock timeout clipboard 安全 解锁 超时 剪贴板",
	"browser":  "extension chrome firefox edge 浏览器 插件 扩展", "vault-settings": "git backup export vault 密码库 备份 导出 同步",
	"config-sync": "r2 s3 cloud sync settings preferences 配置 云 同步", "cli": "terminal command line pass 命令 终端",
	"updates": "version upgrade update 版本 升级 更新", "about": "info license dependencies 关于 许可 依赖",
}

func (a *application) paletteCommandDisabled(id string) string {
	if a.commandPaletteBlocked() {
		return a.t("请先完成当前操作")
	}
	if !a.state.Configured && id != "new" && id != "open-vault" && id != "about" && id != "config-sync" {
		return a.t("请先设置密码库")
	}
	switch id {
	case "copy-password", "copy-username", "edit":
		if a.state.Locked {
			return a.t("请先解锁密码库")
		}
		if a.busy {
			return a.t("请等待当前操作完成")
		}
		if a.editor.open {
			return a.t("请先完成当前编辑")
		}
		if a.setup || a.page != "entries" || a.batch.active || a.selected == "" || a.detail.Entry != a.selected {
			return a.t("请先选择并打开一个条目")
		}
		if id == "copy-password" && a.detail.Password == "" {
			return a.t("当前条目没有密码")
		}
		if id == "copy-username" && a.detail.Username == "" {
			return a.t("当前条目没有用户名")
		}
	case "lock":
		if a.state.Locked {
			return a.t("密码库已锁定")
		}
		if a.editor.open && a.editorBusy() {
			return a.t("请等待当前操作完成")
		}
	case "new", "open-vault":
		if a.busy {
			return a.t("请等待当前操作完成")
		}
	case "search":
		if a.state.Locked && !a.state.Settings.BrowseWhileLocked {
			return a.t("请先解锁密码库")
		}
	case "all-entries", "drafts", "trash":
		if a.editor.open && a.editorBusy() || a.busy && a.page == "recovery" {
			return a.t("请等待当前操作完成")
		}
	}
	return ""
}

// Rebuild availability immediately before dispatch. A panel can outlive the
// selected entry, an unlock session or an asynchronous operation.
func (a *application) executePaletteCommand(id string) bool {
	known := false
	for _, command := range paletteCommandSources() {
		if command.id == id {
			known = true
			break
		}
	}
	if !known || a.paletteCommandDisabled(id) != "" {
		return false
	}
	switch id {
	case "search":
		a.navigate("entries", "")
		a.searchRef.Focus()
	case "new":
		a.newEntry()
	case "edit":
		a.editEntry()
	case "copy-password":
		if a.service == nil {
			return false
		}
		a.copy("password", a.detail.Password, "密码")
	case "copy-username":
		if a.service == nil {
			return false
		}
		a.copy("username", a.detail.Username, "用户名")
	case "lock":
		if a.service == nil {
			return false
		}
		if a.editor.open {
			a.requestEditorClose(a.lock)
		} else {
			a.lock()
		}
	case "all-entries":
		open := func() {
			a.navigateManager("all")
			a.query = ""
			a.syncSelectedIndex()
			a.invalidate()
		}
		if a.editor.open {
			a.requestEditorClose(open)
		} else {
			open()
		}
	case "drafts":
		a.navigateRecovery(1)
	case "trash":
		a.navigateRecovery(0)
	case "open-vault":
		a.openSetup()
	default:
		category := id
		if category == "settings" {
			category = "general"
		} else if category == "vault-settings" {
			category = "vault"
		}
		a.navigateSettingsCategory(category)
	}
	return true
}
