package desktop

import "sync"

// mainThreadGate holds main-thread work that service callbacks post before
// the native event loop is ready. MyGo v0.5.5's Linux backend schedules a
// post that arrives while GTK is still initializing with a nil callback and
// then ignores every later post, so the client deadlocks during startup.
type mainThreadGate struct {
	post    func(func())
	mu      sync.Mutex
	ready   bool
	pending []func()
}

func newMainThreadGate(post func(func())) *mainThreadGate {
	return &mainThreadGate{post: post}
}

// run forwards fn once the gate is open. Before then it queues fn and, like
// mygo.RunOnMain, waits until fn has run, so it must not be called from the
// main thread before open.
func (g *mainThreadGate) run(fn func()) {
	g.mu.Lock()
	if g.ready {
		g.mu.Unlock()
		g.post(fn)
		return
	}
	done := make(chan struct{})
	g.pending = append(g.pending, func() {
		defer close(done)
		fn()
	})
	g.mu.Unlock()
	<-done
}

// open runs the queued work in order on the calling main thread, including
// work queued meanwhile, and then forwards all later work.
func (g *mainThreadGate) open() {
	for {
		g.mu.Lock()
		pending := g.pending
		g.pending = nil
		if len(pending) == 0 {
			g.ready = true
			g.mu.Unlock()
			return
		}
		g.mu.Unlock()
		for _, fn := range pending {
			fn()
		}
	}
}
