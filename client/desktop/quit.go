package desktop

import (
	"context"
	"time"

	"github.com/egoist/mygo"
)

type quitState struct {
	requested, draining, ready bool
	started                    time.Time
	timer                      *time.Timer
	quit                       func()
}

func (a *application) beforeQuit(event *mygo.QuitEvent) {
	if a.quit.ready {
		return
	}
	event.PreventDefault()
	a.requestQuit()
}

func (a *application) requestQuit() {
	if a.quit.requested || a.quit.draining || a.quit.ready {
		return
	}
	if a.editor.open && !a.editorBusy() {
		a.show()
		a.requestEditorClose(a.requestQuit)
		return
	}
	a.quit.requested, a.quit.started = true, time.Now()
	if a.settingsLoaded && a.settings != a.settingsSaved {
		a.settingsDirty = true
	}
	if a.gitRemote != a.state.GitRemote && a.gitRemoteVaultID == a.state.VaultID {
		a.gitRemoteSaveAt = time.Now()
	}
	a.continueQuit()
}

func (a *application) cancelQuitOnSaveFailure() {
	if a.quit.requested {
		a.quit.requested = false
		if a.quit.timer != nil {
			a.quit.timer.Stop()
			a.quit.timer = nil
		}
	}
}

func (a *application) continueQuit() {
	if !a.quit.requested || a.quit.draining {
		return
	}
	if a.quit.timer != nil {
		a.quit.timer.Stop()
		a.quit.timer = nil
	}
	if time.Since(a.quit.started) > 30*time.Second {
		a.quit.requested = false
		a.errorText = a.t("仍有操作尚未完成，请稍后重试退出。")
		a.show()
		a.invalidate()
		return
	}
	if a.editor.open && !a.editorBusy() {
		a.quit.requested = false
		a.requestQuit()
		return
	}
	a.saveSettingsAutomatically()
	if !a.gitRemoteSaveAt.IsZero() {
		a.saveGitRemoteAutomatically()
	}
	if !a.quit.requested {
		a.show()
		a.invalidate()
		return
	}
	if a.busy || a.settingsSaving || a.settingsDirty || a.gitRemoteSaving || !a.gitRemoteSaveAt.IsZero() || a.gitActionPending || a.generator.busy || a.quick.operation != "" {
		a.quit.timer = time.AfterFunc(50*time.Millisecond, func() { a.runOnMain(a.continueQuit) })
		return
	}
	a.saveManagerLayout()
	a.saveQuickSearchPosition()

	a.quit.draining, a.closing = true, true
	a.stopLightweightTimer()
	a.hideQuickSearch()
	if a.window != nil {
		a.window.Invalidate()
	}
	go func() {
		var err error
		if service, ok := a.service.(interface{ Shutdown(context.Context) error }); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err = service.Shutdown(ctx)
			cancel()
		}
		a.runOnMain(func() {
			if err != nil {
				a.closing, a.quit.requested, a.quit.draining = false, false, false
				a.errorText = a.t("退出未完成，请重试：%s", err)
				a.show()
				a.invalidate()
				return
			}
			a.quit.ready = true
			if a.updates.cancel != nil {
				a.updates.cancel()
			}
			close(a.stop)
			a.clearSecrets()
			if a.quit.quit != nil {
				a.quit.quit()
			}
		})
	}()
}
