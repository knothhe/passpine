package app

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/knothhe/passpine/client/cloudsync"
)

func associationPatchFixture(t *testing.T, s *Service, data Data, values map[string]bool) Data {
	t.Helper()
	r := associationNative(t, s, Request{Action: "browser-associations", VaultID: s.prefs.Current, AssociationScope: data.AssociationScope, AssociationPatch: &values})
	if r.Status != "ok" {
		t.Fatalf("association patch: %s", r.Code)
	}
	return r.Data
}

func TestBrowserAssociationsCloudFirstPullManualPauseAndBrowserSelection(t *testing.T) {
	s, _, engine, config, secrets, bucket := newSyncAppSpace(t)
	associationFixtureVault(t, s, "")
	initial := associationRead(t, s)
	hash := strings.Repeat("a", 64)
	key := associationValuePrefix + initial.AssociationScope + "." + hash
	alias := associationAliasPrefix + initial.AssociationScope + "." + strings.Repeat("b", 32)
	associationPatchFixture(t, s, initial, map[string]bool{hash: false})
	if _, err := engine.Sync(t.Context(), config, secrets, syncTestValues(key, `true`, alias, `true`), nil); err != nil {
		t.Fatal(err)
	}
	options := DefaultConfigSyncOptions()
	options.Mode = "manual"
	options.Browser = false
	must(t, s, Request{Action: "config-sync-options", SyncOptions: &options})
	saveSyncFixture(t, s, config, secrets)
	if (*associationRead(t, s).Associations)[hash] {
		t.Fatal("disabled browser imported remote associations")
	}
	cloud, err := engine.Read(t.Context(), config, secrets)
	if err != nil || string(cloud.Values[key]) != "true" {
		t.Fatal("disabled browser overwrote remote association", err)
	}
	options.Browser = true
	options.BrowserFields = SyncFieldSelection{}
	must(t, s, Request{Action: "config-sync-options", SyncOptions: &options})
	must(t, s, Request{Action: "config-sync-now"})
	pulled := associationRead(t, s)
	if !(*pulled.Associations)[hash] || len(pulled.AssociationVaultIDs) != 2 || pulled.AssociationStoreID != initial.AssociationStoreID {
		t.Fatal("first association pull or alias import failed")
	}
	_, _, before := bucket.snapshot()
	associationPatchFixture(t, s, pulled, map[string]bool{hash: false})
	_, _, after := bucket.snapshot()
	if after != before {
		t.Fatal("manual local edit contacted cloud")
	}
	must(t, s, Request{Action: "config-sync-pause"})
	associationPatchFixture(t, s, pulled, map[string]bool{strings.Repeat("c", 64): true})
	if len(*associationRead(t, s).Associations) != 2 {
		t.Fatal("paused cloud prevented local saving")
	}
	_, _, after = bucket.snapshot()
	if after != before {
		t.Fatal("paused local edit contacted cloud")
	}
	must(t, s, Request{Action: "config-sync-resume"})
	must(t, s, Request{Action: "config-sync-now"})
	cloud, err = engine.Read(t.Context(), config, secrets)
	if err != nil || string(cloud.Values[key]) != "false" {
		t.Fatal("explicit cancellation was not uploaded", err)
	}
	for _, raw := range cloud.Values {
		if strings.Contains(string(raw), initial.AssociationStoreID) {
			t.Fatal("machine store identity uploaded")
		}
	}
	if s.entriesVersion != 0 {
		t.Fatal("cloud associations reset entry editor")
	}
}

func TestBrowserAssociationsCloudMergeAcrossClonePathsAndRetainInflightEdits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	s, _, engine, config, secrets, bucket := newSyncAppSpace(t)
	peer := newSyncServiceFixture(t, "", &syncMemorySecrets{}, engine)
	for _, service := range []*Service{s, peer} {
		dir := associationFixtureVault(t, service, "")
		for _, args := range [][]string{{"init"}, {"remote", "add", "origin", "https://git.example.invalid/team/fixture.git"}} {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git fixture: %s %v", out, err)
			}
		}
	}
	first, second := associationRead(t, s), associationRead(t, peer)
	if first.AssociationScope != second.AssociationScope || first.AssociationStoreID == second.AssociationStoreID {
		t.Fatal("clone or store identity is incorrect")
	}
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	associationPatchFixture(t, s, first, map[string]bool{a: true})
	saveSyncFixture(t, s, config, secrets)
	saveSyncFixture(t, peer, config, secrets)
	associationPatchFixture(t, s, first, map[string]bool{a: false})
	associationPatchFixture(t, peer, second, map[string]bool{b: true})
	must(t, s, Request{Action: "config-sync-now"})
	must(t, peer, Request{Action: "config-sync-now"})
	must(t, s, Request{Action: "config-sync-now"})
	merged := associationRead(t, s)
	if (*merged.Associations)[a] || !(*merged.Associations)[b] || len(merged.AssociationVaultIDs) != 2 {
		t.Fatal("independent association edits or clone aliases lost")
	}
	base, err := engine.Read(t.Context(), config, secrets)
	if err != nil {
		t.Fatal(err)
	}
	remote := copySyncValues(base.Values)
	key := associationValuePrefix + first.AssociationScope + "." + a
	remote[key] = json.RawMessage(`true`)
	if _, err = engine.Sync(t.Context(), config, secrets, remote, base.Values); err != nil {
		t.Fatal(err)
	}
	bucket.mu.Lock()
	bucket.getHook = func() { associationPatchFixture(t, s, first, map[string]bool{a: true, strings.Repeat("c", 64): true}) }
	bucket.mu.Unlock()
	must(t, s, Request{Action: "config-sync-now"})
	inflight := associationRead(t, s)
	if !(*inflight.Associations)[a] || !(*inflight.Associations)[strings.Repeat("c", 64)] {
		t.Fatal("network response overwrote an in-flight local association edit")
	}
}

