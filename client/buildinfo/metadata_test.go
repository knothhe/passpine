package buildinfo

import "testing"

func TestBuildMetadataUsesInjectedIdentityAndDoesNotInferReleaseFromVersion(t *testing.T) {
	priorCommit, priorModified, priorExecutor := Commit, Modified, Executor
	t.Cleanup(func() { Commit, Modified, Executor = priorCommit, priorModified, priorExecutor })
	Commit, Modified, Executor = "abc123", "true", "github-actions"
	testChannel(t, "dev")
	m := Current("1.2.3")
	if m.Channel != "dev" || m.Version != "1.2.3-dev" || m.Commit != "abc123" || m.Modified != "true" || m.Executor != "github-actions" || m.OS == "" || m.Arch == "" {
		t.Fatalf("wrong metadata: %#v", m)
	}
}
