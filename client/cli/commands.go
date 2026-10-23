package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/knothhe/passpine/client/i18n"
	"github.com/knothhe/passpine/client/vault"
)

func (c *command) list(args []string) error {
	_, positional, err := c.parseFlags(args)
	if err != nil {
		return err
	}
	if len(positional) > 1 {
		return c.usage("用法：%s ls [子文件夹]", c.program)
	}
	return c.show(positional)
}

func (c *command) show(args []string) error {
	flags, positional, err := c.parseFlags(args, flagSpec{"c", "clip", optionalValue}, flagSpec{"q", "qrcode", optionalValue})
	if err != nil {
		return err
	}
	if len(positional) > 1 {
		return c.usage("用法：%s show [--clip[=行号],-c[行号]] [--qrcode[=行号],-q[行号]] [条目名称]", c.program)
	}
	name := ""
	if len(positional) == 1 {
		name = cleanName(positional[0])
	}
	entry, folder, err := c.lookup(name)
	if err != nil {
		return err
	}
	_, clip := flags["clip"]
	_, qr := flags["qrcode"]
	if !entry {
		if name != "" && len(folder) == 0 {
			return c.fail("密码库中没有 %s。", name)
		}
		if clip || qr {
			return c.fail("%s 是文件夹，不能复制或显示二维码。", name)
		}
		title := name
		if title == "" {
			title = c.t("密码存储")
		}
		writeTree(c.stdout, title, folder)
		return nil
	}
	v, err := c.unlock()
	if err != nil {
		return err
	}
	doc, err := v.Read(name)
	if err != nil {
		return err
	}
	if !clip && !qr {
		_, err = io.WriteString(c.stdout, doc.Raw)
		return err
	}
	key := "clip"
	if qr {
		key = "qrcode"
	}
	number := 1
	if value := flags[key]; value != "" {
		if number, err = strconv.Atoi(value); err != nil || number < 1 {
			return c.fail("行号必须是正整数：%s", value)
		}
	}
	lines := strings.Split(strings.ReplaceAll(doc.Raw, "\r\n", "\n"), "\n")
	if number > len(lines) || lines[number-1] == "" {
		return c.fail("%s 的第 %d 行没有可复制的内容。", name, number)
	}
	if qr {
		return c.qrcode(name, lines[number-1])
	}
	return c.clip(name, []byte(lines[number-1]))
}

// qrcode uses qrencode like pass; Passpine does not render codes itself.
func (c *command) qrcode(name, text string) error {
	if _, err := exec.LookPath("qrencode"); err != nil {
		return c.fail("显示二维码需要安装 qrencode。")
	}
	command := exec.Command("qrencode", "-t", "utf8")
	command.Stdin = strings.NewReader(text)
	command.Stdout, command.Stderr = c.stdout, c.stderr
	fmt.Fprintln(c.stdout, name)
	return command.Run()
}

func (c *command) find(args []string) error {
	if len(args) == 0 {
		return c.usage("用法：%s find 条目名称…", c.program)
	}
	paths, err := c.entries()
	if err != nil {
		return err
	}
	writeTree(c.stdout, c.t("搜索词：%s", strings.Join(args, " ")), matchingPaths(paths, args))
	return nil
}

func (c *command) grep(args []string) error {
	flags, positional, err := c.parseFlags(args, flagSpec{"i", "ignore-case", noValue}, flagSpec{"F", "fixed-strings", noValue}, flagSpec{"E", "extended-regexp", noValue})
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return c.usage("用法：%s grep [-i] [-F] 搜索内容", c.program)
	}
	pattern := positional[0]
	if _, ok := flags["fixed-strings"]; ok {
		pattern = regexp.QuoteMeta(pattern)
	}
	if _, ok := flags["ignore-case"]; ok {
		pattern = "(?i)" + pattern
	}
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return c.fail("无效的搜索表达式：%s", err.Error())
	}
	v, err := c.unlock()
	if err != nil {
		return err
	}
	entries, err := v.List()
	if err != nil {
		return err
	}
	for _, e := range entries {
		doc, err := v.Read(e.Path)
		if err != nil {
			fmt.Fprintln(c.stderr, c.t("无法读取 %s：%s", e.Path, c.describe(err)))
			continue
		}
		matched := []string{}
		for _, line := range strings.Split(strings.ReplaceAll(doc.Raw, "\r\n", "\n"), "\n") {
			if expression.MatchString(line) {
				matched = append(matched, line)
			}
		}
		if len(matched) > 0 {
			fmt.Fprintf(c.stdout, "%s:\n%s\n", e.Path, strings.Join(matched, "\n"))
		}
	}
	return nil
}

