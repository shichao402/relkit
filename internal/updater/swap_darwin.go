//go:build darwin

package updater

import "golang.org/x/sys/unix"

// swapDirs exchanges two paths atomically (APFS renamex_np RENAME_SWAP).
func swapDirsNative(a, b string) error {
	return unix.RenamexNp(a, b, unix.RENAME_SWAP)
}
