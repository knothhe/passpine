package app

import (
	"context"
	"time"

	"github.com/knothhe/passpine/client/vault"
)

const automaticGitIdleDelay = 5 * time.Second
const automaticGitRetryDelay = 30 * time.Second

// Changes are coalesced until input has stopped. The optional interval also
// checks for remote changes, but never starts a sync during active editing.
func (s *Service) autoSyncDueLocked(now time.Time) bool {
	if s.closed || s.active == nil || !s.gitData.GitInitialized || s.gitOperation || !s.prefs.Settings.AutoSync {
		return false
	}
	if now.Before(s.gitRetryAt) {
		return false
	}
	if activity := s.lastActivity.Load(); activity != 0 && now.Sub(time.Unix(0, activity)) < automaticGitIdleDelay {
		return false
	}
	return s.gitSyncPending || s.prefs.Settings.SyncMinutes > 0 && now.Sub(s.lastSync) >= time.Duration(s.prefs.Settings.SyncMinutes)*time.Minute
}

func (s *Service) runAutomaticGitSync(id string) {
	_ = s.executeGit(Request{Action: "git-sync", VaultID: id}, "automatic")
}

func gitStatusOf(ctx context.Context, v *vault.Vault) (Data, error) {
	info, e := v.GitStatus(ctx)
	d := Data{GitAvailable: info.Available, GitInitialized: info.Initialized, GitBranch: info.Branch, GitRemote: info.Remote, GitUpstream: info.Upstream, GitChanges: info.Changes, GitAhead: info.Ahead, GitBehind: info.Behind, GitConflict: info.Conflict, GitStatusAt: time.Now().Format(time.RFC3339)}
	if !info.Available {
		d.GitSummary = "未找到 Git"
	} else if !info.Initialized {
		d.GitSummary = "尚未初始化 Git"
	} else if info.Conflict {
		d.GitSummary = "存在冲突，请先解决"
	} else if info.Remote == "" {
		d.GitSummary = "仅本地仓库 · 尚未配置远端"
	} else if info.Upstream == "" {
		d.GitSummary = "尚未建立远端跟踪"
	} else {
		d.GitSummary = "远端已配置"
	}
	return d, e
}

func copyGitData(out *Data, state Data) {
	out.GitAvailable = state.GitAvailable
	out.GitInitialized = state.GitInitialized
	out.GitBranch = state.GitBranch
	out.GitRemote = state.GitRemote
	out.GitUpstream = state.GitUpstream
	out.GitChanges = state.GitChanges
	out.GitAhead = state.GitAhead
	out.GitBehind = state.GitBehind
	out.GitConflict = state.GitConflict
	out.GitSummary = state.GitSummary
	out.GitSyncing = state.GitSyncing
	out.GitRefreshing = state.GitRefreshing
	out.GitLastResult = state.GitLastResult
	out.GitLastSync = state.GitLastSync
	out.GitStatusAt = state.GitStatusAt
}

func (s *Service) resetGitStateLocked() {
	if s.gitVault == s.active {
		return
	}
	s.gitVault = s.active
	s.gitEpoch++
	s.gitData = Data{}
	s.gitError, s.gitStatusError = "", ""
	s.gitRefreshAt, s.lastSync = time.Time{}, time.Time{}
	s.gitOperation = false
	s.gitSyncPending = false
	s.gitSyncQueued = false
	s.gitRetryAt = time.Time{}
}

func (s *Service) applyGitStatusLocked(data Data, err error) {
	s.gitData.GitRefreshing = false
	if err != nil {
		s.gitStatusError = err.Error()
		return
	}
	s.gitStatusError = ""
	data.GitSyncing = s.gitData.GitSyncing
	data.GitLastResult = s.gitData.GitLastResult
	data.GitLastSync = s.gitData.GitLastSync
	copyGitData(&s.gitData, data)
}

// Only local metadata is inspected here. It runs independently of the secret
// session and outside the service mutex, with no fetch or other network access.
func (s *Service) ensureGitRefreshLocked(force bool) {
	s.resetGitStateLocked()
	if s.closed || s.active == nil || s.gitOperation || s.gitData.GitRefreshing || (!force && time.Since(s.gitRefreshAt) < 5*time.Second) {
		return
	}
	current, id, epoch := s.active, s.prefs.Current, s.gitEpoch
	s.gitData.GitRefreshing = true
	s.gitRefreshAt = time.Now()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		data, err := gitStatusOf(ctx, current)
		cancel()
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed || s.active != current || s.prefs.Current != id || s.gitVault != current || s.gitEpoch != epoch {
			return
		}
		// A mutation begun during the read owns the next final status refresh.
		if !s.gitOperation {
			s.applyGitStatusLocked(data, err)
		} else {
			s.gitData.GitRefreshing = false
		}
		if s.onGitChange != nil {
			go s.onGitChange()
		}
	}()
}

