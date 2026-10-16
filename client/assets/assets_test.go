package assets

import (
	"bytes"
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
