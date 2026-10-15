package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type GitInfo struct {
	Available   bool   `json:"available"`
	Initialized bool   `json:"initialized"`
	Branch      string `json:"branch"`
	Remote      string `json:"remote"`
	Changes     int    `json:"changes"`
	Conflict    bool   `json:"conflict"`
	Ahead       int    `json:"ahead"`
	Behind      int    `json:"behind"`
}

type GitResult struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

func gitCommand(ctx context.Context, directory string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	base := []string{"--literal-pathspecs", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgSign=false", "-c", "protocol.ext.allow=never", "-c", "protocol.file.allow=user", "-C", directory}
	base = append(base, args...)
	command := exec.CommandContext(ctx, "git", base...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_SSH_COMMAND=ssh -o BatchMode=yes -o ConnectTimeout=15", "LC_ALL=C")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if e := command.Run(); e != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("git operation cancelled or timed out: %w", ctx.Err())
		}
		// Don't echo remote URLs, credentials, paths or custom helper output.
		message := stderr.String()
		switch {
		case strings.Contains(message, "Host key verification failed"):
			return "", errors.New("Git SSH host key is not trusted")
		case strings.Contains(message, "Permission denied") || strings.Contains(message, "Authentication failed"):
			return "", errors.New("Git authentication failed")
		case strings.Contains(message, "CONFLICT") || strings.Contains(message, "conflict"):
			return "", errors.New("Git has a merge conflict; resolve it before syncing")
		default:
			return "", fmt.Errorf("git %s failed: %w", args[0], e)
		}
	}
	return strings.TrimSuffix(stdout.String(), "\n"), nil
}

func managedStore(ctx context.Context, directory string) bool {
	top, e := gitCommand(ctx, directory, "rev-parse", "--show-toplevel")
	if e != nil {
		return false
	}
	canonical, e := filepath.EvalSymlinks(top)
	return e == nil && canonical == directory
}

func pendingGitOperation(ctx context.Context, directory string) bool {
	for _, marker := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD"} {
		name, e := gitCommand(ctx, directory, "rev-parse", "--git-path", marker)
		if e != nil {
			return true
		}
		if !filepath.IsAbs(name) {
			name = filepath.Join(directory, name)
		}
		if _, e = os.Stat(name); e == nil {
			return true
		}
	}
	conflicts, e := gitCommand(ctx, directory, "diff", "--name-only", "--diff-filter=U")
	return e != nil || conflicts != ""
}

func (v *Vault) GitStatus(ctx context.Context) (GitInfo, error) {
	return v.gitStatus(ctx)
}

