//go:build !darwin

package updater

// swapDirs has no atomic exchange here; callers fall back to two renames.
func swapDirsNative(_, _ string) error {
	return errSwapUnsupported
}
