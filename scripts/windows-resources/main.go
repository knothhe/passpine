// Build-only tool: share one multi-resolution icon between the Go executable
// and NSIS without requiring a C resource compiler or changing runtime deps.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/tc-hib/winres"
)

func main() {
	iconPath := flag.String("icon", "", "Source PNG artwork")
	output := flag.String("out", "", "Output directory")
	arch := flag.String("arch", "", "Windows architecture (amd64 or arm64)")
	flag.Parse()
	if err := generate(*iconPath, *output, *arch); err != nil {
		log.Fatal(err)
	}
}

func generate(iconPath, output, arch string) error {
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("unsupported Windows architecture %q", arch)
	}
	if iconPath == "" || output == "" {
		return fmt.Errorf("-icon and -out are required")
	}
	data, err := os.ReadFile(iconPath)
	if err != nil {
		return err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	icon, err := winres.NewIconFromResizedImage(img, []int{16, 20, 24, 32, 40, 48, 64, 128, 256})
	if err != nil {
		return err
	}
	var resources winres.ResourceSet
	// MyGo loads resource ID 1 as the initial application/window icon.
	if err := resources.SetIcon(winres.ID(1), icon); err != nil {
		return err
	}
	var ico, object bytes.Buffer
	if err := icon.SaveICO(&ico); err != nil {
		return err
	}
	if err := resources.WriteObject(&object, winres.Arch(arch)); err != nil {
		return err
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(output, "Passpine.ico"), ico.Bytes(), 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(output, "rsrc_windows_"+arch+".syso"), object.Bytes(), 0644)
}