func (v *Vault) gitStatus(ctx context.Context) (GitInfo, error) {
	_, e := exec.LookPath("git")
	info := GitInfo{Available: e == nil}
	if e != nil {
		return info, nil
	}
	if !managedStore(ctx, v.config.Directory) {
		return info, nil
	}
	info.Initialized = true
	info.Branch, _ = gitCommand(ctx, v.config.Directory, "symbolic-ref", "--short", "HEAD")
	if info.Branch == "" {
		info.Branch = "detached"
	}
	info.Remote, _ = gitCommand(ctx, v.config.Directory, "remote", "get-url", "origin")
	if ValidateRemote(info.Remote) != nil {
		info.Remote = ""
	}
	changes, e := gitCommand(ctx, v.config.Directory, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if e != nil {
		return info, e
	}
	for _, record := range strings.Split(changes, "\x00") {
		if len(record) > 3 && isStoreFile(record[3:]) {
			info.Changes++
		}
	}
	counts, e := gitCommand(ctx, v.config.Directory, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if e == nil {
		_, _ = fmt.Sscanf(counts, "%d %d", &info.Ahead, &info.Behind)
	}
	info.Conflict = pendingGitOperation(ctx, v.config.Directory)
	return info, nil
}

func (v *Vault) GitInitialize(ctx context.Context) error {
	v.gitOperations.Lock()
	defer v.gitOperations.Unlock()
	_, token, e := v.token()
	if e != nil {
		return e
	}
	unlock, e := lockStore(v.config.Directory)
	if e != nil {
		return e
	}
	defer unlock()
	if _, e = os.Lstat(filepath.Join(v.config.Directory, ".git")); e == nil {
		return ErrExists
	}
	if _, e = safeRead(v.config.Directory, ".gpg-id"); e != nil {
		return e
	}
	if e = v.validate(token); e != nil {
		return e
	}
	_, e = gitCommand(ctx, v.config.Directory, "init", "-b", "main")
	return e
}

func ValidateRemote(remote string) error {
	if remote == "" || strings.TrimSpace(remote) != remote || strings.HasPrefix(remote, "-") || strings.ContainsAny(remote, "\x00\r\n") || strings.Contains(remote, "::") {
		return errors.New("invalid Git remote")
	}
	if filepath.IsAbs(remote) {
		return nil
	}
	if strings.Contains(remote, "://") {
		u, e := url.Parse(remote)
		if e != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("invalid Git remote")
		}
		if u.Scheme != "https" && u.Scheme != "ssh" {
			return errors.New("use an HTTPS or SSH Git remote")
		}
		if u.User != nil {
			if _, ok := u.User.Password(); ok || u.Scheme != "ssh" {
				return errors.New("credentials cannot be embedded in a remote URL")
			}
		}
		return nil
	}
	// SCP-style SSH remote: git@example.org:team/password-store.git.
	host, destination, ok := strings.Cut(remote, ":")
	if !ok || host == "" || destination == "" || strings.ContainsAny(host, " /\\") {
		return errors.New("invalid Git remote")
	}
	return nil
}

func (v *Vault) GitSetRemote(ctx context.Context, remote string) error {
	if e := ValidateRemote(remote); e != nil {
		return e
	}
	v.gitOperations.Lock()
	defer v.gitOperations.Unlock()
	_, token, e := v.token()
	if e != nil {
		return e
	}
	unlock, e := lockStore(v.config.Directory)
	if e != nil {
		return e
	}
	defer unlock()
	if !managedStore(ctx, v.config.Directory) {
		return errors.New("initialize Git first")
	}
	if e = v.validate(token); e != nil {
		return e
	}
	_, e = gitCommand(ctx, v.config.Directory, "remote", "get-url", "origin")
	if e != nil {
		_, e = gitCommand(ctx, v.config.Directory, "remote", "add", "origin", remote)
	} else {
		_, e = gitCommand(ctx, v.config.Directory, "remote", "set-url", "origin", remote)
	}
	return e
}

func isStoreFile(name string) bool {
	if path.Base(name) == ".gpg-id" {
		dir := path.Dir(name)
		return dir == "." || ValidatePath(dir) == nil
	}
	return strings.HasSuffix(name, ".gpg") && ValidatePath(strings.TrimSuffix(name, ".gpg")) == nil
}

func (v *Vault) GitSync(ctx context.Context) (GitResult, error) {
	v.gitOperations.Lock()
	defer v.gitOperations.Unlock()
	_, token, e := v.token()
	if e != nil {
		return GitResult{}, e
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	v.session.Lock()
	if !v.checkSession() || token != v.generation {
		v.session.Unlock()
		return GitResult{}, ErrCancelled
	}
	v.gitCancel = cancel
	v.session.Unlock()
	defer func() { v.session.Lock(); v.gitCancel = nil; v.session.Unlock() }()
	unlock, e := lockStore(v.config.Directory)
	if e != nil {
		return GitResult{}, e
	}
	defer unlock()
	if !managedStore(ctx, v.config.Directory) {
		return GitResult{}, errors.New("initialize Git in the password store first")
	}
	if pendingGitOperation(ctx, v.config.Directory) {
		return GitResult{}, errors.New("finish the current Git merge or rebase before syncing")
	}
	staged, e := gitCommand(ctx, v.config.Directory, "diff", "--cached", "--name-only")
	if e != nil {
		return GitResult{}, e
	}
	if staged != "" {
		return GitResult{}, errors.New("Git has staged changes; commit or unstage them before syncing")
	}
	files, e := gitCommand(ctx, v.config.Directory, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if e != nil {
		return GitResult{}, e
	}
	args := []string{"add", "-A", "--"}
	seen := map[string]bool{}
	for _, file := range strings.Split(files, "\x00") {
		if !isStoreFile(file) || seen[file] {
			continue
		}
		seen[file] = true
		if _, e := safeRead(v.config.Directory, file); e != nil && !errors.Is(e, os.ErrNotExist) {
			return GitResult{}, fmt.Errorf("unsafe password-store file: %w", e)
		}
		args = append(args, file)
	}
	if e = v.validate(token); e != nil {
		return GitResult{}, e
	}
	if len(args) > 3 {
		if _, e = gitCommand(ctx, v.config.Directory, args...); e != nil {
			return GitResult{}, e
		}
	}
	staged, e = gitCommand(ctx, v.config.Directory, "diff", "--cached", "--name-only")
	if e != nil {
		return GitResult{}, e
	}
	if staged != "" {
		if _, e = gitCommand(ctx, v.config.Directory, "-c", "user.name=Passpine", "-c", "user.email=passpine@localhost", "commit", "-m", "Update password store"); e != nil {
			return GitResult{}, e
		}
	}
	remote, e := gitCommand(ctx, v.config.Directory, "remote", "get-url", "origin")
	if e != nil || remote == "" {
		if e = v.validate(token); e != nil {
			return GitResult{}, e
		}
		return GitResult{State: "local", Message: "Changes committed locally. Add a remote to synchronize."}, nil
	}
	if e = ValidateRemote(remote); e != nil {
		return GitResult{}, e
	}
	if e = v.validate(token); e != nil {
		return GitResult{}, e
	}
	upstream, upstreamErr := gitCommand(ctx, v.config.Directory, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if upstreamErr == nil && upstream != "" {
		if _, e = gitCommand(ctx, v.config.Directory, "pull", "--rebase"); e != nil {
			return GitResult{}, e
		}
	}
	if e = v.validate(token); e != nil {
		return GitResult{}, e
	}
	if upstreamErr != nil {
		_, e = gitCommand(ctx, v.config.Directory, "push", "--set-upstream", "origin", "HEAD")
	} else {
		_, e = gitCommand(ctx, v.config.Directory, "push")
	}
	if e != nil {
		return GitResult{}, e
	}
	if e = v.validate(token); e != nil {
		return GitResult{}, e
	}
	return GitResult{State: "synced", Message: "Encrypted password store synchronized."}, nil
}

// Clone creates a password-store checkout at a new destination. Keys are never
// read or exported by Git. A temporary checkout is inspected before installation.
func Clone(ctx context.Context, remote, destination string) error {
	if e := ValidateRemote(remote); e != nil {
		return e
	}
	absolute, e := filepath.Abs(destination)
	if e != nil {
		return e
	}
	if entries, e := os.ReadDir(absolute); e == nil && len(entries) > 0 {
		return ErrExists
	} else if e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	parent, e := privateDirectory(filepath.Dir(absolute))
	if e != nil {
		return e
	}
	temporary, e := os.MkdirTemp(parent, ".passpine-clone-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temporary)
	checkout := filepath.Join(temporary, "store")
	if _, e = gitCommand(ctx, parent, "clone", "--no-recurse-submodules", "--", remote, checkout); e != nil {
		return e
	}
	if _, e = safeRead(checkout, ".gpg-id"); e != nil {
		return errors.New("remote is not a pass-compatible password store")
	}
	if e = filepath.WalkDir(checkout, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Name() == ".git" && d.IsDir() {
			return filepath.SkipDir
		}
		if d.Type()&os.ModeSymlink != 0 {
			return ErrPath
		}
		return nil
	}); e != nil {
		return e
	}
	// The native rename replaces an empty directory atomically, but refuses to
	// overwrite a file or populated directory created during the download.
	// os.Rename adds a precheck that rejects even an empty destination directory.
	if e = syscall.Rename(checkout, absolute); e != nil {
		return &os.LinkError{Op: "rename", Old: checkout, New: absolute, Err: e}
	}
	return nil
}
