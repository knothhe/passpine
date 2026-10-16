package desktop

import (
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func quickSearchOpenShortcut() string {
	if runtime.GOOS == "darwin" {
		return "⌘O"
	}
	return "Ctrl+O"
}

func quickSearchControlShortcut(key string) string {
	if runtime.GOOS == "darwin" {
		return "⌃" + key
	}
	return "Ctrl+" + key
}

func quickSearchKeycap(c *ui.Context, key string, primary bool) {
	t := c.Theme()
	foreground, background, border := t.TextMuted, t.Text.Alpha(.04), t.Border.Alpha(.65)
	if primary {
		foreground, background, border = t.AccentText.Alpha(.8), t.AccentText.Alpha(.08), t.AccentText.Alpha(.12)
	}
	components.Badge(c, key).FontSize(10).FontWeight(500).Height(20).MinWidth(22).
		Padding(2, 5).Radius(4).AlignSelf(ui.Center).TextAlign(ui.Center).
		TextColor(foreground).Background(background).Border(1, border)
}

// A panel has its own navigation and focus. Opening it must never change the
// manager's current page, search, selection, or unsaved editor.
type quickSearchWindow interface {
	Show()
	Hide()
	Focus()
	Invalidate()
	Destroy()
	Bounds() mygo.Rectangle
	SetBounds(mygo.Rectangle)
}

type quickSearchState struct {
	window                   quickSearchWindow
	visible, focusPending    bool
	clearFeedback            bool
	query, passphrase        string
	errorText, operation     string
	selected                 int
	results                  []app.Entry
	list                     components.ListState
	searchRef, passphraseRef ui.Ref
	request                  uint64
	cacheReady               bool
	cachedVersion            uint64
	cachedCount              int
	cachedQuery, cachedVault string
	cachedResults            []app.Entry
	positionPath             string
	positionTimer            *time.Timer
	savedPosition            quickSearchPosition
	positionSaved            bool
	positionSaveFailed       bool
	pendingCopy              quickSearchCopy
}

// A locked result keeps only its path while authentication is in progress.
// The continuation belongs to this panel invocation and this vault session.
type quickSearchCopy struct {
	entry, vaultID string
	generation     uint64
}

func (pending quickSearchCopy) valid(a *application) bool {
	return pending.entry != "" && pending.vaultID == a.state.VaultID && pending.generation == a.generation && a.quick.visible && a.state.Configured && !a.state.Locked
}

// Spotlight starts horizontally centered in the upper portion of the current
// display. Use work-area coordinates so menu bars, docks, and secondary screens
// (including those with negative origins) are respected.
func quickSearchBounds(area mygo.Rectangle) mygo.Rectangle {
	width, height := min(660, area.Width), min(460, area.Height)
	return mygo.Rectangle{
		X:     area.X + (area.Width-width)/2,
		Y:     area.Y + min(area.Height*18/100, max(0, area.Height-height)),
		Width: width, Height: height,
	}
}

// Keep the dragged position while its display exists. If a monitor is removed,
// put the panel back at the Spotlight default on the display under the cursor;
// clamp a partially visible panel so its search and close controls stay usable.
func quickSearchPlacement(bounds mygo.Rectangle, areas []mygo.Rectangle, fallback mygo.Rectangle) mygo.Rectangle {
	best, overlap := mygo.Rectangle{}, 0
	for _, area := range areas {
		width := max(0, min(bounds.X+bounds.Width, area.X+area.Width)-max(bounds.X, area.X))
		height := max(0, min(bounds.Y+bounds.Height, area.Y+area.Height)-max(bounds.Y, area.Y))
		if width*height > overlap {
			best, overlap = area, width*height
		}
	}
	if overlap == 0 {
		return quickSearchBounds(fallback)
	}
	bounds.Width, bounds.Height = min(bounds.Width, best.Width), min(bounds.Height, best.Height)
	bounds.X = max(best.X, min(bounds.X, best.X+best.Width-bounds.Width))
	bounds.Y = max(best.Y, min(bounds.Y, best.Y+best.Height-bounds.Height))
	return bounds
}

func (a *application) placeQuickSearchWindow() {
	if _, native := a.quick.window.(*mygo.Window); !native {
		return
	}
	areas := []mygo.Rectangle{}
	for _, display := range mygo.Screen.Displays() {
		areas = append(areas, display.WorkArea)
	}
	fallback := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint()).WorkArea
	bounds := a.quick.window.Bounds()
	if placement := quickSearchPlacement(bounds, areas, fallback); placement != bounds {
		a.quick.window.SetBounds(placement)
	}
}

