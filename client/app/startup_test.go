package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/knothhe/passpine-neo/client/nativehost"
)

func TestBrowserEndpointPublishesOnlyAfterActivityInitialization(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	records := make([]Activity, 50000)
	for i := range records {
		records[i] = Activity{Time: now.Add(-8 * 24 * time.Hour).Format(time.RFC3339), Action: "expired", Source: "desktop", Outcome: "ok"}
	}
	records[0] = Activity{Time: now.Add(-time.Hour).Format(time.RFC3339), Action: "retained", Source: "desktop", Outcome: "ok"}
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	activityPath := filepath.Join(dir, "activity.json")
	if err := os.WriteFile(activityPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	type startupResult struct {
		service *Service
		err     error
	}
	started := make(chan startupResult, 1)
	go func() {
		service, err := New(Options{DataDir: dir})
		started <- startupResult{service: service, err: err}
	}()
	var service *Service
	startupReceived := false
	t.Cleanup(func() {
		if !startupReceived {
			result := <-started
			service = result.service
			startupReceived = true
		}
		if service != nil {
			_ = service.Close()
		}
	})
	endpointPath := filepath.Join(dir, "browser-endpoint.json")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(endpointPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("browser endpoint was not published")
		}
		time.Sleep(time.Millisecond)
	}
	// The endpoint is externally visible now, even if New has not returned.
	// Its first caller must see an already loaded and pruned activity history.
	data, err = os.ReadFile(activityPath)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk []Activity
	if err := json.Unmarshal(data, &onDisk); err != nil || len(onDisk) != 1 || onDisk[0].Action != "retained" {
		t.Fatalf("endpoint became visible before activity pruning: count=%d error=%v", len(onDisk), err)
	}
	relay := nativehost.NewRelayClient(dir)
	defer relay.Close()
	var response Response
	if err := json.Unmarshal(relay.Send([]byte(`{"protocol":1,"action":"generate"}`)), &response); err != nil || response.Status != "ok" {
		t.Fatalf("first browser request failed: %+v, %v", response, err)
	}
	result := <-started
	startupReceived = true
	service = result.service
	if result.err != nil {
		t.Fatal(result.err)
	}
	activity := must(t, service, Request{Action: "activity"}).Activity
	if len(activity) != 2 || activity[0].Action != "generate" || activity[1].Action != "retained" {
		t.Fatalf("initialization overwrote the first browser operation: %+v", activity)
	}
}

func TestFailedEndpointStartupCanBeRetried(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "browser-endpoint.json")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if service, err := New(Options{DataDir: dir}); err == nil || service != nil {
		if service != nil {
			_ = service.Close()
		}
		t.Fatal("unsafe endpoint path did not fail startup")
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	service, err := New(Options{DataDir: dir})
	if err != nil {
		t.Fatalf("failed startup left resources that prevented a retry: %v", err)
	}
	defer service.Close()
	if service.server == nil || service.activityPrunedAt.IsZero() {
		t.Fatal("retry did not finish initialization before publishing its endpoint")
	}
}
