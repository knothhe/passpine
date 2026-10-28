package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// BrowserAssociationScope identifies clones without exposing repository URLs,
// credentials, recipients or local paths to the extension. Local-only stores
// intentionally do not share an identity merely because they use the same key.
func (v *Vault) BrowserAssociationScope(localID string) (string, error) {
	v.operations.Lock()
	defer v.operations.Unlock()
	names, err := v.recipients("")
	if err != nil {
		return "", err
	}
	slices.Sort(names)
	names = slices.Compact(names)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	remote := ""
	gitInfo, gitErr := os.Lstat(filepath.Join(v.config.Directory, ".git"))
	if gitErr != nil && !os.IsNotExist(gitErr) {
		return "", gitErr
	}
	if gitErr == nil {
		// Missing .git identifies a local store even inside a parent checkout.
		// Existing but unreadable/broken Git state must fail instead of silently
		// changing a previously shared vault to a new local identity.
		if gitInfo.Mode()&os.ModeSymlink != 0 || (!gitInfo.IsDir() && !gitInfo.Mode().IsRegular()) || !managedStore(ctx, v.config.Directory) {
			return "", errors.New("cannot determine password-store repository identity")
		}
		remotes, err := gitCommand(ctx, v.config.Directory, "remote")
		if err != nil {
			return "", err
		}
		if slices.Contains(strings.Split(remotes, "\n"), "origin") {
			remote, err = gitCommand(ctx, v.config.Directory, "remote", "get-url", "origin")
			if err != nil {
				return "", err
			}
		}
	}
	identity := []any{"passpine-browser-associations-v1", "local", localID, names}
	if canonical := canonicalAssociationRemote(remote); canonical != "" {
		identity = []any{"passpine-browser-associations-v1", "git", canonical, names}
	}
	encoded, _ := json.Marshal(identity)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalAssociationRemote(remote string) string {
	if remote == "" || strings.ContainsAny(remote, "\x00\r\n\\") {
		return ""
	}
	relative := false
	if !strings.Contains(remote, "://") {
		separator := strings.IndexByte(remote, ':')
		if opening := strings.IndexByte(remote, '['); opening >= 0 && opening < separator {
			closing := strings.IndexByte(remote[opening:], ']')
			if closing < 0 {
				return ""
			}
			separator = opening + closing + 1
		}
		if separator < 1 || separator >= len(remote) || remote[separator] != ':' {
			return ""
		}
		authority, target := remote[:separator], remote[separator+1:]
		if strings.ContainsAny(authority, "/ \t") || len(authority) == 1 || target == "" {
			return ""
		}
		relative = !strings.HasPrefix(target, "/")
		remote = "ssh://" + authority + "/" + strings.TrimPrefix(target, "/")
	}
	u, err := url.Parse(remote)
	if err != nil || u.Hostname() == "" || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "ssh") {
		return ""
	}
	hostname := strings.ToLower(u.Hostname())
	host := hostname
	port := u.Port()
	if port != "" && !(u.Scheme == "https" && port == "443") && !(u.Scheme == "ssh" && port == "22") {
		host = net.JoinHostPort(host, port)
	} else if strings.ContainsRune(host, ':') {
		host = "[" + host + "]"
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return ""
		}
	}
	target := strings.Trim(u.Path, "/")
	if target == "" {
		return ""
	}
	username := ""
	if u.User != nil {
		username = u.User.Username()
	}
	// These hosted services use a fixed SSH user and a shared repository path
	// namespace. Generic SSH servers can resolve paths relative to each user's
	// home; dropping the user or treating scp paths as absolute mixes vaults.
	hosted := hostname == "github.com" || hostname == "gitlab.com"
	standardPort := port == "" || u.Scheme == "https" && port == "443" || u.Scheme == "ssh" && port == "22"
	if hosted && standardPort && (u.Scheme == "https" || username == "git") {
		target = strings.TrimSuffix(target, ".git")
		if target == "" {
			return ""
		}
		return hostname + "/" + target
	}
	if u.Scheme == "https" {
		// On custom HTTP servers, repo and repo.git can be different resources.
		return "https://" + host + u.Path
	}
	if username == "" || strings.ContainsAny(username, "\x00\r\n\t") {
		// The implicit SSH user depends on this machine's login/configuration.
		return ""
	}
	pathKind := "absolute"
	if relative {
		pathKind = "home"
	}
	encoded, _ := json.Marshal([]string{"ssh", host, username, pathKind, u.Path})
	return string(encoded)
}