func quickSearchOptions(area mygo.Rectangle) mygo.WindowOptions {
	bounds := quickSearchBounds(area)
	return mygo.WindowOptions{
		Title: "Passpine Quick Search", Width: bounds.Width, Height: bounds.Height,
		X: bounds.X, Y: bounds.Y, Hidden: true, Panel: true, Frameless: true,
		DisableResize:   true,
		BackgroundColor: nativeThemeBackground(app.ThemeStyleOnevra),
	}
}

func (a *application) newQuickSearchWindow() {
	area := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint()).WorkArea
	options := quickSearchOptions(area)
	options.BackgroundColor = nativeThemeBackground(a.themeSettings().ThemeStyle)
	options.Title = a.t("Passpine 快速搜索")
	if directory, ok := a.service.(interface{ DataDirectory() string }); ok {
		a.quick.positionPath = filepath.Join(directory.DataDirectory(), quickSearchPositionFile)
	}
	areas := []mygo.Rectangle{}
	for _, display := range mygo.Screen.Displays() {
		areas = append(areas, display.WorkArea)
	}
	a.restoreQuickSearchPosition(&options, areas, area)
	options.Content = ui.View(a.quickSearchView)
	window := mygo.NewWindow(options)
	a.quick.window = window
	a.updateNativeIcons()
	// NewWindow treats (0, 0) as centering; an explicit saved origin must win.
	if a.quick.positionSaved {
		window.SetBounds(mygo.Rectangle{X: options.X, Y: options.Y, Width: options.Width, Height: options.Height})
	}
	window.OnMove(a.queueQuickSearchPositionSave)
	window.OnClose(func(event *mygo.CloseEvent) {
		if !a.closing {
			event.PreventDefault()
			a.hideQuickSearch()
		}
	})
	window.OnBlur(func() {
		// The OS authentication prompt temporarily takes focus from the panel.
		if a.quick.operation != "device-unlock" {
			a.hideQuickSearch()
		}
	})
	window.OnClosed(func() { a.quick.window = nil; a.quick.visible = false })
}

func (a *application) quickSearch() {
	if a.closing || a.initializing {
		return
	}
	if a.quick.window == nil {
		a.newQuickSearchWindow()
	}
	a.quick.visible, a.quick.focusPending = true, true
	a.quick.errorText, a.quick.clearFeedback = "", true
	a.placeQuickSearchWindow()
	a.quick.window.Show()
	a.quick.window.Focus()
	a.lastInput = time.Now()
	a.queueMetadataRefresh(false)
	a.invalidateQuickSearch()
}

func (a *application) toggleQuickSearch() {
	if a.quick.visible {
		a.hideQuickSearch()
		return
	}
	a.quickSearch()
}

func (a *application) hideQuickSearch() {
	a.saveQuickSearchPosition()
	a.quick.visible = false
	a.quick.passphrase = ""
	a.quick.pendingCopy = quickSearchCopy{}
	a.quick.errorText, a.quick.clearFeedback = "", true
	a.quick.request++
	if a.quick.window != nil {
		a.quick.window.Hide()
	}
}

func (a *application) closeQuickSearch() {
	a.saveQuickSearchPosition()
	window := a.quick.window
	a.quick.window = nil
	a.quick.visible = false
	a.clearQuickSearchSecrets()
	if window != nil {
		window.Destroy()
	}
}

func (a *application) clearQuickSearchSecrets() {
	a.quick.passphrase = ""
	a.quick.errorText, a.quick.clearFeedback = "", true
	a.quick.results, a.quick.cachedResults = nil, nil
	a.quick.cacheReady = false
	a.quick.selected = 0
	a.quick.operation = ""
	a.quick.pendingCopy = quickSearchCopy{}
	a.quick.request++
	a.quick.focusPending = true
}

