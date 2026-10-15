package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/theme"
)

// Shared opaque surfaces distinguish navigation from content in both windows.
func clientTheme(c *ui.Context) *ui.Theme {
	t := *theme.Onevra(c)
	if t.Dark {
		t.Background = ui.Hex("#211f1c")
		t.Surface = ui.Hex("#292723")
	} else {
		t.Background = ui.Hex("#fcfbf9")
		t.Surface = ui.Hex("#f0eee9")
	}
	return &t
}

type notificationState struct {
	kind, typedMessage string
	clear              bool
}

func (a *application) inform(message string) {
	message = a.t(message)
	a.notice = message
	a.notifications.kind, a.notifications.typedMessage = components.ToastInfo, message
}

func (a *application) warn(message string) {
	message = a.t(message)
	a.notice = message
	a.notifications.kind, a.notifications.typedMessage = components.ToastWarning, message
}

func (a *application) clearNotifications() {
	a.notice, a.errorText = "", ""
	a.notifications = notificationState{clear: true}
}

// A follow-up read can start before the completed operation has painted its
// outcome. Dismiss displayed feedback without dropping that pending outcome.
func (a *application) beginOperationFeedback() {
	a.errorText = ""
	a.notifications.clear = true
}

// All operation outcomes share one window-level toast. A later outcome replaces
// the earlier one, repeated copies restart the timer, and failures remain until
// dismissed. Toasts live above dialogs and never shift the underlying controls.
func (a *application) flushNotifications(c *ui.Context) {
	const id = "passpine-feedback"
	if a.notifications.clear {
		c.CloseToast(id)
		a.notifications.clear = false
	}
	if a.errorText != "" {
		c.AddToast(ui.Toast{ID: id, Title: a.t("操作未完成"), Description: a.t(a.errorText), Type: components.ToastError, Timeout: -1})
		a.errorText, a.notice = "", ""
		a.notifications = notificationState{}
		return
	}
	if a.notice == "" {
		return
	}
	kind := components.ToastSuccess
	if a.notice == a.notifications.typedMessage {
		kind = a.notifications.kind
	}
	timeout := 3 * time.Second
	if kind != components.ToastSuccess {
		timeout = 6 * time.Second
	}
	c.AddToast(ui.Toast{ID: id, Title: a.t(a.notice), Type: kind, Timeout: timeout})
	a.notice = ""
	a.notifications = notificationState{}
}
