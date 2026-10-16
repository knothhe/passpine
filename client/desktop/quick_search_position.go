package desktop

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
)

const quickSearchPositionFile = "quick-search-position.json"

type quickSearchPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Panels do not reliably report a normal window state on every platform. Keep
// just their position ourselves rather than depending on StateKey's maximized
// state detection. The fixed panel size remains controlled by the current UI.
func loadQuickSearchPosition(path string) (quickSearchPosition, bool, error) {
	var position quickSearchPosition
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return position, false, nil
	}
	if err != nil {
		return position, false, err
	}
	defer file.Close()
	var saved struct {
		X *int `json:"x"`
		Y *int `json:"y"`
	}
	if err = json.NewDecoder(io.LimitReader(file, 4096)).Decode(&saved); err != nil {
		return position, false, err
	}
	if saved.X == nil || saved.Y == nil {
		return position, false, errors.New("window position is incomplete")
	}
	position.X, position.Y = *saved.X, *saved.Y
	return position, true, nil
}

func storeQuickSearchPosition(path string, position quickSearchPosition) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(position)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".quick-search-position-*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if _, err = file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func (a *application) restoreQuickSearchPosition(options *mygo.WindowOptions, areas []mygo.Rectangle, fallback mygo.Rectangle) {
	if a.quick.mode == quickSearchTray || a.quick.positionPath == "" {
		return
	}
	position, found, err := loadQuickSearchPosition(a.quick.positionPath)
	if err != nil || !found {
		return
	}
	a.quick.savedPosition, a.quick.positionSaved = position, true
	bounds := quickSearchPlacement(mygo.Rectangle{X: position.X, Y: position.Y, Width: options.Width, Height: options.Height}, areas, fallback)
	options.X, options.Y, options.Width, options.Height = bounds.X, bounds.Y, bounds.Width, bounds.Height
}

func (a *application) queueQuickSearchPositionSave() {
	if a.quick.mode == quickSearchTray || a.quick.window == nil || a.quick.positionPath == "" {
		return
	}
	if a.quick.positionTimer != nil {
		a.quick.positionTimer.Stop()
	}
	a.quick.positionTimer = time.AfterFunc(250*time.Millisecond, func() {
		a.runOnMain(func() { a.saveQuickSearchPosition() })
	})
}

func (a *application) saveQuickSearchPosition() {
	if a.quick.positionTimer != nil {
		a.quick.positionTimer.Stop()
		a.quick.positionTimer = nil
	}
	if a.quick.mode == quickSearchTray || a.quick.window == nil || a.quick.positionPath == "" {
		return
	}
	bounds := a.quick.window.Bounds()
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return
	}
	position := quickSearchPosition{X: bounds.X, Y: bounds.Y}
	if a.quick.positionSaved && a.quick.savedPosition == position {
		return
	}
	if err := storeQuickSearchPosition(a.quick.positionPath, position); err != nil {
		if !a.quick.positionSaveFailed {
			a.warn(a.t("无法记住快速搜索窗口的位置：%s", err))
			a.quick.positionSaveFailed = true
		}
		return
	}
	a.quick.savedPosition, a.quick.positionSaved = position, true
	a.quick.positionSaveFailed = false
}
