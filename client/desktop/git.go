package desktop

import (
	"time"

	"github.com/knothhe/passpine/client/app"
)

// Git owns no decrypted UI state and must not hold the desktop's credential
// request busy flag while waiting for a remote. Locking only invalidates secret
// requests; Git results remain valid for the same active vault.
func (a *application) startGitCall(req app.Request, done func(app.Data), userAction bool) {
	mutating := req.Action != "git-status"
	if a.closing || a.service == nil || mutating && a.gitActionPending {
		return
	}
	if userAction {
		a.lastInput = time.Now()
		a.beginOperationFeedback()
	}
	if mutating {
		a.gitActionPending = true
		a.gitActionSerial++
	}
	serial, vaultID := a.gitActionSerial, a.state.VaultID
	if req.VaultID == "" {
		req.VaultID = vaultID
	}
	go func() {
		result := a.service.Execute(req)
		a.runOnMain(func() {
			if mutating && serial == a.gitActionSerial {
				a.gitActionPending = false
			}
			if a.closing || a.state.VaultID != vaultID {
				return
			}
			if result.Data.VaultID == vaultID {
				a.applyGitState(result.Data)
			}
			if result.Status != "ok" {
				a.errorText = result.Message
				if a.errorText == "" {
					a.errorText = a.t("Git 操作未完成，请重试")
				}
			} else if done != nil {
				done(result.Data)
			}
			a.invalidate()
		})
	}()
	a.invalidate()
}

func (a *application) applyGitState(data app.Data) {
	a.state.GitAvailable = data.GitAvailable
	a.state.GitInitialized = data.GitInitialized
	a.state.GitBranch = data.GitBranch
	a.state.GitRemote = data.GitRemote
	a.state.GitUpstream = data.GitUpstream
	a.state.GitChanges = data.GitChanges
	a.state.GitAhead = data.GitAhead
	a.state.GitBehind = data.GitBehind
	a.state.GitConflict = data.GitConflict
	a.state.GitSummary = data.GitSummary
	a.state.GitError = data.GitError
	a.state.GitSyncing = data.GitSyncing
	a.state.GitRefreshing = data.GitRefreshing
	a.state.GitLastResult = data.GitLastResult
	a.state.GitLastSync = data.GitLastSync
	a.state.GitStatusAt = data.GitStatusAt
}
