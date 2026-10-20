package desktop

import (
	"errors"
	"testing"
	"time"
)

type clipboardExpiryTimer struct {
	delay time.Duration
	fire  func()
	timer *time.Timer
}

type clipboardExpiryFixture struct {
	t        *testing.T
	a        *application
	text     string
	clears   int
	writeErr error
	timers   []clipboardExpiryTimer
	pending  []func()
}

func newClipboardExpiryFixture(t *testing.T) *clipboardExpiryFixture {
	t.Helper()
	h := &clipboardExpiryFixture{t: t, a: fixture()}
	h.a.clipboardWriter = func(text string, sensitive bool) error {
		if h.writeErr != nil {
			return h.writeErr
		}
		h.text = text
		return nil
	}
	h.a.clipboardReader = func() string { return h.text }
	h.a.clipboardClearer = func() {
		h.text = ""
		h.clears++
	}
	h.a.clipboardAfterFunc = func(delay time.Duration, callback func()) *time.Timer {
		// The test fires callbacks explicitly, including after cancellation, to
		// reproduce timers which have already posted work to the main thread.
		timer := time.AfterFunc(time.Hour, func() {})
		t.Cleanup(func() { timer.Stop() })
		h.timers = append(h.timers, clipboardExpiryTimer{delay: delay, fire: callback, timer: timer})
		return timer
	}
	h.a.runOnMain = func(callback func()) { h.pending = append(h.pending, callback) }
	return h
}

func (h *clipboardExpiryFixture) copy(text string, seconds int) {
	h.t.Helper()
	if err := h.a.writeClipboard(text, seconds, true); err != nil {
		h.t.Fatalf("cannot copy fixture password: %v", err)
	}
}

func (h *clipboardExpiryFixture) expire(index int) {
	h.t.Helper()
	if index >= len(h.timers) {
		h.t.Fatalf("clipboard timer %d was not scheduled", index)
	}
	if !h.timers[index].timer.Stop() {
		h.t.Fatalf("clipboard timer %d was cancelled before its expiration", index)
	}
	h.timers[index].fire()
	if len(h.pending) != 1 {
		h.t.Fatalf("expiry did not post exactly one main-thread callback: %d", len(h.pending))
	}
}

func (h *clipboardExpiryFixture) drain() {
	h.t.Helper()
	for len(h.pending) > 0 {
		callback := h.pending[0]
		h.pending = h.pending[1:]
		callback()
	}
}

func TestClipboardExpiryUsesConfiguredSecondsAndClearsCopiedPassword(t *testing.T) {
	h := newClipboardExpiryFixture(t)
	h.copy("fixture-secret", 45)
	if len(h.timers) != 1 || h.timers[0].delay != 45*time.Second {
		t.Fatalf("clipboard expiration ignored configured seconds: %+v", h.timers)
	}
	h.expire(0)
	if h.text != "fixture-secret" || h.clears != 0 {
		t.Fatal("clipboard expiration ran before its main-thread callback")
	}
	h.drain()
	if h.text != "" || h.a.clipboard != "" || h.a.clipboardTimer != nil || h.clears != 1 {
		t.Fatalf("expired password remains: clipboard=%q tracked=%q clears=%d", h.text, h.a.clipboard, h.clears)
	}
}

func TestClipboardExpiryPreservesTextCopiedByAnotherApplication(t *testing.T) {
	h := newClipboardExpiryFixture(t)
	h.copy("fixture-secret", 30)
	h.text = "text copied elsewhere"
	h.expire(0)
	h.drain()
	if h.text != "text copied elsewhere" || h.clears != 0 {
		t.Fatal("clipboard expiration erased text copied by another application")
	}
	if h.a.clipboard != "" {
		t.Fatal("clipboard expiration retained an expired password in application state")
	}
}

func TestClipboardExpiryQueuedBeforeAnotherCopyCannotExpireItsReplacement(t *testing.T) {
	for _, replacement := range []string{"fixture-secret", "another-secret"} {
		t.Run(replacement, func(t *testing.T) {
			h := newClipboardExpiryFixture(t)
			h.copy("fixture-secret", 15)
			h.expire(0)
			// A timer can finish while the UI thread is busy. Stop cannot retract
			// the callback it has already queued, even when the text is identical.
			h.copy(replacement, 60)
			h.drain()
			if h.text != replacement || h.a.clipboard != replacement || h.clears != 0 {
				t.Fatalf("old expiration changed a newer copy: clipboard=%q tracked=%q clears=%d", h.text, h.a.clipboard, h.clears)
			}
			h.expire(1)
			h.drain()
			if h.text != "" || h.a.clipboard != "" || h.clears != 1 {
				t.Fatal("replacement password did not expire with its own timer")
			}
		})
	}
}

func TestClipboardExpiryQueuedBeforeLockCannotExpireSamePasswordCopiedAfterward(t *testing.T) {
	h := newClipboardExpiryFixture(t)
	h.copy("fixture-secret", 15)
	h.expire(0)
	h.a.state.Locked = true
	h.a.clearSecrets()
	if h.text != "" || h.clears != 1 {
		t.Fatal("clearing locked application secrets did not clear its clipboard password")
	}
	h.copy("fixture-secret", 60)
	h.drain()
	if h.text != "fixture-secret" || h.a.clipboard != "fixture-secret" || h.clears != 1 {
		t.Fatal("expiration from before locking cleared the password copied afterward")
	}
	h.expire(1)
	h.drain()
	if h.text != "" || h.a.clipboard != "" || h.clears != 2 {
		t.Fatal("password copied after clearing secrets did not expire with its own timer")
	}
}

func TestClipboardExpirySurvivesFailedReplacementWrite(t *testing.T) {
	h := newClipboardExpiryFixture(t)
	h.copy("fixture-secret", 30)
	previousTimer := h.a.clipboardTimer
	failure := errors.New("fixture clipboard write failure")
	h.writeErr = failure
	if err := h.a.writeClipboard("replacement-secret", 60, true); !errors.Is(err, failure) {
		t.Fatalf("replacement write hid its failure: %v", err)
	}
	if len(h.timers) != 1 || h.a.clipboardTimer != previousTimer || h.text != "fixture-secret" {
		t.Fatal("failed replacement changed the existing copy or its timer")
	}
	h.expire(0)
	h.drain()
	if h.text != "" || h.a.clipboard != "" || h.clears != 1 {
		t.Fatal("failed replacement disabled expiration of the existing copy")
	}
}

func TestClipboardExpiryClearPreservesCopyMadeWhileReadingClipboard(t *testing.T) {
	for _, replacement := range []string{"fixture-secret", "another-secret"} {
		t.Run(replacement, func(t *testing.T) {
			h := newClipboardExpiryFixture(t)
			h.copy("fixture-secret", 15)
			replaced := false
			h.a.clipboardReader = func() string {
				before := h.text
				if !replaced {
					replaced = true
					h.copy(replacement, 60)
				}
				return before
			}
			h.a.clearClipboard()
			if h.text != replacement || h.a.clipboard != replacement || h.clears != 0 {
				t.Fatalf("clipboard read reentry changed a newer copy: clipboard=%q tracked=%q clears=%d", h.text, h.a.clipboard, h.clears)
			}
			h.expire(1)
			h.drain()
			if h.text != "" || h.a.clipboard != "" || h.clears != 1 {
				t.Fatal("copy made during clipboard reading lost its own expiration")
			}
		})
	}
}
