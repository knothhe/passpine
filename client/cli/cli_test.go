package cli

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/i18n"
)

func TestInvokedKeepsDesktopLaunches(t *testing.T) {
	for _, test := range []struct {
		args []string
		want bool
	}{
		{[]string{"/usr/bin/passpine"}, false},
		{[]string{"passpine", "--preview"}, false},
		{[]string{"passpine", "--data-dir", "/tmp/x"}, false},
		{[]string{"passpine", "show", "mail"}, true},
		{[]string{"passpine", "mail"}, true},
		{[]string{"passpine", "--help"}, true},
		{[]string{"/home/u/.local/bin/pass"}, true},
		{[]string{"/opt/bin/Pass.exe", "-c", "mail"}, true},
	} {
		if got := commandArgs(test.args); got != test.want {
			t.Errorf("commandArgs(%q) = %v, want %v", test.args, got, test.want)
		}
	}
}

func TestParseFlagsFollowsGetopt(t *testing.T) {
	c := &command{locale: i18n.English}
	specs := []flagSpec{{"c", "clip", optionalValue}, {"f", "force", noValue}, {"", "dir", requiredValue}}
	flags, positional, err := c.parseFlags([]string{"a", "-fc2", "--dir", "/x", "b", "--", "-c"}, specs...)
	if err != nil {
		t.Fatal(err)
	}
	if flags["clip"] != "2" || flags["dir"] != "/x" || strings.Join(positional, ",") != "a,b,-c" {
		t.Fatalf("flags=%v positional=%v", flags, positional)
	}
	if _, ok := flags["force"]; !ok {
		t.Fatal("combined short flag was lost")
	}
	if flags, _, err = c.parseFlags([]string{"--clip=3", "x"}, specs...); err != nil || flags["clip"] != "3" {
		t.Fatalf("long optional value: %v %v", flags, err)
	}
	if _, _, err = c.parseFlags([]string{"--force=1"}, specs...); err == nil {
		t.Fatal("value accepted for a flag without one")
	}
	if _, _, err = c.parseFlags([]string{"-z"}, specs...); err == nil || err.Error() != "Invalid option: -z" {
		t.Fatalf("unknown flag error = %v", err)
	}
}

func TestTreeMatchesPassLayout(t *testing.T) {
	var out bytes.Buffer
	writeTree(&out, "Password Store", []string{"web/b", "mail", "web/a/x", "Bank"})
	want := "Password Store\n├── Bank\n├── mail\n└── web\n    ├── a\n    │   └── x\n    └── b\n"
	if out.String() != want {
		t.Fatalf("tree:\n%s\nwant:\n%s", out.String(), want)
	}
	got := matchingPaths([]string{"Work/Mail", "home/bank", "other"}, []string{"MAIL", "work", "bank"})
	if strings.Join(got, ",") != "Work/Mail,home/bank" {
		t.Fatalf("find matched %v", got)
	}
}

// Every command-line message must have an English translation, and only
// complete keys are passed to the catalog.
func TestMessagesHaveEnglish(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "t" && selector.Sel.Name != "fail" && selector.Sel.Name != "usage" && selector.Sel.Name != "confirm") {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				return true
			}
			key, err := strconv.Unquote(literal.Value)
			if err != nil || !i18n.HasEnglish(key) {
				t.Errorf("%s: missing English translation for %s", name, literal.Value)
			}
			return true
		})
	}
	if !i18n.HasEnglish(i18n.CLIHelp) {
		t.Error("help text has no English translation")
	}
}

const testPassphrase = "correct horse battery"

type harness struct {
	t      *testing.T
	data   string
	store  string
	editor string
}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, data: t.TempDir(), store: filepath.Join(t.TempDir(), "store")}
	if err := os.Mkdir(h.store, 0o700); err != nil {
		t.Fatal(err)
	}
	c := h.command("")
	store, err := c.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Vault.Create("Test", "test@example.com", testPassphrase); err != nil {
		t.Fatal(err)
	}
	c.close()
	return h
}

// command builds an isolated invocation. The vault is unlocked directly
// because the passphrase prompt only reads from the controlling terminal.
func (h *harness) command(input string) *command {
	stdin, err := os.CreateTemp(h.t.TempDir(), "stdin")
	if err != nil {
		h.t.Fatal(err)
	}
	stdin.WriteString(input)
	stdin.Seek(0, 0)
	h.t.Cleanup(func() { stdin.Close() })
	env := map[string]string{"PASSWORD_STORE_DIR": h.store, "EDITOR": h.editor, "PASSWORD_STORE_GENERATED_LENGTH": ""}
	return &command{program: "pass", locale: i18n.English, dataDir: h.data, settings: app.DefaultSettings(), stdin: stdin, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, getenv: func(k string) string { return env[k] }}
}

func (h *harness) run(input string, args ...string) (string, error) {
	c := h.command(input)
	defer c.close()
	store, err := c.open()
	if err != nil {
		h.t.Fatal(err)
	}
	if err := store.Vault.Unlock(testPassphrase); err != nil {
		h.t.Fatal(err)
	}
	run, ok := commands[args[0]]
	if ok {
		args = args[1:]
	} else {
		run = (*command).show
	}
	err = run(c, args)
	return c.stdout.(*bytes.Buffer).String() + c.stderr.(*bytes.Buffer).String(), err
}

