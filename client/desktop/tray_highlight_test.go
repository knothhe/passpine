package desktop

import (
	"testing"

	"github.com/knothhe/passpine/client/app"
)

type fakeTrayButton struct {
	highlighted bool
	closed      bool
}

func (b *fakeTrayButton) SetHighlighted(value bool) { b.highlighted = value }
func (b *fakeTrayButton) Close()                    { b.closed = true }

func TestTrayHighlightFollowsPanelLifecycle(t *testing.T) {
	for _, dismiss := range []struct {
		name string
		run  func(*application)
	}{
		{"second click", (*application).trayClicked},
		{"blur", (*application).quickSearchBlurred},
		{"hide", (*application).hideQuickSearch},
		{"destroy", (*application).closeQuickSearch},
		{"native close", (*application).quickSearchClosed},
		{"global search", (*application).quickSearch},
		{"main window", func(a *application) {
			a.initializing = true
			a.state.Settings.TrayClickAction = app.TrayClickMainWindow
			a.trayClicked()
		}},
	} {
		t.Run(dismiss.name, func(t *testing.T) {
			a, _ := quickSearchFixture()
			a.quick.visible, a.busy = false, true
			button := &fakeTrayButton{}
			a.lifecycle.trayButton = button
			a.trayClicked()
			if !button.highlighted || !a.quick.visible {
				t.Fatal("opening the tray panel did not retain its button highlight")
			}
			dismiss.run(a)
			if button.highlighted {
				t.Fatal("the tray button stayed highlighted after leaving the tray panel")
			}
		})
	}
}

func TestTrayHighlightSurvivesDeviceAuthenticationBlur(t *testing.T) {
	a, _ := quickSearchFixture()
	a.quick.visible, a.busy = false, true
	button := &fakeTrayButton{}
	a.lifecycle.trayButton = button
	a.trayClicked()
	a.quick.operation = "device-unlock"
	a.quickSearchBlurred()
	if !a.quick.visible || !button.highlighted {
		t.Fatal("the authentication prompt dismissed or unhighlighted the tray panel")
	}
	a.quick.operation = ""
	a.quickSearchBlurred()
	if a.quick.visible || button.highlighted {
		t.Fatal("normal focus loss did not dismiss the panel and clear the highlight")
	}
}

func TestGlobalSearchDoesNotHighlightTrayAndMenuTrackingCanResync(t *testing.T) {
	a, _ := quickSearchFixture()
	a.quick.visible, a.busy = false, true
	button := &fakeTrayButton{}
	a.lifecycle.trayButton = button
	a.quickSearch()
	if button.highlighted {
		t.Fatal("global search highlighted the tray button")
	}
	a.trayClicked()
	button.highlighted = false // AppKit clears this when menu tracking ends.
	a.updateTrayHighlight()
	if !button.highlighted {
		t.Fatal("menu tracking lost the open tray panel's highlight")
	}
}
