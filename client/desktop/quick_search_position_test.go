package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
)

func TestQuickSearchPositionPersistsOnHideAndRestoresAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), quickSearchPositionFile)
	a, window := quickSearchFixture()
	a.quick.positionPath = path
	window.bounds = mygo.Rectangle{X: -1500, Y: 280, Width: 660, Height: 460}
	a.hideQuickSearch()
	position, found, err := loadQuickSearchPosition(path)
	if err != nil || !found || position != (quickSearchPosition{X: -1500, Y: 280}) {
		t.Fatalf("hide did not persist the dragged position: %#v, %v, %v", position, found, err)
	}
	// A fresh application reads the file, without sharing the earlier window or
	// relying on MyGo's process-level state cache.
	restarted, _ := quickSearchFixture()
	restarted.quick.positionPath = path
	primary := mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
	secondary := mygo.Rectangle{X: -1920, Y: 0, Width: 1920, Height: 1080}
	options := quickSearchOptions(primary)
	restarted.restoreQuickSearchPosition(&options, []mygo.Rectangle{primary, secondary}, primary)
	if options.X != -1500 || options.Y != 280 || options.Width != 660 || options.Height != 460 {
		t.Fatalf("restart did not restore the saved position: %#v", options)
	}
	options = quickSearchOptions(primary)
	restarted.restoreQuickSearchPosition(&options, []mygo.Rectangle{primary}, primary)
	fallback := quickSearchBounds(primary)
	if options.X != fallback.X || options.Y != fallback.Y {
		t.Fatal("restart after disconnecting the saved monitor did not use Spotlight placement")
	}
}

func TestQuickSearchPositionSavesAfterMoveAndBeforeDestroy(t *testing.T) {
	a, window := quickSearchFixture()
	a.quick.positionPath = filepath.Join(t.TempDir(), quickSearchPositionFile)
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	window.bounds = mygo.Rectangle{X: 100, Y: 180, Width: 660, Height: 460}
	a.queueQuickSearchPositionSave()
	window.bounds.X = 250
	a.queueQuickSearchPositionSave()
	awaitMain(t, callbacks)
	position, found, err := loadQuickSearchPosition(a.quick.positionPath)
	if err != nil || !found || position.X != 250 || position.Y != 180 {
		t.Fatalf("drag completion did not save the newest position: %#v, %v", position, err)
	}
	window.bounds.X, window.bounds.Y = 0, 0
	a.closeQuickSearch()
	position, found, err = loadQuickSearchPosition(a.quick.positionPath)
	if err != nil || !found || position != (quickSearchPosition{}) || window.destroyed != 1 {
		t.Fatalf("destroy did not flush the final position, including a valid origin: %#v, %v", position, err)
	}
}

func TestQuickSearchPositionSaveRemainsAvailableDuringQuit(t *testing.T) {
	a, window := quickSearchFixture()
	a.quick.positionPath = filepath.Join(t.TempDir(), quickSearchPositionFile)
	window.bounds = mygo.Rectangle{X: 270, Y: 200, Width: 660, Height: 460}
	a.closing = true
	a.saveQuickSearchPosition()
	position, found, err := loadQuickSearchPosition(a.quick.positionPath)
	if err != nil || !found || position.X != 270 || position.Y != 200 {
		t.Fatalf("quit failed to persist position: %#v, %v", position, err)
	}
}

func TestQuickSearchPositionMissingOrDamagedFileUsesDefault(t *testing.T) {
	for _, data := range []string{"", "{", "{}", `{"x": 0}`, `{"x": null, "y": 20}`} {
		a, _ := quickSearchFixture()
		a.quick.positionPath = filepath.Join(t.TempDir(), quickSearchPositionFile)
		if data != "" {
			if err := os.WriteFile(a.quick.positionPath, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
		}
		area := mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
		options := quickSearchOptions(area)
		expectedX, expectedY := options.X, options.Y
		a.restoreQuickSearchPosition(&options, []mygo.Rectangle{area}, area)
		if options.X != expectedX || options.Y != expectedY || a.quick.positionSaved {
			t.Fatalf("invalid saved position %q changed the default", data)
		}
	}
}

func TestQuickSearchPositionSaveFailureDoesNotBlockPanel(t *testing.T) {
	a, window := quickSearchFixture()
	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	a.quick.positionPath = filepath.Join(path, quickSearchPositionFile)
	window.bounds = mygo.Rectangle{X: 100, Y: 180, Width: 660, Height: 460}
	a.hideQuickSearch()
	if a.quick.visible || window.hidden != 1 || a.busy || a.quick.positionSaved || !a.quick.positionSaveFailed || a.notice == "" {
		t.Fatal("failed persistence blocked the panel lifecycle or was silently treated as saved")
	}
}
