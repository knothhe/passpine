package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/knothhe/passpine/client/i18n"
)

func TestCommandLinksAreUnsupportedOnWindows(t *testing.T) {
	for _, operation := range []string{"install", "uninstall", "status"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{passAlias, commandName()} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("existing command"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			c := &command{program: commandName(), locale: i18n.English, stdout: &out}
			err := c.cli([]string{operation, "--pass-alias", "--dir", dir})
			if err == nil || err.Error() != "Installing command links automatically is not supported on Windows yet." {
				t.Fatalf("%s error = %v", operation, err)
			}
			if out.Len() != 0 {
				t.Fatalf("unsupported operation produced output: %q", out.String())
			}
			for _, name := range []string{passAlias, commandName()} {
				contents, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || string(contents) != "existing command" {
					t.Fatalf("%s changed an existing command: %q, %v", operation, contents, err)
				}
			}
			missing := filepath.Join(dir, "missing")
			if err := c.cli([]string{operation, "--dir", missing}); err == nil {
				t.Fatal("unsupported operation accepted an absent directory")
			}
			if _, err := os.Stat(missing); !os.IsNotExist(err) {
				t.Fatalf("unsupported operation created a directory: %v", err)
			}
		})
	}
}
