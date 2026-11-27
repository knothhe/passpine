package desktop

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
)

func TestManualBrowserPickerSelectsMacApplicationBundle(t *testing.T) {
	a := fixture()
	a.state.Preview = false
	a.preferences.browser.open = true
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	chosen := "/Volumes/External Apps/Browser With Spaces.app"
	var options mygo.OpenDialogOptions
	a.preferences.openDialog = func(o mygo.OpenDialogOptions) ([]string, error) {
		options = o
		return []string{chosen}, nil
	}
	for _, wantDirectory := range []string{"/Applications", filepath.Dir(chosen)} {
		a.chooseBrowserProgram()
		awaitMain(t, callbacks)
		if len(options.Filters) != 0 || options.Directory || options.TreatPackagesAsDirectories {
			t.Fatalf("picker must allow whole application bundles: %+v", options)
		}
		if options.DefaultPath != wantDirectory || a.preferences.browser.value.Path != chosen {
			t.Fatalf("picker opened at %q, selected %q", options.DefaultPath, a.preferences.browser.value.Path)
		}
	}
}