func (c *command) insert(args []string) error {
	flags, positional, err := c.parseFlags(args, flagSpec{"e", "echo", noValue}, flagSpec{"m", "multiline", noValue}, flagSpec{"f", "force", noValue})
	if err != nil {
		return err
	}
	_, echo := flags["echo"]
	_, multiline := flags["multiline"]
	_, force := flags["force"]
	if len(positional) != 1 || (echo && multiline) {
		return c.usage("用法：%s insert [--echo,-e | --multiline,-m] [--force,-f] 条目名称", c.program)
	}
	name := cleanName(positional[0])
	exists, err := c.exists(name)
	if err != nil {
		return err
	}
	if exists && !force {
		if err := c.confirm("%s 已存在。要覆盖吗？", name); err != nil {
			return err
		}
	}
	var contents []byte
	switch {
	case multiline:
		fmt.Fprintln(c.stdout, c.t("输入 %s 的内容，完成后按 Ctrl+D：", name))
		fmt.Fprintln(c.stdout)
		if contents, err = io.ReadAll(io.LimitReader(c.stdin, 8<<20)); err != nil {
			return err
		}
	case echo:
		fmt.Fprint(c.stdout, c.t("输入 %s 的密码：", name))
		if contents, err = readLine(c.stdin); err != nil {
			return err
		}
		contents = append(contents, '\n')
	default:
		first, err := readHidden(c.stdin, c.stdout, c.t("输入 %s 的密码：", name))
		if err != nil {
			return err
		}
		defer clear(first)
		if isTerminal(c.stdin) {
			second, err := readHidden(c.stdin, c.stdout, c.t("再次输入 %s 的密码：", name))
			if err != nil {
				return err
			}
			same := bytes.Equal(first, second)
			clear(second)
			if !same {
				return c.fail("两次输入的密码不一致。")
			}
		}
		contents = append(append([]byte{}, first...), '\n')
	}
	defer clear(contents)
	if err := c.write(name, string(contents)); err != nil {
		return err
	}
	c.commit(fmt.Sprintf("Add given password for %s to store.", name))
	return nil
}

// write saves contents, replacing an existing entry at its current revision.
func (c *command) write(name, contents string) error {
	v, err := c.unlock()
	if err != nil {
		return err
	}
	revision, err := c.revision(v, name)
	if err != nil {
		return err
	}
	_, err = v.Save(name, contents, revision)
	return err
}

