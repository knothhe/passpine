package linuxdesktop

import "testing"

func TestRecordAccelerator(t *testing.T) {
	for _, test := range []struct {
		name  string
		state uint32
		want  string
	}{
		{"Control_L", 0, ""}, {"Shift_R", 4, ""}, {"p", 4 | 8, "Ctrl+Alt+P"},
		{"F24", 4 | 8 | 1, "Ctrl+Alt+Shift+F24"}, {"Escape", 0, "Escape"},
		{"space", 1<<26 | 1, "Cmd+Shift+Space"}, {"plus", 4, "Ctrl+Plus"},
		{"KP_4", 4, "Ctrl+Num4"}, {"XF86AudioMute", 0, "VolumeMute"},
		{"exclam", 4 | 1, "Ctrl+Shift+1"},
	} {
		if got := recordAccelerator(test.name, test.state); got != test.want {
			t.Errorf("%s: %q, want %q", test.name, got, test.want)
		}
	}
}
