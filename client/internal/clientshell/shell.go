// Adapted from Onevra v0.2026.9 shells/sidebar; see LICENSE.
package clientshell

import (
	"math"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// ShellOptions describes a window and its default sidebar-and-content page.
// Content builders run synchronously on the UI thread.
type ShellOptions struct {
	Title string
	// Hidden creates the window without showing or focusing it. Applications
	// can later call Window().Show or Window().ShowInactive.
	Hidden bool
	// Window dimensions and constraints are outer DIPs. Non-positive width and
	// height default to 820 and 600, respectively, for both initial and minimum
	// sizes. Explicit positive values override each default independently.
	// Initial dimensions are raised to the corresponding minimum if needed.
	Width, Height, MinWidth, MinHeight int
	// StateKey, when non-empty, has MyGo remember the window's bounds and
	// maximized or full-screen state under this key and restore them at
	// creation. MyGo stores them in its user-data directory, or in the store
	// set with mygo.App.SetWindowStateStore. Use a distinct key per window kind.
	StateKey string
	// Left and Content build full-height regions behind the floating header.
	// Use HeaderHeight(c) as top padding to keep controls below it while
	// allowing a region's background to extend to the top. Nil leaves it empty.
	Left, Content func(*ui.Context)
	// Shortcuts registers application shortcuts at window scope, including
	// when no control or a control in the navigation sidebar has focus.
	Shortcuts func(*ui.Context)
	// LeftBackground paints the entire sidebar, including its header and
	// bottom toolbar. Nil preserves the platform material or theme surface.
	LeftBackground func(*ui.Context) ui.Color
	// HeaderBackground paints the full header and adds a bottom separator.
	// Nil preserves the transparent header over the underlying regions.
	HeaderBackground func(*ui.Context) ui.Color
	// ContentTitle builds the leading main-content titlebar area. It may contain
	// a title, breadcrumbs or actions; nil leaves it blank. Shell clips it to
	// the room after the sidebar control and hides it on replacement pages.
	ContentTitle func(*ui.Context)
	// LeadingHeader owns persistent navigation before the sidebar toggle.
	LeadingHeader      func(*ui.Context)
	LeadingHeaderWidth float32
	// LeftHeader builds trailing actions beside the sidebar toggle, within the
	// expanded sidebar's header. It is clipped to the available safe area and
	// omitted while collapsed, animating, or showing a replacement page.
	LeftHeader func(*ui.Context)
	// LeftToolbar builds a fixed bottom toolbar beneath Left; nil omits it.
	// Use IconButton for actions and ui.Spacer for trailing alignment.
	LeftToolbar func(*ui.Context)
	// Default left sidebar width in DIPs; non-positive values use 220.
	// Clamped to LeftMinWidth/LeftMaxWidth at creation. Dragging or focused
	// arrow keys adjust the width; double-clicking the separator restores it.
	// Shell retains the chosen width across toggles and page changes.
	LeftWidth int
	// Optional reset width, separate from a restored initial width.
	LeftResetWidth    int
	OnLeftWidthChange func(float32)
	// LeftMinWidth defaults to 120 DIPs when non-positive. A non-positive
	// LeftMaxWidth adds no fixed limit; a positive maximum below the minimum
	// is raised to the minimum. Visible width is capped only to keep the resize
	// handle inside the viewport, even below the minimum, without discarding
	// the chosen width.
	LeftMinWidth, LeftMaxWidth int
	// The left sidebar starts open.
	LeftCollapsed bool
	// LeftOverlayBelow automatically hides the sidebar at or below this
	// viewport width. The toggle then opens it over the content instead of
	// reducing the content width. Non-positive values retain the docked layout.
	// Returning to a wider viewport restores the user's docked expansion state.
	LeftOverlayBelow int
	// Labels allow applications to localize the sidebar controls.
	LeftToggleLabel string
	// LeftResizeLabel names the draggable, keyboard-accessible separator.
	LeftResizeLabel string
	// LeftOverlayCloseLabel names the clickable overlay backdrop.
	LeftOverlayCloseLabel string
	// Windows caption button labels; empty values use English defaults.
	MinimizeLabel, MaximizeLabel, RestoreLabel, CloseLabel string
	// Toasts places the window's toasts; top placements also clear the header.
	Toasts components.ToastOptions
	// Theme, when set, returns the frame's theme; Shell installs it before
	// building anything, so its own surfaces and all content use it. It runs
	// on every frame, so it can follow application state; pass theme.Onevra
	// for Onevra's palettes. Nil keeps MyGo's frame theme. Light or dark
	// belongs to mygo.Theme.SetSource, which also covers native chrome.
	Theme func(*ui.Context) *ui.Theme
}

// Shell owns window presentation, sidebar state and the active page.
// Applications retain ownership of business state and application lifetime.
// Call its methods on the UI thread, after MyGo is ready.
type Shell struct {
	window    *mygo.Window
	host      shellWindow
	options   ShellOptions
	page      func(*ui.Context)
	leftOpen  bool
	leftWidth float32
	// Narrow overlays have their own expansion state so a temporary reveal
	// never overwrites the user's docked sidebar preference.
	leftOverlay, leftOverlayOpen bool
}

type shellWindow interface {
	Invalidate()
}

// NewShell creates a native window and installs the page container.
// It requests a titlebar material on macOS and Mica with custom caption buttons
// on Windows. If no material is effective, it draws an opaque theme background.
// Other platforms retain their native controls. It persists no state itself;
// StateKey delegates window geometry to MyGo.
func NewShell(options ShellOptions) *Shell {
	s := newShell(options)
	s.window = mygo.NewWindow(s.nativeOptions())
	s.host = s.window
	if runtime.GOOS == "windows" {
		s.window.OnMaximize(s.refresh)
		s.window.OnUnmaximize(s.refresh)
	}
	return s
}

// Layout creates the same shell presentation without creating a native window.
// It supports embedding and headless UI testing; Window returns nil, and the
// host rendering the returned view owns invalidation after shell state changes.
func Layout(options ShellOptions) (*Shell, func(*ui.Context)) {
	s := newShell(options)
	return s, s.view
}

const (
	defaultWindowWidth  = 820
	defaultWindowHeight = 600
)

func newShell(options ShellOptions) *Shell {
	if options.Width <= 0 {
		options.Width = defaultWindowWidth
	}
	if options.Height <= 0 {
		options.Height = defaultWindowHeight
	}
	if options.MinWidth <= 0 {
		options.MinWidth = defaultWindowWidth
	}
	if options.MinHeight <= 0 {
		options.MinHeight = defaultWindowHeight
	}
	options.Width = max(options.Width, options.MinWidth)
	options.Height = max(options.Height, options.MinHeight)
	if options.LeftWidth <= 0 {
		options.LeftWidth = 220
	}
	if options.LeftMinWidth <= 0 {
		options.LeftMinWidth = 120
	}
	options.LeftWidth = max(options.LeftWidth, options.LeftMinWidth)
	if options.LeftMaxWidth > 0 {
		options.LeftMaxWidth = max(options.LeftMaxWidth, options.LeftMinWidth)
		options.LeftWidth = min(options.LeftWidth, options.LeftMaxWidth)
	}
	if options.LeftToggleLabel == "" {
		options.LeftToggleLabel = "Toggle sidebar"
	}
	if options.LeftResizeLabel == "" {
		options.LeftResizeLabel = "Resize sidebar"
	}
	if options.LeftOverlayCloseLabel == "" {
		options.LeftOverlayCloseLabel = "Close sidebar"
	}
	return &Shell{options: options, leftOpen: !options.LeftCollapsed, leftWidth: float32(options.LeftWidth)}
}

func (s *Shell) nativeOptions() mygo.WindowOptions {
	o := s.options
	return mygo.WindowOptions{
		Hidden: o.Hidden, Title: o.Title, Width: o.Width, Height: o.Height,
		MinWidth: o.MinWidth, MinHeight: o.MinHeight, StateKey: o.StateKey,
		TitleBarStyle:        mygo.TitleBarHidden,
		Frameless:            runtime.GOOS == "windows",
		TrafficLightPosition: &mygo.Point{X: 16, Y: 16},
		Transparent:          runtime.GOOS == "darwin",
		Vibrancy:             shellVibrancy(),
		Content:              ui.View(s.view),
	}
}

func shellVibrancy() mygo.Vibrancy {
	switch runtime.GOOS {
	case "darwin":
		return mygo.VibrancySidebar
	case "windows":
		return mygo.VibrancyMica
	default:
		return mygo.VibrancyNone
	}
}

// Window exposes the original MyGo handle for events and lifecycle operations.
// Shell owns its content, title-bar configuration and sidebar layout.
func (s *Shell) Window() *mygo.Window { return s.window }

// SetPage replaces the entire default page, including sidebar controls.
// Nil restores the default page and its previous sidebar expansion state.
// Replacement pages receive an opaque background and the same floating header
// as the default page. The builder owns its top inset; use
// HeaderHeight(c) to keep interactive content below the header.
func (s *Shell) SetPage(page func(*ui.Context)) {
	s.page = page
	s.refresh()
}

// LeftWidth reports the chosen sidebar width in DIPs, independent of viewport
// clipping, collapse or page replacement.
func (s *Shell) LeftWidth() float32 { return s.leftWidth }

// SetLeftWidth restores or changes the chosen width within the configured limits.
// Non-finite values are ignored. This does not change the double-click default.
func (s *Shell) SetLeftWidth(width float32) {
	if math.IsNaN(float64(width)) || math.IsInf(float64(width), 0) {
		return
	}
	next := max(float32(s.options.LeftMinWidth), width)
	if s.options.LeftMaxWidth > 0 {
		next = min(next, float32(s.options.LeftMaxWidth))
	}
	if next != s.leftWidth {
		s.leftWidth = next
		if s.options.OnLeftWidthChange != nil {
			s.options.OnLeftWidthChange(next)
		}
	}
	s.refresh()
}

// LeftExpanded reports the requested state in the current viewport, including
// during an animation. Narrow overlays start closed on each layout change.
func (s *Shell) LeftExpanded() bool {
	if s.leftOverlay {
		return s.leftOverlayOpen
	}
	return s.leftOpen
}

// LeftDockedExpanded reports the user's expansion preference for wide windows.
// Unlike LeftExpanded, temporary narrow overlays do not change this value.
// Applications persisting sidebar preferences should store this state.
func (s *Shell) LeftDockedExpanded() bool { return s.leftOpen }

// LeftOverlayActive reports whether the last frame used the narrow layout.
func (s *Shell) LeftOverlayActive() bool { return s.leftOverlay }

// SetLeftExpanded animates the sidebar without resizing the window. In a narrow
// viewport it only opens or closes the overlay, retaining the docked preference.
func (s *Shell) SetLeftExpanded(open bool) {
	if s.leftOverlay {
		s.leftOverlayOpen = open
	} else {
		s.leftOpen = open
	}
	s.refresh()
}

func (s *Shell) overlayAt(width float32) bool {
	return s.options.LeftOverlayBelow > 0 && width <= float32(s.options.LeftOverlayBelow)
}

func (s *Shell) maxLeftWidth(viewportWidth float32) float32 {
	// Keep the full resize handle reachable at the window's right edge.
	maxWidth := max(float32(0), viewportWidth-3)
	if s.options.LeftMaxWidth > 0 {
		maxWidth = min(maxWidth, float32(s.options.LeftMaxWidth))
	}
	return maxWidth
}

func (s *Shell) visibleLeftWidth(viewportWidth float32) float32 {
	return min(s.leftWidth, s.maxLeftWidth(viewportWidth))
}

// ContentWidth reports the main region's target width in DIPs. Narrow overlays
// do not consume content space. During docked sidebar animations the actual
// width approaches this value; replacement pages always use the full viewport.
// This is a pure query and may be called before the next frame is built.
func (s *Shell) ContentWidth(viewportWidth float32) float32 {
	if s.page != nil || s.overlayAt(viewportWidth) || !s.leftOpen {
		return max(0, viewportWidth)
	}
	return max(0, viewportWidth-s.visibleLeftWidth(viewportWidth))
}

func (s *Shell) refresh() {
	if s.host != nil {
		s.host.Invalidate()
	}
}

// HeaderHeight returns the floating Shell header's height in DIPs for this
// frame, accounting for native window controls and the active theme's font size.
// Call it from a content builder; Shell does not automatically inset Left,
// Content or pages.
// The header remains above content for window dragging and header controls;
// keep interactive body content below this height.
func HeaderHeight(c *ui.Context) float32 {
	return max(48, c.TitleBar().Height, c.Theme().Rem(3))
}

const sidebarDuration = 220 * time.Millisecond

func openFraction(open bool) float32 {
	if open {
		return 1
	}
	return 0
}

func (s *Shell) view(c *ui.Context) {
	// The theme must be installed before any widget is created.
	if s.options.Theme != nil {
		c.SetTheme(s.options.Theme(c))
	}
	if s.options.Shortcuts != nil {
		s.options.Shortcuts(c)
	}
	components.WithDialogWindowDrag(c, HeaderHeight(c), func() { s.build(c) })
	toasts := s.options.Toasts
	if toasts.Placement.Top() {
		toasts.Inset += HeaderHeight(c)
	}
	components.Toasts(c, toasts)
}

// SetToasts changes toast placement without recreating the window or toasts.
// Call on the UI thread.
func (s *Shell) SetToasts(options components.ToastOptions) {
	s.options.Toasts = options
	s.refresh()
}

func (s *Shell) build(c *ui.Context) {
	vibrant := c.Vibrancy()
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	background := c.Theme().Background.Alpha(1)
	root := ui.Column(c).Fill().Clip()
	if !vibrant {
		root.Background(background)
	}
	// Snapshot the page for this frame; callbacks can change it for the next.
	page := s.page
	width, _ := c.Size()
	if overlay := s.overlayAt(width); overlay != s.leftOverlay {
		s.leftOverlay, s.leftOverlayOpen = overlay, false
	}
	leftOpen := s.LeftExpanded()
	leftTarget := openFraction(leftOpen)
	leftProgress := root.Animate("left", leftTarget, sidebarDuration)
	// Also snap an animation already running when Reduce Motion is enabled.
	if c.Preferences().ReduceMotion {
		leftProgress = leftTarget
	}
	bar := c.TitleBar()
	headerHeight := HeaderHeight(c)
	leftWidth := s.visibleLeftWidth(width)
	contentInset := leftWidth * leftProgress
	if s.leftOverlay {
		contentInset = 0
	}
	// Use the shell's earlier build order for Escape priority. Popovers and
	// dialogs in either region render above the sidebar and must close first.
	if page == nil && s.leftOverlay && leftProgress > 0 && root.OverlayShortcut(0, ui.KeyEscape) {
		s.SetLeftExpanded(false)
	}
	root.Children(func() {
		ui.Column(c).Key(page != nil).Fill().Clip().Children(func() {
			if page != nil {
				ui.Column(c).Fill().Background(background).Children(func() { page(c) })
				return
			}
			ui.Box(c).Fill().Children(func() {
				// Stable parents and keys retain editor, focus and scroll state
				// when the same sidebar changes between docked and overlay layouts.
				ui.Column(c).Key("content").Absolute().Left(contentInset).Top(0).Right(0).Bottom(0).
					Background(background).Clip().Children(func() {
					ui.Column(c).Fill().Children(func() {
						if s.options.Content != nil {
							s.options.Content(c)
						}
					})
					if leftProgress > 0 && !s.leftOverlay {
						shellSeparator(c)
					}
				})
				if s.leftOverlay && leftProgress > 0 {
					backdrop := ui.Box(c).Key("left-backdrop").Absolute().Left(0).Top(headerHeight).Right(0).Bottom(0).
						Background(ui.RGBA(0, 0, 0, .14*leftProgress)).Role(ui.RoleButton).
						Label(s.options.LeftOverlayCloseLabel).KeepFocus()
					if backdrop.Clicked() {
						s.SetLeftExpanded(false)
					}
				}
				sidebar := ui.Column(c).Key("left").Absolute().Left(0).Top(0).Bottom(0).
					Width(leftWidth * leftProgress).Clip()
				if s.options.LeftBackground != nil {
					sidebar.Background(s.options.LeftBackground(c))
				} else if !vibrant || s.leftOverlay {
					sidebar.Background(c.Theme().Surface.Alpha(1))
				}
				// Blank sidebar space is a hit target too, so it never dismisses
				// the backdrop beneath it.
				sidebar.Clicked()
				sidebar.Children(func() {
					ui.Column(c).Width(leftWidth).Shrink(0).FillHeight().
						Disabled(!leftOpen).Children(func() {
						ui.Column(c).FillWidth().Grow(1).MinHeight(0).Clip().Children(func() {
							if s.options.Left != nil {
								s.options.Left(c)
							}
						})
						if s.options.LeftToolbar != nil {
							ui.Toolbar(c, func() { s.options.LeftToolbar(c) }).
								FillWidth().Shrink(0).Padding(8)
						}
					})
					if s.leftOverlay && leftProgress > 0 {
						ui.Box(c).Absolute().Right(0).Top(0).Bottom(0).Width(1).Background(c.Theme().Border)
					}
				})
			})
		})
		// The header overlays the full-height columns. Only its controls occupy
		// the safe area; builders decide whether to inset their content.
		leftInset := max(8, bar.Left)
		if runtime.GOOS == "darwin" && bar.Left > 0 {
			leftInset += 16
		}
		rightInset := max(8, bar.Right)
		header := ui.Row(c).Key("header").Absolute().Top(0).Left(0).Right(0).Height(headerHeight).
			AlignItems(ui.Center).Padding(6, rightInset, 6, leftInset).DragWindow()
		if s.options.HeaderBackground != nil {
			header.Background(s.options.HeaderBackground(c))
		}
		header.Children(func() {
			if s.options.HeaderBackground != nil {
				ui.Box(c).Key("header-separator").Absolute().Left(0).Right(0).Bottom(0).Height(1).Background(c.Theme().Border)
			}
			if page == nil {
				leadingWidth := float32(0)
				if s.options.LeadingHeader != nil {
					leadingWidth = s.options.LeadingHeaderWidth
					ui.Row(c).Key("leading-header").Width(leadingWidth).Shrink(0).AlignItems(ui.Center).Children(func() {
						s.options.LeadingHeader(c)
					})
				}
				if IconButton(c, lucide.PanelLeft(), s.options.LeftToggleLabel).Clicked() {
					s.SetLeftExpanded(!leftOpen)
				}
				// Align the title with the main surface, but keep it beyond the
				// leading toggle when the sidebar is narrow or collapsed.
				titleInset := max(8, contentInset+16-leftInset-max(32, c.Theme().Rem(2))-leadingWidth)
				if s.options.LeftHeader != nil && leftOpen && !s.leftOverlay && leftProgress == 1 && titleInset > 24 {
					ui.Row(c).Key("left-header").Width(titleInset - 24).Shrink(0).FillHeight().
						AlignItems(ui.Center).Clip().Children(func() {
						ui.Spacer(c)
						s.options.LeftHeader(c)
					})
					titleInset = 24
				}
				ui.Row(c).Key("content-title").Grow(1).MinWidth(0).FillHeight().Clip().Padding(0, 8, 0, titleInset).Children(func() {
					if s.options.ContentTitle != nil {
						s.options.ContentTitle(c)
					}
				})
			}
			if runtime.GOOS == "windows" {
				if page != nil {
					ui.Spacer(c)
				}
				s.captionButtons(c)
			}
		})
		// The full-height handle wins hit testing over the draggable header.
		if page == nil && leftOpen && !s.leftOverlay && leftProgress == 1 {
			s.resizeHandle(c, leftWidth, s.maxLeftWidth(width))
		}
	})
}

// resizeHandle uses MyGo's pointer capture and keyboard input, overlaid on
// the separator rather than inserting another layout column.
func (s *Shell) resizeHandle(c *ui.Context, width, maxWidth float32) {
	handle := ui.Box(c).Key("left-resize").Absolute().Left(width - 3).Top(0).Bottom(0).Width(6).
		Cursor(ui.CursorResizeEW).Focusable().Role(ui.RoleSplitter).Label(s.options.LeftResizeLabel)
	next := width
	if dx, _, held := handle.Dragged(); held {
		handle.Focus()
		next += dx
	}
	if handle.Shortcut(0, ui.KeyLeft) {
		next -= 10
	}
	if handle.Shortcut(0, ui.KeyRight) {
		next += 10
	}
	if handle.DoubleClicked() {
		reset := s.options.LeftResetWidth
		if reset <= 0 {
			reset = s.options.LeftWidth
		}
		s.SetLeftWidth(float32(reset))
		c.Invalidate()
	} else if next != width {
		s.SetLeftWidth(min(next, maxWidth))
		c.Invalidate()
	}
	handle.FocusRing(handle.FocusVisible())
}

// Separators overlay the surface; they never reserve layout space.
func shellSeparator(c *ui.Context) {
	ui.Box(c).Key("separator").Absolute().Left(0).Top(0).Bottom(0).Width(1).Background(c.Theme().Border)
}
