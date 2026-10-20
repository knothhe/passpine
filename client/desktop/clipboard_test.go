package desktop

import (
	"errors"
	"testing"
	"time"

	"github.com/knothhe/passpine/client/app"
)

type clipboardLeaseBackend struct{ Backend }

func (clipboardLeaseBackend) ClipboardLease() func(func()) error {
	return func(callback func()) error { callback(); return nil }
}

func TestCustomFieldNamedUsernameRetainsSensitiveClipboardHints(t *testing.T) {
	for _, matchesUsername := range []bool{false, true} {
		t.Run(map[bool]string{false: "distinct value", true: "same value"}[matchesUsername], func(t *testing.T) {
			a := fixture()
			a.state.Settings.ClipboardSeconds = 0
			value := "custom-field-secret"
			if matchesUsername {
				value = a.detail.Username
			}
			requests := make(chan app.Request, 1)
			a.service = clipboardLeaseBackend{backendFunc(func(request app.Request) app.Response {
				requests <- request
				return app.Response{}
			})}
			callbacks := make(chan func(), 1)
			a.runOnMain = func(callback func()) { callbacks <- callback }
			writes := 0
			a.clipboardWriter = func(text string, sensitive bool) error {
				if text != value || !sensitive {
					t.Fatal("custom field lost its sensitive clipboard hints")
				}
				writes++
				return nil
			}
			a.copy("custom:0", value, "用户名")
			awaitMain(t, callbacks)
			select {
			case request := <-requests:
				t.Fatalf("custom label was treated as a credential field: %+v", request)
			default:
			}
			if writes != 1 {
				t.Fatalf("expected one sensitive custom-field write, got %d", writes)
			}
		})
	}
}

func TestClipboardCallbackValidatesLeaseOnMainThread(t *testing.T) {
	a := fixture()
	onMain := false
	a.runOnMain = func(f func()) {
		onMain = true
		defer func() { onMain = false }()
		f()
	}
	writes := 0
	a.clipboardWriter = func(text string, sensitive bool) error {
		if !onMain || text != "generated-password" || !sensitive {
			t.Fatal("clipboard write lost its main-thread or sensitive context")
		}
		writes++
		return nil
	}
	write := app.ClipboardWrite{Text: "generated-password", Sensitive: true, Lease: func(f func()) error {
		if !onMain {
			t.Fatal("clipboard lease was checked before reaching the main thread")
		}
		f()
		return nil
	}}
	// Standalone generated passwords may be copied while the vault is locked.
	a.state.Locked = true
	if err := a.copyToClipboard(write); err != nil || writes != 1 {
		t.Fatalf("generated clipboard write failed: err=%v writes=%d", err, writes)
	}
	denied := errors.New("expired clipboard lease")
	write.Lease = func(func()) error { return denied }
	if err := a.copyToClipboard(write); !errors.Is(err, denied) || writes != 1 {
		t.Fatal("expired clipboard lease was ignored")
	}
	a.closing = true
	write.Lease = func(func()) error { t.Fatal("closing application checked a clipboard lease"); return nil }
	if err := a.copyToClipboard(write); err == nil || writes != 1 {
		t.Fatal("closing application accepted a clipboard write")
	}
}

func TestFailedClipboardWritePreservesPreviousCleanup(t *testing.T) {
	a := fixture()
	a.runOnMain = func(f func()) { f() }
	a.clipboard = "previous-secret"
	previousTimer := time.AfterFunc(time.Hour, func() {})
	a.clipboardTimer = previousTimer
	t.Cleanup(func() { previousTimer.Stop() })
	failure := errors.New("native clipboard unavailable")
	a.clipboardWriter = func(string, bool) error { return failure }
	err := a.copyToClipboard(app.ClipboardWrite{Text: "next-secret", Seconds: 30, Sensitive: true, Lease: func(f func()) error { f(); return nil }})
	if !errors.Is(err, failure) {
		t.Fatalf("native write error was hidden: %v", err)
	}
	a.state.Settings.Language = "en"
	if a.t(err.Error()) != "Could not write to the clipboard. Please try again." {
		t.Fatal("native clipboard failure was not presented through the language catalog")
	}
	if a.clipboard != "previous-secret" || a.clipboardTimer != previousTimer || !previousTimer.Stop() {
		t.Fatal("failed write replaced or cancelled the previous clipboard cleanup")
	}
}

func TestClipboardCallbackRequiresLeaseAndPreservesNonSensitiveWrites(t *testing.T) {
	a := fixture()
	a.runOnMain = func(f func()) { f() }
	writes := 0
	a.clipboardWriter = func(text string, sensitive bool) error {
		if text != "account-name" || sensitive {
			t.Fatal("non-sensitive username was marked as a secret")
		}
		writes++
		return nil
	}
	write := app.ClipboardWrite{Text: "account-name"}
	if err := a.copyToClipboard(write); err == nil || writes != 0 {
		t.Fatal("clipboard write without a lease was accepted")
	}
	write.Lease = func(f func()) error { f(); return nil }
	if err := a.copyToClipboard(write); err != nil || writes != 1 || a.clipboard != "account-name" {
		t.Fatalf("non-sensitive write failed: err=%v writes=%d", err, writes)
	}
}
