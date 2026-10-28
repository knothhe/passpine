package vault

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBrowserAssociationScopeStableAcrossClonePathsAndTransport(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	var first string
	for i, origin := range []string{"https://user:private@github.com:443/Team/vault.git", "git@github.com:Team/vault.git", "ssh://git@github.com:22/Team/vault"} {
		dir := t.TempDir()
		for _, args := range [][]string{{"init"}, {"remote", "add", "origin", origin}} {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git fixture: %s %v", out, err)
			}
		}
		policy := "B\nA\nA # duplicate\n"
		if i == 1 {
			policy = "A\nB\n"
		}
		if err := os.WriteFile(filepath.Join(dir, ".gpg-id"), []byte(policy), 0600); err != nil {
			t.Fatal(err)
		}
		v, err := New(Config{Directory: dir, KeyFile: filepath.Join(t.TempDir(), "key")})
		if err != nil {
			t.Fatal(err)
		}
		scope, err := v.BrowserAssociationScope(string(rune('a' + i)))
		if err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = scope
		} else if scope != first {
			t.Fatal("clone identity depends on local path, credentials or transport")
		}
		if err := os.WriteFile(filepath.Join(dir, ".gpg-id"), []byte("C\n"), 0600); err != nil {
			t.Fatal(err)
		}
		changed, err := v.BrowserAssociationScope("a")
		if err != nil || changed == first {
			t.Fatal("recipient policy change retained scope")
		}
		v.Close()
	}
}

func TestBrowserAssociationRemotePreservesCustomRepositoryIdentity(t *testing.T) {
	for _, pair := range [][2]string{
		{"alice@selfhost.example:password-store.git", "bob@selfhost.example:password-store.git"},
		{"alice@selfhost.example:password-store.git", "ssh://alice@selfhost.example/password-store.git"},
		{"alice@selfhost.example:password-store.git", "alice@selfhost.example:/password-store.git"},
		{"alice@selfhost.example:password-store.git", "alice@selfhost.example:password-store"},
		{"https://selfhost.example/password-store.git", "https://selfhost.example/password-store"},
		{"git@github.com:Team/vault.git", "ssh://other@github.com/Team/vault.git"},
		{"git@github.com:Team/vault.git", "ssh://git@github.com:2222/Team/vault.git"},
		{"alice@[2001:db8::1]:password-store.git", "bob@[2001:db8::1]:password-store.git"},
	} {
		first, second := canonicalAssociationRemote(pair[0]), canonicalAssociationRemote(pair[1])
		if first == "" || second == "" || first == second {
			t.Fatalf("different repositories share identity: %q => %q, %q => %q", pair[0], first, pair[1], second)
		}
	}
	for _, pair := range [][2]string{
		{"git@gitlab.com:Team/vault.git", "https://token@gitlab.com/Team/vault"},
		{"alice@selfhost.example:/password-store.git", "ssh://alice@selfhost.example:22/password-store.git"},
		{"https://alice:secret@selfhost.example:443/password-store.git", "https://bob:other@selfhost.example/password-store.git"},
	} {
		first, second := canonicalAssociationRemote(pair[0]), canonicalAssociationRemote(pair[1])
		if first == "" || first != second {
			t.Fatalf("equivalent repositories have different identity: %q => %q, %q => %q", pair[0], first, pair[1], second)
		}
	}
	for _, remote := range []string{"selfhost.example:password-store.git", "ssh://selfhost.example/password-store.git"} {
		if canonicalAssociationRemote(remote) != "" {
			t.Fatalf("implicit SSH user acquired a shared repository identity: %q", remote)
		}
	}
}

func TestBrowserAssociationScopeSeparatesCustomSSHUsers(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	var first string
	for _, user := range []string{"alice", "bob"} {
		dir := t.TempDir()
		for _, args := range [][]string{{"init"}, {"remote", "add", "origin", user + "@selfhost.example:password-store.git"}} {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git fixture: %s %v", out, err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, ".gpg-id"), []byte("same-recipient\n"), 0600); err != nil {
			t.Fatal(err)
		}
		v, err := New(Config{Directory: dir, KeyFile: filepath.Join(t.TempDir(), "key")})
		if err != nil {
			t.Fatal(err)
		}
		scope, err := v.BrowserAssociationScope(user)
		v.Close()
		if err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = scope
		} else if first == scope {
			t.Fatal("separate SSH users' home repositories share website associations")
		}
	}
}

func TestBrowserAssociationScopeSeparatesLocalVaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gpg-id"), []byte("same-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err := New(Config{Directory: dir, KeyFile: filepath.Join(t.TempDir(), "key")})
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	a, err := v.BrowserAssociationScope("local-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := v.BrowserAssociationScope("local-b")
	if err != nil || a == b {
		t.Fatal("same-key local vaults shared a scope")
	}
	for _, remote := range []string{"/tmp/vault.git", "file:///tmp/vault.git", "https://host/repo/../other.git", "https://host/%2e%2e/other.git"} {
		if canonicalAssociationRemote(remote) != "" {
			t.Fatal("unsafe origin accepted", remote)
		}
	}
}

func TestBrowserAssociationScopeDoesNotInheritParentRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	parent := t.TempDir()
	for _, args := range [][]string{{"init"}, {"remote", "add", "origin", "https://git.example.invalid/parent.git"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = parent
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %s %v", out, err)
		}
	}
	var first string
	for _, name := range []string{"one", "two"} {
		dir := filepath.Join(parent, name)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".gpg-id"), []byte("shared-recipient\n"), 0600); err != nil {
			t.Fatal(err)
		}
		v, err := New(Config{Directory: dir, KeyFile: filepath.Join(t.TempDir(), "key")})
		if err != nil {
			t.Fatal(err)
		}
		scope, err := v.BrowserAssociationScope(name)
		if err != nil {
			t.Fatal(err)
		}
		v.Close()
		if first == "" {
			first = scope
		} else if scope == first {
			t.Fatal("child password stores inherited parent repository scope")
		}
	}
}

func TestBrowserAssociationScopeRejectsBrokenGitIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gpg-id"), []byte("fixture-recipient\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: missing-repository\n"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err := New(Config{Directory: dir, KeyFile: filepath.Join(t.TempDir(), "key")})
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if scope, err := v.BrowserAssociationScope("local-id"); err == nil || scope != "" {
		t.Fatal("broken repository silently fell back to local identity")
	}
}
