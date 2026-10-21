package desktop

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"
)

const managerLayoutFile = "manager-layout.json"

type managerLayoutState struct {
	path              string
	timer             *time.Timer
	serial            uint64
	dirty             bool
	saved             bool
	savedWidth        float32
	savedSidebarWidth float32
	saveFailed        bool
}

type managerLayoutValues struct {
	ListWidth    float32 `json:"listWidth"`
	SidebarWidth float32 `json:"sidebarWidth,omitempty"`
}

func validSidebarWidth(width float32) bool {
	return !math.IsNaN(float64(width)) && !math.IsInf(float64(width), 0) && (width == 0 || width >= 180 && width <= 280)
}

func validManagerListWidth(width float32) bool {
	return !math.IsNaN(float64(width)) && !math.IsInf(float64(width), 0) &&
		(width == 0 || width >= managerListMinWidth && width <= managerListMaxWidth)
}

func loadManagerLayout(path string) (float32, bool, error) {
	values, found, err := loadManagerLayoutValues(path)
	return values.ListWidth, found, err
}

func loadManagerLayoutValues(path string) (managerLayoutValues, bool, error) {
	var empty managerLayoutValues
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return empty, false, nil
	}
	if err != nil {
		return empty, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return empty, false, errors.New("invalid manager layout file")
	}
	file, err := os.Open(path)
	if err != nil {
		return empty, false, err
	}
	defer file.Close()
	var saved struct {
		ListWidth    *float32 `json:"listWidth"`
		SidebarWidth float32  `json:"sidebarWidth"`
	}
	decoder := json.NewDecoder(io.LimitReader(file, 4097))
	if err = decoder.Decode(&saved); err != nil {
		return empty, false, err
	}
	if saved.ListWidth == nil || !validManagerListWidth(*saved.ListWidth) || !validSidebarWidth(saved.SidebarWidth) {
		return empty, false, errors.New("invalid saved layout width")
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return empty, false, errors.New("invalid manager layout data")
	}
	return managerLayoutValues{ListWidth: *saved.ListWidth, SidebarWidth: saved.SidebarWidth}, true, nil
}

func storeManagerLayout(path string, width float32) error {
	return storeManagerLayoutValues(path, managerLayoutValues{ListWidth: width})
}

func storeManagerLayoutValues(path string, values managerLayoutValues) error {
	if !validManagerListWidth(values.ListWidth) || !validSidebarWidth(values.SidebarWidth) {
		return errors.New("invalid list width")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".manager-layout-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// Load before the first window is built, from the same directory as the
// service's existing local preferences. The saved value is independent of the
// current screen size; only the rendered split is temporarily constrained.
func (a *application) restoreManagerLayout() {
	directory, ok := a.service.(interface{ DataDirectory() string })
	if !ok || directory.DataDirectory() == "" {
		return
	}
	a.managerLayout.path = filepath.Join(directory.DataDirectory(), managerLayoutFile)
	if values, found, err := loadManagerLayoutValues(a.managerLayout.path); err == nil && found {
		a.managerListPreferredWidth = values.ListWidth
		a.lifecycle.leftWidth = values.SidebarWidth
		a.managerLayout.savedWidth, a.managerLayout.saved = values.ListWidth, true
		a.managerLayout.savedSidebarWidth = values.SidebarWidth
	}
}

func (a *application) queueManagerLayoutSave() {
	state := &a.managerLayout
	if state.path == "" || a.closing {
		return
	}
	state.serial++
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
	state.dirty = !state.saved || state.savedWidth != a.managerListPreferredWidth || state.savedSidebarWidth != a.lifecycle.leftWidth
	if !state.dirty {
		return
	}
	serial := state.serial
	state.timer = time.AfterFunc(250*time.Millisecond, func() {
		a.runOnMain(func() {
			if serial == a.managerLayout.serial && !a.closing {
				a.saveManagerLayout()
			}
		})
	})
}

// Normal exit flushes synchronously, including a last drag or reset made before
// the debounce expires. A queued timer callback cannot overwrite that result.
func (a *application) saveManagerLayout() {
	state := &a.managerLayout
	state.serial++
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
	if state.path == "" || !state.dirty {
		return
	}
	values := managerLayoutValues{ListWidth: a.managerListPreferredWidth, SidebarWidth: a.lifecycle.leftWidth}
	if err := storeManagerLayoutValues(state.path, values); err != nil {
		if !state.saveFailed {
			a.warn(a.t("无法记住列表宽度：%s", err))
			state.saveFailed = true
		}
		return
	}
	state.savedWidth, state.saved = a.managerListPreferredWidth, true
	state.savedSidebarWidth = a.lifecycle.leftWidth
	state.dirty, state.saveFailed = false, false
}
