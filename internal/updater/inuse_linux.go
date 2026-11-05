//go:build linux

package updater

import (
	"os"
	"path/filepath"
	"strconv"
)

func runningExecutables() []string {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		exe, err := os.Readlink(filepath.Join("/proc", e.Name(), "exe"))
		if err == nil && exe != "" {
			out = append(out, exe)
		}
	}
	return out
}