func (a *application) invalidateQuickSearch() {
	if a.quick.window != nil && a.quick.visible && !a.closing {
		a.quick.window.Invalidate()
	}
}

func (a *application) quickSearchEntries() []app.Entry {
	q := &a.quick
	if !a.state.Configured || a.state.Locked && !a.state.Settings.BrowseWhileLocked {
		q.cacheReady = false
		q.cachedResults = nil
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(q.query))
	if q.cacheReady && q.cachedVersion == a.entriesVersion && q.cachedCount == len(a.state.Entries) && q.cachedVault == a.state.VaultID && q.cachedQuery == query {
		return q.cachedResults
	}
	var result []app.Entry
	for _, entry := range a.state.Entries {
		if query == "" || strings.Contains(strings.ToLower(entry.Path), query) {
			result = append(result, entry)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return strings.ToLower(result[i].Path) < strings.ToLower(result[j].Path)
	})
	q.cacheReady, q.cachedVersion, q.cachedCount = true, a.entriesVersion, len(a.state.Entries)
	q.cachedVault, q.cachedQuery, q.cachedResults = a.state.VaultID, query, result
	return result
}

func (a *application) prepareQuickSearchList() {
	q := &a.quick
	if q.pendingCopy.entry != "" && (q.pendingCopy.vaultID != a.state.VaultID || q.pendingCopy.generation != a.generation || !a.state.Configured) {
		q.pendingCopy = quickSearchCopy{}
		q.passphrase = ""
		q.focusPending = true
	}
	q.results = a.quickSearchEntries()
	q.selected = max(0, min(q.selected, len(q.results)-1))
	if q.list.Selected == nil {
		q.list = components.ListState{
			Selected: &q.selected,
			Key:      func(i int) any { return q.results[i].Path },
			Label:    func(i int) string { return q.results[i].Name },
			OnChange: func() { a.lastInput = time.Now() },
			OnSubmit: func(int) { a.copyQuickSearchResult() },
		}
	}
}

