package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func RepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine repo root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func ConformanceRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(RepoRoot(t), "conformance")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("conformance root not found: %s", root)
	}
	return root
}
