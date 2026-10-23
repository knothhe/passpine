// Package cli implements a pass-compatible command line on top of the desktop
// client's vaults and protected private keys. Commands start with `passpine`;
// the same executable behaves as `pass` when invoked through an opt-in alias.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/buildinfo"
	"github.com/knothhe/passpine/client/deviceunlock"
	"github.com/knothhe/passpine/client/i18n"
	"github.com/knothhe/passpine/client/vault"
)

const passAlias = "pass"

// commands lists pass's built-in commands plus Passpine's own `cli`
// management command. Any other first argument is shown like `pass NAME`.
var commands = map[string]func(*command, []string) error{
	"ls": (*command).list, "list": (*command).list, "show": (*command).show,
	"find": (*command).find, "search": (*command).find, "grep": (*command).grep,
	"insert": (*command).insert, "add": (*command).insert, "edit": (*command).edit,
	"generate": (*command).generate, "rm": (*command).remove, "remove": (*command).remove,
	"delete": (*command).remove, "mv": (*command).move, "rename": (*command).move,
	"cp": (*command).copy, "copy": (*command).copy, "git": (*command).git,
	"otp": (*command).otp, "init": (*command).init, "help": (*command).help,
	"--help": (*command).help, "-h": (*command).help, "version": (*command).version,
	"--version": (*command).version, "cli": (*command).cli,
}

type command struct {
	program  string
	release  string
	locale   string
	dataDir  string
	settings app.Settings
	stdin    *os.File
	stdout   io.Writer
	stderr   io.Writer
	getenv   func(string) string
	store    *app.CLIVault
}

// Invoked reports whether the executable should run the command line instead
// of the desktop client. A bare `passpine` and flag-only launches keep opening
// the desktop app; the `pass` alias always uses the command line. On Windows a
// command is recognised only when started from a console.
func Invoked(args []string) bool {
	return commandArgs(args) && attachParentConsole()
}

func commandArgs(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return programName(args[0]) == passAlias || len(args) > 1 && (!strings.HasPrefix(args[1], "-") || args[1] == "-h" || args[1] == "--help")
}

func programName(arg0 string) string {
	name := strings.ToLower(filepath.Base(arg0))
	if strings.HasSuffix(name, ".exe") {
		name = strings.TrimSuffix(name, ".exe")
	}
	if name == passAlias {
		return passAlias
	}
	return commandName()
}

// commandName is the installed command for this build channel.
func commandName() string {
	if buildinfo.IsRelease() {
		return "passpine"
	}
	return "passpine-dev"
}

// Main runs one command and returns the process exit status.
func Main(args []string, release string) int {
	c := &command{program: programName(args[0]), release: release, stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, getenv: os.Getenv, settings: app.DefaultSettings()}
	c.locale = i18n.Resolve("", i18n.SystemLocale())
	if dir, err := buildinfo.DataDir(); err == nil {
		c.dataDir = dir
		if settings, _, _, err := app.CLIPreferences(dir); err == nil {
			c.settings = settings
			c.locale = i18n.Resolve(settings.Language, i18n.SystemLocale())
		}
	}
	defer c.close()
	if c.unsupportedConsole() {
		return 1
	}
	args = args[1:]
	if len(args) == 0 {
		args = []string{"ls"}
	}
	run, ok := commands[args[0]]
	if ok {
		args = args[1:]
	} else {
		run = (*command).show
	}
	if err := run(c, args); err != nil {
		var exit exitError
		if errors.As(err, &exit) {
			return int(exit)
		}
		var user userError
		if errors.As(err, &user) && user.usage {
			fmt.Fprintln(c.stderr, user.message)
		} else {
			fmt.Fprintln(c.stderr, c.t("错误：%s", c.describe(err)))
		}
		return 1
	}
	return 0
}

// exitError ends a command with a status after it has reported its own output.
type exitError int

func (e exitError) Error() string { return "exit " + strconv.Itoa(int(e)) }

func (c *command) t(key string, args ...any) string { return i18n.Text(c.locale, key, args...) }

func (c *command) close() {
	if c.store != nil {
		c.store.Vault.Close()
	}
}

