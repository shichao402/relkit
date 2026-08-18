package apply_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"cnb.cool/shichao402/relkit/sdk/apply"
)

func TestReplaceFileAndCleanup(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "app")
	if runtime.GOOS == "windows" {
		target += ".exe"
	}
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	newer := filepath.Join(dir, "new")
	if err := os.WriteFile(newer, []byte("new-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := apply.ReplaceFile(newer, target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-bytes" {
		t.Fatalf("got %q", got)
	}
	_ = apply.CleanupStale(target)
}
