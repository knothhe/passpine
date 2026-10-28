package vault

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGitFetchAndPushPreserveConcurrentSavesAndDeletes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture requires a Unix executable shell wrapper")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	v := newFixture(t)
	t.Cleanup(v.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := func(directory string, args ...string) string {
		t.Helper()
		out, err := gitCommand(ctx, directory, args...)
		if err != nil {
			t.Fatalf("git %s: %v", args[0], err)
		}
		return out
	}
	remote := filepath.Join(t.TempDir(), "remote.git")
	command(v.config.Directory, "init", "--bare", "--initial-branch=main", remote)
	if err = v.GitInitialize(ctx); err != nil {
		t.Fatal(err)
	}
	original := mustSave(t, v, "entry", "original", "")
	deleted := mustSave(t, v, "delete-me", "delete this", "")
	if err = v.GitSetRemote(ctx, remote); err != nil {
		t.Fatal(err)
	}
	if _, err = v.GitSync(ctx); err != nil {
		t.Fatal(err)
	}

	// A peer adds a valid encrypted entry using the same vault recipient.
	remoteDocument := mustSave(t, v, "remote-entry", "from remote", "")
	ciphertext, err := os.ReadFile(filepath.Join(v.config.Directory, "remote-entry.gpg"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = v.Trash(remoteDocument.Path, remoteDocument.Revision); err != nil {
		t.Fatal(err)
	}
	peer := filepath.Join(t.TempDir(), "peer")
	command(v.config.Directory, "clone", remote, peer)
	if err = os.WriteFile(filepath.Join(peer, "remote-entry.gpg"), ciphertext, 0600); err != nil {
		t.Fatal(err)
	}
	command(peer, "add", "--", "remote-entry.gpg")
	command(peer, "-c", "user.name=Peer", "-c", "user.email=peer@localhost", "commit", "-m", "Add remote entry")
	command(peer, "push")

	wrapperDirectory := t.TempDir()
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	marker := func(phase string) string { return filepath.Join(wrapperDirectory, phase+"-started") }
	release := func(phase string) string { return filepath.Join(wrapperDirectory, phase+"-release") }
	script := "#!/bin/sh\nfor arg in \"$@\"; do\n case \"$arg\" in\n fetch|push)\n  touch " + quote(wrapperDirectory) + "/\"$arg\"-started\n  while [ ! -e " + quote(wrapperDirectory) + "/\"$arg\"-release ]; do sleep 0.01; done\n  ;;\n esac\ndone\nexec " + quote(git) + " \"$@\"\n"
	if err = os.WriteFile(filepath.Join(wrapperDirectory, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", wrapperDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	unblock := func(phase string) {
		t.Helper()
		if err := os.WriteFile(release(phase), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = os.WriteFile(release("fetch"), nil, 0600)
		_ = os.WriteFile(release("push"), nil, 0600)
	})
	await := func(phase string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(marker(phase)); err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("sync never reached %s", phase)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	type outcome struct {
		result GitResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := v.GitSync(ctx)
		done <- outcome{result, err}
	}()
	await("fetch")
	mustSave(t, v, "during-fetch", "saved during fetch", "")
	mustSave(t, v, "entry", "edited during fetch", original.Revision)
	if _, err = v.Trash(deleted.Path, deleted.Revision); err != nil {
		t.Fatal("fetch blocked deletion", err)
	}
	unblock("fetch")
	await("push")
	mustSave(t, v, "during-push", "saved during push", "")
	unblock("push")
	select {
	case got := <-done:
		if got.err != nil || !got.result.Changed {
			t.Fatal("remote change was not integrated", got)
		}
	case <-ctx.Done():
		t.Fatal("sync did not finish")
	}
	for name, want := range map[string]string{"entry": "edited during fetch", "during-fetch": "saved during fetch", "during-push": "saved during push", "remote-entry": "from remote"} {
		if doc, err := v.Read(name); err != nil || doc.Raw != want {
			t.Fatalf("%s was lost or overwritten: %v", name, err)
		}
	}
	tracked := command(remote, "ls-tree", "-r", "--name-only", "main")
	if !strings.Contains(tracked, "during-fetch.gpg") || strings.Contains(tracked, "delete-me.gpg") || strings.Contains(tracked, "during-push.gpg") {
		t.Fatalf("unexpected first push: %s", tracked)
	}
	result, err := v.GitSync(ctx)
	if err != nil || result.Changed {
		t.Fatal("follow-up local-only edit changed remote working-tree state", result, err)
	}
	if tracked = command(remote, "ls-tree", "-r", "--name-only", "main"); !strings.Contains(tracked, "during-push.gpg") {
		t.Fatal("follow-up sync lost the edit made during push")
	}
}

func TestGitSyncUsesQualifiedUpstreamRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	for _, scenario := range []string{"leading-dash-remote", "ambiguous-tag"} {
		t.Run(scenario, func(t *testing.T) {
			v := newFixture(t)
			t.Cleanup(v.Close)
			// This verifies ref selection, not whole-fixture latency. Each sync
			// starts many native Git processes; a shared short deadline also
			// charged repository setup and encryption against later commands on
			// busy Windows race runners. Bound each operation independently and
			// report its phase so a real stalled command remains diagnosable.
			operation := func(name string, run func(context.Context) error) {
				t.Helper()
				ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
				defer cancel()
				started := time.Now()
				if err := run(ctx); err != nil {
					t.Fatalf("%s after %s: %v", name, time.Since(started).Round(time.Millisecond), err)
				}
				t.Logf("%s completed in %s", name, time.Since(started).Round(time.Millisecond))
			}
			command := func(directory string, args ...string) {
				t.Helper()
				operation("git "+strings.Join(args, " "), func(ctx context.Context) error {
					_, err := gitCommand(ctx, directory, args...)
					return err
				})
			}
			remote := filepath.Join(t.TempDir(), "remote.git")
			command(v.config.Directory, "init", "--bare", "--initial-branch=main", remote)
			operation("initialize local store", v.GitInitialize)
			mustSave(t, v, "entry", "original", "")
			operation("set local remote", func(ctx context.Context) error { return v.GitSetRemote(ctx, remote) })
			operation("initial sync", func(ctx context.Context) error {
				_, err := v.GitSync(ctx)
				return err
			})
			if scenario == "leading-dash-remote" {
				command(v.config.Directory, "remote", "add", "--", "-peer", remote)
				command(v.config.Directory, "fetch", "--", "-peer")
				command(v.config.Directory, "branch", "--set-upstream-to=-peer/main")
			} else {
				command(v.config.Directory, "-c", "tag.gpgSign=false", "tag", "origin/main")
				command(v.config.Directory, "config", "core.warnAmbiguousRefs", "false")
			}
			peer := filepath.Join(t.TempDir(), "peer")
			command(v.config.Directory, "clone", remote, peer)
			ciphertext := []byte("remote encrypted fixture")
			if err := os.WriteFile(filepath.Join(peer, "remote-entry.gpg"), ciphertext, 0600); err != nil {
				t.Fatal(err)
			}
			command(peer, "add", "remote-entry.gpg")
			command(peer, "-c", "user.name=Peer", "-c", "user.email=peer@localhost", "commit", "-m", "Add remote entry")
			command(peer, "push")
			var result GitResult
			operation("sync tracked remote branch", func(ctx context.Context) error {
				var err error
				result, err = v.GitSync(ctx)
				return err
			})
			if !result.Changed {
				t.Fatal("sync did not integrate the tracked remote branch", result)
			}
			if got, err := os.ReadFile(filepath.Join(v.config.Directory, "remote-entry.gpg")); err != nil || string(got) != string(ciphertext) {
				t.Fatal("tracked remote entry was not integrated", err)
			}
		})
	}
}
