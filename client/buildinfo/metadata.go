package buildinfo

import (
	"runtime"
	"runtime/debug"
	"sync"
)

// Injected by the build script; direct go run/build uses Go's VCS metadata.
var Commit, Modified, Executor string

var sourceMetadata = sync.OnceValue(func() map[string]string {
	values := make(map[string]string)
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			values[setting.Key] = setting.Value
		}
	}
	return values
})

type Metadata struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Channel  string `json:"channel"`
	Commit   string `json:"commit"`
	Modified string `json:"modified"`
	Executor string `json:"executor"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

func Current(version string) Metadata {
	if version == "" {
		version = "development"
	}
	m := Metadata{Name: Name(), Version: Version(version), Channel: ChannelName(), Commit: Commit, Modified: Modified, Executor: Executor, OS: runtime.GOOS, Arch: runtime.GOARCH}
	if m.Executor == "" {
		m.Executor = "local"
	}
	if m.Commit == "" {
		m.Commit = sourceMetadata()["vcs.revision"]
	}
	if m.Modified == "" {
		m.Modified = sourceMetadata()["vcs.modified"]
	}
	return m
}
