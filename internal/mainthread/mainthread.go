// Package mainthread lets a background goroutine run code on the process main
// thread. AppKit only accepts UI calls from the main thread, and on darwin the
// Go runtime only keeps the main goroutine there when it is locked during
// init, which this package does.
//
// Run must be called from the main goroutine (main.main or a function it
// calls directly). While Run serves, Post hands functions to the main thread;
// a function may block it for as long as it needs, for example to spin an
// AppKit run loop. Off darwin nothing is locked and Run is a plain call
// followed by a drained queue, so callers need no build tags.
package mainthread

import "sync"

var (
	mu    sync.Mutex
	queue chan func()
)

// Run calls f on a new goroutine and executes posted functions on the calling
// goroutine until f returns. Functions posted before f returns always run.
func Run(f func() error) error {
	q := make(chan func(), 16)
	mu.Lock()
	if queue != nil {
		mu.Unlock()
		panic("mainthread: Run is already serving")
	}
	queue = q
	mu.Unlock()

	done := make(chan error, 1)
	go func() { done <- f() }()
	for {
		select {
		case fn := <-q:
			fn()
		case err := <-done:
			mu.Lock()
			queue = nil
			mu.Unlock()
			for {
				select {
				case fn := <-q:
					fn()
				default:
					return err
				}
			}
		}
	}
}

// Post queues fn for the main thread. It reports false when no Run is serving
// or the queue is full; the caller then falls back to not using the main
// thread at all.
func Post(fn func()) bool {
	mu.Lock()
	defer mu.Unlock()
	if queue == nil {
		return false
	}
	select {
	case queue <- fn:
		return true
	default:
		return false
	}
}
