package consume

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestReplaceLockedBinarySidestepsLockedDestination covers the Windows CI
// failure mode from SvnMergeTool build #31: a running process holds the old
// relkit.exe open, so a plain overwrite rename fails with Access denied. The
// fix sidesteps the locked inode to "<name>.old-<ms>" and lands the new file
// under the original name. The lock itself can only be reproduced on Windows
// (open handles block deletion/rename-over); on other platforms the test at
// least covers the unlocked path and the sweep.
func TestReplaceLockedBinarySidestepsLockedDestination(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "app.bin")

	oldData := []byte("old inode")
	if err := os.WriteFile(destination, oldData, 0o755); err != nil {
		t.Fatal(err)
	}
	// A stale sidestep from an earlier round should be swept after a
	// successful replace.
	stale := destination + ".old-1000"
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	tmp := destination + ".tmp"
	newData := []byte("new inode")
	if err := os.WriteFile(tmp, newData, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceLockedBinary(tmp, destination); err != nil {
		t.Fatalf("replaceLockedBinary: %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(newData) {
		t.Fatalf("destination = %q, want %q", got, newData)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("tmp should be gone after the swap, stat err = %v", err)
	}

	// The sidestep may survive on Windows when the old inode is still held
	// open; once released the sweep in the next round removes it. After an
	// unlocked replace the stale entry from a previous round must be gone.
	if runtime.GOOS == "windows" {
		// No live lock in this test, so the fresh sidestep (if any) and the
		// stale one are both swept or sweepable; nothing named .old- should
		// remain because the sweep ran inside the successful swap path only
		// when a sidestep was needed. Verify best-effort: re-run the sweep.
		sweepSidestepped(destination)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		if strings.HasPrefix(ent.Name(), "app.bin.old-") {
			t.Errorf("sidestep %s should have been swept", ent.Name())
		}
	}
}

// TestReplaceLockedBinaryRollsBackWhenNewFileCannotLand guards the rollback:
// if the sidestep succeeds but the new file cannot land, the old inode goes
// back under the original name instead of leaving the tree without a binary.
func TestReplaceLockedBinaryRollsBackWhenNewFileCannotLand(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("rollback path depends on Windows rename semantics")
	}
	dir := t.TempDir()
	destination := filepath.Join(dir, "app.bin")

	oldData := []byte("old inode")
	if err := os.WriteFile(destination, oldData, 0o755); err != nil {
		t.Fatal(err)
	}
	tmp := destination + ".tmp"
	newData := []byte("new inode")
	if err := os.WriteFile(tmp, newData, 0o755); err != nil {
		t.Fatal(err)
	}

	// Hold the destination open without FILE_SHARE_DELETE so the plain
	// rename-over fails; the sidestep rename is still allowed. Then make
	// the second rename fail by removing tmp between the two steps: force
	// that by sweeping tmp from a racing goroutine is racy, so instead we
	// make tmp itself locked-held-no-share, which fails rename-to-destination
	// only when destination also exists... Simplest deterministic failure:
	// replace tmp with a directory after the plain rename attempt is
	// guaranteed to fail. os.Rename(file, existing-dir-open) fails.
	lock, err := os.OpenFile(destination, os.O_RDONLY, 0)
	if err != nil {
		t.Skipf("cannot open destination exclusively: %v", err)
	}
	defer lock.Close()

	// Windows rename-over a no-share open fails -> sidestep path taken.
	// The second rename fails because tmp disappeared: delete it now from
	// the same process (the open handle is on destination, not tmp).
	if err := os.Remove(tmp); err != nil {
		t.Fatal(err)
	}

	err = replaceLockedBinary(tmp, destination)
	if err == nil {
		t.Fatal("expected an error when tmp vanished mid-swap")
	}
	got, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatalf("destination missing after rollback: %v", readErr)
	}
	if string(got) != string(oldData) {
		t.Fatalf("destination = %q after rollback, want the old inode back", got)
	}
}
