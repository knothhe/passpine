package clientshell

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestWholeViewDecoratorAndOptInDevTools(t *testing.T) {
	calls := 0
	shell, view := Layout(ShellOptions{
		Decorate: func(content func(*ui.Context)) func(*ui.Context) {
			return func(c *ui.Context) {
				calls++
				content(c)
				ui.Overlay(c, func() { ui.Text(c, "Decorator").Absolute().Top(100).Left(400) })
			}
		},
		Content: func(c *ui.Context) { ui.Text(c, "Original content") },
	})
	tester := ui.NewTester(view, 1000, 700)
	if calls == 0 || !tester.HasText("Decorator") || !tester.HasText("Original content") {
		t.Fatal("decorator did not retain the shell and add its overlay")
	}
	if shell.nativeOptions().Page.DevTools != mygo.DevToolsAuto {
		t.Fatal("normal shell changed default inspector policy")
	}
	shell.options.DevTools = true
	if shell.nativeOptions().Page.DevTools != mygo.DevToolsEnabled {
		t.Fatal("audit shell did not enable the native inspector")
	}
}