type userError struct {
	message string
	usage   bool
}

func (e userError) Error() string { return e.message }

func (c *command) fail(key string, args ...any) error { return userError{message: c.t(key, args...)} }

// usage reports incorrect arguments without the error prefix, like pass.
func (c *command) usage(key string, args ...any) error {
	return userError{message: c.t(key, args...), usage: true}
}

func (c *command) describe(err error) string {
	var user userError
	switch {
	case errors.As(err, &user):
		return user.message
	case errors.Is(err, vault.ErrNotConfigured):
		return c.t("找不到此密码库的 Passpine 私钥。请先在 Passpine 客户端中创建、打开或导入该密码库。")
	case errors.Is(err, vault.ErrBusy):
		return c.t("密码库正被另一个操作使用，请稍后重试。")
	case errors.Is(err, vault.ErrPath):
		return c.t("条目名称无效或不安全。")
	case errors.Is(err, vault.ErrConflict):
		return c.t("条目已在其他位置更改，请重试。")
	case errors.Is(err, vault.ErrExists):
		return c.t("目标条目已存在。")
	case errors.Is(err, vault.ErrPassphrase):
		return c.t("私钥口令不正确。")
	case errors.Is(err, errNoClipboard):
		return c.t("找不到可用的剪贴板工具。请安装 wl-clipboard、xclip 或 xsel。")
	}
	return err.Error()
}

// open selects PASSWORD_STORE_DIR when set, otherwise the desktop client's
// current vault. It never unlocks; commands unlock only when they decrypt or write.
func (c *command) open() (*app.CLIVault, error) {
	if c.store != nil {
		return c.store, nil
	}
	if c.dataDir == "" {
		dir, err := buildinfo.DataDir()
		if err != nil {
			return nil, err
		}
		c.dataDir = dir
	}
	store, err := app.OpenCLIVault(c.dataDir, c.getenv("PASSWORD_STORE_DIR"))
	if err != nil {
		return nil, err
	}
	c.store = store
	return store, nil
}

func (c *command) unlock() (*vault.Vault, error) {
	store, err := c.open()
	if err != nil {
		return nil, err
	}
	v := store.Vault
	if v.Status().Unlocked {
		return v, nil
	}
	if store.DeviceFile != "" {
		if err := c.unlockDevice(store); err == nil {
			return v, nil
		}
		fmt.Fprintln(c.stderr, c.t("系统解锁未完成，请改用私钥口令。"))
	}
	in, out, err := openTerminal()
	if err != nil {
		return nil, c.fail("需要在终端中输入私钥口令才能解锁密码库。")
	}
	defer in.Close()
	if out != in {
		defer out.Close()
	}
	for attempt := 0; attempt < 3; attempt++ {
		secret, err := readHidden(in, out, c.t("%s 的 Passpine 私钥口令：", store.Info.Name))
		if err != nil {
			return nil, err
		}
		err = v.Unlock(string(secret))
		clear(secret)
		if err == nil {
			return v, nil
		}
		if !errors.Is(err, vault.ErrPassphrase) {
			return nil, err
		}
		fmt.Fprintln(out, c.t("私钥口令不正确。"))
	}
	return nil, vault.ErrPassphrase
}

func (c *command) unlockDevice(store *app.CLIVault) error {
	provider := deviceunlock.New()
	if !provider.Available() {
		return errors.New("unavailable")
	}
	envelope, err := os.ReadFile(store.DeviceFile)
	if err != nil {
		return err
	}
	defer clear(envelope)
	ctx, cancel := context.WithTimeout(deviceunlock.WithLanguage(context.Background(), store.Settings.Language), 2*time.Minute)
	defer cancel()
	return store.Vault.UnlockWithProvider(func() ([]byte, error) {
		return provider.Unlock(ctx, filepath.Dir(store.DeviceFile), envelope)
	})
}

