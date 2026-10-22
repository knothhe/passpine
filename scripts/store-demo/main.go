// Store screenshot fixture. Run only with a newly created temporary data directory.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/nativehost"
)

func main() {
	if len(os.Args) < 3 {
		panic("expected serve|host and temporary data directory")
	}
	dir := os.Args[2]
	if os.Args[1] == "host" {
		if err := nativehost.ServeStdio(dir, os.Args[3:], os.Stdin, os.Stdout); err != nil {
			os.Exit(1)
		}
		return
	}
	if os.Args[1] != "serve" || len(os.Args) != 4 {
		panic("expected serve data-directory ready-file")
	}
	// Refuse to open an existing data directory, including a personal vault.
	if err := os.Mkdir(dir, 0700); err != nil {
		panic(err)
	}
	service, err := app.New(app.Options{DataDir: dir})
	if err != nil {
		panic(err)
	}
	defer service.Close()
	execute := func(r app.Request) {
		if response := service.Execute(r); response.Status != "ok" {
			panic(fmt.Sprintf("fixture %s: %s", r.Action, response.Code))
		}
	}
	settings := app.DefaultSettings()
	settings.DeviceUnlock, settings.AutoSync, settings.RecordActivity = false, false, false
	settings.IdleMinutes, settings.Language, settings.Theme = 30, "en", "light"
	execute(app.Request{Action: "settings", Settings: &settings})
	execute(app.Request{Action: "create", Directory: filepath.Join(dir, "vault"), Name: "Demo vault", Email: "demo@example.invalid", Passphrase: app.DemoPassphrase})
	for _, entry := range []struct{ path, username, url string }{
		{"Personal/Email", "alex@example.com", "https://mail.example.com"},
		{"Personal/Cloud", "alex@example.com", "https://cloud.example.com"},
		{"Work/Design", "alex@studio.example", "https://design.example.com"},
		{"Work/Projects", "alex@example.com", "https://projects.example.com"},
		{"Work/Code", "alex@example.com", "https://code.example.com"},
	} {
		execute(app.Request{Action: "save", Entry: entry.path, Mode: "create", Contents: fmt.Sprintf("Demo-only!not-a-real-password\nusername: %s\nurl: %s\n\nFictional account for store screenshots.\n", entry.username, entry.url)})
	}
	if err := os.WriteFile(os.Args[3], []byte("ready"), 0600); err != nil {
		panic(err)
	}
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt, syscall.SIGTERM)
	<-stopped
}
