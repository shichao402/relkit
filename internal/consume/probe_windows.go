//go:build windows

package consume

import (
	"syscall"
)

// detachForProbe isolates the --version probe process from the CLI's process
// group so a hung child cannot capture Ctrl+C aimed at the parent.
func detachForProbe() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
