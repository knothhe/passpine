package main

import (
	"fmt"
	"github.com/knothhe/passpine/client/buildinfo"
	"github.com/knothhe/passpine/client/i18n"
	"github.com/knothhe/passpine/client/nativehost"
	"os"
)

var version = "development"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--build-channel" {
		fmt.Println(buildinfo.ChannelName())
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(buildinfo.Name() + " Host " + buildinfo.Version(version))
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
		fmt.Fprintln(os.Stderr, i18n.Text(i18n.SystemLocale(), "%s 浏览器连接已注册", buildinfo.Name()))
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