func TestBrowserAssociationsResavingSameSyncSpaceRetainsOfflineEdit(t *testing.T) {
	s, _, engine, config, secrets, _ := newSyncAppSpace(t)
	associationFixtureVault(t, s, "")
	initial := associationRead(t, s)
	hash := strings.Repeat("a", 64)
	associationPatchFixture(t, s, initial, map[string]bool{hash: true})
	saveSyncFixture(t, s, config, secrets)
	associationPatchFixture(t, s, initial, map[string]bool{hash: false})
	saveSyncFixture(t, s, config, secrets)
	key := associationValuePrefix + initial.AssociationScope + "." + hash
	remote, err := engine.Read(t.Context(), config, secrets)
	if err != nil || string(remote.Values[key]) != "false" || (*associationRead(t, s).Associations)[hash] {
		t.Fatal("same-space reconnect replaced offline cancellation with old remote value", err)
	}
}

func TestBrowserAssociationsRejectInvalidCloudRecordsAndLimits(t *testing.T) {
	scope, hash := strings.Repeat("a", 64), strings.Repeat("b", 64)
	key := associationValuePrefix + scope + "." + hash
	for _, values := range []cloudsync.Values{
		syncTestValues(key, `null`), syncTestValues(key, `"true"`), syncTestValues(associationValuePrefix+"bad", `true`), syncTestValues(associationAliasPrefix+scope+"."+strings.Repeat("a", 32), `false`),
	} {
		if validateSyncedValues(values) == nil {
			t.Fatal("invalid association cloud record was accepted", values)
		}
	}
	values := cloudsync.Values{}
	for i := 0; i <= maxAssociationValues; i++ {
		values[associationValuePrefix+scope+"."+fmt.Sprintf("%064x", i)] = json.RawMessage(`true`)
	}
	if validateSyncedValues(values) == nil {
		t.Fatal("association cloud record limit was ignored")
	}
}

func TestBrowserAssociationsDisabledSyncRetainsDeletedRemoteBaseline(t *testing.T) {
	s, _, engine, config, secrets, _ := newSyncAppSpace(t)
	associationFixtureVault(t, s, "")
	initial := associationRead(t, s)
	hash := strings.Repeat("a", 64)
	associationPatchFixture(t, s, initial, map[string]bool{hash: true})
	saveSyncFixture(t, s, config, secrets)
	options := DefaultConfigSyncOptions()
	options.Browser = false
	must(t, s, Request{Action: "config-sync-options", SyncOptions: &options})
	base, err := engine.Read(t.Context(), config, secrets)
	if err != nil {
		t.Fatal(err)
	}
	key := associationValuePrefix + initial.AssociationScope + "." + hash
	remote := copySyncValues(base.Values)
	delete(remote, key)
	if _, err = engine.Sync(t.Context(), config, secrets, remote, base.Values); err != nil {
		t.Fatal(err)
	}
	must(t, s, Request{Action: "config-sync-now"})
	if !(*associationRead(t, s).Associations)[hash] {
		t.Fatal("disabled synchronization applied remote deletion")
	}
	options.Browser = true
	must(t, s, Request{Action: "config-sync-options", SyncOptions: &options})
	must(t, s, Request{Action: "config-sync-now"})
	if _, ok := (*associationRead(t, s).Associations)[hash]; ok {
		t.Fatal("reenabling restored deleted remote association")
	}
	cloud, err := engine.Read(t.Context(), config, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cloud.Values[key]; ok {
		t.Fatal("deleted remote association resurrected")
	}
}