func (h *harness) must(input string, args ...string) string {
	out, err := h.run(input, args...)
	if err != nil {
		h.t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return out
}

func TestCommandsRoundTripThroughVault(t *testing.T) {
	h := newHarness(t)
	h.must("s3cret\nignored\n", "insert", "web/mail")
	if out := h.must("", "show", "web/mail"); out != "s3cret\n" {
		t.Fatalf("show = %q", out)
	}
	if out := h.must("", "web/mail"); out != "s3cret\n" {
		t.Fatalf("implicit show = %q", out)
	}
	h.must("pw\nuser: alice\notpauth://totp/x?secret=JBSWY3DPEHPK3PXP\n", "insert", "-m", "bank")
	if out := h.must("", "ls"); out != "Password Store\n├── bank\n└── web\n    └── mail\n" {
		t.Fatalf("ls = %q", out)
	}
	if out := h.must("", "grep", "-i", "ALICE"); out != "bank:\nuser: alice\n" {
		t.Fatalf("grep = %q", out)
	}
	if out := h.must("", "otp", "bank"); len(strings.TrimSpace(out)) != 6 {
		t.Fatalf("otp = %q", out)
	}
	if out := h.must("", "generate", "-n", "gen", "30"); !strings.Contains(out, "The generated password for gen is:") {
		t.Fatalf("generate = %q", out)
	}
	generated := h.must("", "show", "gen")
	if len(generated) != 31 || strings.ContainsAny(generated[:30], "!@#$%^&*") {
		t.Fatalf("generated password = %q", generated)
	}
	h.must("", "generate", "-i", "bank", "12")
	if out := h.must("", "show", "bank"); !strings.HasSuffix(out, "\nuser: alice\notpauth://totp/x?secret=JBSWY3DPEHPK3PXP\n") || len(strings.SplitN(out, "\n", 2)[0]) != 12 {
		t.Fatalf("in-place generate = %q", out)
	}
	h.must("", "cp", "web/mail", "archive/")
	h.must("", "mv", "web", "old")
	if out := h.must("", "ls"); out != "Password Store\n├── archive\n│   └── mail\n├── bank\n├── gen\n└── old\n    └── mail\n" {
		t.Fatalf("after cp/mv = %q", out)
	}
	if _, err := h.run("", "rm", "old"); err == nil {
		t.Fatal("folder removed without --recursive")
	}
	h.must("", "rm", "-rf", "old")
	if _, err := h.run("", "show", "old/mail"); err == nil || !strings.Contains(err.Error(), "not in the password store") {
		t.Fatalf("removed entry still shown: %v", err)
	}
	if out := h.must("", "find", "MAIL"); out != "Search Terms: MAIL\n└── archive\n    └── mail\n" {
		t.Fatalf("find = %q", out)
	}
}

func TestEditUsesEditorAndRejectsUnchanged(t *testing.T) {
	h := newHarness(t)
	h.must("one\n", "insert", "-e", "note")
	h.editor = "sed -i s/one/two/"
	if runtime.GOOS == "darwin" {
		h.editor = "sed -i '' s/one/two/"
	}
	h.must("", "edit", "note")
	if out := h.must("", "show", "note"); out != "two\n" {
		t.Fatalf("edited = %q", out)
	}
	h.editor = "true"
	if _, err := h.run("", "edit", "note"); err == nil {
		t.Fatal("unchanged edit succeeded")
	}
}

func TestWritesCommitToGitStores(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	h := newHarness(t)
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "T"}} {
		if out, err := exec.Command("git", append([]string{"-C", h.store}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	h.must("x\n", "insert", "-e", "mail")
	out, err := exec.Command("git", "-C", h.store, "log", "--format=%s").Output()
	if err != nil || strings.TrimSpace(string(out)) != "Add given password for mail to store." {
		t.Fatalf("git log = %q %v", out, err)
	}
}

func TestPassAliasInstallNeverReplacesOtherCommands(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, passAlias)
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &command{program: commandName(), locale: i18n.English, stdout: &bytes.Buffer{}, getenv: func(string) string { return dir }}
	if err := c.cli([]string{"install", "--pass-alias", "--dir", dir}); err == nil {
		t.Fatal("existing pass command was replaced")
	}
	os.Remove(other)
	if err := c.cli([]string{"install", "--pass-alias", "--dir", dir}); err != nil {
		t.Fatal(err)
	}
	executable, _ := os.Executable()
	executable, _ = filepath.EvalSymlinks(executable)
	for _, name := range []string{passAlias, commandName()} {
		if target, err := filepath.EvalSymlinks(filepath.Join(dir, name)); err != nil || target != executable {
			t.Fatalf("%s -> %q, %v", name, target, err)
		}
	}
	if err := c.cli([]string{"uninstall", "--pass-alias", "--dir", dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dir, passAlias)); !os.IsNotExist(err) {
		t.Fatal("pass alias was not removed")
	}
	if _, err := os.Lstat(filepath.Join(dir, commandName())); err != nil {
		t.Fatal("alias-only uninstall removed the passpine command")
	}
}
