package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

type notificationState struct {
	kind, typedMessage string
	clear              bool
	expiresAt          time.Time
	// An actionable success, such as Undo, keeps its description and action
	// for exactly the message it was raised with.
	description, action string
	onAction            func()
	timeout             time.Duration
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

// notifyWithAction raises a success toast with one action, such as Undo. A
// zero timeout uses the standard success duration.
func (a *application) notifyWithAction(message, description, action string, timeout time.Duration, onAction func()) {
	a.notice = message
	a.notifications.kind, a.notifications.typedMessage = components.ToastSuccess, message
	a.notifications.description, a.notifications.action, a.notifications.onAction = description, action, onAction
	a.notifications.timeout = timeout
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
		a.notifications.expiresAt = time.Time{}
	}
	// MyGo pauses its toast timer when the window loses focus. Operation
	// feedback instead expires by wall time, including while in the background.
	if deadline := a.notifications.expiresAt; !deadline.IsZero() {
		if remaining := deadline.Sub(c.Now()); remaining > 0 {
			c.After(remaining)
		} else {
			c.CloseToast(id)
			a.notifications.expiresAt = time.Time{}
		}
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
	toast := ui.Toast{ID: id, Title: a.t(a.notice), Timeout: -1}
	if a.notice == a.notifications.typedMessage {
		kind = a.notifications.kind
		toast.Description, toast.Action, toast.OnAction = a.notifications.description, a.notifications.action, a.notifications.onAction
	}
	timeout := 3 * time.Second
	if kind != components.ToastSuccess {
		timeout = 6 * time.Second
	}
	if a.notice == a.notifications.typedMessage && a.notifications.timeout > 0 {
		timeout = a.notifications.timeout
	}
	toast.Type = kind
	deadline := c.Now().Add(timeout)
	toast.OnClose = func() {
		if a.notifications.expiresAt == deadline {
			a.notifications.expiresAt = time.Time{}
		}
	}
	c.AddToast(toast)
	c.After(timeout)
	a.notice = ""
	a.notifications = notificationState{expiresAt: deadline}
}
