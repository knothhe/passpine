package app

import (
	"context"
	"errors"
	"github.com/knothhe/passpine-neo/client/vault"
	"time"
)

func gitStatusOf(ctx context.Context, v *vault.Vault) (Data, error) {
	info, e := v.GitStatus(ctx)
	d := Data{GitInitialized: info.Initialized, GitBranch: info.Branch, GitRemote: info.Remote, GitChanges: info.Changes}
	if !info.Available {
		d.GitSummary = "未找到 Git"
	} else if !info.Initialized {
		d.GitSummary = "尚未初始化 Git"
	} else if info.Conflict {
		d.GitSummary = "存在冲突，请先解决"
	} else if info.Remote == "" {
		d.GitSummary = "仅本地仓库 · 尚未配置远端"
	} else {
		d.GitSummary = "远端已配置"
	}
	return d, e
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
	s.lastSync = time.Now()
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var data Data
	var err error
	switch r.Action {
	case "git-status":
		data, err = gitStatusOf(ctx, current)
	case "git-init":
		err = current.GitInitialize(ctx)
		if err == nil {
			data, err = gitStatusOf(ctx, current)
		}
	case "git-remote":
		err = current.GitSetRemote(ctx, r.URL)
		if err == nil {
			data, err = gitStatusOf(ctx, current)
		}
	case "git-sync":
		var result vault.GitResult
		result, err = current.GitSync(ctx)
		if err == nil {
			data, _ = gitStatusOf(ctx, current)
			data.GitSummary = result.Message
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current != s.active || id != s.prefs.Current {
		err = errors.New("vault_changed")
		data = Data{}
	}
	if r.Action == "git-sync" {
		s.cachedAt = time.Time{}
		s.entriesVersion++
	}
	if err != nil {
		s.gitError = err.Error()
	} else {
		s.gitError = ""
	}
	out := Response{Protocol: 1, Status: "ok", Data: s.snapshot()}
	mergeData(&out.Data, data)
	if err != nil {
		out.Status = "error"
		out.Code, out.Message = classify(err)
	}
	s.log(r.Action, source, out.Status)
	if err == nil && r.Action == "git-sync" && s.onChange != nil {
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
