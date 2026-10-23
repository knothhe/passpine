package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	executable, err := os.Executable()
	if err == nil {
		executable, err = filepath.EvalSymlinks(executable)
	}
	if err != nil {
		return err
	}
	dir := flags["dir"]
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".local", "bin")
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return err
	}
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

// ours reports whether path already runs this executable, either as a link
// created here or because it is the installed executable itself.
func ours(path, executable string) (exists, same bool) {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return false, false
	}
	resolved, err := filepath.EvalSymlinks(path)
	return true, err == nil && resolved == executable
}

func (c *command) installLink(l link, executable string) error {
	exists, same := ours(l.path, executable)
	if same {
		fmt.Fprintln(c.stdout, c.t("%s 已可用：%s", l.name, l.path))
		return nil
	}
	if exists {
		return c.fail("%s 已存在且不是 Passpine，未覆盖。请使用 --dir 选择其他目录。", l.path)
	}
	if err := os.Symlink(executable, l.path); err != nil {
		return err
	}
	fmt.Fprintln(c.stdout, c.t("已创建 %s：%s", l.name, l.path))
	return nil
}

// removeLink deletes only symbolic links that point to this executable, never
// another pass installation or the installed client binary itself.
func (c *command) removeLink(l link, executable string) error {
	exists, same := ours(l.path, executable)
	info, err := os.Lstat(l.path)
	switch {
	case !exists:
		fmt.Fprintln(c.stdout, c.t("%s 未安装：%s", l.name, l.path))
		return nil
	case err != nil:
		return err
	case !same || info.Mode()&os.ModeSymlink == 0:
		fmt.Fprintln(c.stdout, c.t("%s 不是 Passpine 创建的链接，已保留。", l.path))
		return nil
	}
	if err := os.Remove(l.path); err != nil {
		return err
	}
	fmt.Fprintln(c.stdout, c.t("已移除 %s：%s", l.name, l.path))
	return nil
}

func (c *command) linkStatus(l link, executable string) string {
	exists, same := ours(l.path, executable)
	switch {
	case same:
		return c.t("%s：已启用（%s）", l.name, l.path)
	case exists:
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
