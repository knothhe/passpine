package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShutdownDrainsRequestsBeforeClosingAndRefusesNewRequests(t *testing.T) {
	s, err := New(Options{DataDir: t.TempDir(), Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.updateRequests.RLock()
	done := make(chan error, 1)
	go func() { done <- s.Shutdown(context.Background()) }()
	deadline := time.After(time.Second)
	for !s.updatePaused.Load() {
		select {
		case <-deadline:
			t.Fatal("shutdown never closed admission")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if s.Execute(Request{Action: "settings"}).Status != "error" {
		t.Fatal("shutdown admitted a new request")
	}
	select {
	case <-done:
		t.Fatal("shutdown interrupted an existing request")
	default:
	}
	s.updateRequests.RUnlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !s.closed {
		t.Fatal("service did not close after the request finished")
	}
}

func TestShutdownTimeoutRestoresAdmissionAndAllowsRetry(t *testing.T) {
	s, err := New(Options{DataDir: t.TempDir(), Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.updateRequests.RLock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = s.Shutdown(ctx)
	s.updateRequests.RUnlock()
	if !errors.Is(err, context.DeadlineExceeded) || s.closed || s.updatePaused.Load() {
		t.Fatal("timeout did not preserve the service")
	}
	if s.Execute(Request{Action: "settings"}).Status != "ok" {
		t.Fatal("timeout did not restore requests")
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCanceledShutdownDoesNotCloseService(t *testing.T) {
	s, err := New(Options{DataDir: t.TempDir(), Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Shutdown(ctx); !errors.Is(err, context.Canceled) || s.closed || s.updatePaused.Load() {
		t.Fatal("canceled shutdown changed the service")
	}
}