func (a *application) quickSearchView(c *ui.Context) {
	c.SetTheme(a.clientTheme(c))
	a.prepareQuickSearchList()
	c.OnShortcut(0, ui.KeyEscape, a.hideQuickSearch)
	c.OnShortcut(ui.Cmd, ui.KeyF, func() { a.quick.searchRef.Focus() })
	c.OnShortcut(ui.Cmd, ui.KeyO, a.openQuickSearchInManager)
	c.OnShortcut(ui.Ctrl, ui.KeyN, func() { a.moveQuickSearchSelection(1) })
	c.OnShortcut(ui.Ctrl, ui.KeyP, func() { a.moveQuickSearchSelection(-1) })
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyL, a.lock)
	ui.Column(c).Fill().Background(c.Theme().Background).Border(1, c.Theme().Border).Radius(14).Clip().Children(func() {
		ui.Row(c).FillWidth().Height(38).Padding(6, 16).AlignItems(ui.Center).Gap(7).DragWindow().Children(func() {
			components.Icon(c, lucide.KeyRound()).Size(13, 13).TextColor(c.Theme().TextMuted)
			components.Text(c, "Passpine").FontSize(11).TextColor(c.Theme().TextMuted)
			ui.Spacer(c)
			components.Text(c, a.state.VaultName).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
			if components.Button(c, "esc", components.ButtonGhost).Label(a.t("关闭快速搜索")).Height(25).Clicked() {
				a.hideQuickSearch()
			}
		})
		if !a.state.Configured {
			a.quickSearchEmptyView(c, a.t("先设置一个密码库"), a.t("打开主窗口创建或选择密码库"))
		} else if a.quickSearchNeedsUnlock() {
			a.quickSearchUnlockView(c)
		} else {
			a.quickSearchResultsView(c)
		}
		ui.Box(c).FillWidth().Height(1).Background(c.Theme().Border)
		ui.Row(c).FillWidth().Height(48).Padding(8, 16).AlignItems(ui.Center).Gap(10).Children(func() {
			if a.state.Configured && !a.quickSearchNeedsUnlock() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Shrink(0).Children(func() {
					ui.Row(c).Gap(3).Children(func() {
						quickSearchKeycap(c, "↑", false)
						quickSearchKeycap(c, "↓", false)
					})
					ui.Row(c).Gap(3).Tooltip("Ctrl+N / Ctrl+P").Children(func() {
						quickSearchKeycap(c, quickSearchControlShortcut("N"), false)
						quickSearchKeycap(c, quickSearchControlShortcut("P"), false)
					})
					components.Text(c, a.t("选择")).FontSize(11).SingleLine().TextColor(c.Theme().TextMuted)
				})
			}
			ui.Spacer(c)
			if components.Button(c, "", components.ButtonGhost).Label(a.t("打开主窗口")).Height(30).Padding(5, 9).Gap(9).Shrink(0).Children(func() {
				components.Text(c, a.t("打开主窗口")).FontSize(12).SingleLine()
				quickSearchKeycap(c, quickSearchOpenShortcut(), false)
			}).Clicked() {
				a.openQuickSearchInManager()
			}
			if a.state.Configured && !a.quickSearchNeedsUnlock() {
				label := a.t("复制密码")
				if a.quick.operation == "copy" {
					label = a.t("正在复制…")
				}
				if components.Button(c, "", components.ButtonPrimary).Label(label).Height(30).Padding(5, 12).Gap(10).Shrink(0).Disabled(a.busy || len(a.quick.results) == 0).Children(func() {
					components.Text(c, label).FontSize(12).SingleLine()
					quickSearchKeycap(c, "↵", true)
				}).Clicked() {
					a.copyQuickSearchResult()
				}
			}
		})
	})
	if a.quick.clearFeedback {
		c.CloseToast("quick-search-error")
		a.quick.clearFeedback = false
	}
	if a.quick.errorText != "" {
		c.AddToast(ui.Toast{ID: "quick-search-error", Title: a.t("操作未完成"), Description: a.t(a.quick.errorText), Type: components.ToastError, Timeout: -1})
		a.quick.errorText = ""
	}
	components.WithLocalization(c, components.Localization{CloseToast: a.t("关闭提示")}, func() {
		components.Toasts(c, components.ToastOptions{Inset: 48})
	})
}

func (a *application) quickSearchResultsView(c *ui.Context) {
	q := &a.quick
	ui.Column(c).FillWidth().Grow(1).MinHeight(0).Padding(0, 14, 10, 14).Gap(10).Children(func() {
		input := components.TextBox(c, &q.query, components.TextBoxOptions{LeadingIcon: lucide.Search()}).
			Ref(&q.searchRef).Label(a.t("快速搜索条目")).Placeholder(a.t("搜索名称或路径…")).FillWidth().Height(48).FontSize(17)
		input.OnShortcut(0, ui.KeyDown, func() { a.moveQuickSearchSelection(1) })
		input.OnShortcut(0, ui.KeyUp, func() { a.moveQuickSearchSelection(-1) })
		// macOS text fields consume Control N/P as caret navigation before
		// ordinary shortcuts. Handle these keys before the native text editor.
		input.OnTextKey(func(key ui.TextKey) bool {
			if key.Mods == ui.Ctrl && (key.Key == ui.KeyN || key.Key == ui.KeyP) {
				delta := 1
				if key.Key == ui.KeyP {
					delta = -1
				}
				a.moveQuickSearchSelection(delta)
				return true
			}
			return false
		})
		if q.focusPending {
			q.searchRef.Focus()
			q.focusPending = false
		}
		if input.Changed() {
			q.selected = 0
			a.lastInput = time.Now()
			q.results = a.quickSearchEntries()
			q.list.ScrollTo(0, ui.Start)
		}
		if input.Submitted() {
			a.copyQuickSearchResult()
		}
		ui.Row(c).FillWidth().Padding(0, 6).Children(func() {
			components.Text(c, a.countLabel(len(q.results))).FontSize(11).TextColor(c.Theme().TextMuted)
		})
		if len(q.results) == 0 {
			ui.Column(c).FillWidth().Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(10).Children(func() {
				components.Icon(c, lucide.SearchX).Size(28, 28).TextColor(c.Theme().TextMuted)
				muted(c, a.t("没有匹配的条目"))
			})
			return
		}
		components.List(c, &q.list, len(q.results), func(i int) {
			entry := q.results[i]
			ui.Row(c).FillWidth().Height(58).Padding(9, 12).Gap(12).AlignItems(ui.Center).Children(func() {
				components.Icon(c, lucide.KeyRound()).Size(18, 18).TextColor(c.Theme().TextMuted)
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					components.Text(c, entry.Name).FontSize(14).SingleLine()
					folder := entry.Folder
					if folder == "" {
						folder = a.state.VaultName
					}
					components.Text(c, folder).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
				})
				if i == q.selected {
					components.Icon(c, lucide.CornerDownLeft()).Size(14, 14).TextColor(c.Theme().TextMuted)
				}
			})
		}).FillWidth().Grow(1)
	})
}

