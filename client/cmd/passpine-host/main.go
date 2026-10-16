package main

import (
	"fmt"
	"github.com/knothhe/passpine/client/nativehost"
	"os"
)

var version = "development"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("Passpine Host " + version)
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--register" {
		p, e := os.Executable()
		if e == nil {
			_, e = nativehost.Register(p)
		}
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "Passpine native messaging host registered")
		return
	}
	dir, e := nativehost.DataDir()
	if e != nil {
		os.Exit(1)
	}
	if e = nativehost.ServeStdio(dir, os.Args[1:], os.Stdin, os.Stdout); e != nil {
		os.Exit(1)
	}
}
