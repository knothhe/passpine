package assets

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestDefaultIconIsMonochromeAndExplicitTerracottaIsPreserved(t *testing.T) {
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
	expected := map[string]int{
		"icp4": 16, "icp5": 32, "ic07": 128, "ic08": 256, "ic09": 512,
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
			config, err := png.DecodeConfig(bytes.NewReader(data[offset+8 : offset+length]))
			if size, ok := expected[kind]; !ok || seen[kind] || err != nil || config.Width != size || config.Height != size {
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