func (a *application) quickSearchEmptyView(c *ui.Context, title, description string) {
	ui.Column(c).FillWidth().Grow(1).Padding(30).AlignItems(ui.Center).Justify(ui.Center).Gap(12).Children(func() {
		components.Icon(c, lucide.FolderKey()).Size(32, 32).TextColor(c.Theme().Accent)
		components.Text(c, title).FontSize(19).Bold()
		muted(c, description)
	})
}

func (a *application) quickSearchUnlockView(c *ui.Context) {
	ui.Column(c).FillWidth().Grow(1).Padding(24, 70).AlignItems(ui.Center).Justify(ui.Center).Gap(16).Children(func() {
		components.Icon(c, lucide.LockKeyhole()).Size(30, 30).TextColor(c.Theme().Accent)
		title := a.t("解锁以快速搜索")
		if a.quick.pendingCopy.entry != "" {
			title = a.t("解锁以复制密码")
		}
		components.Text(c, title).FontSize(20).Bold()
		muted(c, a.t("验证后即可搜索并复制密码"))
		if a.quick.pendingCopy.entry != "" && a.state.Settings.BrowseWhileLocked {
			components.Text(c, entryName(a.quick.pendingCopy.entry)).Bold().SingleLine()
		}
		if a.state.DeviceUnlockEnrolled && a.state.Settings.DeviceUnlock {
			if components.Button(c, a.t("使用 Touch ID 或 Mac 登录密码"), components.ButtonPrimary).FillWidth().Disabled(a.busy || !a.state.DeviceUnlockAvailable).Clicked() {
				a.unlockQuickSearch("device")
			}
			if !a.state.DeviceUnlockAvailable {
				muted(c, a.t("系统解锁暂不可用，请打开主窗口检查安全设置"))
			}
		} else {
			input := components.TextBox(c, &a.quick.passphrase, components.TextBoxOptions{}).Ref(&a.quick.passphraseRef).
				Password().Label(a.t("快速搜索私钥口令")).Placeholder(a.t("私钥口令")).FillWidth().Disabled(a.busy)
			if a.quick.focusPending {
				a.quick.passphraseRef.Focus()
				a.quick.focusPending = false
			}
			if input.Submitted() || components.Button(c, a.t("解锁"), components.ButtonPrimary).FillWidth().Disabled(a.busy || a.quick.passphrase == "").Clicked() {
				a.unlockQuickSearch("")
			}
		}
		if a.quick.pendingCopy.entry != "" && a.state.Settings.BrowseWhileLocked {
			if components.Button(c, a.t("返回搜索"), components.ButtonGhost).Disabled(a.busy).Clicked() {
				a.quick.pendingCopy = quickSearchCopy{}
				a.quick.passphrase = ""
				a.quick.focusPending = true
				a.quick.request++
			}
		}
	})
}

func (a *application) quickSearchNeedsUnlock() bool {
	return a.state.Locked && (!a.state.Settings.BrowseWhileLocked || a.quick.pendingCopy.entry != "")
}

