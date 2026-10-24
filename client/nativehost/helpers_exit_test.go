package nativehost

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDevHelperExitRequiresMatchingExecutableAndFreshRequest(t *testing.T) {
	directory := t.TempDir()
	done, stop := watchHelperExit(directory)
	defer stop()
	executable, _ := os.Executable()
	executable, _ = filepath.EvalSymlinks(executable)
	write := func(request helperExit) {
		data, _ := json.Marshal(request)
		if err := writePrivate(filepath.Join(directory, "helpers-exit.json"), data); err != nil {
			t.Fatal(err)
		}
	}
	for _, request := range []helperExit{{"/another/host", time.Now()}, {executable, time.Now().Add(-2 * time.Minute)}, {executable, time.Now().Add(time.Hour)}} {
		write(request)
		select {
		case <-done:
			t.Fatal("unrelated or stale request stopped helper")
		case <-time.After(80 * time.Millisecond):
		}
	}
	write(helperExit{executable, time.Now()})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("matching helper did not release its input")
	}
}

func TestDevUninstallEndsRealStdioHelperWithoutKillingIt(t *testing.T) {
	helper := os.Getenv("PASSPINE_TEST_HOST")
	if helper == "" {
		t.Skip("set PASSPINE_TEST_HOST to the compiled test helper")
	}
	helper, err := filepath.Abs(helper)
	if err != nil {
		t.Fatal(err)
	}
	helper, err = filepath.EvalSymlinks(helper)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	command := exec.Command(helper, "chrome-extension://"+ChromiumID+"/")
	command.Env = append(os.Environ(), "PASSPINE_DEV_HOME="+directory)
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	// An initial reply proves this helper is watching for exit requests before
	// the request is written; process creation alone does not establish that.
	if err := WriteFrame(input, []byte(`{"protocol":1,"action":"ping"}`)); err != nil {
		t.Fatal(err)
	}
	ready := make(chan error, 1)
	go func() { _, err := ReadFrame(output); ready <- err }()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not enter the message loop")
	}
	data, _ := json.Marshal(helperExit{helper, time.Now()})
	if err := writePrivate(filepath.Join(directory, "helpers-exit.json"), data); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("uninstall did not unblock the real helper's idle stdin")
	}
}

func TestNewDevHelperIgnoresPreviousUninstallRequest(t *testing.T) {
	directory := t.TempDir()
	executable, _ := os.Executable()
	executable, _ = filepath.EvalSymlinks(executable)
	file := filepath.Join(directory, "helpers-exit.json")
	data, _ := json.Marshal(helperExit{executable, time.Now()})
	if err := writePrivate(file, data); err != nil {
		t.Fatal(err)
	}
	done, stop := watchHelperExit(directory)
	defer stop()
	select {
	case <-done:
		t.Fatal("a previous uninstall request closed the newly started helper")
	case <-time.After(100 * time.Millisecond):
	}
	data, _ = json.Marshal(helperExit{executable, time.Now()})
	if err := writePrivate(file, data); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the new uninstall request did not close the helper")
	}
}
