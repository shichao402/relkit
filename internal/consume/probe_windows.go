package consume

import (
	"context"
	"syscall"
	"time"
)

// timeoutAfter wraps context.WithTimeout for the smoke probe.
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

// detachForProbe isolates the --version probe process from the CLI's process
// group so a hung child cannot capture Ctrl+C aimed at the parent.
func detachForProbe() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
