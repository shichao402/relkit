package consume

import (
	"context"
	"time"
)

// timeoutAfter wraps context.WithTimeout for the smoke probe. Defined here
// (not in probe_windows.go) so every cross-compile target links it.
func timeoutAfter(d time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	// The probe process exit releases everything; the timer context only
	// guards a hung child, so cancel fires no later than process exit.
	go func() {
		time.Sleep(d + time.Second)
		cancel()
	}()
	return ctx
}