func (c *command) revision(v *vault.Vault, name string) (string, error) {
	doc, err := v.Read(name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return doc.Revision, err
}

func (c *command) edit(args []string) error {
	_, positional, err := c.parseFlags(args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return c.usage("用法：%s edit 条目名称", c.program)
	}
	name := cleanName(positional[0])
	if err := vault.ValidatePath(name); err != nil {
		return err
	}
	v, err := c.unlock()
	if err != nil {
		return err
	}
	doc, err := v.Read(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	base, memory := plaintextDirectory()
	if !memory {
		fmt.Fprintln(c.stderr, c.t("此系统没有内存文件系统，解密后的内容会临时写入磁盘目录 %s。", base))
		if err := c.confirm("仍要继续吗？"); err != nil {
			return err
		}
	}
	dir, err := os.MkdirTemp(base, "passpine.")
	if err != nil {
		return err
	}
	file := filepath.Join(dir, strings.ReplaceAll(name, "/", "-")+".txt")
	defer removePlaintext(dir, file)
	original := []byte(doc.Raw)
	defer clear(original)
	if err := os.WriteFile(file, original, 0o600); err != nil {
		return err
	}
	editor := c.getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	command := editorCommand(editor, file)
	command.Stdin, command.Stdout, command.Stderr = c.stdin, c.stdout, c.stderr
	if err := command.Run(); err != nil {
		return c.fail("编辑器未正常退出：%s", err.Error())
	}
	edited, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	defer clear(edited)
	if bytes.Equal(edited, original) {
		fmt.Fprintln(c.stderr, c.t("%s 的内容未更改。", name))
		return exitError(1)
	}
	if _, err := v.Save(name, string(edited), doc.Revision); err != nil {
		return err
	}
	action := "Add"
	if doc.Revision != "" {
		action = "Edit"
	}
	c.commit(fmt.Sprintf("%s password for %s using %s.", action, name, editor))
	return nil
}

// removePlaintext overwrites the edited copy before unlinking it. On copy-on-write
// or journaled filesystems this is best effort, which is why tmpfs is preferred.
func removePlaintext(dir, file string) {
	if info, err := os.Stat(file); err == nil {
		if f, err := os.OpenFile(file, os.O_WRONLY, 0); err == nil {
			_, _ = f.Write(make([]byte, info.Size()))
			_ = f.Sync()
			_ = f.Close()
		}
	}
	_ = os.RemoveAll(dir)
}

func (c *command) generate(args []string) error {
	flags, positional, err := c.parseFlags(args, flagSpec{"n", "no-symbols", noValue}, flagSpec{"c", "clip", noValue}, flagSpec{"q", "qrcode", noValue}, flagSpec{"i", "in-place", noValue}, flagSpec{"f", "force", noValue})
	if err != nil {
		return err
	}
	_, noSymbols := flags["no-symbols"]
	_, clip := flags["clip"]
	_, qr := flags["qrcode"]
	_, inPlace := flags["in-place"]
	_, force := flags["force"]
	if len(positional) < 1 || len(positional) > 2 || (inPlace && force) || (clip && qr) {
		return c.usage("用法：%s generate [--no-symbols,-n] [--clip,-c] [--qrcode,-q] [--in-place,-i | --force,-f] 条目名称 [长度]", c.program)
	}
	name := cleanName(positional[0])
	length := 25
	if value := c.getenv("PASSWORD_STORE_GENERATED_LENGTH"); value != "" {
		length, _ = strconv.Atoi(value)
	}
	if len(positional) == 2 {
		if length, err = strconv.Atoi(positional[1]); err != nil {
			return c.fail("密码长度必须是数字：%s", positional[1])
		}
	}
	if length < 4 || length > 256 {
		return c.fail("密码长度必须在 4 到 256 之间。")
	}
	exists, err := c.exists(name)
	if err != nil {
		return err
	}
	if inPlace && !exists {
		return c.fail("密码库中没有 %s。", name)
	}
	if exists && !inPlace && !force {
		if err := c.confirm("%s 已存在。要覆盖吗？", name); err != nil {
			return err
		}
	}
	password, err := vault.GeneratePassword(vault.PasswordOptions{Length: length, Lowercase: true, Uppercase: true, Numbers: true, Symbols: !noSymbols})
	if err != nil {
		return err
	}
	contents := password + "\n"
	message := fmt.Sprintf("Add generated password for %s to store.", name)
	if inPlace {
		v, err := c.unlock()
		if err != nil {
			return err
		}
		doc, err := v.Read(name)
		if err != nil {
			return err
		}
		if _, rest, ok := strings.Cut(doc.Raw, "\n"); ok {
			contents = password + "\n" + rest
		}
		if _, err := v.Save(name, contents, doc.Revision); err != nil {
			return err
		}
		message = fmt.Sprintf("Replace generated password for %s.", name)
	} else if err := c.write(name, contents); err != nil {
		return err
	}
	c.commit(message)
	switch {
	case clip:
		return c.clip(name, []byte(password))
	case qr:
		return c.qrcode(name, password)
	}
	fmt.Fprintln(c.stdout, c.t("%s 的新密码：", name))
	fmt.Fprintln(c.stdout, password)
	return nil
}

func (c *command) remove(args []string) error {
	flags, positional, err := c.parseFlags(args, flagSpec{"r", "recursive", noValue}, flagSpec{"f", "force", noValue})
	if err != nil {
		return err
	}
	_, recursive := flags["recursive"]
	_, force := flags["force"]
	if len(positional) != 1 {
		return c.usage("用法：%s rm [--recursive,-r] [--force,-f] 条目名称", c.program)
	}
	name := cleanName(positional[0])
	entry, folder, err := c.lookup(name)
	if err != nil {
		return err
	}
	targets := []string{}
	switch {
	case entry:
		targets = append(targets, name)
	case len(folder) > 0 && !recursive:
		return c.fail("%s 是文件夹，删除时请使用 --recursive。", name)
	case len(folder) > 0:
		for _, p := range folder {
			targets = append(targets, path.Join(name, p))
		}
	default:
		return c.fail("密码库中没有 %s。", name)
	}
	if !force {
		if err := c.confirm("确定要删除 %s 吗？", name); err != nil {
			return err
		}
	}
	v, err := c.unlock()
	if err != nil {
		return err
	}
	for _, target := range targets {
		doc, err := v.Read(target)
		if err != nil {
			return err
		}
		if _, err := v.Trash(target, doc.Revision); err != nil {
			return err
		}
		fmt.Fprintln(c.stdout, c.t("已移到回收站：%s", target))
	}
	c.commit(fmt.Sprintf("Remove %s from store.", name))
	return nil
}

func (c *command) move(args []string) error { return c.transfer(args, true) }
func (c *command) copy(args []string) error { return c.transfer(args, false) }

// transfer implements mv and cp. Entries are re-encrypted for the destination
// folder's .gpg-id; moved sources are removed only after the copy is durable.
func (c *command) transfer(args []string, move bool) error {
	flags, positional, err := c.parseFlags(args, flagSpec{"f", "force", noValue})
	if err != nil {
		return err
	}
	_, force := flags["force"]
	if len(positional) != 2 {
		if move {
			return c.usage("用法：%s mv [--force,-f] 原名称 新名称", c.program)
		}
		return c.usage("用法：%s cp [--force,-f] 原名称 新名称", c.program)
	}
	from, to := cleanName(positional[0]), cleanName(positional[1])
	entry, folder, err := c.lookup(from)
	if err != nil {
		return err
	}
	_, destinationFolder, err := c.lookup(to)
	if err != nil {
		return err
	}
	intoFolder := strings.HasSuffix(positional[1], "/") || len(destinationFolder) > 0
	pairs := [][2]string{}
	switch {
	case entry:
		destination := to
		if intoFolder {
			destination = path.Join(to, path.Base(from))
		}
		pairs = append(pairs, [2]string{from, destination})
	case len(folder) > 0:
		root := to
		if intoFolder {
			root = path.Join(to, path.Base(from))
		}
		for _, p := range folder {
			pairs = append(pairs, [2]string{path.Join(from, p), path.Join(root, p)})
		}
	default:
		return c.fail("密码库中没有 %s。", from)
	}
	v, err := c.unlock()
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		if pair[0] == pair[1] {
			continue
		}
		destinationRevision, err := c.revision(v, pair[1])
		if err != nil {
			return err
		}
		if destinationRevision != "" && !force {
			if err := c.confirm("%s 已存在。要覆盖吗？", pair[1]); err != nil {
				return err
			}
		}
		doc, err := v.Read(pair[0])
		if err != nil {
			return err
		}
		if move {
			// Move refuses existing destinations; an approved overwrite keeps
			// the replaced entry recoverable in the trash instead.
			if destinationRevision != "" {
				if _, err := v.Trash(pair[1], destinationRevision); err != nil {
					return err
				}
			}
			err = v.Move(pair[0], pair[1], doc.Revision)
		} else {
			_, err = v.Save(pair[1], doc.Raw, destinationRevision)
		}
		if err != nil {
			return err
		}
	}
	if move {
		c.commit(fmt.Sprintf("Rename %s to %s.", from, to))
	} else {
		c.commit(fmt.Sprintf("Copy %s to %s.", from, to))
	}
	return nil
}

// git runs Git inside the password store, like `pass git`.
func (c *command) git(args []string) error {
	store, err := c.open()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return c.usage("用法：%s git Git 命令参数…", c.program)
	}
	if _, err := os.Stat(filepath.Join(store.Info.Directory, ".git")); err != nil && args[0] != "init" {
		return c.fail("密码库还不是 Git 仓库。请运行“%s git init”。", c.program)
	}
	command := exec.Command("git", append([]string{"-C", store.Info.Directory}, args...)...)
	command.Stdin, command.Stdout, command.Stderr = c.stdin, c.stdout, c.stderr
	if err := command.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exitError(exit.ExitCode())
		}
		return err
	}
	if args[0] == "init" {
		c.commit("Add current contents of password store.")
	}
	return nil
}

