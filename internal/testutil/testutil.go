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

func UpdateSpecRoot(t *testing.T) string {
	t.Helper()
	candidates := []string{
		filepath.Clean(filepath.Join(RepoRoot(t), "..", "AgentsHelpMe", "update-spec")),
		filepath.Clean(filepath.Join(RepoRoot(t), "testdata")),
	}
	for _, root := range candidates {
		if _, err := os.Stat(filepath.Join(root, "conformance")); err == nil {
			return root
		}
	}
	t.Fatalf("update-spec/conformance not found; tried: %v", candidates)
	return ""
}

func ConformanceRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(UpdateSpecRoot(t), "conformance")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("conformance root not found: %s", root)
	}
	return root
}
