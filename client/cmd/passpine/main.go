package main

import (
	"errors"
	"flag"
	"log"

	"github.com/knothhe/passpine-neo/client/app"
	"github.com/knothhe/passpine-neo/client/desktop"
	"github.com/knothhe/passpine-neo/client/nativehost"
)

func main() {
	preview := flag.Bool("preview", false, "Create and open an isolated vault with fictional entries")
	dataDir := flag.String("data-dir", "", "Override the application data directory")
	flag.Parse()
	service, err := app.New(app.Options{DataDir: *dataDir, Preview: *preview})
	if errors.Is(err, nativehost.ErrAlreadyRunning) {
		directory := *dataDir
		if directory == "" {
			directory, err = nativehost.DataDir()
			if err != nil {
				log.Fatal(err)
			}
		}
		// This request only brings forward the running client's unlock screen;
		// it never provides a passphrase or unlocks a vault without the user.
		nativehost.Relay(directory, []byte(`{"protocol":1,"action":"unlock"}`))
		return
	}
	if err != nil {
		log.Fatal(err)
	}
	defer service.Close()
	if err := desktop.Run(service); err != nil {
		log.Fatal(err)
	}
}
