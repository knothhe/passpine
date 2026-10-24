package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/knothhe/passpine/client/internal/commandlink"
)

type link struct {
	name string
	path string
}

func (c *command) cli(args []string) error {
	usage := c.usage("用法：%s cli install|uninstall|status [--pass-alias] [--dir 目录]", c.program)
	if len(args) == 0 {
		return usage
	}
	flags, positional, err := c.parseFlags(args[1:], flagSpec{"", "pass-alias", noValue}, flagSpec{"", "dir", requiredValue})
	if err != nil {
		return err
	}
	if len(positional) != 0 {
		return usage
	}
	if runtime.GOOS == "windows" {
		return c.fail("Windows 暂不支持自动安装命令链接。")
	}
	manager, err := commandlink.Current(flags["dir"])
	if err != nil {
		return err
	}
	dir, executable := manager.Directory, manager.Executable
	main, alias := link{commandName(), filepath.Join(dir, commandName())}, link{passAlias, filepath.Join(dir, passAlias)}
	_, aliasOnly := flags["pass-alias"]
	switch args[0] {
	case "install":
		links := []link{main}
		if aliasOnly {
			links = append(links, alias)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for _, l := range links {
			if err := c.installLink(l, executable); err != nil {
				return err
			}
		}
		c.pathNotes(dir, links)
	case "uninstall":
		links := []link{alias}
		if !aliasOnly {
			links = append(links, main)
		}
		for _, l := range links {
			if err := c.removeLink(l, executable); err != nil {
				return err
			}
		}
	case "status":
		for _, l := range []link{main, alias} {
			fmt.Fprintln(c.stdout, c.linkStatus(l, executable))
		}
		c.pathNotes(dir, []link{main, alias})
	default:
		return usage
	}
	return nil
}

func linkManager(l link, executable string) commandlink.Manager {
	return commandlink.Manager{Directory: filepath.Dir(l.path), Executable: executable}
}

func (c *command) installLink(l link, executable string) error {
	manager := linkManager(l, executable)
	entry, err := manager.Inspect(l.name)
	if err != nil {
		return err
	}
	if entry.Owned {
		fmt.Fprintln(c.stdout, c.t("%s 已可用：%s", l.name, l.path))
		return nil
	}
	if entry.Exists {
		return c.fail("%s 已存在且不是 Passpine，未覆盖。请使用 --dir 选择其他目录。", l.path)
	}
	if _, err := manager.InstallLink(l.name); err != nil {
		return err
	}
	fmt.Fprintln(c.stdout, c.t("已创建 %s：%s", l.name, l.path))
	return nil
}

// removeLink deletes only symbolic links that point to this executable, never
// another pass installation or the installed client binary itself.
func (c *command) removeLink(l link, executable string) error {
	manager := linkManager(l, executable)
	entry, err := manager.Inspect(l.name)
	switch {
	case err != nil:
		return err
	case !entry.Exists:
		fmt.Fprintln(c.stdout, c.t("%s 未安装：%s", l.name, l.path))
		return nil
	case !entry.Removable:
		fmt.Fprintln(c.stdout, c.t("%s 不是 Passpine 创建的链接，已保留。", l.path))
		return nil
	}
	if _, err := manager.RemoveLink(l.name); err != nil {
		return err
	}
	fmt.Fprintln(c.stdout, c.t("已移除 %s：%s", l.name, l.path))
	return nil
}

func (c *command) linkStatus(l link, executable string) string {
	entry, err := linkManager(l, executable).Inspect(l.name)
	switch {
	case err != nil:
		return c.t("无法检查 %s：%s", l.name, err)
	case entry.Owned:
		return c.t("%s：已启用（%s）", l.name, l.path)
	case entry.Exists:
		return c.t("%s：%s 被其他程序占用", l.name, l.path)
	}
	return c.t("%s：未启用", l.name)
}

// pathNotes warns when the link directory is not searched or another command
// of the same name is found first, such as the original pass script.
func (c *command) pathNotes(dir string, links []link) {
	inPath := false
	for _, entry := range filepath.SplitList(c.getenv("PATH")) {
		if filepath.Clean(entry) == dir {
			inPath = true
		}
	}
	if !inPath {
		fmt.Fprintln(c.stdout, c.t("%s 不在 PATH 中，请将它加入 PATH。", dir))
		return
	}
	for _, l := range links {
		if _, err := os.Lstat(l.path); err != nil {
			continue
		}
		if found, err := exec.LookPath(l.name); err == nil && !strings.EqualFold(filepath.Clean(found), l.path) {
			fmt.Fprintln(c.stdout, c.t("PATH 中的 %s 会优先于 %s。", found, l.path))
		}
	}
}
