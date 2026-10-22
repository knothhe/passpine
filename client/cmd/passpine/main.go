package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/buildinfo"
	"github.com/knothhe/passpine/client/desktop"
	"github.com/knothhe/passpine/client/deviceunlock"
	"github.com/knothhe/passpine/client/nativehost"
	"github.com/knothhe/passpine/client/update"
)

var version = "development"
var updateFeedURL string
var updatePublicKey string

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--build-channel" {
		fmt.Println(buildinfo.ChannelName())
		return
	}
	version = buildinfo.Version(version)
	if buildinfo.IsRelease() && update.RunHelper(version, updatePublicKey) {
		return
	}
	if deviceunlock.RunHelper() {
		return
	}
	showVersion := flag.Bool("version", false, "Print the application version and exit")
	preview := flag.Bool("preview", false, "Create and open an isolated vault with fictional entries")
	dataDir := flag.String("data-dir", "", "Override the application data directory")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.Name() + " " + version)
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
	var updates update.Config
	if buildinfo.IsRelease() {
		updates = update.Config{FeedURL: updateFeedURL, PublicKey: updatePublicKey}
	}
	if err := desktop.Run(service, version, updates); err != nil {
		log.Fatal(err)
	}
}
