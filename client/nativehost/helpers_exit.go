package nativehost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/knothhe/passpine/client/buildinfo"
)

type helperExit struct {
	Executable string
	Time       time.Time
}

func RequestDevelopmentHelpersExit() error {
	if buildinfo.IsRelease() {
		return nil
	}
	directory, err := DataDir()
	if err != nil {
		return err
	}
	if err = privateDirectory(directory); err != nil {
		return err
	}
	helper, err := filepath.EvalSymlinks(HelperPath())
	if err != nil {
		return err
	}
	data, _ := json.Marshal(helperExit{helper, time.Now()})
	return writePrivate(filepath.Join(directory, "helpers-exit.json"), data)
}

// Keep control independent of a blocked OS stdin read: closing stdin from
// another goroutine is not guaranteed to cancel that read on every platform.
func watchHelperExit(directory string) (<-chan struct{}, func()) {
	if buildinfo.IsRelease() {
		return nil, func() {}
	}
	started := time.Now()
	executable, err := os.Executable()
	if err != nil {
		return nil, func() {}
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, func() {}
	}
	done := make(chan struct{})
	exit := make(chan struct{})
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case now := <-ticker.C:
				file := filepath.Join(directory, "helpers-exit.json")
				info, err := os.Lstat(file)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 || !ownedPrivate(info) {
					continue
				}
				data, err := os.ReadFile(file)
				var request helperExit
				// A quick reinstall can reuse the same executable path. Requests
				// from before this helper started must not stop the new instance.
				if err == nil && json.Unmarshal(data, &request) == nil && request.Executable == executable && !request.Time.Before(started) && !request.Time.After(now) && now.Sub(request.Time) < time.Minute {
					close(exit)
					return
				}
			}
		}
	}()
	return exit, func() { close(done) }
}
