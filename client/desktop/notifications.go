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
	a.notificationActionSerial++
	message = a.t(message)
	a.notice = message
	a.notifications = notificationState{kind: components.ToastInfo, typedMessage: message, clear: a.notifications.clear}
}

func (a *application) warn(message string) {
	a.notificationActionSerial++
	message = a.t(message)
	a.notice = message
	a.notifications = notificationState{kind: components.ToastWarning, typedMessage: message, clear: a.notifications.clear}
}

// notifyWithAction raises a success toast with one action, such as Undo. A
// zero timeout uses the standard success duration.
func (a *application) notifyWithAction(message, description, action string, timeout time.Duration, onAction func()) {
	a.setActionNotification(message, description, action, components.ToastSuccess, timeout, onAction)
}

func (a *application) notifyErrorWithAction(message, description, action string, onAction func()) {
	a.setActionNotification(message, description, action, components.ToastError, -1, onAction)
}

func (a *application) setActionNotification(message, description, action, kind string, timeout time.Duration, onAction func()) {
	a.notificationActionSerial++
	serial, generation, vaultID := a.notificationActionSerial, a.generation, a.state.VaultID
	deadline := time.Time{}
	if timeout >= 0 {
		if timeout == 0 {
			timeout = 3 * time.Second
		}
		deadline = time.Now().Add(timeout)
	}
	a.errorText = ""
	a.notice = message
	a.notifications.expiresAt = time.Time{}
	a.notifications.kind, a.notifications.typedMessage = kind, message
	a.notifications.description, a.notifications.action = description, action
	a.notifications.onAction = func() {
		if a.notificationActionSerial != serial || a.generation != generation || a.state.VaultID != vaultID || a.busy || a.closing || a.editor.open || a.updates.preparing || !deadline.IsZero() && !time.Now().Before(deadline) {
			return
		}
		a.notificationActionSerial++
		if onAction != nil {
			onAction()
		}
	}
	a.notifications.timeout = timeout
}

func (a *application) clearNotifications() {
	a.notificationActionSerial++
	a.notice, a.errorText = "", ""
	a.notifications = notificationState{clear: true}
}

// A follow-up read can start before the completed operation has painted its
// outcome. Dismiss displayed feedback without dropping that pending outcome.
func (a *application) beginOperationFeedback() {
	// A queued read can begin in the completion callback before its actionable
	// outcome is painted. Keep that pending action's ownership with its message.
	if a.notice == "" || a.notice != a.notifications.typedMessage || a.notifications.onAction == nil {
		a.notificationActionSerial++
	}
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
			a.notificationActionSerial++
			a.notifications.expiresAt = time.Time{}
		}
	}
	if a.errorText != "" {
		a.notificationActionSerial++
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
	if a.notice == a.notifications.typedMessage && a.notifications.timeout != 0 {
		timeout = a.notifications.timeout
	}
	toast.Type = kind
	if timeout < 0 {
		serial := a.notificationActionSerial
		toast.OnClose = func() {
			if a.notificationActionSerial == serial {
				a.notificationActionSerial++
			}
		}
		c.AddToast(toast)
		a.notice = ""
		a.notifications = notificationState{}
		return
	}
	deadline := c.Now().Add(timeout)
	serial := a.notificationActionSerial
	toast.OnClose = func() {
		if a.notificationActionSerial == serial && a.notifications.expiresAt == deadline {
			a.notifications.expiresAt = time.Time{}
			a.notificationActionSerial++
		}
	}
	c.AddToast(toast)
	c.After(timeout)
	a.notice = ""
	a.notifications = notificationState{expiresAt: deadline}
}
