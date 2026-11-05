//go:build !darwin && !linux && !windows

package updater

func runningExecutables() []string { return nil }
