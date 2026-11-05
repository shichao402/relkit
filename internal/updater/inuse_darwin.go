//go:build darwin

package updater

import (
	"bytes"

	"golang.org/x/sys/unix"
)

// runningExecutables lists processes with sysctl so it works without cgo.
// kern.procargs2 starts with argc (4 bytes) followed by the exec path, which
// is what proc_pidpath reports for a process launched by path. The kernel
// refuses other users' processes; those are skipped.
func runningExecutables() []string {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(procs))
	for i := range procs {
		pid := int(procs[i].Proc.P_pid)
		if pid <= 0 {
			continue
		}
		raw, err := unix.SysctlRaw("kern.procargs2", pid)
		if err != nil || len(raw) <= 4 {
			continue
		}
		rest := raw[4:]
		if end := bytes.IndexByte(rest, 0); end > 0 {
			out = append(out, string(rest[:end]))
		}
	}
	return out
}