// otp prints the current TOTP code, compatible with the pass-otp extension.
func (c *command) otp(args []string) error {
	if len(args) > 0 && args[0] == "code" {
		args = args[1:]
	}
	flags, positional, err := c.parseFlags(args, flagSpec{"c", "clip", noValue})
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return c.usage("用法：%s otp [code] [--clip,-c] 条目名称", c.program)
	}
	name := cleanName(positional[0])
	v, err := c.unlock()
	if err != nil {
		return err
	}
	doc, err := v.Read(name)
	if err != nil {
		return err
	}
	secret := doc.OTP
	for _, line := range strings.Split(strings.ReplaceAll(doc.Raw, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "otpauth://") {
			secret = strings.TrimSpace(line)
			break
		}
	}
	if secret == "" {
		return c.fail("%s 中没有 OTP 密钥。", name)
	}
	code, err := vault.TOTP(secret, time.Now())
	if err != nil {
		return c.fail("%s 中的 OTP 密钥无效。", name)
	}
	if _, ok := flags["clip"]; ok {
		return c.clip(name, []byte(code.Code))
	}
	fmt.Fprintln(c.stdout, code.Code)
	return nil
}

func (c *command) init(args []string) error {
	return c.fail("Passpine 在客户端中创建或导入密码库并管理私钥。请打开 Passpine 客户端完成设置。")
}

func (c *command) version(args []string) error {
	fmt.Fprintln(c.stdout, c.t("Passpine %s 命令行，兼容 pass", c.release))
	return nil
}

func (c *command) help(args []string) error {
	fmt.Fprintln(c.stdout, c.t(i18n.CLIHelp, c.program))
	return nil
}
