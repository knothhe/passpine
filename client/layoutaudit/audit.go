// Package layoutaudit adds visual layout guides and a two-point ruler to any
// MyGo view. It uses only public ui APIs and never reads widget text or values.
// Full tree selection, computed typography and box-model inspection are owned
// by MyGo's native inspector, enabled by the host's Page.DevTools option.
package layoutaudit

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
)

// Labels are supplied by the host's localization catalog. No application or
// language catalog is imported by this component.
type Labels struct {
	Title, Outlines, Grid, Ruler, Clear, Collapse, Expand string
	PickFirst, PickSecond, Complete, Legend, Inspector    string
	Viewport                                              func(width, height float32) string
	Distance                                              func(horizontal, vertical float32) string
}

// Options configures the view wrapper. Labels is evaluated each frame so that
// changing language does not recreate the application view or inspector state.
type Options struct {
	Labels   func() Labels
	GridStep float32
	TopInset float32
}

// Measurement records two manually selected points in window DIPs. These are
// visual point distances, not an inferred element gap or a CSS pixel value.
type Measurement struct{ Start, End ui.Point }

func (m Measurement) Horizontal() float32 { return float32(math.Abs(float64(m.End.X - m.Start.X))) }
func (m Measurement) Vertical() float32   { return float32(math.Abs(float64(m.End.Y - m.Start.Y))) }

// State belongs to one window, on the UI thread. The zero value is disabled.
// Only the ruler captures pointer input; outlines and grid are pass-through.
type State struct {
	Enabled, Outlines, Grid, Ruler, Collapsed bool
	measurement                               Measurement
	points                                    int
	viewport                                  ui.Rect
}

func (s *State) Measurement() (Measurement, bool) { return s.measurement, s.points == 2 }
func (s *State) Viewport() ui.Rect                { return s.viewport }
func (s *State) Clear()                           { s.points = 0; s.measurement = Measurement{} }

// Pick records a point. A third pick begins a new measurement. Non-finite and
// out-of-viewport coordinates are ignored. Disabling the ruler stops picking.
func (s *State) Pick(point ui.Point) {
	if !s.Enabled || !s.Ruler || !finite(point.X) || !finite(point.Y) || point.X < 0 || point.Y < 0 || point.X > s.viewport.W || point.Y > s.viewport.H {
		return
	}
	if s.points != 1 {
		s.measurement = Measurement{Start: point, End: point}
		s.points = 1
	} else {
		s.measurement.End = point
		s.points = 2
	}
}

// Wrap decorates the whole window without introducing a layout parent around
// view. Its element identities, available size and focus behavior stay intact.
// The caller must enable MyGo's native developer tools separately for F12.
func (s *State) Wrap(view func(*ui.Context), options Options) func(*ui.Context) {
	return func(c *ui.Context) {
		view(c)
		if !s.Enabled {
			return
		}
		w, h := c.Size()
		next := ui.Rect{W: w, H: h}
		if next != s.viewport {
			s.Clear()
			s.viewport = next
		}
		if s.Outlines {
			c.Root().Debug()
		}
		labels := Labels{}
		if options.Labels != nil {
			labels = options.Labels()
		}
		step := options.GridStep
		if !finite(step) || step < 4 {
			step = 8
		}
		ui.Overlay(c, func() {
			layer := ui.Box(c).Key("layoutaudit-layer").Absolute().Left(0).Top(0).Right(0).Bottom(0)
			if s.Ruler {
				layer.Cursor(ui.CursorCrosshair).Label(labels.Ruler)
				x, y, over := layer.PointerPosition()
				if layer.Clicked() && over {
					s.Pick(ui.Point{X: x, Y: y})
				}
				if layer.OverlayShortcut(0, ui.KeyEscape) {
					s.Ruler = false
					s.Clear()
				}
			} else {
				layer.PassThrough()
			}
			layer.Draw(func(p *ui.Painter, bounds ui.Rect) {
				paint(p, bounds, s.Grid, step, s.measurement, s.points)
			})
			s.toolbar(c, options, labels)
		})
	}
}

func (s *State) toolbar(c *ui.Context, options Options, labels Labels) {
	w, _ := c.Size()
	top := max(options.TopInset, float32(8))
	panel := ui.Column(c).Key("layoutaudit-toolbar").Absolute().Top(top).Right(12).
		Width(min(float32(344), max(float32(180), w-24))).Padding(12).Gap(9).
		Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(8)
	panel.Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, labels.Title).FontSize(12).Bold().Grow(1)
			label := labels.Collapse
			if s.Collapsed {
				label = labels.Expand
			}
			if ui.Button(c, label).FontSize(11).Clicked() {
				s.Collapsed = !s.Collapsed
			}
		})
		if s.Collapsed {
			return
		}
		ui.Row(c).Gap(8).Children(func() {
			ui.Checkbox(c, &s.Outlines, labels.Outlines).FontSize(11)
			ui.Checkbox(c, &s.Grid, labels.Grid).FontSize(11)
			if ui.Checkbox(c, &s.Ruler, labels.Ruler).FontSize(11).Changed() {
				s.Clear()
			}
		})
		if labels.Viewport != nil {
			ui.Text(c, labels.Viewport(s.viewport.W, s.viewport.H)).FontSize(11)
		}
		if s.Ruler {
			help := labels.PickFirst
			if s.points == 1 {
				help = labels.PickSecond
			}
			if s.points == 2 {
				help = labels.Complete
			}
			ui.Text(c, help).FontSize(11).Wrap().TextColor(c.Theme().TextMuted)
		}
		if m, ok := s.Measurement(); ok {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				if labels.Distance != nil {
					ui.Text(c, labels.Distance(m.Horizontal(), m.Vertical())).FontSize(12).Grow(1)
				}
				if ui.Button(c, labels.Clear).FontSize(11).Clicked() {
					s.Clear()
				}
			})
		}
		ui.Text(c, labels.Legend).FontSize(10).Wrap().TextColor(c.Theme().TextMuted)
		ui.Text(c, labels.Inspector).FontSize(10).Wrap().TextColor(c.Theme().TextMuted)
	})
}

func finite(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }

func paint(p *ui.Painter, r ui.Rect, grid bool, step float32, m Measurement, points int) {
	if grid {
		color := ui.RGBA(38, 145, 130, .16)
		for x := r.X; x < r.X+r.W; x += step {
			p.Line(x, r.Y, x, r.Y+r.H, 1, color)
		}
		for y := r.Y; y < r.Y+r.H; y += step {
			p.Line(r.X, y, r.X+r.W, y, 1, color)
		}
	}
	if points == 0 {
		return
	}
	color := ui.Hex("#d8508a")
	x, y := r.X+m.Start.X, r.Y+m.Start.Y
	p.Line(x-5, y, x+5, y, 1, color)
	p.Line(x, y-5, x, y+5, 1, color)
	if points != 2 {
		return
	}
	x2, y2 := r.X+m.End.X, r.Y+m.End.Y
	p.Line(x, y, x2, y, 1, color)
	p.Line(x2, y, x2, y2, 1, color)
	p.Line(x2-5, y2, x2+5, y2, 1, color)
	p.Line(x2, y2-5, x2, y2+5, 1, color)
	// Numeric dimension labels carry no natural language; the toolbar provides
	// translated names and explicitly identifies these values as DIPs.
	p.Text((x+x2)/2, max(r.Y, y-15), fmt.Sprintf("%.1f", m.Horizontal()), 11, color)
	p.Text(min(r.X+r.W-48, x2+6), (y+y2)/2, fmt.Sprintf("%.1f", m.Vertical()), 11, color)
}
