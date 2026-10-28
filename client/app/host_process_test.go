package app

import (
	"encoding/json"
	"github.com/knothhe/passpine/client/buildinfo"
	"github.com/knothhe/passpine/client/nativehost"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// This test uses the shipped helper executable and its real stdin/stdout pipes.
// scripts/check.sh sets the path after building it; no browser is launched.
func TestNativeHelperProcess(t *testing.T) {
	binary := os.Getenv("PASSPINE_TEST_HOST")
	if binary == "" {
		t.Skip("set PASSPINE_TEST_HOST to the built passpine-host binary")
	}
	s := testService(t)
	state := must(t, s, Request{Action: "status"})
	saved := must(t, s, Request{Action: "save", Entry: "example.com/login", Contents: "fixture-password\nusername: test-user\n", Mode: "create"})
	cmd := exec.Command(binary, "chrome-extension://"+nativehost.ChromiumID+"/")
	cmd.Env = append(os.Environ(), buildinfo.HomeEnvironment()+"="+s.directory)
	stdin, e := cmd.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { stdin.Close(); _ = cmd.Wait() })
	roundtrip := func(r Request) Response {
		t.Helper()
		r.Protocol = 1
		r.RequestID = "fixture-request"
		b, _ := json.Marshal(r)
		if e = nativehost.WriteFrame(stdin, b); e != nil {
			t.Fatal(e)
		}
		b, e = nativehost.ReadFrame(stdout)
		if e != nil {
			t.Fatal(e)
		}
		var response Response
		if e = json.Unmarshal(b, &response); e != nil {
			t.Fatal(e)
		}
		if response.RequestID != r.RequestID {
			t.Fatalf("lost response correlation: %s", b)
		}
		return response
	}
	if r := roundtrip(Request{Action: "configure"}); r.Status != "ok" || r.Data.VaultID != state.VaultID || !slices.Contains(r.Data.Capabilities, "browser-associations-v1") || !slices.Contains(r.Data.Capabilities, "browser-association-operations-v1") {
		t.Fatal(r)
	}
	identity := roundtrip(Request{Action: "browser-associations", VaultID: state.VaultID})
	if identity.Status != "ok" || identity.Data.AssociationScope == "" || identity.Data.Associations == nil || len(*identity.Data.Associations) != 0 {
		t.Fatalf("empty association store did not survive native framing: %+v", identity)
	}
	key := strings.Repeat("a", 64)
	patch := map[string]bool{key: false}
	operation := Request{Action: "browser-associations", VaultID: state.VaultID, AssociationScope: identity.Data.AssociationScope, AssociationStoreID: identity.Data.AssociationStoreID, AssociationOperationID: strings.Repeat("b", 64), AssociationPatch: &patch}
	linked := roundtrip(operation)
	if linked.Status != "ok" || linked.Data.Associations == nil {
		t.Fatalf("association patch failed through native helper: %+v", linked)
	}
	if value, present := (*linked.Data.Associations)[key]; !present || value {
		t.Fatal("native helper lost an explicit unlink")
	}
	newer := map[string]bool{key: true}
	if r := roundtrip(Request{Action: "browser-associations", VaultID: state.VaultID, AssociationScope: identity.Data.AssociationScope, AssociationStoreID: identity.Data.AssociationStoreID, AssociationPatch: &newer}); r.Status != "ok" {
		t.Fatal("newer association failed through native helper", r.Code)
	}
	if r := roundtrip(operation); r.Status != "ok" || r.Data.Associations == nil || !(*r.Data.Associations)[key] {
		t.Fatal("native helper replayed an acknowledged operation over a newer association")
	}
	if r := roundtrip(Request{Action: "fetch", VaultID: state.VaultID, Entry: saved.Entry}); r.Status != "ok" || r.Data.Password != "fixture-password" {
		t.Fatal(r)
	}
	if r := roundtrip(Request{Action: "fetch", Entry: saved.Entry}); r.Code != "vault_changed" {
		t.Fatal(r)
	}
	must(t, s, Request{Action: "lock"})
	if r := roundtrip(Request{Action: "fetch", VaultID: state.VaultID, Entry: saved.Entry}); r.Code != "locked" || r.Data.Password != "" {
		t.Fatal(r)
	}
	must(t, s, Request{Action: "unlock", Passphrase: "test-passphrase-123"})
	if r := roundtrip(Request{Action: "fetch", VaultID: state.VaultID, Entry: saved.Entry}); r.Status != "ok" {
		t.Fatal(r)
	}
}
