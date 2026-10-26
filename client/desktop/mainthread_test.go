package desktop

import (
	"sync"
	"testing"
	"time"
)

// Service callbacks can fire while MyGo is still initializing GTK. They must
// not reach the native post until the ready callback opens the gate.
func TestMainThreadGateHoldsEarlyWorkUntilOpen(t *testing.T) {
	var mu sync.Mutex
	var posted, ran []int
	gate := newMainThreadGate(func(fn func()) {
		mu.Lock()
		posted = append(posted, len(posted))
		mu.Unlock()
		fn()
	})
	returned := make(chan int, 3)
	for i := range 3 {
		go func() {
			gate.run(func() {
				mu.Lock()
				ran = append(ran, i)
				mu.Unlock()
			})
			returned <- i
		}()
		// Queue in a known order.
		for {
			gate.mu.Lock()
			queued := len(gate.pending)
			gate.mu.Unlock()
			if queued == i+1 {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}
	select {
	case i := <-returned:
		t.Fatalf("early work %d returned before the gate opened", i)
	case <-time.After(20 * time.Millisecond):
	}
	mu.Lock()
	if len(posted) != 0 || len(ran) != 0 {
		t.Fatalf("early work reached the native loop: posted %v, ran %v", posted, ran)
	}
	mu.Unlock()

	gate.open()
	for range 3 {
		select {
		case <-returned:
		case <-time.After(time.Second):
			t.Fatal("early work was still waiting after the gate opened")
		}
	}
	mu.Lock()
	if len(posted) != 0 || len(ran) != 3 || ran[0] != 0 || ran[1] != 1 || ran[2] != 2 {
		t.Fatalf("open must run queued work in order on the main thread: posted %v, ran %v", posted, ran)
	}
	mu.Unlock()

	gate.run(func() {})
	mu.Lock()
	defer mu.Unlock()
	if len(posted) != 1 {
		t.Fatalf("work after open must use the native post, got %d posts", len(posted))
	}
}

// Work queued while open drains the backlog still runs before later work is
// forwarded, so ready-time callbacks keep their order.
func TestMainThreadGateDrainsWorkQueuedDuringOpen(t *testing.T) {
	var order []string
	gate := newMainThreadGate(func(fn func()) {
		order = append(order, "posted")
		fn()
	})
	late := make(chan struct{})
	go gate.run(func() {
		order = append(order, "first")
		go func() {
			gate.run(func() { order = append(order, "second") })
			close(late)
		}()
		for {
			gate.mu.Lock()
			queued := len(gate.pending)
			gate.mu.Unlock()
			if queued == 1 {
				return
			}
			time.Sleep(time.Millisecond)
		}
	})
	for {
		gate.mu.Lock()
		queued := len(gate.pending)
		gate.mu.Unlock()
		if queued == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	gate.open()
	<-late
	gate.run(func() { order = append(order, "third") })
	if len(order) != 4 || order[0] != "first" || order[1] != "second" || order[2] != "posted" || order[3] != "third" {
		t.Fatalf("unexpected order %v", order)
	}
}
