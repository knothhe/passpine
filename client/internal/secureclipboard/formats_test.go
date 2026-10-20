package secureclipboard

import (
	"bytes"
	"slices"
	"testing"

	"github.com/egoist/mygo/transfer"
)

func TestSensitiveClipboardFormats(t *testing.T) {
	const password = "密码\x00\nclipboard-test"
	for _, test := range []struct {
		name string
		data transfer.Data
		want map[transfer.Format][]byte
	}{
		{
			name: "windows",
			data: windowsSensitiveData(password),
			want: map[transfer.Format][]byte{
				transfer.Text:         []byte(password),
				windowsExcludeMonitor: {1},
				windowsAllowHistory:   {0, 0, 0, 0},
				windowsAllowCloud:     {0, 0, 0, 0},
			},
		},
		{
			name: "linux",
			data: linuxSensitiveData(password),
			want: map[transfer.Format][]byte{
				transfer.Text:     []byte(password),
				linuxPasswordHint: []byte("secret"),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			items := test.data.Items()
			if len(items) != 1 || len(items[0].Formats()) != len(test.want) {
				t.Fatalf("text and privacy hints must share one item: %v", test.data.Formats())
			}
			for format, want := range test.want {
				if !slices.Contains(items[0].Formats(), format) {
					t.Errorf("missing raw format %q", format)
				}
				got, err := items[0].Read(format)
				if err != nil || !bytes.Equal(got, want) {
					t.Errorf("%s: got %v, error %v; want %v", format, got, err, want)
				}
			}
		})
	}
}
