package nativehost

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/knothhe/passpine/client/buildinfo"
)

// Installation control is separate from the browser protocol. The private
// endpoint only accepts a quit request for its own exact executable path.
func StartDevelopmentControl(directory string, quit func()) (*Server, error) {
	if buildinfo.IsRelease() {
		return nil, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, err
	}
	return Start(filepath.Join(directory, "control"), func(raw json.RawMessage) json.RawMessage {
		var request struct {
			Protocol           int
			Action, Executable string
		}
		if json.Unmarshal(raw, &request) != nil || request.Protocol != Protocol || request.Action != "quit" || request.Executable != executable {
			return failure("wrong_installation")
		}
		quit()
		return json.RawMessage(`{"status":"ok"}`)
	})
}

func QuitDevelopmentInstance() error {
	if buildinfo.IsRelease() {
		return errors.New("only development installations accept this command")
	}
	directory, err := DataDir()
	if err != nil {
		return err
	}
	endpoint := filepath.Join(directory, "control", "browser-endpoint.json")
	if _, err := os.Lstat(endpoint); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	request, _ := json.Marshal(map[string]any{"protocol": Protocol, "action": "quit", "executable": executable})
	var response struct{ Status, Code string }
	err = json.Unmarshal(Relay(filepath.Dir(endpoint), request), &response)
	// A crashed application's private endpoint may remain. The uninstaller
	// independently checks exact running executable paths before any removal.
	if err == nil && response.Code == "app_unavailable" {
		return nil
	}
	if err != nil || response.Status != "ok" {
		return errors.New("cannot request exit from this installation; quit it manually and retry")
	}
	deadline := time.Now().Add(65 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(endpoint); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("application has not exited; finish or cancel pending edits and retry")
}
