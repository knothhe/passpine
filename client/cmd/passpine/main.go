package main

import (
	"errors"
	"flag"
	"fmt"
	"log"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/desktop"
	"github.com/knothhe/passpine/client/deviceunlock"
	"github.com/knothhe/passpine/client/nativehost"
)

var version = "development"

func main() {
	if deviceunlock.RunHelper() {
		return
	}
	showVersion := flag.Bool("version", false, "Print the application version and exit")
	preview := flag.Bool("preview", false, "Create and open an isolated vault with fictional entries")
	dataDir := flag.String("data-dir", "", "Override the application data directory")
	flag.Parse()
	if *showVersion {
		fmt.Println("Passpine " + version)
		return
	}
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
	if err := desktop.Run(service, version); err != nil {
		log.Fatal(err)
	}
}
