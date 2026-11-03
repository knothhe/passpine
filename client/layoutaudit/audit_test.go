package layoutaudit

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func testLabels() Labels {
	return Labels{Title: "Audit", Outlines: "Bounds", Grid: "Grid", Ruler: "Measure", Clear: "Clear", Collapse: "Collapse", Expand: "Expand", PickFirst: "First point", PickSecond: "Second point", Complete: "Complete", Legend: "Legend", Inspector: "Inspector"}
}

func TestMeasurementCoordinatesAndLifecycle(t *testing.T) {
	s := State{Enabled: true, Ruler: true, viewport: ui.Rect{W: 800, H: 600}}
	s.Pick(ui.Point{X: 250, Y: 120})
	for _, point := range []ui.Point{{X: -1}, {Y: 601}, {X: 801}, {X: float32(math.NaN())}, {Y: float32(math.Inf(1))}} {
		s.Pick(point)
	}
	if _, ready := s.Measurement(); ready {
		t.Fatal("invalid coordinates completed measurement")
	}
	s.Pick(ui.Point{X: 50, Y: 80})
	m, ready := s.Measurement()
	if !ready || m.Horizontal() != 200 || m.Vertical() != 40 {
		t.Fatalf("reverse measurement = %#v, %v", m, ready)
	}
	s.Ruler = false
	s.Pick(ui.Point{X: 1, Y: 1})
	if got, _ := s.Measurement(); got != m {
		t.Fatal("disabled ruler accepted a point")
	}
	s.Ruler = true
	s.Pick(ui.Point{X: 20, Y: 30})
	if _, ready := s.Measurement(); ready || s.points != 1 {
		t.Fatal("third point did not begin a new measurement")
	}
	s.Clear()
	if s.points != 0 || s.measurement != (Measurement{}) {
		t.Fatal("clear retained coordinates")
	}
}

func TestWrapperPreservesLayoutAndPassesThroughUntilMeasuring(t *testing.T) {
	clicks, escapes, input := 0, 0, "draft"
	content := func(c *ui.Context) {
		c.OnShortcut(0, ui.KeyEscape, func() { escapes++ })
		ui.Column(c).Fill().Padding(24).Gap(12).Children(func() {
			if ui.Button(c, "Application action").Width(160).Clicked() {
				clicks++
			}
			ui.TextInput(c, &input).Width(180).Label("Draft")
		})
	}
	plain := ui.NewTester(content, 900, 600)
	want, ok := plain.Find("Application action")
	if !ok {
		t.Fatal("missing reference button")
	}
	s := State{}
	view := ui.NewTester(s.Wrap(content, Options{Labels: testLabels}), 900, 600)
	if view.HasText("Audit") {
		t.Fatal("disabled wrapper rendered toolbar")
	}
	got, _ := view.Find("Application action")
	if got != want {
		t.Fatalf("wrapper moved content: %#v != %#v", got, want)
	}
	s.Enabled, s.Outlines, s.Grid = true, true, true
	view.Frame()
	got, _ = view.Find("Application action")
	if got != want {
		t.Fatal("guides changed application layout")
	}
	if err := view.Click("Application action"); err != nil || clicks != 1 {
		t.Fatalf("guides intercepted button: clicks=%d, err=%v", clicks, err)
	}
	if err := view.Click("Draft"); err != nil {
		t.Fatal(err)
	}
	view.Type(" saved")
	if input != "draft saved" {
		t.Fatalf("guides intercepted typing: %q", input)
	}
	if err := view.Click("Measure"); err != nil || !s.Ruler {
		t.Fatalf("could not enable ruler: %v", err)
	}
	view.ClickAt(40, 40)
	view.ClickAt(210, 140)
	if clicks != 1 {
		t.Fatal("ruler activated underlying application button")
	}
	m, ready := s.Measurement()
	if !ready || m.Horizontal() != 170 || m.Vertical() != 100 {
		t.Fatalf("pointer coordinates were not measured: %#v, %v", m, ready)
	}
	view.Key(0, ui.KeyEscape)
	if s.Ruler {
		t.Fatal("Escape did not release ruler input capture")
	}
	if escapes != 0 {
		t.Fatal("ruler exit also triggered an underlying Escape action")
	}
	if err := view.Click("Application action"); err != nil || clicks != 2 {
		t.Fatalf("ruler exit did not restore input: %d, %v", clicks, err)
	}
}

func TestResizeClearsStaleCoordinatesAndLabelsUpdate(t *testing.T) {
	s := State{Enabled: true, Ruler: true}
	labels := testLabels()
	view := ui.NewTester(s.Wrap(func(*ui.Context) {}, Options{Labels: func() Labels { return labels }}), 800, 600)
	s.Pick(ui.Point{X: 20, Y: 30})
	s.Pick(ui.Point{X: 80, Y: 90})
	labels.Title = "Translated audit"
	view.Frame()
	if !view.HasText(labels.Title) {
		t.Fatal("labels did not update")
	}
	if _, ready := s.Measurement(); !ready {
		t.Fatal("localization discarded measurement")
	}
	view.SetSize(640, 480)
	if _, ready := s.Measurement(); ready || s.Viewport() != (ui.Rect{W: 640, H: 480}) {
		t.Fatal("resize left stale window coordinates")
	}
}
