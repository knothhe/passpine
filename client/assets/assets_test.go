package assets

import (
	"bytes"
	"encoding/binary"
	"github.com/knothhe/passpine/client/buildinfo"
	"image/png"
	"testing"
)

func TestDefaultIconIsMonochromeAndExplicitTerracottaIsPreserved(t *testing.T) {
	prior := buildinfo.Channel
	buildinfo.Channel = "release"
	t.Cleanup(func() { buildinfo.Channel = prior })
	for _, style := range []string{"", "monochrome", "unknown"} {
		if !bytes.Equal(IconForStyle(style), AppIconMonochromePNG) {
			t.Fatalf("%q did not use the default monochrome icon", style)
		}
	}
	if !bytes.Equal(IconForStyle("onevra"), AppIconPNG) {
		t.Fatal("explicit terracotta icon preference was replaced")
	}
}

func TestDockArtworkIncludesOpticalAndRetinaRepresentations(t *testing.T) {
	prior := buildinfo.Channel
	buildinfo.Channel = "release"
	t.Cleanup(func() { buildinfo.Channel = prior })
	expected := map[string]int{
		"ic07": 128, "ic08": 256, "ic09": 512,
		"ic11": 32, "ic12": 64, "ic13": 256, "ic14": 512, "ic10": 1024,
	}
	for _, style := range []string{"", "monochrome", "unknown", "onevra"} {
		data := DockIconForStyle(style)
		if len(data) < 8 || string(data[:4]) != "icns" || int(binary.BigEndian.Uint32(data[4:8])) != len(data) {
			t.Fatalf("%q: Dock artwork is not a complete ICNS file", style)
		}
		seen := make(map[string]bool)
		for offset := 8; offset < len(data); {
			if offset+8 > len(data) {
				t.Fatalf("%q: truncated ICNS representation header", style)
			}
			kind := string(data[offset : offset+4])
			length := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
			if length < 8 || offset+length > len(data) {
				t.Fatalf("%q: invalid %s representation length", style, kind)
			}
			payload := data[offset+8 : offset+length]
			if kind == "info" || kind == "TOC " {
				offset += length
				continue
			}
			size, ok := expected[kind]
			if !ok || seen[kind] {
				t.Fatalf("%q: unexpected or duplicate %s representation", style, kind)
			}
			config, err := png.DecodeConfig(bytes.NewReader(payload))
			if err != nil || config.Width != size || config.Height != size {
				t.Fatalf("%q: invalid %s representation: %+v (%v)", style, kind, config, err)
			}
			seen[kind] = true
			offset += length
		}
		if len(seen) != len(expected) {
			t.Fatalf("%q: missing standard or Retina representations", style)
		}
	}
}

func TestDevelopmentIconsAreDistinctForBothStylesAndHaveRetinaDockResources(t *testing.T) {
	prior := buildinfo.Channel
	buildinfo.Channel = "dev"
	t.Cleanup(func() { buildinfo.Channel = prior })
	for _, style := range []string{"monochrome", "onevra"} {
		original := AppIconMonochromePNG
		if style == "onevra" {
			original = AppIconPNG
		}
		data := IconForStyle(style)
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width != 256 || bytes.Equal(data, original) {
			t.Fatalf("missing Dev variant for %s", style)
		}
		data = DockIconForStyle(style)
		seen := make(map[int]bool)
		for offset := 8; offset+8 <= len(data); {
			length := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
			if length < 8 || offset+length > len(data) {
				t.Fatal("invalid Dev ICNS")
			}
			if string(data[offset:offset+4]) != "TOC " {
				config, err := png.DecodeConfig(bytes.NewReader(data[offset+8 : offset+length]))
				if err != nil {
					t.Fatal(err)
				}
				seen[config.Width] = true
			}
			offset += length
		}
		for _, size := range []int{32, 64, 128, 256, 512, 1024} {
			if !seen[size] {
				t.Fatalf("Dev Dock icon missing %dpx", size)
			}
		}
	}
}

// macOS 27 IconServices renders icp4/icp5 PNG chunks as colored noise and
// puts ic04/ic05 ARGB artwork on a grey legacy plate. Check every shipped
// variant, including resources used by Dev builds.
func TestAllDockIconsOmitStandardResolutionSmallChunks(t *testing.T) {
	for name, data := range map[string][]byte{
		"onevra": appIconICNS, "monochrome": monochromeIconICNS,
		"dev-onevra": devIconICNS, "dev-monochrome": devMonochromeICNS,
	} {
		if len(data) < 8 || string(data[:4]) != "icns" || int(binary.BigEndian.Uint32(data[4:8])) != len(data) {
			t.Fatalf("%s: invalid ICNS container", name)
		}
		for offset := 8; offset < len(data); {
			if offset+8 > len(data) {
				t.Fatalf("%s: truncated ICNS header", name)
			}
			kind := string(data[offset : offset+4])
			length := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
			if length < 8 || length > len(data)-offset {
				t.Fatalf("%s: invalid ICNS chunk length", name)
			}
			switch kind {
			case "icp4", "icp5", "ic04", "ic05":
				t.Fatalf("%s: %s renders incorrectly in macOS IconServices; regenerate with generate.swift", name, kind)
			}
			offset += length
		}
	}
}