func (a *application) moveQuickSearchSelection(delta int) {
	if a.quickSearchNeedsUnlock() || len(a.quick.results) == 0 {
		return
	}
	a.quick.selected = max(0, min(len(a.quick.results)-1, a.quick.selected+delta))
	a.quick.list.ScrollIntoView(a.quick.selected)
	a.lastInput = time.Now()
	a.invalidateQuickSearch()
}

func (a *application) copyQuickSearchResult() {
	q := &a.quick
	if a.busy || !q.visible || !a.state.Configured || q.selected < 0 || q.selected >= len(q.results) {
		return
	}
	entry := q.results[q.selected].Path
	if a.state.Locked {
		if !a.state.Settings.BrowseWhileLocked {
			return
		}
		q.pendingCopy = quickSearchCopy{entry: entry, vaultID: a.state.VaultID, generation: a.generation}
		q.passphrase, q.errorText, q.clearFeedback = "", "", true
		q.focusPending = true
		if a.state.Settings.DeviceUnlock && a.state.DeviceUnlockEnrolled && a.state.DeviceUnlockAvailable {
			a.unlockQuickSearch("device")
		}
		a.invalidateQuickSearch()
		return
	}
	a.callQuickSearch(app.Request{Action: "copy", Entry: entry, Field: "password"})
}

func (a *application) unlockQuickSearch(mode string) {
	request := app.Request{Action: "unlock", Mode: mode}
	if mode != "device" {
		request.Passphrase = a.quick.passphrase
	}
	a.callQuickSearch(request)
}

// Reuse the application's request lifecycle and the service's clipboard lease.
// Copy never reads a decrypted document into the panel's presentation state.
func (a *application) callQuickSearch(request app.Request) {
	if a.busy || a.closing || a.service == nil || !a.quick.visible {
		return
	}
	request.VaultID = a.state.VaultID
	a.busy, a.backgroundBusy = true, false
	a.activeRequestAction = request.Action
	a.quick.operation = request.Action
	if request.Action == "unlock" && request.Mode == "device" {
		a.quick.operation = "device-unlock"
	}
	a.quick.errorText, a.quick.clearFeedback = "", true
	a.quick.request++
	id, generation, pendingCopy := a.quick.request, a.generation, a.quick.pendingCopy
	a.lastInput = time.Now()
	go func() {
		response := a.service.Execute(request)
		a.runOnMain(func() {
			if a.closing || generation != a.generation {
				return
			}
			if a.state.VaultID != request.VaultID {
				// Another surface may have changed the vault while an OS prompt
				// was open. Its late snapshot cannot restore the previous vault.
				a.busy, a.backgroundBusy = false, false
				a.activeRequestAction, a.quick.operation = "", ""
				a.invalidate()
				return
			}
			active := a.quick.visible && a.quick.request == id && a.state.VaultID == request.VaultID
			a.quick.operation = ""
			a.receiveResponse(generation, response, func(d app.Data) {
				if request.Action == "unlock" {
					if !a.state.Locked {
						a.lastInput = time.Now()
					}
					if active {
						a.quick.passphrase = ""
						a.quick.focusPending = true
						if a.quick.window != nil {
							a.quick.window.Focus()
						}
						if pendingCopy == a.quick.pendingCopy {
							a.quick.pendingCopy = quickSearchCopy{}
							if pendingCopy.valid(a) {
								a.callQuickSearch(app.Request{Action: "copy", Entry: pendingCopy.entry, Field: "password"})
							}
						}
					}
				}
				if active && request.Action == "copy" {
					a.hideQuickSearch()
				}
			})
			if response.Status != "ok" {
				if active {
					a.quick.errorText = a.errorText
				}
				a.errorText = ""
			}
			a.invalidate()
		})
	}()
	a.invalidate()
}

func (a *application) openQuickSearchInManager() {
	a.hideQuickSearch()
	if !a.editor.open {
		a.query = a.quick.query
		a.navigate("entries", "all")
		if a.quick.selected >= 0 && a.quick.selected < len(a.quick.results) {
			a.selectEntry(a.quick.results[a.quick.selected].Path)
		}
	}
	a.show()
}
