package assets

import (
	"bytes"
	"encoding/binary"
	"github.com/knothhe/passpine/client/buildinfo"
	"image/color"
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
		"ic04": 16, "ic05": 32, "ic07": 128, "ic08": 256, "ic09": 512,
		"ic11": 32, "ic12": 64, "ic13": 256, "ic14": 512, "ic10": 1024,
	}
	for _, style := range []string{"", "monochrome", "unknown", "onevra"} {
		data := DockIconForStyle(style)
		if len(data) < 8 || string(data[:4]) != "icns" || int(binary.BigEndian.Uint32(data[4:8])) != len(data) {
			t.Fatalf("%q: Dock artwork is not a complete ICNS file", style)
		}
		seen := make(map[string]bool)
		var small32 []byte
		var retina16 []byte
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
			if kind == "ic04" || kind == "ic05" {
				decoded := decodeIconARGB(t, payload, size)
				if size == 32 {
					small32 = decoded
				}
			} else {
				config, err := png.DecodeConfig(bytes.NewReader(payload))
				if err != nil || config.Width != size || config.Height != size {
					t.Fatalf("%q: invalid %s representation: %+v (%v)", style, kind, config, err)
				}
			}
			if kind == "ic11" {
				retina16 = payload
			}
			seen[kind] = true
			offset += length
		}
		if len(seen) != len(expected) {
			t.Fatalf("%q: missing standard or Retina representations", style)
		}
		// The 32px standard and 16pt@2x drawings must match, including straight
		// alpha and orientation. The PNG is decoded independently of ARGB.
		retina, err := png.Decode(bytes.NewReader(retina16))
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 32; y++ {
			for x := 0; x < 32; x++ {
				want := color.NRGBAModel.Convert(retina.At(x, y)).(color.NRGBA)
				for channel, value := range []byte{want.A, want.R, want.G, want.B} {
					got := small32[channel*32*32+y*32+x]
					if int(got)-int(value) > 1 || int(value)-int(got) > 1 {
						t.Fatalf("%q: ARGB differs from Retina PNG at %d,%d channel %d: got %d, want %d", style, x, y, channel, got, value)
					}
				}
			}
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

// IconServices on macOS 27 misrenders small PNG chunks as colored noise.
// Check every shipped variant, including resources used by Dev builds.
func TestAllDockIconsAvoidSmallPNGChunks(t *testing.T) {
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
			if kind == "icp4" || kind == "icp5" {
				t.Fatalf("%s: %s is incompatible with macOS IconServices; regenerate with generate.swift", name, kind)
			}
			offset += length
		}
	}
}

func decodeIconARGB(t *testing.T, payload []byte, size int) []byte {
	t.Helper()
	if len(payload) < 4 || string(payload[:4]) != "ARGB" {
		t.Fatal("small ICNS must use ARGB")
	}
	var pixels []byte
	for offset := 4; offset < len(payload); {
		control := int(payload[offset])
		offset++
		if control < 128 {
			count := control + 1
			if count > len(payload)-offset {
				t.Fatal("truncated ARGB literal")
			}
			pixels = append(pixels, payload[offset:offset+count]...)
			offset += count
		} else {
			if offset == len(payload) {
				t.Fatal("truncated ARGB repeat")
			}
			pixels = append(pixels, bytes.Repeat(payload[offset:offset+1], control-125)...)
			offset++
		}
		if len(pixels) > 4*size*size {
			t.Fatal("oversized ARGB planes")
		}
	}
	if len(pixels) != 4*size*size {
		t.Fatalf("ARGB has %d bytes, want %d", len(pixels), 4*size*size)
	}
	return pixels
}
