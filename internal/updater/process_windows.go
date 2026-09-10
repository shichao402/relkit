//go:build windows

package updater

import "syscall"

const (
	processSynchronize = 0x00100000
	waitTimeout        = 258
)

func processAlive(pid int) bool {
	handle, err := syscall.OpenProcess(processSynchronize, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	result, err := syscall.WaitForSingleObject(handle, 0)
	return err == nil && result == waitTimeout
}