// readHidden prompts on out and reads one line from in without echo when in is
// a terminal. Piped input is read as-is, like pass insert.
func readHidden(in *os.File, out io.Writer, prompt string) ([]byte, error) {
	if isTerminal(in) {
		fmt.Fprint(out, prompt)
		restore, err := withoutEcho(in)
		if err != nil {
			return nil, err
		}
		line, err := readLine(in)
		restore()
		fmt.Fprintln(out)
		return line, err
	}
	return readLine(in)
}

// readLine reads byte by byte so input after the first line stays unread.
func readLine(in io.Reader) ([]byte, error) {
	line := []byte{}
	b := make([]byte, 1)
	for {
		n, err := in.Read(b)
		if n == 1 {
			if b[0] == '\n' {
				break
			}
			line = append(line, b[0])
		}
		if err == io.EOF {
			if len(line) == 0 {
				return nil, io.ErrUnexpectedEOF
			}
			break
		}
		if err != nil {
			clear(line)
			return nil, err
		}
	}
	if n := len(line); n > 0 && line[n-1] == '\r' {
		line = line[:n-1]
	}
	return line, nil
}

// confirm mirrors pass's yesno: non-interactive input answers yes.
func (c *command) confirm(key string, args ...any) error {
	if !isTerminal(c.stdin) {
		return nil
	}
	fmt.Fprint(c.stdout, c.t(key, args...)+" [y/N] ")
	answer, err := readLine(c.stdin)
	if err == nil && (strings.EqualFold(string(answer), "y") || strings.EqualFold(string(answer), "yes")) {
		return nil
	}
	return exitError(1)
}

// entries lists the store without unlocking when the desktop client allows
// browsing names while locked, matching pass's key-free listing.
func (c *command) entries() ([]string, error) {
	store, err := c.open()
	if err != nil {
		return nil, err
	}
	var list []vault.Entry
	if store.Settings.BrowseWhileLocked && !store.Vault.Status().Unlocked {
		list, err = store.Vault.ListMetadata()
	} else {
		var v *vault.Vault
		if v, err = c.unlock(); err == nil {
			list, err = v.List()
		}
	}
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(list))
	for i, e := range list {
		paths[i] = e.Path
	}
	return paths, nil
}

func cleanName(name string) string {
	return strings.TrimSuffix(strings.Trim(path.Clean("/"+filepath.ToSlash(name)), "/"), ".gpg")
}

// lookup classifies name as an entry, a folder with entries, or missing.
func (c *command) lookup(name string) (entry bool, folder []string, err error) {
	name = cleanName(name)
	paths, err := c.entries()
	if err != nil {
		return false, nil, err
	}
	prefix := name + "/"
	if name == "" {
		prefix = ""
	}
	for _, p := range paths {
		if p == name {
			entry = true
		} else if strings.HasPrefix(p, prefix) {
			folder = append(folder, strings.TrimPrefix(p, prefix))
		}
	}
	return entry, folder, nil
}

func (c *command) exists(name string) (bool, error) {
	store, err := c.open()
	if err != nil {
		return false, err
	}
	if err := vault.ValidatePath(name); err != nil {
		return false, err
	}
	_, err = os.Lstat(filepath.Join(store.Info.Directory, filepath.FromSlash(name)+".gpg"))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// commit records a change when the store is a Git checkout, as pass does.
func (c *command) commit(message string) {
	if c.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := c.store.Vault.GitCommit(ctx, message); err != nil {
		fmt.Fprintln(c.stderr, c.t("已保存，但未能提交到 Git：%s", err.Error()))
	}
}

func (c *command) clipSeconds() int {
	if value, err := strconv.Atoi(c.getenv("PASSWORD_STORE_CLIP_TIME")); err == nil && value > 0 {
		return value
	}
	if c.settings.ClipboardSeconds > 0 {
		return c.settings.ClipboardSeconds
	}
	return 45
}

func (c *command) clip(name string, text []byte) error {
	seconds := c.clipSeconds()
	if err := copySecret(text, seconds, c.getenv("PASSWORD_STORE_X_SELECTION")); err != nil {
		return err
	}
	fmt.Fprintln(c.stdout, c.t("已将 %s 复制到剪贴板，将在 %d 秒后清除。", name, seconds))
	return nil
}
