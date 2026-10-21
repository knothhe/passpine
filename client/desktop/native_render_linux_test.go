package desktop

import (
	"os"
	"testing"
)

func TestLinuxDefaultsToSoftwareRendering(t *testing.T) {
	for _, test := range []struct{ value, want string }{{"", "0"}, {"0", "0"}, {"1", "1"}} {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv("MYGO_GPU", test.value)
			configureNativeRendering()
			if got := os.Getenv("MYGO_GPU"); got != test.want {
				t.Fatalf("renderer = %q, want %q", got, test.want)
			}
		})
	}
}