// Network-bound Git work runs outside the application state mutex. A slow
// remote never blocks browsing, decrypting an entry, copying, or locking.
func (s *Service) executeGit(r Request, source string) Response {
	s.mu.Lock()
	current := s.active
	id := s.prefs.Current
	if s.closed || current == nil {
		s.mu.Unlock()
		return Response{Protocol: 1, Status: "error", Code: "not_configured", Message: "请先打开密码库"}
	}
	if r.VaultID != "" && r.VaultID != id {
		s.mu.Unlock()
		return Response{Protocol: 1, Status: "error", Code: "vault_changed", Message: "密码库已切换"}
	}
	s.resetGitStateLocked()
	if source == "automatic" {
		s.gitSyncQueued = false
		// Recheck under the same mutex that begins the operation. Input,
		// settings or the active vault may have changed after the timer tick.
		if !s.autoSyncDueLocked(time.Now()) {
			out := Response{RequestID: r.RequestID, Protocol: 1, Status: "ok", Data: s.snapshot()}
			s.mu.Unlock()
			return out
		}
	}
	mutation := r.Action != "git-status"
	if !mutation && s.gitOperation {
		out := Response{RequestID: r.RequestID, Protocol: 1, Status: "ok", Data: s.snapshot()}
		s.mu.Unlock()
		return out
	}
	if mutation && s.gitOperation {
		out := Response{RequestID: r.RequestID, Protocol: 1, Status: "error", Code: "vault_busy", Message: "Git 操作正在进行，请稍后重试", Data: s.snapshot()}
		s.mu.Unlock()
		return out
	}
	if mutation {
		s.gitOperation = true
		s.gitEpoch++
	}
	epoch := s.gitEpoch
	pendingBeforeSync := s.gitSyncPending
	if r.Action == "git-sync" {
		s.lastSync = time.Now()
		s.gitSyncPending = false
		s.gitRetryAt = time.Time{}
		s.gitData.GitSyncing = true
		s.gitError = ""
	}
	if s.onGitChange != nil && mutation {
		go s.onGitChange()
	}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var data Data
	var err error
	var result vault.GitResult
	switch r.Action {
	case "git-status":
		data, err = gitStatusOf(ctx, current)
	case "git-init":
		err = current.GitInitialize(ctx)
	case "git-remote":
		err = current.GitSetRemote(ctx, r.URL)
	case "git-sync":
		result, err = current.GitSync(ctx)
	}
	var statusErr error
	if mutation {
		statusContext, statusCancel := context.WithTimeout(context.Background(), 5*time.Second)
		data, statusErr = gitStatusOf(statusContext, current)
		statusCancel()
	} else {
		statusErr = err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || current != s.active || id != s.prefs.Current {
		code, message := "vault_changed", "密码库已切换"
		if s.closed {
			code, message = "app_unavailable", "应用已关闭"
		}
		return Response{RequestID: r.RequestID, Protocol: 1, Status: "error", Code: code, Message: message, Data: s.snapshot()}
	}
	if mutation {
		s.gitOperation = false
		s.gitData.GitSyncing = false
	}
	if mutation || (!s.gitOperation && s.gitEpoch == epoch) {
		s.gitRefreshAt = time.Now()
		s.applyGitStatusLocked(data, statusErr)
	}
	if r.Action == "git-sync" {
		if err != nil && (source == "automatic" || s.prefs.Settings.AutoSync || pendingBeforeSync || s.gitSyncPending) {
			s.gitSyncPending = true
			s.gitRetryAt = time.Now().Add(automaticGitRetryDelay)
		}
		if result.Changed {
			s.cachedAt = time.Time{}
			s.entriesVersion++
		}
		s.gitData.GitLastSync = time.Now().Format(time.RFC3339)
		if err == nil {
			if result.State == "local" {
				s.gitData.GitLastResult = "加密条目已提交到本地"
			} else {
				s.gitData.GitLastResult = "加密密码库同步完成"
			}
		} else {
			s.gitData.GitLastResult = "同步未完成"
		}
	}
	if mutation {
		if err != nil {
			s.gitError = err.Error()
		} else {
			s.gitError = ""
		}
	}
	out := Response{RequestID: r.RequestID, Protocol: 1, Status: "ok", Data: s.snapshot()}
	if err != nil {
		out.Status = "error"
		out.Code, out.Message = classify(err)
	}
	s.log(r.Action, source, out.Status)
	if mutation && s.onGitChange != nil {
		go s.onGitChange()
	}
	if r.Action == "git-sync" && result.Changed && s.onChange != nil {
		callback := s.onChange
		go callback()
	}
	return out
}

// Clone installs only into a new destination and leaves a failed destination
// untouched. It is a separate step from importing the recovery key.
func (s *Service) executeClone(r Request) Response {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return Response{Protocol: 1, Status: "error", Code: "app_unavailable", Message: "应用已关闭"}
	}
	if s.cloneCancel != nil {
		s.mu.Unlock()
		return Response{Protocol: 1, Status: "error", Code: "busy", Message: "已有下载正在进行"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	s.cloneCancel = cancel
	s.mu.Unlock()
	defer cancel()
	err := vault.Clone(ctx, r.URL, r.Directory)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cloneCancel = nil
	out := Response{Protocol: 1, Status: "ok", Data: s.snapshot()}
	out.Data.Destination = r.Directory
	if err != nil {
		out.Status = "error"
		out.Code, out.Message = classify(err)
	}
	s.log("clone", "desktop", out.Status)
	return out
}
