package cloudsync

import (
	"context"
	"reflect"
	"testing"
)

func TestSyncPolicyPreservesExcludedRemoteValuesAndInputs(t *testing.T) {
	engine, config, secrets, _ := newTestSpace(t)
	remote := values("desktop.theme", `"dark"`, "browser.theme", `"light"`, "future.feature", `true`)
	if _, err := engine.Sync(context.Background(), config, secrets, remote, nil); err != nil {
		t.Fatal(err)
	}
	baseline := values("desktop.theme", `"system"`, "browser.theme", `"system"`)
	local := values("desktop.theme", `"light"`, "browser.theme", `"dark"`)
	originalLocal, originalBaseline := cloneValues(local), cloneValues(baseline)
	result, err := engine.SyncValidatedWithPolicy(context.Background(), config, secrets, local, baseline, nil, MergePolicy{IgnoredKeys: []string{"desktop.theme", "browser.theme"}})
	if err != nil || !equalValues(result.Values, remote) || result.Uploaded {
		t.Fatalf("excluded values changed: %+v %v", result, err)
	}
	if !reflect.DeepEqual(local, originalLocal) || !reflect.DeepEqual(baseline, originalBaseline) {
		t.Fatal("policy changed its input maps")
	}
}

func TestSyncPolicyDoesNotRecreateExcludedValues(t *testing.T) {
	for _, missingObject := range []bool{false, true} {
		t.Run(map[bool]string{false: "deleted field", true: "deleted object"}[missingObject], func(t *testing.T) {
			engine, config, secrets, _ := newTestSpace(t)
			if !missingObject {
				if _, err := engine.Sync(context.Background(), config, secrets, values("future.feature", `true`), nil); err != nil {
					t.Fatal(err)
				}
			}
			local := values("desktop.theme", `"dark"`, "browser.theme", `"light"`)
			baseline := values("desktop.theme", `"system"`, "browser.theme", `"light"`)
			result, err := engine.SyncValidatedWithPolicy(context.Background(), config, secrets, local, baseline, nil, MergePolicy{IgnoredKeys: []string{"desktop.theme"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := result.Values["desktop.theme"]; ok {
				t.Fatalf("excluded value recreated: %v", result.Values)
			}
		})
	}
}

func TestSyncPolicyImportsInitialKeysAndUploadsMissingKeys(t *testing.T) {
	engine, config, secrets, _ := newTestSpace(t)
	if _, err := engine.Sync(context.Background(), config, secrets, values("desktop.theme", `"dark"`, "browser.theme", `"light"`), nil); err != nil {
		t.Fatal(err)
	}
	baseline := values("browser.theme", `"light"`)
	local := values("desktop.theme", `"system"`, "desktop.language", `"en"`, "browser.theme", `"dark"`)
	result, err := engine.SyncValidatedWithPolicy(context.Background(), config, secrets, local, baseline, nil, MergePolicy{InitialKeys: []string{"desktop.theme", "desktop.language"}})
	want := values("desktop.theme", `"dark"`, "desktop.language", `"en"`, "browser.theme", `"dark"`)
	if err != nil || !equalValues(result.Values, want) {
		t.Fatalf("initial namespace merge: %+v %v", result, err)
	}
}

func TestSyncPolicyReevaluatesRacingRemoteRevision(t *testing.T) {
	engine, config, secrets, bucket := newTestSpace(t)
	remote := values("desktop.theme", `"dark"`, "desktop.language", `"en"`, "browser.theme", `"system"`)
	if _, err := engine.Sync(context.Background(), config, secrets, remote, nil); err != nil {
		t.Fatal(err)
	}
	identity := config.Endpoint + "\n" + config.Bucket + "\n" + config.Prefix + "/" + objectName
	racing, err := seal(values("desktop.theme", `"light"`, "desktop.language", `"zh-CN"`, "browser.theme", `"system"`), secrets.Passphrase, identity)
	if err != nil {
		t.Fatal(err)
	}
	bucket.mu.Lock()
	bucket.beforePut = func() { bucket.body = racing; bucket.version++ }
	bucket.mu.Unlock()
	local := values("desktop.theme", `"system"`, "desktop.language", `"en"`, "browser.theme", `"dark"`)
	baseline := values("desktop.theme", `"system"`, "browser.theme", `"system"`)
	policy := MergePolicy{IgnoredKeys: []string{"desktop.theme"}, InitialKeys: []string{"desktop.language"}}
	result, err := engine.SyncValidatedWithPolicy(context.Background(), config, secrets, local, baseline, nil, policy)
	want := values("desktop.theme", `"light"`, "desktop.language", `"zh-CN"`, "browser.theme", `"dark"`)
	if err != nil || !equalValues(result.Values, want) {
		t.Fatalf("racing writer lost: %+v %v", result, err)
	}
}
