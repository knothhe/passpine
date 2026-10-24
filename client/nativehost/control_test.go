package nativehost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDevelopmentControlRejectsAnotherInstallationAndBrowserMessages(t *testing.T) {
	dir := t.TempDir()
	requests := make(chan struct{}, 1)
	server, err := StartDevelopmentControl(dir, func() { requests <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	if server == nil {
		t.Skip("release build does not expose Dev control")
	}
	defer server.Close()
	for _, raw := range []string{`{"protocol":1,"action":"quit","executable":"/another/Passpine Dev"}`, `{"protocol":1,"action":"unlock"}`} {
		var response struct{ Status string }
		if json.Unmarshal(Relay(filepath.Join(dir, "control"), []byte(raw)), &response) != nil || response.Status != "error" {
			t.Fatal("invalid control request accepted")
		}
	}
	executable, _ := os.Executable()
	executable, _ = filepath.EvalSymlinks(executable)
	data, _ := json.Marshal(map[string]any{"protocol": Protocol, "action": "quit", "executable": executable})
	Relay(filepath.Join(dir, "control"), data)
	select {
	case <-requests:
	case <-time.After(time.Second):
		t.Fatal("matching installation was not asked to quit")
	}
}

func TestQuitIgnoresStaleEndpointAfterCrash(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PASSPINE_DEV_HOME", dir)
	control := filepath.Join(dir, "control")
	if err := os.Mkdir(control, 0700); err != nil {
		t.Fatal(err)
	}
	endpoint, _ := json.Marshal(Endpoint{Address: "127.0.0.1:1", Token: strings.Repeat("a", 64), PID: os.Getpid()})
	if err := os.WriteFile(filepath.Join(control, "browser-endpoint.json"), endpoint, 0600); err != nil {
		t.Fatal(err)
	}
	if err := QuitDevelopmentInstance(); err != nil {
		t.Fatal(err)
	}
}
