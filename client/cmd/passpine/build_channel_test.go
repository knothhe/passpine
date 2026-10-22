package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knothhe/passpine/client/buildinfo"
)

func TestBuildChannelReturnsBeforeDataOrDesktopInitialization(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "must-not-be-created")
	t.Setenv(buildinfo.HomeEnvironment(), directory)
	priorArgs, priorOut := os.Args, os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Args, os.Stdout = priorArgs, priorOut; reader.Close(); writer.Close() })
	os.Args, os.Stdout = []string{"passpine", "--build-channel"}, writer
	main()
	writer.Close()
	output, err := io.ReadAll(reader)
	if err != nil || strings.TrimSpace(string(output)) != buildinfo.ChannelName() {
		t.Fatalf("wrong build channel: %q, %v", output, err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("metadata inspection initialized application data")
	}
}
