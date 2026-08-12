package apply

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ReplaceExecutable replaces the running process binary with newPath.
//
// On Windows the running executable cannot be deleted, but it can usually be
// renamed aside; the new file is then written to the original path. A leftover
// ".old" sibling is removed on the next successful replace (CleanupStale).
func ReplaceExecutable(newPath string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("eval symlinks: %w", err)
	}
	return ReplaceFile(newPath, execPath)
}

// ReplaceFile replaces targetPath with the contents of newPath.
func ReplaceFile(newPath, targetPath string) error {
	if err := os.Chmod(newPath, 0o755); err != nil {
		return fmt.Errorf("chmod new: %w", err)
	}
	_ = CleanupStale(targetPath)

	backupPath := targetPath + ".old"
	_ = os.Remove(backupPath)
	if err := os.Rename(targetPath, backupPath); err != nil {
		return fmt.Errorf("rename running binary aside: %w", err)
	}
	if err := copyFile(newPath, targetPath); err != nil {
		_ = os.Rename(backupPath, targetPath)
		return fmt.Errorf("write new binary: %w", err)
	}
	if err := os.Chmod(targetPath, 0o755); err != nil {
		_ = os.Remove(targetPath)
		_ = os.Rename(backupPath, targetPath)
		return fmt.Errorf("chmod target: %w", err)
	}
	if runtime.GOOS == "darwin" {
		_ = exec.Command("xattr", "-cr", targetPath).Run()
	}
	// Best-effort: Windows often cannot delete the renamed-aside running binary
	// until process exit; CleanupStale removes it next launch.
	_ = os.Remove(backupPath)
	return nil
}

// CleanupStale removes targetPath+".old" left from a previous self-replace.
func CleanupStale(targetPath string) error {
	return os.Remove(targetPath + ".old")
}

// CleanupStaleExecutable cleans beside the current executable.
func CleanupStaleExecutable() error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return err
	}
	return CleanupStale(execPath)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
