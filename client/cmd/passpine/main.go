package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/buildinfo"
	"github.com/knothhe/passpine/client/cli"
	"github.com/knothhe/passpine/client/desktop"
	"github.com/knothhe/passpine/client/deviceunlock"
	"github.com/knothhe/passpine/client/nativehost"
	"github.com/knothhe/passpine/client/update"
)

var version = "development"
var updateFeedURL string
var updatePublicKey string

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--build-info" {
		_ = json.NewEncoder(os.Stdout).Encode(buildinfo.Current(version))
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--quit-for-uninstall" {
		if err := nativehost.QuitDevelopmentInstance(); err != nil {
			log.Fatal(err)
		}
		if err := nativehost.RequestDevelopmentHelpersExit(); err != nil {
			log.Fatal(err)
		}
		if err := desktop.PrepareDevelopmentUninstall(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--build-channel" {
		fmt.Println(buildinfo.ChannelName())
		return
	}
	version = buildinfo.Version(version)
	if buildinfo.IsRelease() && update.RunHelper(version, updatePublicKey) {
		return
	}
	if deviceunlock.RunHelper() || cli.RunHelper() {
		return
	}
	// `passpine <command>` and the opt-in `pass` alias run the pass-compatible
	// command line; a bare launch still opens the desktop client.
	if cli.Invoked(os.Args) {
		os.Exit(cli.Main(os.Args, version))
	}
	showVersion := flag.Bool("version", false, "Print the application version and exit")
	preview := flag.Bool("preview", false, "Create and open an isolated vault with fictional entries")
	dataDir := flag.String("data-dir", "", "Override the application data directory")
	seedTestVault := flag.Bool("seed-test-vault", false, "Development builds: create a fictional test vault if it does not exist")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.Name() + " " + version)
		return
	}
	if *seedTestVault {
		directory, created, err := app.SeedDevelopmentVault(*dataDir)
		switch {
		case errors.Is(err, nativehost.ErrAlreadyRunning):
			log.Fatalf("Quit %s, then run it again with --seed-test-vault.", buildinfo.Name())
		case err != nil:
			log.Fatal(err)
		case created:
			fmt.Printf("Created test vault: %s\nPassphrase: %s\n", directory, app.DemoPassphrase)
		default:
			fmt.Printf("Test vault already exists: %s\n", directory)
		}
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
