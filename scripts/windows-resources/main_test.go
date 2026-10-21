package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tc-hib/winres"
)

// Check the linked PE, since generating an ICO alone does not make Explorer
// or MyGo find it in the executable. Exercise both supported architectures.
func TestLinkedExecutableContainsInstallerArtwork(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			directory := t.TempDir()
			if err := generate("../../client/assets/app-icon-monochrome.png", directory, arch); err != nil {
				t.Fatal(err)
			}
			ico, err := os.ReadFile(filepath.Join(directory, "Passpine.ico"))
			if err != nil {
				t.Fatal(err)
			}
			if len(ico) < 6 || binary.LittleEndian.Uint16(ico[2:4]) != 1 || binary.LittleEndian.Uint16(ico[4:6]) != 9 {
				t.Fatal("missing multi-resolution installer ICO")
			}
			seen := map[int]bool{}
			for i := 0; i < 9; i++ {
				size := int(ico[6+16*i])
				if size == 0 {
					size = 256
				}
				seen[size] = true
			}
			for _, size := range []int{16, 20, 24, 32, 40, 48, 64, 128, 256} {
				if !seen[size] {
					t.Fatalf("missing %dpx ICO representation", size)
				}
			}
			for name, data := range map[string]string{
				"go.mod":  "module icon-fixture\n\ngo 1.19\n",
				"main.go": "package main\nfunc main() {}\n",
			} {
				if err := os.WriteFile(filepath.Join(directory, name), []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("GOOS", "windows")
			t.Setenv("GOARCH", arch)
			t.Setenv("CGO_ENABLED", "0")
			command := exec.Command("go", "build", "-o", "fixture.exe", ".")
			command.Dir = directory
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("link Windows resource: %v\n%s", err, output)
			}
			executable, err := os.Open(filepath.Join(directory, "fixture.exe"))
			if err != nil {
				t.Fatal(err)
			}
			defer executable.Close()
			resources, err := winres.LoadFromEXE(executable)
			if err != nil {
				t.Fatal(err)
			}
			icon, err := resources.GetIcon(winres.ID(1))
			if err != nil {
				t.Fatal(err)
			}
			var embedded bytes.Buffer
			if err := icon.SaveICO(&embedded); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(ico, embedded.Bytes()) {
				t.Fatal("linked application icon differs from installer artwork")
			}
		})
	}
}
