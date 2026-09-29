package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// buildProbeBinary compiles a tiny main package that answers --version with
// exit 0, so updaterAlreadyPlaced exercises the real smoke probe.
func buildProbeBinary(t *testing.T, dir string) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	src := filepath.Join(dir, "probe.go")
	if err := os.WriteFile(src, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "probe")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, src)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build probe: %v\n%s", err, output)
	}
	return out
}

func TestUpdaterAlreadyPlacedMissing(t *testing.T) {
	if !updaterAlreadyPlaced(t.TempDir(), "windows-amd64") {
		return
	}
	t.Fatal("empty root must not count as placed")
}

func TestUpdaterAlreadyPlacedWorkingBinary(t *testing.T) {
	root := t.TempDir()
	binary := buildProbeBinary(t, root)
	binDir := filepath.Join(root, "tools", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "relkit-updater"
	target := "linux-amd64"
	if runtime.GOOS == "windows" {
		name = "relkit-updater.exe"
		target = "windows-amd64"
	}
	if err := os.Rename(binary, filepath.Join(binDir, name)); err != nil {
		t.Fatal(err)
	}
	if !updaterAlreadyPlaced(root, target) {
		t.Fatal("a placed, probe-passing updater must short-circuit the module channel")
	}
}

func TestUpdaterAlreadyPlacedBrokenBinary(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "tools", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "relkit-updater"
	target := "linux-amd64"
	if runtime.GOOS == "windows" {
		name = "relkit-updater.exe"
		target = "windows-amd64"
	}
	// Not an executable: file exists but the --version probe fails.
	if err := os.WriteFile(filepath.Join(binDir, name), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if updaterAlreadyPlaced(root, target) {
		t.Fatal("a broken placed updater must still go through the module channel")
	}
}
